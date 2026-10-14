package night

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// Night is one run of the relay over a queue (§1): its plan, a sitting per
// row tonight, the night machine's state and the reason it stopped.
type Night struct {
	ID       string
	Plan     *Plan
	Sittings map[string]*relay.Sitting
	State    relay.NightState
	Reason   relay.StopReason
	Started  time.Time
	// Woken is each orchestrator sitting the wake halted (AR68 ruling 5)
	// once the loop has cut its next sitting: failed usage-limit, yet
	// recovered. nextSitting's red list, the stop line's red and resume, a
	// row's resume and Clean skip it. A night that stops before the cut
	// names the halted sitting as red (the fix round's F1).
	Woken map[string]bool
}

// NightOptions shape a night. Rows, Until, MaxSittings (0: none), MaxUSD (0:
// none, summed from the exit lines' cost_usd, an unread one at its row's
// --max-budget-usd), Parallel (default 1), Words
// (the owner's words for this run, else the queue's), NoTmux and Clock are
// §9's flags. The rest are what the caller injects: Plan (nil: BuildPlan
// with PlanOptions), State (the state directory), Row (the row's prompt and
// RunOptions; nil: RenderPrompt and no gate; Log and Notify are filled when
// nil), Merge (merge-on-green, gate.MergeIfGreen, serialised in the loop),
// Out (the night's own lines), Poll (the control file's check between
// events, default a second), Tmux and TmuxMode (the windows: auto, the
// default, never, which NoTmux forces, or required; nil Tmux is file
// windows), Notify (a notice for every event Notices accepts; nil: none).
type NightOptions struct {
	Rows        []string
	Until       string
	MaxSittings int
	MaxUSD      float64
	Parallel    int
	Words       string
	NoTmux      bool
	Clock       effects.Clock
	// StallSeconds is every sitting's watchdog (`--stall-seconds`; 0:
	// DefaultStallSeconds), §15.
	StallSeconds int

	Plan        *Plan
	PlanOptions Options
	State       string
	Row         func(r queue.Row, rp *RowPlan) ([]byte, RunOptions, error)
	Merge       func(r queue.Row, g relay.Gate) (string, error)
	Out         io.Writer
	Poll        time.Duration
	Tmux        effects.Tmux
	TmuxMode    string
	Notify      effects.Notify
	// Signals is where the night hears INT, TERM and HUP (ADV6-2); nil,
	// Run traps them itself (signal.Notify) for the night's length.
	Signals <-chan os.Signal
	// Orchestrate runs the queue's orchestrator row (orchestrator r1 §4):
	// its sittings `<id>.<n>` when nothing runs and no build row is ready,
	// one child at a time; QueuePath is the queue's path under the root,
	// re-read at main after each of its merges (effects.DefaultQueue when
	// empty).
	Orchestrate bool
	QueuePath   string
}

// ErrBug wraps a Step or NightStep error: the loop asked the machine for a
// pair it does not list, which is the driver's bug (the verb exits 2).
var ErrBug = errors.New("relay bug")

// RowVar is the variable effects.ChildEnv adds to every spawned row.
const RowVar = "RELAY_ROW"

// ErrNested: Run refuses a night under a measurement or a sitting (the run
// guard, relay-r1 §15, AR11; R6 §3 C4): RELAY_MEASURE_DEPTH or RELAY_ROW
// set. A pre-check like an invalid plan: nothing staged, nothing spawned.
var ErrNested = errors.New("nested: never under a measurement or a sitting")

// Nested reports whether getenv puts this process under a measurement or a
// sitting; nil is never nested.
func Nested(getenv func(string) string) bool {
	return effects.Nested(getenv) || (getenv != nil && getenv(RowVar) != "")
}

// HelperVar marks a helper's process as a sitting's (the hooks' inRow).
const HelperVar = "CLAUDE_SESSION_HELPER"

// InRow reports whether getenv puts this process inside a sitting or its
// helper: RELAY_ROW or CLAUDE_SESSION_HELPER non-empty. There a plan takes
// no pid lock, measures nothing and writes no ledger line (orchestrator
// r1 §6).
func InRow(getenv func(string) string) bool {
	return getenv != nil && (getenv(RowVar) != "" || getenv(HelperVar) != "")
}

// ControlPoll is the default Poll.
var ControlPoll = time.Second

// ReadBuildInfo is the relay's build info (debug.ReadBuildInfo); a
// variable so a test can fake it.
var ReadBuildInfo = debug.ReadBuildInfo

// Revision is the relay's own revision, the night-open line's `revision`
// (AR56 ruling 3, relay-r1 §5): the build's vcs.revision, with "+dirty"
// when vcs.modified is true; "" when the build has none (a test binary),
// so a reader tells a night on one commit from a night on another.
func Revision() string {
	info, ok := ReadBuildInfo()
	if !ok || info == nil {
		return ""
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev != "" && modified == "true" {
		rev += "+dirty"
	}
	return rev
}

// NightDir is <state>/nights/<night>.
func NightDir(state, night string) string { return filepath.Join(state, "nights", night) }

// RenderPrompt is the row's stdin: its packet (read in dirs, then the root) rendered
// with the tree, branch, base, the satisfied dependencies not yet in main,
// the device rule and the words.
func RenderPrompt(fx effects.Effects, p *Plan, r queue.Row, rp *RowPlan, words string, dirs ...string) ([]byte, string, string, error) {
	var src []byte
	err := fmt.Errorf("no packet path")
	for _, d := range append(dirs, p.Root) {
		if src, err = fx.FS.ReadFile(filepath.Join(d, r.PacketPath)); err == nil {
			break
		}
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("row %s: the packet: %w", r.ID, err)
	}
	var unmerged []string
	for _, dep := range rp.Satisfied {
		if ok, err := fx.Git.FirstParentMerged(p.Root, dep, "main"); err == nil && !ok {
			unmerged = append(unmerged, dep)
		}
	}
	stdin, rules, hash := prompt.Render(prompt.Context{Row: r.ID, Tree: rp.Worktree, Branch: rp.Branch,
		Base: rp.Base, Packet: r.PacketPath, Unmerged: unmerged, Device: r.Device, Words: words,
		Ceiling: r.Context, Soft: int(SoftLine * float64(r.Context)), Kill: int(HardCap * float64(r.Context)), Turns: r.Turns,
		Verify: VerifyName(p), VerifySeconds: p.VerifySeconds}, p.Block, src)
	return stdin, rules, hash, nil
}

// held is a Spawner that keeps the row's Process for `agentik-relay kill`; a kill
// before the child starts is delivered on Start. A kill escalates as the
// ceiling's does (§7): SIGTERM, then SIGKILL after KillGrace unless the
// child has been waited on. The child's group is in groups (with its
// start time from procs, and row) from its start to its Wait (ADV6-2).
type held struct {
	effects.Spawner
	groups *effects.Groups
	procs  effects.Procs
	row    string
	mu     sync.Mutex
	proc   effects.Process
	killed bool
	exited chan struct{}
	cancel context.CancelCauseFunc // the row's context: a kill before the spawn ends its wait
}

func (h *held) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	p, out, err := h.Spawner.Start(sp)
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		return p, out, err
	}
	h.exited = make(chan struct{})
	pgid := p.Pid()
	h.proc = &waited{Process: p, done: h.exited, gone: func() { h.groups.Done(pgid) }}
	started := ""
	if h.procs != nil {
		started, _ = h.procs.Started(pgid)
	}
	h.groups.Add(effects.Group{Pgid: pgid, Started: started, Row: h.row}, h.proc.Signal)
	if h.killed {
		h.term()
	}
	return h.proc, out, err
}

func (h *held) kill() { h.killFor(ErrOwnerKill) }

// stopFor is another row's account stop (run 10 ruling 1): this row killed
// the owner's way, its context's cause the reason.
func (h *held) stopFor(r relay.StopReason) { h.killFor(accountCause(r)) }

func (h *held) killFor(cause error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.killed {
		return
	}
	h.killed = true
	if h.cancel != nil {
		h.cancel(cause)
	}
	if h.proc != nil {
		h.term()
	}
}

// term sends SIGTERM and arms the SIGKILL; h.mu is held.
func (h *held) term() {
	h.proc.Signal(syscall.SIGTERM)
	proc, exited, grace := h.proc, h.exited, KillGrace
	go func() {
		select {
		case <-exited:
		case <-time.After(grace):
			proc.Signal(syscall.SIGKILL)
		}
	}()
}

// waited closes done, and calls gone, once Wait returns: the child is gone.
type waited struct {
	effects.Process
	done chan struct{}
	gone func()
	once sync.Once
}

func (w *waited) Wait() (int, error) {
	exit, err := w.Process.Wait()
	w.once.Do(func() {
		close(w.done)
		if w.gone != nil {
			w.gone()
		}
	})
	return exit, err
}

type outcome struct {
	id   string
	s    *relay.Sitting
	g    relay.Gate
	hold *heldLine // the part's held sitting line, its stage still locked (AR65); nil: written and unlocked
	st   *Staged
}

// tap is the night's ledger: every line goes through, and the rows' burn is
// kept for status.json.
type tap struct {
	effects.Ledger
	mu   sync.Mutex
	rows map[string]*RowStatus
	usd  float64 // the exit lines' read costs
	// held is the unread sittings' spend, each at its spawn line's usd
	// (AR28); budget that usd by row.
	held   float64
	budget map[string]float64
	// lines is what nightBurn reads for status.json (AR51 ruling 2): each
	// row's `spawn`, `exit` and `sitting` lines as the ledger took them and
	// its last `turn` line (turnAt is where), so the dashboard's burn is the
	// report's count of the same lines.
	lines  []map[string]any
	turnAt map[string]int
	// ended is when each row's `sitting` line (or its `skip`) was written
	// (AR51 ruling 4): the row's elapsed stops there, read on clock.
	ended map[string]time.Time
	clock effects.Clock

	night  string
	notify func(effects.Notice)
}

func (t *tap) Append(event string, m map[string]any) error {
	err := t.Ledger.Append(event, m)
	if t.notify != nil {
		f := map[string]any{"night": t.night}
		for k, v := range m {
			f[k] = v
		}
		if nt, ok := effects.Notices(event, f); ok {
			t.notify(nt)
		}
	}
	id, _ := m["row"].(string)
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.keep(event, id, m)
	}
	if (event == "sitting" || event == "skip") && t.clock != nil {
		if t.ended == nil {
			t.ended = map[string]time.Time{}
		}
		t.ended[id] = t.clock.Now()
	}
	rs := t.rows[id]
	if rs == nil {
		return err
	}
	num := func(k string) float64 { return number(m[k]) }
	switch event {
	case "spawn":
		if t.budget == nil {
			t.budget = map[string]float64{}
		}
		t.budget[id] = num("usd")
	case "turn":
		rs.Turns, rs.Context = int(num("turns")), int(num("context"))
		rs.LastTool, _ = m["last_tool"].(string)
	case "exit":
		rs.Turns, rs.CostUSD = int(num("turns")), num("cost_usd")
		if c := int(num("context")); c > 0 {
			rs.Context = c
		}
		if rs.Unread = m["cost_read"] == false; rs.Unread {
			t.held += t.budget[id]
		} else {
			t.usd += num("cost_usd")
		}
	case "sitting":
		if b, ok := m["builders"].([]map[string]any); ok {
			rs.Builders = len(b)
		}
	}
	return err
}

// keep holds a burn line for nightBurn (AR51 ruling 2); a row's `turn` line
// takes the place of its last one, so a sitting holds four lines at most.
func (t *tap) keep(event, id string, m map[string]any) {
	switch event {
	case "spawn", "turn", "exit", "sitting":
	default:
		return
	}
	l := map[string]any{"event": event}
	for k, v := range m {
		l[k] = v
	}
	if event == "turn" {
		if i, ok := t.turnAt[id]; ok {
			t.lines[i] = l
			return
		}
		if t.turnAt == nil {
			t.turnAt = map[string]int{}
		}
		t.turnAt[id] = len(t.lines)
	} else if event == "spawn" {
		delete(t.turnAt, id) // a new sitting's turns are its own
	}
	t.lines = append(t.lines, l)
}

// Run is the night (§1, §8, §9): the plan, the `night-open` line, then the
// loop: pick, stage, RunRow in a goroutine per slot, collect, NightStep per
// outcome, status.json and the control file after every event, and at the
// end the `stop` and `night-close` lines. Rows still waiting when the night
// stops are skipped. An error before night-open is the caller's (exit 2);
// an error wrapping ErrBug is a pair the machine refused.
//
// The dead relay (ADV6-2): Run first reaps what a killed relay left (Reap),
// then keeps every group it starts in <night>/groups.json until its exit,
// and traps INT, TERM and HUP once: the first signal sends every live group
// SIGTERM, then SIGKILL after KillGrace, and stops the night owner-kill
// with `signal:<name>` as the stop line's detail; the sittings running
// still write their lines before Run returns. A second signal is the
// default's: the process dies.
func Run(fx effects.Effects, sp effects.Spawner, q *queue.Queue, opts NightOptions) (*Night, error) {
	if Nested(fx.Getenv) {
		return nil, ErrNested
	}
	Reap(fx, opts.State)
	groups := fx.Groups
	if groups == nil {
		groups = &effects.Groups{}
		fx.Groups = groups
	}
	sigs := opts.Signals
	var trap chan os.Signal
	if sigs == nil {
		trap = make(chan os.Signal, 1)
		signal.Notify(trap, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		defer signal.Stop(trap)
		sigs = trap
	}
	caught, quit, grace := make(chan string, 1), make(chan struct{}), KillGrace
	defer close(quit)
	go func() {
		select {
		case sig := <-sigs:
			if trap != nil {
				signal.Stop(trap) // once: a second signal is the default's
			}
			groups.Kill(grace)
			caught <- signalName(sig)
		case <-quit:
		}
	}()
	if opts.Clock != nil {
		fx.Clock = opts.Clock
	}
	if opts.Parallel < 1 {
		opts.Parallel = 1
	}
	if opts.Poll <= 0 {
		opts.Poll = ControlPoll
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	words := opts.Words
	if words == "" {
		words = q.Words
	}
	p := opts.Plan
	if p == nil {
		po := opts.PlanOptions
		po.Rows, po.Until, po.State = opts.Rows, opts.Until, opts.State
		po.Orchestrate = po.Orchestrate || opts.Orchestrate
		var err error
		if p, err = BuildPlan(fx, q, po); err != nil {
			return nil, err
		}
	}
	if !p.Valid {
		var why []string
		for _, pr := range p.Problems {
			why = append(why, strings.TrimSpace(pr.Row+" "+pr.Msg))
		}
		return nil, fmt.Errorf("the plan is not valid: %s", strings.Join(why, "; "))
	}
	var until time.Time
	n := &Night{ID: p.Night, Plan: p, Sittings: map[string]*relay.Sitting{}, State: relay.Planning, Started: fx.Clock.Now(), Woken: map[string]bool{}}
	if opts.Until != "" {
		hm, err := time.Parse("15:04", opts.Until)
		if err != nil {
			return nil, fmt.Errorf("--until %q: want HH:MM", opts.Until)
		}
		s := n.Started
		until = time.Date(s.Year(), s.Month(), s.Day(), hm.Hour(), hm.Minute(), 0, 0, s.Location())
		if !until.After(s) {
			until = until.Add(24 * time.Hour)
		}
	}
	dir := NightDir(opts.State, n.ID)
	if err := fx.FS.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// AR34 act 1: a stop or a kill said during the plan (at or after its
	// start) is this night's, read before the first spawn; older, removed.
	control, since := ControlPath(opts.State, n.ID), p.opts.Now
	if since.IsZero() { // a plan not built by BuildPlan: the night's own start
		since = n.Started
	}
	saidStop, saidKills := OpenControl(fx.FS, control, since)
	gfile, self := filepath.Join(dir, GroupsFile), groupsDoc{PID: os.Getpid(), Started: selfStarted(fx.Procs), Root: p.Root}
	groups.Watch(func(gs []effects.Group) {
		doc := self
		doc.Groups = gs
		b, _ := json.Marshal(doc)
		fx.FS.WriteFile(gfile, append(b, '\n'), 0o600)
	})
	defer func() {
		groups.Watch(nil)
		os.Remove(gfile)
	}()

	rows := map[string]queue.Row{}
	for _, r := range q.Rows {
		rows[r.ID] = r
	}
	lg := &tap{Ledger: fx.Ledger, rows: map[string]*RowStatus{}, night: n.ID, clock: fx.Clock}
	var nmu sync.Mutex
	send := func(nt effects.Notice) {
		if opts.Notify == nil {
			return
		}
		nmu.Lock()
		defer nmu.Unlock()
		if err := opts.Notify.Send(nt); err != nil {
			fmt.Fprintf(out, "relay: notify %s: %v\n", nt.Event, err)
		}
	}
	lg.notify = send
	fx.Ledger = lg
	mode := opts.TmuxMode
	if mode == "" {
		mode = "auto"
	}
	if opts.NoTmux {
		mode = "never"
	}
	win, err := Open(opts.Tmux, mode, n.ID, opts.State)
	if err != nil {
		return nil, err
	}
	for _, l := range win.Warn { // the sweep's failures: the night goes on (AR33 act 2)
		fmt.Fprintln(out, l)
	}
	var orc *orchestra // the standing row, under Orchestrate (AR32A act 4)
	if opts.Orchestrate {
		for _, rp := range p.Rows {
			if r := rows[rp.ID]; r.IsOrchestrator() && rp.State == StateReady {
				qp := opts.QueuePath
				if qp == "" {
					qp = effects.DefaultQueue
				}
				orc = &orchestra{row: r, qpath: qp, sittings: map[string]bool{}, added: map[string]bool{}}
				if sha, err := mainSha(fx.Git, p.Root); err == nil {
					orc.open = openAt(fx.Git, p.Root, sha)
				}
				opts.Parallel = 1 // one child at a time (§6)
			}
		}
	}
	var ids []string
	satisfiedAt := map[string][]string{}
	byID := map[string]*RowPlan{}
	// A row refused at plan is main's stage-skipped row from the open on
	// (AR58 hardening round): its skip line in stage's words, its worktree
	// in status, and the scheduler dooms it behind a failed dependency.
	refused := map[string]bool{}
	var unskipped []string // refused rows registered, their skip line not yet written
	register := func(rp *RowPlan) {
		st := relay.Skipped
		switch rp.State {
		case StateReady:
			st = relay.Ready
		case StatePending:
			st = relay.Waiting
		}
		n.Sittings[rp.ID] = &relay.Sitting{ID: rp.ID, State: st}
		rs := &RowStatus{ID: rp.ID, State: string(st)}
		switch why := RefusedWhy(*rp); {
		case why != "": // stage's words, as Stage's own refusal reads (AR58)
			rs.Reason, rs.Worktree = "stage "+rp.ID+": "+why, rp.Worktree
			refused[rp.ID] = true
			unskipped = append(unskipped, rp.ID)
		case st == relay.Skipped:
			rs.Reason = rp.State
		default:
			rs.Worktree = rp.Worktree
		}
		lg.mu.Lock()
		lg.rows[rp.ID] = rs
		lg.mu.Unlock()
		satisfiedAt[rp.ID] = rp.Satisfied
		ids = append(ids, rp.ID)
		byID[rp.ID] = rp
	}
	for i := range p.Rows {
		if orc != nil && p.Rows[i].ID == orc.row.ID {
			continue // a template: each sitting is registered as it starts
		}
		register(&p.Rows[i])
	}
	put := func(event string, m map[string]any) { lg.Append(event, m) }
	put("night-open", map[string]any{"rows": ids, "parallel": opts.Parallel, "until": opts.Until,
		"max_sittings": opts.MaxSittings, "max_usd": opts.MaxUSD, "root": p.Root,
		"template_hash": p.TemplateHash, "queue_hash": p.QueueHash, "no_tmux": opts.NoTmux,
		"tmux": win.Mode, "session": win.Session, "revision": Revision()})
	// skipRefused writes each registered refused row's skip line, as skip
	// writes a stage refusal's: after night-open, and at a re-read's
	// registration (AR58 hardening round).
	skipRefused := func() {
		for _, id := range unskipped {
			lg.mu.Lock()
			why := lg.rows[id].Reason
			lg.mu.Unlock()
			put("skip", map[string]any{"row": id, "reason": why})
		}
		unskipped = nil
	}
	skipRefused()
	if len(win.Closed) > 0 { // the old nights' sessions Open killed (AR33 act 2)
		put("windows-closed", map[string]any{"sessions": win.Closed})
	}
	if p.MeasureFrom != "" { // the measure's budget and its source (AR38 act 1)
		fmt.Fprintln(out, MeasureLine(p))
	}

	var bug error
	step := func(e relay.NightEvent, r relay.StopReason) {
		next, err := relay.NightStep(n.State, e, r)
		if err != nil {
			if bug == nil {
				bug = fmt.Errorf("%w: NightStep(%s, %s, %q): %v", ErrBug, n.State, e, r, err)
			}
			return
		}
		n.State = next
		if reason, ok := relay.StopReasonOf(next); ok {
			n.Reason = reason
		}
	}
	stopWith := func(r relay.StopReason) { n.State, n.Reason = relay.Stopped(r), r }
	stopDetail := "" // the stop line's detail: why the night stopped, past its reason
	stalls := 0      // the night's stalled rows (R6 §2)
	step(relay.OnPlanned, "")
	// kept is AR34 act 1's rule over what the owner said before the first
	// row: any kill stops the night owner-kill (a kill already stops the
	// whole night), a stop drains it at once; no row spawns.
	kept := func(stop bool, ks []string, when string) bool {
		switch {
		case len(ks) > 0:
			step(relay.OnOwnerKill, "")
		case stop:
			put("owner-stop", map[string]any{})
			step(relay.OnOwnerStop, "")
		default:
			return false
		}
		stopDetail = when + controlLines(stop, ks)
		return true
	}
	switch {
	case kept(saidStop, saidKills, "said during the plan: "):
	case p.Budget > 0: // a baseline the measure left unrun anchors nothing (§15)
		stopWith(relay.MeasureTimeout)
		stopDetail = strings.TrimPrefix(p.RunStop, string(relay.MeasureTimeout)+": ") // AR54 act 2: the plan's own words
	}

	var (
		running   = map[string]*held{}
		started   = map[string]time.Time{}
		picked    = map[string]bool{}
		killedBy  = map[string]bool{}
		accountBy = map[string]relay.StopReason{} // the rows an account stop killed (run 10 ruling 1)
		done      = make(chan outcome)
		heldSt    = map[string]*Staged{} // the paused rows' stages, locked until the halt (AR65)
		kits      = map[string]rowKit{}  // each started row's prompt and options, for its continuation (AR67)
		halted    = map[string]bool{}    // the orchestrator sittings the wake halted, their next sitting not yet cut (AR68)
		wake      time.Time              // the paused night's wake (AR65)
		scope     []string               // the paused scope's model words, sorted; nil: every row (AR71)
		pauseOf   = map[string]int{}     // each killed paused row's pause, its number (AR71's fix round, F3)
		fellBack  bool                   // the night last woke from a fallback wait: the probe is armed (AR71's fix round, F2)
		pauses    int                    // the night's pauses (AR65)
		keptReset time.Time              // the pause's reset, zero for a fallback's (AR69 ruling 4)
		waited    time.Time              // the reset the night last woke from, zero: none or a fallback's (AR69)
		probeOK   bool                   // the probe passed: a sitting since the last fallback wake ended without a usage-limit refusal (AR69's F1)
		fresh     = map[string]bool{}    // the rows whose part launched since the last fallback wake: only theirs passes the probe (AR70)
		count     int
		kills     = len(saidKills) // the control file's kill lines already read (the open's too, AR34)
	)
	// The night's context (R6 §4): the owner's stop cancels it, a kill the
	// row's own, so a row still waiting on the breaker or the spawn's
	// spacing ends within one poll.
	nightCtx, stopAll := context.WithCancelCause(context.Background())
	defer stopAll(nil)
	var space *spacer // spawns SpawnSpacing apart, when the breaker reads a load (ADV6-13)
	if fx.Load != nil {
		space = &spacer{clock: fx.Clock}
	}
	views := map[string]*actView{} // the running rows' acts (AR30 act 3)
	readActs := func(id string) {
		if acts, ok := views[id].read(fx.Git, p.Root); ok {
			lg.mu.Lock()
			lg.rows[id].Acts = acts
			lg.mu.Unlock()
		}
	}
	status := func() {
		for id := range running { // each write reads a running row's tip
			readActs(id)
		}
		lg.mu.Lock()
		defer lg.mu.Unlock()
		now := fx.Clock.Now()
		for id, t := range started {
			if rs := lg.rows[id]; rs != nil {
				end, over := lg.ended[id] // AR51 ruling 4: a row's elapsed ends with its sitting
				if !over {
					end = now
				}
				rs.Elapsed = end.Sub(t).Round(time.Second).String()
			}
		}
		_, burn := nightBurn(lg.lines, "")
		st := buildStatus(n, ids, lg.rows, burn, lg.usd, lg.held, count, opts, until)
		st.PID, st.Started = self.PID, self.Started
		writeStatus(fx.FS, dir, st) // whole or not at all (AR37 fix)
	}
	setRow := func(id string, s *relay.Sitting) {
		lg.mu.Lock()
		defer lg.mu.Unlock()
		rs := lg.rows[id]
		rs.State, rs.Reason, rs.MergeSHA = string(s.State), string(s.Reason), s.MergeSHA
		rs.Resume = ""
		if (s.State == relay.Red || s.State == relay.Failed) && !n.Woken[id] { // a woken sitting is recovered (AR68)
			rs.Resume = relay.ResumeLine(id)
		}
	}
	skip := func(id, why string) {
		s := n.Sittings[id]
		s.State = relay.Skipped
		setRow(id, s)
		lg.mu.Lock()
		lg.rows[id].Reason = why
		lg.mu.Unlock()
		put("skip", map[string]any{"row": id, "reason": why})
	}
	// newSession mints a continuation's session id (AR67 ruling 5): the
	// plan options' UUID, else NewUUID.
	newSession := opts.PlanOptions.UUID
	if newSession == nil {
		newSession = NewUUID
	}
	// launch runs one part of a staged row (AR67 ruling 5 splits it from
	// start): the row's log opened, its window at the first part only, the
	// part's own context and held line, the child under RunRow.
	launch := func(id string, rp *RowPlan, st *Staged, stdin []byte, ro RunOptions, first bool) {
		var logf *os.File
		var err error
		if ro.Log == nil {
			if logf, err = os.OpenFile(RowLog(opts.State, n.ID, id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
				ro.Log = logf
			}
		}
		if first {
			if err := win.RowWindow(id, RowLog(opts.State, n.ID, id)); err != nil {
				fmt.Fprintf(out, "relay %s: tmux window: %v\n", id, err)
			}
		}
		if ro.Notify == nil {
			ro.Notify = func(s string) { fmt.Fprintln(out, s) }
		}
		rctx, cancel := context.WithCancelCause(nightCtx)
		ro.Ctx, ro.space, ro.hold = rctx, space, &heldLine{}
		h := &held{Spawner: sp, groups: groups, procs: fx.Procs, row: id, cancel: cancel}
		running[id] = h
		fresh[id] = true
		if first {
			started[id] = fx.Clock.Now()
		}
		lg.mu.Lock()
		lg.rows[id].Ceiling = ro.Context
		lg.mu.Unlock()
		setRow(id, n.Sittings[id])
		fmt.Fprintf(out, "relay %s: running in %s\n", id, TildeQuote([]string{rp.Worktree}, os.Getenv("HOME")))
		go func() {
			rs, g := RunRow(fx, h, st, rp, stdin, ro)
			cancel(nil)
			o := outcome{id: id, s: rs, g: g, st: st}
			if ro.hold.write != nil { // held: collect writes the line and unlocks unless paused (AR65)
				o.hold = ro.hold
			} else {
				st.Unlock()
			}
			if logf != nil {
				logf.Close()
			}
			done <- o
		}()
	}
	start := func(id string) {
		r, rp := rows[id], byID[id]
		picked[id] = true
		s := n.Sittings[id]
		if s.State == relay.Waiting {
			if s.State, bug = stepSitting(s.State, relay.DepsSatisfied, bug); bug != nil {
				return
			}
		}
		var stdin []byte
		var ro RunOptions
		var err error
		if opts.Row != nil {
			stdin, ro, err = opts.Row(r, rp)
		} else if orc.is(id) {
			stdin, ro.RulesHash, _ = RenderSitting(p, r, rp, words)
			ro.Block = p.BlockSource
		} else {
			stdin, _, _, err = RenderPrompt(fx, p, r, rp, words)
		}
		if err != nil {
			skip(id, "prepare: "+err.Error())
			return
		}
		st, err := Stage(fx, rp)
		if err != nil {
			skip(id, err.Error())
			return
		}
		count++ // a sitting is a row that staged (AR58 ruling 3): --max-sittings counts rows that staged
		views[id] = viewOf(st)
		if ro.Context == 0 {
			ro.Context = r.Context
		}
		if ro.Wall == 0 {
			ro.Wall = r.Wall
		}
		if ro.StallSeconds == 0 {
			ro.StallSeconds = opts.StallSeconds
		}
		if ro.RulesHash == "" { // the plan's template hash, the block's (AR30 act 2)
			ro.RulesHash, ro.Block = p.TemplateHash, p.BlockSource
		}
		if ro.Deps == nil { // §3 at the start: satisfied at plan, or green tonight
			ro.Deps = map[string]bool{}
			for _, d := range edgesOf(r) {
				ds := n.Sittings[d]
				ro.Deps[d] = contains(satisfiedAt[id], d) || (ds != nil && ds.State == relay.Green)
			}
		}
		kits[id] = rowKit{stdin: stdin, ro: ro, part: 1, session: rp.SessionID} // before Ctx, hold and Log: each part its own (AR67 ruling 5)
		s.State = relay.Running
		launch(id, rp, st, stdin, ro, true)
	}
	// continueRow is a paused row's continuation at the wake (AR67 ruling
	// 5): Paused steps Spawn, the account's marks go, and the row's kept
	// prompt and options run its plan under a new session and the budget
	// its parts left, on the first stage's Staged: no Stage, no stage line,
	// no window, no pick and no count. Limited again, it pauses again. Its
	// stdin is resumeStdin's: the row's, a blank line, then the RESUME
	// block (AR68 ruling 1), so the child reads the part's ceilings, not
	// only CONTEXT's. The part is the row's sitting (AR77 ruling 1): no
	// reason until its verdict, and status's counts its own.
	continueRow := func(id string) {
		s := n.Sittings[id]
		if s.State, bug = stepSitting(s.State, relay.Spawn, bug); bug != nil {
			return
		}
		s.Reason = ""
		delete(accountBy, id)
		delete(killedBy, id)
		k := kits[id]
		last := k.session
		k.part++
		st := heldSt[id]
		delete(heldSt, id)
		cp := *byID[id]
		cp.SessionID = newSession()
		k.session = cp.SessionID
		kits[id] = k
		turns, usd := remainder(rows[id].Turns, rows[id].USD, lg.parts(id))
		cp.Spawn = continueArgv(cp.Spawn, cp.SessionID, turns, usd)
		stdin := resumeStdin(k.stdin, resumeOf(fx.Git, st, k.part, last, turns, usd))
		ro := k.ro
		ro.Part = k.part
		views[id] = viewOf(st)
		lg.mu.Lock()
		delete(lg.ended, id) // running again: status counts its elapsed on from part 1's start (the fix round's F1)
		// The part's own lines fill its counts; the spend (usd, held) keeps every part's.
		if rs := lg.rows[id]; rs != nil {
			rs.Turns, rs.Context, rs.CostUSD, rs.Unread, rs.LastTool, rs.Builders, rs.Cause = 0, 0, 0, false, "", 0, ""
		}
		lg.mu.Unlock()
		launch(id, &cp, st, stdin, ro, false)
	}
	// The orchestrated night (AR32A act 4, orchestrator r1 §4, §6, §7).
	// nextSitting registers the standing row's next sitting as ready.
	nextSitting := func() (string, error) {
		lines, err := fx.Ledger.Lines()
		if err != nil {
			return "", err
		}
		for id := range orc.sittings { // tonight's, whatever the ledger kept
			lines = append(lines, map[string]any{"row": id})
		}
		charter, act, err := NextAct(fx.Git, p.Root, orc.row.ID)
		if err != nil {
			return "", err
		}
		sha, err := mainSha(fx.Git, p.Root)
		if err != nil {
			return "", err
		}
		var red, green []string
		for _, id := range ids {
			switch s := n.Sittings[id]; {
			case n.Woken[id] || halted[id]: // halted at the wake, its next sitting this one: recovered (AR68 ruling 5)
			case s.State == relay.Red || s.State == relay.Failed:
				red = append(red, id)
			case s.State == relay.Green && s.MergeSHA == "" && !orc.is(id):
				green = append(green, id) // ar32b act 2: a branch the sitting may merge
			}
		}
		for _, r := range q.Rows {
			if r.Parked != "" {
				red = append(red, r.ID+" (parked)")
			}
		}
		rp := SittingPlan(p, orc.row, lines)
		orc.start = SittingStart{Began: q, Main: sha, Charter: charter, Act: act, Red: red, Green: green, Open: openAt(fx.Git, p.Root, sha)}
		rp.Sitting = &orc.start
		orc.sittings[rp.ID], orc.current, orc.startKey = true, rp.ID, ActKey(act)
		rows[rp.ID] = orc.row
		register(&rp)
		return rp.ID, nil
	}
	// questions writes a question line when main's open questions rose.
	questions := func(sha string) {
		if k := openAt(fx.Git, p.Root, sha); k > orc.open {
			put("question", map[string]any{"open": k, "file": OpenQuestionsFile, "at": short(sha)})
			orc.open = k
		} else {
			orc.open = k
		}
	}
	// reread loads the queue at main after the standing row's merge and
	// plans the rows it added (§6); a problem is the reason it failed.
	reread := func() string {
		nq, err := queue.Load(p.Root, orc.qpath)
		if err != nil {
			return err.Error()
		}
		if probs := nq.Validate(); len(probs) > 0 {
			return strings.TrimSpace(probs[0].Row + " " + probs[0].Msg)
		}
		had := map[string]bool{}
		for _, r := range q.Rows {
			had[r.ID] = true
		}
		added := []string{}
		for _, r := range nq.Rows {
			if !had[r.ID] && !r.IsOrchestrator() {
				added = append(added, r.ID)
			}
		}
		put("queue-reread", map[string]any{"hash": nq.Hash, "added": added})
		np := &Plan{}
		if len(added) == 0 {
			if why := readBlock(fx.Git, p.Root, np); why != "" {
				return why
			}
			np.TemplateHash = prompt.TemplateHashFor(np.Block)
		} else {
			po := p.opts
			po.Rows, po.Night, po.Orchestrate = added, n.ID, false
			// The loop reads the control file from here on (checkControl): a
			// re-read never watches it, nor kills the night's groups (AR34 fix, F1).
			po.Control, po.Groups = "", nil
			if np, err = BuildPlan(fx, nq, po); err != nil {
				return err.Error()
			}
			if !np.Valid || np.Budget > 0 {
				var why []string
				for _, pr := range np.Problems {
					why = append(why, strings.TrimSpace(pr.Row+" "+pr.Msg))
				}
				if np.Budget > 0 {
					why = append(why, MeasureTimeoutDetail(np))
				}
				return "the added rows' plan: " + strings.Join(why, "; ")
			}
		}
		p.Block, p.BlockSource, p.TemplateHash = np.Block, np.BlockSource, np.TemplateHash
		for _, r := range nq.Rows {
			if !had[r.ID] {
				rows[r.ID] = r
			}
		}
		for i := range np.Rows {
			orc.added[np.Rows[i].ID] = true
			register(&np.Rows[i])
		}
		skipRefused()
		q = nq
		return ""
	}
	// judge reads the Next act at main after the merge (§4): DONE ends
	// the row once its rows are through, else holds as WAITING; a second
	// stuck sitting in a row stops the night.
	judge := func(sha string) {
		_, act, err := NextAct(fx.Git, p.Root, orc.row.ID)
		if err != nil {
			stopWith(relay.NoProgress)
			stopDetail = "the Next act at main: " + err.Error()
			return
		}
		key := ActKey(act)
		orc.start.Act = act
		switch actWord(key) {
		case "DONE":
			if orc.pending(ids, n.Sittings) != "" {
				orc.hold = true
				return
			}
			stopWith(relay.Done)
			stopDetail = key
		case "WAITING":
			orc.hold = true
		default:
			stuck, err := stuckAfter(fx.Git, p.Root, orc.start.Main, sha, orc.startKey, key)
			switch {
			case err != nil:
				stopWith(relay.NoProgress)
				stopDetail = "the sitting's diff: " + err.Error()
			case !stuck:
				orc.stuck = 0
			default:
				if orc.stuck++; orc.stuck >= 2 {
					stopWith(relay.NoProgress)
					stopDetail = "stuck twice at: " + key
				}
			}
		}
	}
	// landSitting merges a green sitting (§4): a refused merge reads red,
	// a plain error stops merge-held; after the merge, the questions, the
	// queue re-read and the Next act.
	landSitting := func(o outcome, s *relay.Sitting) {
		err := errors.New("no merge configured")
		var sha string
		if opts.Merge != nil {
			mr := orc.row
			mr.ID = o.id // the sitting's branch
			sha, err = opts.Merge(mr, o.g)
		}
		var refused *relay.MergeRefused
		switch {
		case err == nil:
			s.MergeSHA = sha
			// ar32b act 3 (§6): a green build row the sitting merged has
			// landed through it and takes the sitting's merge, so a DONE
			// ends; before any early return below (review round 1, F1),
			// so a stop, a question or a queue refused names it merged.
			// Post-review F1: each writes its own `merge` line, merged at
			// the sitting's merge via the sitting, so `agentik-relay report`
			// counts it merged, never waiting on a hand merge.
			for _, id := range ids {
				if bs := n.Sittings[id]; !orc.is(id) && bs.State == relay.Green && bs.MergeSHA == "" &&
					landed(fx.Git, p.Root, baseOf(q, id), id, orc.row.ID) {
					bs.MergeSHA = sha
					setRow(id, bs)
					put("merge", map[string]any{"row": id, "root": p.Root, "result": "merged", "sha": sha, "via": o.id,
						"detail": "landed through " + o.id})
				}
			}
		case errors.As(err, &refused):
			fmt.Fprintf(out, "relay %s: merge: %v\n", o.id, err)
			s.State, s.Reason = relay.Red, refused.Reason
			return
		default:
			fmt.Fprintf(out, "relay %s: merge: %v\n", o.id, err)
			stopWith(relay.MergeHeld)
			stopDetail = fmt.Sprintf("branch %s: %v", o.id, err)
			return
		}
		questions(sha)
		if n.State != relay.NightRunning {
			return
		}
		if why := reread(); why != "" {
			stopWith(relay.QueueInvalid)
			stopDetail = why
			return
		}
		judge(sha)
	}
	// inScope is whether row id is in the paused scope (AR71 ruling 3):
	// every row, or a row whose queue model holds one of its words.
	inScope := func(id string) bool {
		if scope == nil {
			return true
		}
		for _, w := range scope {
			if ofModel(rows[id].Model, w) {
				return true
			}
		}
		return false
	}
	// widen takes a hit's scope, word ("" every row), into the night's
	// (AR71 ruling 3): the first pause sets it; a later model's hit adds
	// its word to a scope of words and leaves an every-row scope every row
	// (N1); any other later hit makes it every row. It reports whether a
	// later hit changed it.
	widen := func(first bool, word string) bool {
		switch {
		case first:
			scope = nil
			if word != "" {
				scope = []string{word}
			}
			return false
		case scope == nil:
			return false
		case word == "":
			scope = nil
		case !contains(scope, word):
			scope = append(scope, word)
			sort.Strings(scope)
		default:
			return false
		}
		return true
	}
	// holdRunning kills every running row of the scope not already
	// stopped, and pauses it at once (AR65 ruling 4; AR71: a model's
	// scope, only its rows), adding it to paused, sorted.
	holdRunning := func(paused []string) []string {
		for id, h := range running {
			if killedBy[id] || accountBy[id] != "" || !inScope(id) {
				continue
			}
			accountBy[id] = relay.UsageLimit
			h.stopFor(relay.UsageLimit)
			put("kill", map[string]any{"row": id, "reason": string(relay.UsageLimit)})
			pauseOf[id] = pauses
			ps := n.Sittings[id]
			ps.State, bug = stepSitting(relay.Exited, relay.Limited, bug)
			ps.Reason = relay.UsageLimit
			setRow(id, ps)
			paused = append(paused, id)
		}
		sort.Strings(paused)
		return paused
	}
	// accountKill is an account stop's (run 10 ruling 1): every other
	// running row is killed and fails with the same reason; no worktree is
	// touched, so each keeps its files.
	accountKill := func(r relay.StopReason) {
		for id, h := range running {
			if accountBy[id] == "" && !killedBy[id] {
				accountBy[id] = r
				h.stopFor(r)
				put("kill", map[string]any{"row": id, "reason": string(r)})
			}
		}
	}
	collect := func(o outcome) {
		delete(running, o.id)
		readActs(o.id) // one last read after the exit
		delete(views, o.id)
		s := n.Sittings[o.id]
		if s.State == relay.Paused {
			// A killed sibling's late exit (ar64-r1): its held line
			// written paused. Its own hit (AR69 ruling 5) asks pauseFor
			// with the pause's inputs: a stop writes the line unpaused,
			// unlocks the stage and stops the night; a reset's wake later
			// than the night's moves the wake there, with a second pause
			// line; one at the night's wake writes the line too, the wake
			// unmoved; a later scope (AR71 ruling 3) widened to every row
			// kills and pauses the running rows outside the old one, named
			// by the line. The count stays. A line that broke an invariant halts
			// the row failed invariant, its stage unlocked, and stops the
			// night (AR70 ruling 4). With no held line, the part ran its
			// gate to the end (killed in its gate) or failed on its own
			// (auth, orphans): its verdict lands as any outcome below,
			// never gated again (AR70 ruling 3); a skipped one stays
			// paused for the halt. A held line collected on a running
			// night (after the wake) takes the ordinary path: the pause's
			// usage-limit mark kept, it fails usage-limit and stops the
			// night, as before AR70 (the fix round's F1).
			if h := o.hold; h != nil && (n.State == relay.NightPaused || relay.NightTerminal(n.State)) {
				if h.Limit.Kind != "" && o.s.Reason == relay.UsageLimit && !relay.NightTerminal(n.State) { // a broken invariant: as before (F1)
					// Its count is its own pause's less one (AR69 ruling 5;
					// a fresh hit since may have raised the night's), and
					// the probe is armed only by a fallback wake (AR71's
					// fix round, F2, F3). A stop kills every running row,
					// as an account stop does (F1).
					c, ok := pauseOf[o.id]
					if !ok {
						c = pauses
					}
					pz := pauseFor(pauseIn{Limit: h.Limit, Now: fx.Clock.Now().Round(0), Until: until,
						Draining: n.State == relay.Draining, Count: c - 1, Waited: waited, ProbePassed: probeOK || !fellBack})
					if pz.Stop {
						h.write(false)
						o.st.Unlock()
						stopDetail = pz.Detail
						accountKill(relay.UsageLimit)
						step(relay.OnSittingFailed, relay.UsageLimit)
						return
					}
					named, widened := []string{o.id}, widen(false, pz.Scope)
					if widened {
						named = holdRunning(named)
					}
					// The line when the wake moves later or the scope
					// widened, and for a model's hit at the wake (N1);
					// else as today (AR69 ruling 5; the fix round's F4).
					l := h.Limit
					later := !l.Reset.IsZero() && pz.Wake.After(wake)
					meets := l.Kind == stream.KindModel && !l.Reset.IsZero() && pz.Wake.Equal(wake)
					if later {
						wake, keptReset = pz.Wake, l.Reset
					}
					if widened || later || meets {
						put("pause", map[string]any{"rows": named, "kind": l.Kind, "resets_at": l.Line()["resets_at"], "source": pz.Source,
							"wake": wake.Format(time.RFC3339), "count": pauses, "scope": strings.Join(scope, " ")})
						fmt.Fprintf(out, "relay: paused until %s (%s, %s): %s\n", wake.Format(time.RFC3339), l.Kind, pz.Source, strings.Join(named, " "))
					}
				}
				h.write(true)
				if o.s.Reason != relay.UsageLimit { // its line broke an invariant (AR70 ruling 4)
					if s.State, bug = stepSitting(s.State, relay.Halt, bug); bug != nil {
						return
					}
					s.Reason = o.s.Reason
					o.st.Unlock()
					setRow(o.id, s)
					fmt.Fprintf(out, "relay %s: %s %s\n", o.id, s.State, s.Reason)
					fmt.Fprintln(out, relay.ResumeLine(o.id))
					if !relay.NightTerminal(n.State) {
						step(relay.OnSittingFailed, s.Reason)
					}
					return
				}
				heldSt[o.id] = o.st
				return
			}
			var ev relay.Event // the gate's event by the part's state (AR70 ruling 3)
			switch o.s.State {
			case relay.Green:
				ev = relay.GateGreen
			case relay.Red:
				ev = relay.GateRed
			case relay.Failed:
				ev = relay.GateFailed
			default: // skipped: paused for the halt
				return
			}
			if o.hold == nil { // a verdict, not the pause's kill: its own reason (F1: a held line keeps the mark)
				delete(accountBy, o.id)
			}
			if s.State, bug = stepSitting(s.State, ev, bug); bug != nil {
				return
			}
		}
		// An end with no usage-limit refusal passes the probe (AR69's F1),
		// its part launched since the last fallback wake (AR70: a verdict
		// landing from a part killed before it does not).
		if o.s.Reason != relay.UsageLimit && fresh[o.id] {
			probeOK = true
		}
		// The held line first (AR65 ruling 3): the night decides on the
		// part's Limit, writes the line, then acts; a pause keeps the
		// stage locked.
		pz := pause{Stop: true}
		if h := o.hold; h != nil {
			if h.Limit.Kind != "" && !killedBy[o.id] && accountBy[o.id] == "" && !relay.NightTerminal(n.State) {
				pz = pauseFor(pauseIn{Limit: h.Limit, Now: fx.Clock.Now().Round(0), Until: until, Draining: n.State == relay.Draining,
					Count: pauses, Waited: waited, ProbePassed: probeOK || !fellBack}) // the probe armed only by a fallback wake (F2)
			}
			h.write(!pz.Stop)
			if pz.Stop || o.s.Reason != relay.UsageLimit { // a broken invariant: no `limit` on the line, the part failed as before AR65 (F1)
				pz.Stop = true
				o.st.Unlock()
			}
		}
		s.State, s.Reason = o.s.State, o.s.Reason
		if !pz.Stop { // the pause (AR65 ruling 4)
			// A model's pauses only its rows (AR71 ruling 3): the scope
			// takes the hit's, and every running row of it is killed and
			// paused at once, the others run on. A fresh hit while paused
			// (a row outside the scope) counts one more pause and moves
			// the wake and the kept reset only later.
			first := n.State != relay.NightPaused
			s.State, bug = stepSitting(relay.Exited, relay.Limited, bug)
			s.Reason = relay.UsageLimit
			heldSt[o.id] = o.st
			setRow(o.id, s)
			widen(first, pz.Scope)
			pauses++
			paused := holdRunning([]string{o.id})
			if first || pz.Wake.After(wake) {
				wake, keptReset = pz.Wake, o.hold.Limit.Reset
			}
			step(relay.OnLimited, "")
			l := o.hold.Limit
			put("pause", map[string]any{"rows": paused, "kind": l.Kind, "resets_at": l.Line()["resets_at"], "source": pz.Source,
				"wake": wake.Format(time.RFC3339), "count": pauses, "scope": strings.Join(scope, " ")})
			fmt.Fprintf(out, "relay %s: paused usage-limit\n", o.id)
			fmt.Fprintf(out, "relay: paused until %s (%s, %s): %s\n", wake.Format(time.RFC3339), l.Kind, pz.Source, strings.Join(paused, " "))
			return
		}
		if pz.Detail != "" && !relay.NightTerminal(n.State) { // pauseFor's stop: the stop line's detail
			stopDetail = pz.Detail
		}
		if killedBy[o.id] {
			s.State, s.Reason = relay.Failed, relay.OwnerKill
		} else if r := accountBy[o.id]; r != "" {
			s.State, s.Reason = relay.Failed, r
		}
		if s.State == relay.Failed && relay.AccountStop(s.Reason) {
			accountKill(s.Reason) // the account is spent or signed out (run 10 ruling 1)
		}
		r := rows[o.id]
		switch {
		case orc.is(o.id):
			if s.State == relay.Green {
				landSitting(o, s)
			}
			orc.red = s.State == relay.Red
		case s.State == relay.Green && r.Merge == queue.MergeOnGreen && opts.Merge != nil:
			var refused *relay.MergeRefused
			if sha, err := opts.Merge(r, o.g); err == nil {
				s.MergeSHA = sha
				if orc != nil { // a row landed: the stuck count restarts (§4)
					orc.stuck = 0
					questions(sha)
				}
			} else {
				fmt.Fprintf(out, "relay %s: merge: %v\n", o.id, err)
				if errors.As(err, &refused) { // git-state-moved, merge-tree-differs: the row is red
					s.State, s.Reason = relay.Red, refused.Reason
				}
			}
		}
		if orc != nil && !orc.is(o.id) {
			orc.hold = false // a build row ended: WAITING releases (§4)
		}
		if !orc.is(o.id) && s.State != relay.Green {
			// Review round 1, F2 (§5): a row whose merge was refused, or
			// that was killed after a green gate, ends red or failed; its
			// head is then no merge an orchestrator's gate may prove.
			ForgetHead(p.Root, o.id)
		}
		setRow(o.id, s)
		line := fmt.Sprintf("relay %s: %s", o.id, s.State)
		if s.Reason != "" {
			line += " " + string(s.Reason)
		}
		fmt.Fprintln(out, line)
		// AR31 act 2: each log the row's gate kept, under its line (a gate
		// stopped after its lines ran writes no check line to the ledger).
		for _, l := range o.g.Logs() {
			fmt.Fprintf(out, "relay %s: log: %s\n", o.id, l)
		}
		if s.State != relay.Green {
			fmt.Fprintln(out, relay.ResumeLine(o.id))
		}
		// AR62 act 2: a red or failed row's cause, after its resume line
		// (the state, log and resume lines stay adjacent).
		cause := ""
		if (s.State == relay.Red || s.State == relay.Failed) && RedCause != nil {
			cause = RedCause(o.g)
		}
		lg.mu.Lock()
		lg.rows[o.id].Cause = cause
		lg.mu.Unlock()
		if cause != "" {
			fmt.Fprintln(out, clip("relay "+o.id+": cause: "+cause, 99))
		}
		if relay.NightTerminal(n.State) {
			return
		}
		switch s.State {
		case relay.Green:
			if n.State != relay.Draining || len(running) == 0 {
				step(relay.OnSittingGreen, "")
			}
		case relay.Failed:
			step(relay.OnSittingFailed, s.Reason)
		default: // red (an unrun gate too): the packet's, the night moves on
			if s.Reason == relay.Stalled { // a second stall is the driver's (R6 §2)
				if stalls++; stalls == 2 {
					stopWith(relay.StalledTwice)
					stopDetail = "a second stalled row: " + o.id
					return
				}
			}
			if n.State != relay.Draining || len(running) == 0 {
				step(relay.OnSittingRed, "")
			}
		}
	}
	// checkControl reads the owner's stop and kills after every event. While
	// paused (AR66 rulings 1, 2) a stop is owner-stop at once, no draining,
	// and a kill naming a paused row stops the night owner-kill as `kill *`
	// does, with no `kill` line (the row has no child); the halt then fails
	// each paused row usage-limit.
	checkControl := func() {
		stop, ks := ReadControl(fx.FS, control)
		for _, k := range ks[min(kills, len(ks)):] {
			for id, h := range running {
				if k == "*" || k == id {
					killedBy[id] = true
					h.kill()
					put("kill", map[string]any{"row": id})
				}
			}
			paused := n.Sittings[k] != nil && n.Sittings[k].State == relay.Paused
			if !relay.NightTerminal(n.State) && (k == "*" || running[k] != nil || paused) {
				step(relay.OnOwnerKill, "")
			}
		}
		kills = len(ks)
		if stop && (n.State == relay.NightRunning || n.State == relay.NightPaused) {
			put("owner-stop", map[string]any{})
			step(relay.OnOwnerStop, "")
			stopAll(ErrOwnerStop)
		}
	}

	// checkWindows follows every loop event: a session lost under auto, or
	// under required once the night is draining (NightStep lists no
	// tmux-gone there), is a notice and file windows for the rest of the
	// night; under required while running, the tmux-gone stop (§1).
	checkWindows := func() {
		fallback, gone := win.Check()
		if gone && n.State == relay.NightRunning {
			step(relay.OnTmuxGone, "")
			return
		}
		if fallback || gone {
			win.Mode = "never"
			fmt.Fprintf(out, "relay: tmux session %s is gone; file windows for the rest of the night\n", win.Session)
			send(effects.Notice{Event: "tmux-gone", Night: n.ID, Priority: 4,
				Title: "relay " + n.ID + " tmux-gone", Body: "session=" + win.Session + " fallback=file-windows"})
		}
	}

	// onSignal is the trapped signal (ADV6-2): every group already has its
	// SIGTERM; the running rows are the owner's kill.
	onSignal := func(name string) {
		for id, h := range running {
			killedBy[id] = true
			h.kill()
			put("kill", map[string]any{"row": id, "signal": name})
		}
		if !relay.NightTerminal(n.State) {
			step(relay.OnOwnerKill, "")
			stopDetail = "signal:" + name
		} else if stopDetail == "" {
			stopDetail = "signal:" + name
		}
	}

	// checkLock is the holder's poll (AR22 act 2, ADV13-4): relay.pid gone
	// or a new inode at its path stops the night lock-moved; no new
	// sitting starts, the running ones finish.
	checkLock := func() {
		if why := LockMoved(); why != "" && !relay.NightTerminal(n.State) {
			stopWith(relay.LockMoved)
			stopDetail = why
		}
	}

	// wakeUp is the paused night at its wake (AR67 rulings 3, 4): OnWake,
	// the `wake` line, then --until and --max-usd, not --max-sittings
	// (ar64-r1); either stops and the halt follows. Then a paused
	// orchestrator sitting is halted, never continued (AR68 ruling 4):
	// Paused steps Halt, failed usage-limit, its stage unlocked, its tree
	// kept, printed with no resume line; the loop cuts the standing row's
	// next sitting, and the cut marks it woken (a stop first leaves it red,
	// the fix round's F1). Then every other paused row is checked
	// before any continues: a moved tree stops the night usage-limit
	// naming it, and the halt fails every paused row. Else each continues,
	// in queue order. A paused row whose outcome is still out (its gate
	// outlasts the pause), the orchestrator's sitting included, is named
	// by the `wake` line and neither halted, checked nor continued: it
	// lands when collected (AR70 ruling 5).
	wakeUp := func(now time.Time) {
		var paused []string
		for _, id := range ids {
			if s := n.Sittings[id]; s != nil && s.State == relay.Paused {
				paused = append(paused, id)
			}
		}
		named := append([]string(nil), paused...)
		sort.Strings(named)
		step(relay.OnWake, "")
		scope = nil        // the wake clears the scope (AR71 ruling 3)
		waited = keptReset // the guards' reset waited out (AR69 ruling 4)
		fellBack = waited.IsZero()
		if waited.IsZero() {
			probeOK = false // a fallback wake arms the probe (AR69's F1)
			clear(fresh)
		}
		put("wake", map[string]any{"rows": named, "wake": wake.Format(time.RFC3339), "count": pauses})
		switch {
		case !until.IsZero() && !now.Before(until):
			step(relay.OnUntil, "")
			return
		case opts.MaxUSD > 0 && lg.total() >= opts.MaxUSD:
			step(relay.OnBudget, "")
			return
		}
		var builds []string
		for _, id := range paused {
			if running[id] != nil { // its outcome still out (AR70 ruling 5)
				continue
			}
			if !orc.is(id) {
				builds = append(builds, id)
				continue
			}
			s := n.Sittings[id]
			if s.State, bug = stepSitting(s.State, relay.Halt, bug); bug != nil {
				return
			}
			s.Reason = relay.UsageLimit
			if st := heldSt[id]; st != nil {
				st.Unlock()
				delete(heldSt, id)
			}
			halted[id] = true // woken once its next sitting is cut (the fix round's F1)
			setRow(id, s)
			fmt.Fprintf(out, "relay %s: %s %s\n", id, s.State, s.Reason)
		}
		paused = builds
		for _, id := range paused {
			st := heldSt[id]
			if st == nil { // a paused row with no outcome out holds its stage
				bug = fmt.Errorf("%w: %s paused with no held stage at the wake", ErrBug, id)
				return
			}
			if !treeHeld(fx.Git, st) {
				stopDetail = id + ": the tree moved while paused"
				step(relay.OnSittingFailed, relay.UsageLimit)
				return
			}
		}
		for _, id := range paused {
			continueRow(id)
		}
	}

	// pickPaused is the pick while a model's scope holds (AR71 ruling 4,
	// N2): as a running night's, but none once --until, --max-sittings or
	// --max-usd is reached; busy the running and the paused rows, each
	// keeping its slot and tree, so the wake's continuations stay within
	// --parallel; the scope's rows held. It steps none of until, count,
	// budget, queue-empty, blocked or an orchestrator's cut, and checks no
	// window: the paused rows wait for the wake, which checks --until and
	// --max-usd. It reports whether a row started.
	pickPaused := func() bool {
		if now := fx.Clock.Now(); !until.IsZero() && !now.Before(until) || opts.MaxSittings > 0 && count >= opts.MaxSittings || opts.MaxUSD > 0 && lg.total() >= opts.MaxUSD {
			return false
		}
		var busy []string
		for id := range running {
			busy = append(busy, id)
		}
		for _, id := range ids {
			if s := n.Sittings[id]; running[id] == nil && s != nil && s.State == relay.Paused {
				busy = append(busy, id)
			}
		}
		sort.Strings(busy)
		next, _, err := PickIn(q, n.Sittings, busy, opts.Parallel, PickOptions{Satisfied: satisfiedAt, Refused: refused, Hold: inScope})
		if err != nil { // the night's order, read before the pause: nothing starts
			return false
		}
		progressed := false
		for _, id := range next {
			if picked[id] {
				stopWith(relay.NoProgress)
				break
			}
			if opts.MaxSittings > 0 && count >= opts.MaxSittings {
				break
			}
			start(id) // counts the row once it stages (AR58)
			progressed = true
		}
		return progressed
	}

	tick := time.NewTicker(opts.Poll)
	defer tick.Stop()
	status()
	// AR34 fix (F2): a stop or a kill written after the open's read and
	// before this first status.json was told "before any row starts" (the
	// verb found no status.json); it is read here, before the first pick,
	// by act 1's rule. From here the verb says "after its next event".
	if n.State == relay.NightRunning {
		if stop, ks := ReadControl(fx.FS, control); kept(stop, ks[min(kills, len(ks)):], "said before the first row: ") {
			kills = len(ks)
		}
	}
	for bug == nil {
		select { // a signal before the loop (the plan's lines) or between its events
		case name := <-caught:
			onSignal(name)
		default:
		}
		checkLock()
		if n.State == relay.NightPaused {
			// The poll (AR65 ruling 5): the wall clock against the wake,
			// no timer. At the wake, wakeUp (AR67). Every row paused, no
			// pick; a model's scope, pickPaused (AR71).
			if now := fx.Clock.Now().Round(0); !now.Before(wake) {
				wakeUp(now)
			}
		}
		if n.State == relay.NightPaused && scope != nil && pickPaused() { // a model's scope: the rows outside it run on (AR71)
			status()
			continue
		}
		if n.State == relay.NightRunning {
			now := fx.Clock.Now()
			var limit relay.NightEvent
			switch {
			case !until.IsZero() && !now.Before(until):
				limit = relay.OnUntil
			case opts.MaxSittings > 0 && count >= opts.MaxSittings:
				limit = relay.OnCount
			case opts.MaxUSD > 0 && lg.total() >= opts.MaxUSD:
				limit = relay.OnBudget
			}
			if limit != "" {
				if len(running) == 0 {
					step(limit, "")
					continue
				}
			} else {
				var busy []string
				for id := range running {
					busy = append(busy, id)
				}
				sort.Strings(busy)
				next, blocked, err := PickIn(q, n.Sittings, busy, opts.Parallel, PickOptions{Satisfied: satisfiedAt, Refused: refused})
				if err != nil { // nothing can start: §1's no-progress, never blocked
					stopWith(relay.NoProgress)
					stopDetail = err.Error()
				}
				progressed := false
				for _, id := range next {
					if picked[id] {
						stopWith(relay.NoProgress)
						break
					}
					if opts.MaxSittings > 0 && count >= opts.MaxSittings {
						break
					}
					start(id) // counts the row once it stages (AR58)
					progressed = true
				}
				if progressed {
					checkWindows()
					status()
					continue // a row that could not stage frees its slot: pick again
				}
				if orc != nil && len(running) == 0 && n.State == relay.NightRunning {
					// The standing row before queue-empty and blocked (§4).
					switch {
					case orc.red:
						stopWith(relay.OrchestratorRed)
						stopDetail = "red: " + orc.current
					case orc.hold:
						stopWith(relay.StillWaiting)
						stopDetail = "no build row ready: " + ActKey(orc.start.Act)
					default:
						id, err := nextSitting()
						if err != nil {
							stopWith(relay.NoProgress)
							stopDetail = "the next sitting: " + err.Error()
							continue
						}
						start(id)                             // counts the sitting once it stages (AR58)
						if running[id] == nil && bug == nil { // it could not stage: never cut another
							stopWith(relay.NoProgress)
							stopDetail = id + ": " + lg.rows[id].Reason
						} else if running[id] != nil {
							// Cut: each sitting the wake halted is recovered
							// (AR68 ruling 5; the fix round's F1).
							for h := range halted {
								n.Woken[h] = true
								delete(halted, h)
								setRow(h, n.Sittings[h])
							}
						}
						checkWindows()
						status()
					}
					continue
				}
				if len(running) == 0 && n.State == relay.NightRunning {
					if blocked {
						step(relay.OnBlocked, "")
					} else {
						step(relay.OnQueueEmpty, "")
					}
					continue
				}
			}
		}
		if len(running) == 0 {
			if n.State == relay.Draining {
				step(relay.OnDrained, "")
			}
			if relay.NightTerminal(n.State) {
				break
			}
		}
		select {
		case o := <-done:
			collect(o)
		case name := <-caught:
			onSignal(name)
		case <-tick.C:
		}
		checkControl()
		if n.State != relay.NightPaused { // no window check while paused (N1)
			checkWindows()
		}
		status()
	}
	for len(running) > 0 { // a bug: let the children finish before returning
		collect(<-done)
	}
	// The halt (AR65 ruling 6): a night that ends paused fails each paused
	// row usage-limit, unlocks its stage and keeps its tree; its sitting
	// line is already written, so no second line and no notice.
	for _, id := range ids {
		s := n.Sittings[id]
		if s == nil || s.State != relay.Paused {
			continue
		}
		s.State, bug = stepSitting(s.State, relay.Halt, bug)
		s.Reason = relay.UsageLimit
		if st := heldSt[id]; st != nil {
			st.Unlock()
		}
		setRow(id, s)
		fmt.Fprintf(out, "relay %s: %s %s\n", id, s.State, s.Reason)
		fmt.Fprintln(out, relay.ResumeLine(id))
	}
	for _, id := range ids { // a sitting the wake halted, the night stopped before its next cut (the fix round's F1)
		if halted[id] {
			fmt.Fprintln(out, relay.ResumeLine(id))
		}
	}
	var resume []string
	unmerged := []string{} // ar32b act 4 (§6): build rows green on their branches
	red := 0
	for _, id := range ids {
		s := n.Sittings[id]
		if !relay.Terminal(s.State) {
			skip(id, "the night stopped: "+string(n.Reason))
		}
		if (s.State == relay.Red || s.State == relay.Failed) && !n.Woken[id] { // a woken sitting is recovered (AR68)
			red++
			resume = append(resume, relay.ResumeLine(id))
		}
		if s.State == relay.Green && s.MergeSHA == "" && !orc.is(id) {
			unmerged = append(unmerged, id)
		}
	}
	status()
	if bug != nil {
		put("night-close", map[string]any{"state": string(n.State), "error": bug.Error()})
		return n, bug
	}
	put("stop", map[string]any{"reason": string(n.Reason), "detail": stopDetail, "state": string(n.State), "sittings": count,
		"red": red, "cost_usd": lg.usd, "unread": unreadOf(lg.rows), "spent": lg.total(), "resume": resume, "unmerged": unmerged, "budget": p.Budget,
		"root": p.Root}) // the ledger is the account's: the day's refusal matches on it (ar32a review round 2)
	put("night-close", map[string]any{"state": string(n.State),
		"duration_ms": fx.Clock.Now().Sub(n.Started).Milliseconds()})
	return n, nil
}

func (t *tap) total() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.usd + t.held
}

// stepSitting is relay.Step with its error kept as a bug.
func stepSitting(s relay.State, e relay.Event, bug error) (relay.State, error) {
	next, err := relay.Step(s, e)
	if err != nil && bug == nil {
		return s, fmt.Errorf("%w: Step(%s, %s): %v", ErrBug, s, e, err)
	}
	return next, bug
}

// Clean reports whether a night stopped clean (§9: exit 0): queue-empty,
// count-reached, until-reached, owner-stop or an orchestrator's done
// (ar32a review round 1), and no row red or failed but a sitting the wake
// halted, which is recovered (AR68 ruling 5).
func Clean(n *Night) bool {
	switch n.Reason {
	case relay.QueueEmpty, relay.CountReached, relay.UntilReached, relay.OwnerStop, relay.Done:
	default:
		return false
	}
	for id, s := range n.Sittings {
		if (s.State == relay.Red || s.State == relay.Failed) && !n.Woken[id] {
			return false
		}
	}
	return true
}
