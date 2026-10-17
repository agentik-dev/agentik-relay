package night

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/hooks"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/relay"
	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// KillGrace is §7's wait between SIGTERM and SIGKILL; a variable so a test
// can shorten it.
var KillGrace = 30 * time.Second

// Ceiling is the share of the row's context that writes the `ceiling` line
// and notifies (§7).
const Ceiling = 0.8

// SoftLine is the share of the row's context whose first top-level message
// writes the soft request (hooks.SoftPath) and the `soft` line: the signal
// to pause, which the child hears through its PostToolUse hook. HardCap is
// the share that kills (ceiling-kill), a runaway's stop alone; nothing kills
// at the whole (AR28, the owner's word of 2026-09-25 07:58).
const (
	SoftLine = 0.9
	HardCap  = 1.2
)

// Tune is the `exit` line's record for tuning HardCap (AR28): the turn of
// the soft request (0: none), the first turn whose head moved after it (0:
// none), the context at the exit, and whether HardCap killed the child.
type Tune struct {
	SoftTurn    int  `json:"soft_turn"`
	CommitTurn  int  `json:"commit_turn"`
	ContextExit int  `json:"context_exit"`
	Killed      bool `json:"killed"`
}

// RunOptions shape one sitting. Context is the row's context ceiling in
// tokens and Wall its wall clock (zero: none). Log takes the live line
// (D16: <state>/nights/<night>/<row>.log without tmux); Notify the ceiling's
// warning. Gate is A6's gate over the staged tree with the stream's exit
// facts (injected: the gate package imports this one); nil leaves the
// sitting exited. Env is the environment ChildEnv strips (nil: the
// process's); Transcripts the directory holding <session>/subagents/ (empty:
// ~/.claude/projects/<the worktree's slug>). StallSeconds is the
// watchdog's `--stall-seconds` (0: DefaultStallSeconds; §15); Wall zero is
// DefaultWall, never none. Deps is, per edge of the row, whether §3
// satisfied it when the night started the row (nil: not read); the
// sitting line's invariants read it. Ctx is the night's context for the
// row (nil: none): cancelled with ErrOwnerStop or ErrOwnerKill, it ends a
// breaker wait or the spawn's spacing within one poll (R6 §4).
type RunOptions struct {
	Ctx          context.Context
	Context      int
	Deps         map[string]bool
	Wall         time.Duration
	StallSeconds int
	Log          io.Writer
	Notify       func(string)
	Gate         func(st *Staged, seen, isError bool, w GateWait) (relay.Result, relay.StopReason, relay.Gate)
	Env          []string
	Transcripts  string
	// RulesHash is the template hash the row's prompt was rendered under
	// and Block the plan's block source (AR30): the spawn and sitting
	// lines record them; empty, the template hash with no block.
	RulesHash, Block string
	// Part numbers a paused row's part (AR67 ruling 5): from 2, a
	// continuation's, each line through appendLine carries `part`.
	Part int

	// Night names the night in a band ask's tool use id and Until is the
	// night's --until (zero: none), which ends a band hold (AR80).
	Night string
	Until time.Time

	space *spacer    // the night's: spawns SpawnSpacing apart (ADV6-13)
	hold  *heldLine  // the night's: a usage-limit part's held line (AR65; nil: written here)
	band  *bandQueue // the night's: the rows asking the band, in launch order (AR80 ruling 6; nil: none)
	// asked is the night's: called after each re-ask in a hold with its
	// verdict, which writes no line (AR81 ruling 2), and with `queued` and
	// `behind <head>` when a queued row's head changes (the fix round's
	// finding 1); nil: none.
	asked func(effects.BandVerdict)
	// wait is the night's: the row's kill channel and the gate lock's mark,
	// which RunRow passes to Gate (AR78; zero: a kill never ends the wait).
	wait GateWait
}

// GateWait is what a row's gate waits for its root's gate lock with
// (AR78; GateLock): Killed, closed by the owner's kill alone, ends the
// wait; Waiting marks it (behind the holder's row, then "" at its end).
type GateWait struct {
	Killed  <-chan struct{}
	Waiting func(behind string, since time.Time)
}

// The causes the night cancels a row's Ctx with (R6 §4): a stop leaves a
// row still waiting to spawn skipped, a kill fails it owner-kill.
var (
	ErrOwnerStop = errors.New(string(relay.OwnerStop))
	ErrOwnerKill = errors.New(string(relay.OwnerKill))
)

// SpawnSpacing is the least time between two spawns of one night, one
// breaker poll (ADV6-13): the 1-minute load a spawn's breaker reads lags,
// so a burst of spawns would all pass it.
var SpawnSpacing = DefaultBreaker.Poll

// spacer keeps a night's spawns SpawnSpacing apart through the clock.
type spacer struct {
	clock effects.Clock
	mu    sync.Mutex
	last  time.Time
}

// wait holds a spawn until SpawnSpacing after the last one, then marks
// it: false when ctx ended first.
func (sp *spacer) wait(ctx context.Context) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if !sp.last.IsZero() {
		if d := sp.last.Add(SpawnSpacing).Sub(sp.clock.Now()); d > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-sp.clock.After(d):
			}
		}
	}
	sp.last = sp.clock.Now()
	return true
}

// BandPoll is a held row's wait between two asks of the session band,
// through the clock (band-pacing-r1 §2.4; AR80).
var BandPoll = time.Minute

// BandSettle is the longest a parallel night's next ask waits for the row
// admitted ahead of it to show to the band, through the clock: twice the
// worst second message seen after a spawn, with SpawnSpacing
// (band-pacing-r1 §2.4; AR87 ruling 3).
var BandSettle = 2 * time.Minute

// The band's verdicts (band-pacing-r1 §2.3): these spawn, these hold; any
// other is unchecked and spawns.
var (
	bandSpawns = map[string]bool{"admitted": true, "unread": true, "stale": true}
	bandHolds  = map[string]bool{"not-started": true, "deferred": true, "wait": true, "denied": true}
)

// bandQueue is a night's rows asking the band, in launch order (AR80
// ruling 6): only the head asks; the next asks once a head that left on a
// spawning verdict has shown to the band, exited, or waited BandSettle
// (AR87), and no sooner than one BandPoll after a held one. moved closes
// at every change.
type bandQueue struct {
	clock    effects.Clock
	mu       sync.Mutex
	rows     []string
	holding  map[string]bool // the rows whose hold is open, a queued one's too (the fix round's finding 3)
	after    time.Time       // the head asks no sooner: one BandPoll after a held head's leave
	settling string          // the head that left to spawn and has not shown (AR87 ruling 2; "": none)
	settleBy time.Time       // its leave plus BandSettle
	moved    chan struct{}
}

// join takes row's place at the back; launch calls it, so places keep the
// launch order.
func (q *bandQueue) join(row string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.rows = append(q.rows, row)
}

// place reads row's place: whether it heads the queue (a row with no place
// does), whether the head holds and which it is, the row that settles
// when its cap is after the BandPoll mark ("": none), the head's earliest
// ask (the later of the two), and the channel the next change closes.
func (q *bandQueue) place(row string) (head, held bool, first, waits string, after time.Time, moved <-chan struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.moved == nil {
		q.moved = make(chan struct{})
	}
	if len(q.rows) == 0 || !slices.Contains(q.rows, row) {
		return true, false, row, "", time.Time{}, q.moved
	}
	after = q.after
	if q.settling != "" && q.settleBy.After(after) {
		waits, after = q.settling, q.settleBy
	}
	return q.rows[0] == row, q.holding[q.rows[0]], q.rows[0], waits, after, q.moved
}

// changed wakes every waiter; under mu.
func (q *bandQueue) changed() {
	if q.moved != nil {
		close(q.moved)
	}
	q.moved = make(chan struct{})
}

// hold marks row's hold open, after its line is written (the fix
// round's finding 4): a head whose hold is open, a queued row's that came
// to the head included, makes the rows behind it queue.
func (q *bandQueue) hold(row string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.holding == nil {
		q.holding = map[string]bool{}
	}
	if !q.holding[row] {
		q.holding[row] = true
		q.changed()
	}
}

// leave gives up row's place; the head's leaving sets the next head's
// earliest ask, now after an unheld head, one BandPoll on after a held one,
// and a head that leaves to spawn settles until it shows or BandSettle
// (AR87 ruling 2); a non-spawning leave settles nothing.
func (q *bandQueue) leave(row string, held, spawns bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, r := range q.rows {
		if r != row {
			continue
		}
		if i == 0 {
			now := q.clock.Now()
			q.after = time.Time{}
			if held {
				q.after = now.Add(BandPoll)
			}
			if spawns {
				q.settling, q.settleBy = row, now.Add(BandSettle)
			}
		}
		delete(q.holding, row)
		q.rows = append(q.rows[:i], q.rows[i+1:]...)
		q.changed()
		return
	}
}

// shown ends row's settle: it has shown to the band (its second
// main-thread message), its child exited, or its part returned before a
// spawn (AR87 ruling 2). A nil queue, or a row not settling: nothing.
func (q *bandQueue) shown(row string) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if row == "" || q.settling != row {
		return
	}
	q.settling, q.settleBy = "", time.Time{}
	q.changed()
}

// bandRecord is a part's band: its asks, its hold's wait and the verdict
// it spawned on (the plugin's, `reset` or `unchecked`; a cut hold's last).
type bandRecord struct {
	asks, waitS int
	verdict     string
	peak        *float64
}

// How a band hold ended: the row spawns, the context cut it, or --until.
type bandEnd int

const (
	bandSpawn bandEnd = iota
	bandCut
	bandUntil
)

// peakOf is a line's peak: the number, or null.
func peakOf(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// askBand is a row's first part before the breaker (band-pacing-r1 §2.2 to
// §2.4; AR80 rulings 3 to 6). At its turn in the queue it asks; a verdict
// that spawns ends it, one that holds opens a hold (its one Reset read, a
// `band` line `held`), and a row behind a held head opens its hold
// `queued`. A hold asks again every BandPoll at the head, and ends at the
// nearest of a spawning verdict (`admitted`), its reset (`reset`, the row
// spawning unasked), --until or the context. An error, a verdict outside
// the seven, or a reset that errs or is not after now is `unchecked` and
// spawns. A re-ask writes no line but a hold's first `denied` that did not
// open it.
func askBand(ctx context.Context, fx effects.Effects, rp *RowPlan, opts RunOptions, log io.Writer) (*bandRecord, bandEnd) {
	q, clock := opts.band, fx.Clock
	r := &bandRecord{}
	var since, reset, last time.Time // the hold's start (zero: unheld), its reset, the last ask
	denied := false                  // the hold has a denied line
	behind := ""                     // the head a queued row last read behind
	said := ""                       // the settling row the log last named (AR87 ruling 5)
	line := func(m map[string]any) {
		m["row"], m["part"] = rp.ID, 1
		fx.Ledger.Append("band", m)
	}
	waited := func() int {
		if since.IsZero() {
			return 0
		}
		return int(clock.Now().Sub(since).Seconds())
	}
	leave := func(spawns bool) {
		r.waitS = waited()
		if q != nil {
			q.leave(rp.ID, !since.IsZero(), spawns)
		}
	}
	ends := func(m map[string]any) {
		if !since.IsZero() {
			m["asks"], m["wait_s"] = r.asks, r.waitS
		}
		line(m)
	}
	unchecked := func(why string) (*bandRecord, bandEnd) {
		leave(true)
		r.verdict, r.peak = "unchecked", nil
		ends(map[string]any{"state": "unchecked", "error": why})
		fmt.Fprintf(log, "relay %s: the band unchecked: %s\n", rp.ID, why)
		return r, bandSpawn
	}
	open := func(now time.Time) string { // the hold's one reset; "" when it opens
		at, err := fx.Band.Reset()
		if err != nil {
			return "reset: " + err.Error()
		}
		if !at.After(now) {
			return "reset: " + at.Format(time.RFC3339) + " not after " + now.Format(time.RFC3339)
		}
		since, reset = now, at
		return ""
	}
	for {
		now := clock.Now()
		switch {
		case ctx.Err() != nil:
			leave(false)
			return r, bandCut
		case !opts.Until.IsZero() && !now.Before(opts.Until):
			leave(false)
			return r, bandUntil
		case !since.IsZero() && !now.Before(reset):
			leave(false)
			r.verdict, r.peak = "reset", nil
			ends(map[string]any{"state": "reset"})
			return r, bandSpawn
		}
		head, headHeld, first, waits, after, moved := true, false, rp.ID, "", time.Time{}, (<-chan struct{})(nil)
		if q != nil {
			head, headHeld, first, waits, after, moved = q.place(rp.ID)
		}
		if !head && !since.IsZero() && behind != "" && first != behind { // a new head: status reads it (AR81 fix round, finding 1)
			behind = first
			if opts.asked != nil {
				opts.asked(effects.BandVerdict{Verdict: "queued", Text: "behind " + first})
			}
		}
		if !head && headHeld && since.IsZero() { // behind a held head: queued
			if why := open(now); why != "" {
				return unchecked(why)
			}
			r.verdict, behind = "queued", first
			line(map[string]any{"state": "held", "verdict": "queued", "peak": nil, "text": "behind " + first})
			q.hold(rp.ID)
			fmt.Fprintf(log, "relay %s: the band holds it behind %s\n", rp.ID, first)
			continue
		}
		var at time.Time // the next ask (zero: not the head's)
		if head {
			at = after
			if !last.IsZero() && last.Add(BandPoll).After(at) {
				at = last.Add(BandPoll)
			}
		}
		if !head || at.After(now) {
			if head && waits != "" && at.Equal(after) && waits != said { // one sentence per settle it waits on (AR87 ruling 5)
				said = waits
				fmt.Fprintf(log, "relay %s: the band waits for %s to show\n", rp.ID, waits)
			}
			wake := at
			for _, t := range []time.Time{reset, opts.Until} {
				if !t.IsZero() && (wake.IsZero() || t.Before(wake)) {
					wake = t
				}
			}
			var tick <-chan time.Time
			if !wake.IsZero() {
				tick = clock.After(wake.Sub(now))
				if !clock.Now().Before(wake) { // the clock reached it before the timer was set
					continue
				}
			}
			select {
			case <-ctx.Done():
			case <-moved:
			case <-tick:
			}
			continue
		}
		last = now
		r.asks++
		// An orchestrator sitting (Row set) asks as `relay-orchestrator`,
		// every other part as `relay-row` (fresh-session-r1 §2.5; AR85
		// ruling 1).
		kind := "relay-row"
		if rp.Row != "" {
			kind = "relay-orchestrator"
		}
		v, err := fx.Band.Ask(ctx, rp.SessionID, effects.BandRequest{Model: flagOf(rp.Spawn, "--model"),
			SubagentType: kind, Background: true, ToolUseID: opts.Night + "/" + rp.ID + "/1"})
		switch {
		case err != nil && ctx.Err() != nil: // the ask the context cut (ruling 3)
			continue
		case err != nil:
			return unchecked(err.Error())
		case bandSpawns[v.Verdict]:
			held := !since.IsZero()
			leave(true)
			r.verdict, r.peak = v.Verdict, v.Peak
			if held {
				ends(map[string]any{"state": "admitted"})
			}
			return r, bandSpawn
		case !bandHolds[v.Verdict]:
			return unchecked(fmt.Sprintf("verdict %q", v.Verdict))
		}
		r.verdict, r.peak = v.Verdict, v.Peak
		held := map[string]any{"state": "held", "verdict": v.Verdict, "peak": peakOf(v.Peak), "text": v.Text}
		if since.IsZero() {
			if why := open(now); why != "" {
				return unchecked(why)
			}
			denied = v.Verdict == "denied"
			line(held) // the line, then the rows behind wake (the fix round's finding 4)
			if q != nil {
				q.hold(rp.ID)
			}
			fmt.Fprintf(log, "relay %s: the band holds it: %s\n", rp.ID, v.Verdict)
			continue
		}
		if opts.asked != nil { // status counts the ask (AR81 ruling 2)
			opts.asked(v)
		}
		if v.Verdict == "denied" && !denied {
			denied = true
			line(held)
		}
	}
}

// wallOf is the sitting's wall clock: opts.Wall, else DefaultWall.
func wallOf(opts RunOptions) time.Duration {
	if opts.Wall > 0 {
		return opts.Wall
	}
	return DefaultWall
}

// zone is where a context sits against the ceiling: under half, under the
// 80% line, under the whole, past it.
func zone(context, ceiling int) int {
	switch {
	case ceiling <= 0 || context*2 < ceiling:
		return 0
	case float64(context) < Ceiling*float64(ceiling):
		return 1
	case context < ceiling:
		return 2
	}
	return 3
}

// RunRow is one sitting (§7): the breaker (§15: still open at its Max, no
// spawn, failed load), the `spawn` line, the child started in the
// staged tree with ChildEnv, the stream parsed with the live line written
// on every assistant message, a `turn` line every tenth turn and at each
// zone crossing, the `ceiling` line and a notify at 80%, the soft request
// and the `soft` line at SoftLine, SIGTERM then SIGKILL after KillGrace at
// HardCap (ceiling-kill) or at the wall (wall-clock), then the `exit` line
// with its Tune (the soft request removed after it), the builders from the transcript's
// subagents/ directory (its usage fields only), the gate, and the `sitting`
// line. The lock stays the caller's to release.
func RunRow(fx effects.Effects, sp effects.Spawner, st *Staged, rp *RowPlan, prompt []byte, opts RunOptions) (*relay.Sitting, relay.Gate) {
	s := &relay.Sitting{ID: rp.ID, State: relay.Running}
	log := opts.Log
	if log == nil {
		log = io.Discard
	}
	notify := opts.Notify
	if notify == nil {
		notify = func(string) {}
	}
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	appendLine := func(event string, m map[string]any) error {
		m["row"] = rp.ID
		if opts.Part >= 2 { // a continuation's line (AR67 ruling 5)
			m["part"] = opts.Part
		}
		return fx.Ledger.Append(event, m)
	}
	put := func(event string, m map[string]any) { appendLine(event, m) }
	sum := sha256.Sum256(prompt)
	promptHash := hex.EncodeToString(sum[:])
	sit := sitting{fx: fx, put: put, s: s, st: st, rp: rp, promptHash: promptHash, rulesHash: opts.rulesHash(), exit: -1, source: "none", deps: opts.Deps}
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// unspawned is a row never spawned (R6 §4): the owner's stop or kill, or an
	// account's stop, during a wait, or --until reached in a band hold.
	unspawned := func(until bool) (*relay.Sitting, relay.Gate) {
		s.State, s.Reason, sit.stop = relay.Skipped, "", relay.OwnerStop
		var as accountCause
		switch {
		case until:
			sit.stop = relay.UntilReached
		case errors.Is(context.Cause(ctx), ErrOwnerKill):
			s.State, s.Reason, sit.stop = relay.Failed, relay.OwnerKill, relay.OwnerKill
		case errors.As(context.Cause(ctx), &as):
			s.State, s.Reason, sit.stop = relay.Failed, relay.StopReason(as), relay.StopReason(as)
		}
		fmt.Fprintf(log, "relay %s: %s before the spawn\n", rp.ID, sit.stop)
		sit.end(opts.hold, stream.Limit{})
		return s, relay.Gate{}
	}
	// The band before the breaker (band-pacing-r1 §2.2; AR80 ruling 3): a
	// row's first part asks, and holds while the band says no room; the
	// sitting's clock starts after the hold.
	var spawnBand map[string]any
	defer opts.band.shown(rp.ID) // a part that returns ends its settle, a return before the spawn too (AR87 ruling 2)
	if fx.Band != nil && opts.Part < 2 {
		b, end := askBand(ctx, fx, rp, opts, log)
		if b.asks > 0 { // a part that never asked has no band key (ruling 7; the fix round's finding 5)
			sit.band = map[string]any{"asks": b.asks, "wait_s": b.waitS, "verdict": b.verdict}
			spawnBand = map[string]any{"verdict": b.verdict, "peak": peakOf(b.peak)}
		}
		if end != bandSpawn {
			sit.start = fx.Clock.Now()
			return unspawned(end == bandUntil)
		}
	}
	sit.start = fx.Clock.Now()
	// The breaker before the spawn (§15): still open at its Max, the row is
	// never spawned, no spawn or exit line, and the night stops load.
	breaker := DefaultBreaker
	breaker.Idle = fx.Idle
	brk, ok := breaker.wait(ctx, fx.Load, fx.Clock, func(state string, r reading) {
		breakerLine(fx.Ledger, rp.ID, state, r)
	})
	sit.breaker = brk
	if ok && opts.space != nil {
		ok = opts.space.wait(ctx)
	}
	if !ok && ctx.Err() != nil {
		return unspawned(false)
	}
	if !ok {
		fmt.Fprintf(log, "relay %s: load %.2f over %d cores: the breaker still open after %d s; not spawned\n", rp.ID, brk.last, brk.cores, brk.WaitS)
		s.State, s.Reason = relay.Failed, relay.LoadOpen
		sit.stop = relay.LoadOpen
		sit.write()
		return s, relay.Gate{}
	}
	// The spawn line is appended before the child starts (§7); a failed
	// append spawns nothing and fails the sitting.
	spawn := map[string]any{"argv": rp.Spawn, "prompt_hash": promptHash,
		"rules_hash": opts.rulesHash(), "block": opts.Block, "model": flagOf(rp.Spawn, "--model"),
		"turns": atoi(flagOf(rp.Spawn, "--max-turns")), "usd": atoi(flagOf(rp.Spawn, "--max-budget-usd")),
		"session_id": rp.SessionID, "wall_limit_s": int(wallOf(opts).Seconds())}
	if spawnBand != nil { // a part that asked (AR80 ruling 7)
		spawn["band"] = spawnBand
	}
	if err := appendLine("spawn", spawn); err != nil {
		fmt.Fprintf(log, "relay %s: the spawn ledger line: %v\n", rp.ID, err)
		s.State, s.Reason = relay.Failed, relay.LedgerError
		sit.write()
		return s, relay.Gate{}
	}

	// An orchestrator sitting <id>.<n> is its row's (AR32A act 1): Row,
	// SittingPlan's mark, set. Its child is metered as the session plugin's
	// orchestrator, every other row's as a builder (AR41).
	relayRow, profile := rp.ID, "builder"
	if rp.Row != "" {
		relayRow, profile = rp.Row, "orchestrator"
	}
	childEnv := effects.ChildEnv(env, relayRow, profile, flagOf(rp.Spawn, "--model"), atoi(flagOf(rp.Spawn, "--max-turns")), st.Remotes, effects.RelayBin())
	// Every part's start records the stage's head (AR68 ruling 3): ChildEnv
	// dropped a parent's, so the child holds this one alone.
	childEnv = append(childEnv, hooks.HeadBeforeVar+"="+st.HeadBefore)
	softPath := hooks.SoftPath(lookupEnv(childEnv), rp.SessionID) // where the child's hook reads it
	proc, out, err := sp.Start(effects.Spawn{Dir: st.Path, Argv: rp.Spawn, Stdin: prompt, Env: childEnv})
	if err != nil {
		put("exit", map[string]any{"child_exit": -1, "error": err.Error(), "turns": 0, "results": 0, "errors": 0})
		s.State, s.Reason = relay.Failed, relay.StreamError
		sit.write()
		return s, relay.Gate{}
	}
	gone := liveChild(st.Path)

	var (
		mu     sync.Mutex
		reason relay.StopReason
		done   = make(chan struct{})
	)
	kill := func(r relay.StopReason) {
		mu.Lock()
		defer mu.Unlock()
		if reason != "" {
			return
		}
		reason = r
		proc.Signal(syscall.SIGTERM)
		grace := KillGrace
		go func() {
			select {
			case <-done:
			case <-time.After(grace):
				proc.Signal(syscall.SIGKILL)
			}
		}()
	}
	t := time.AfterFunc(wallOf(opts), // a row's wall is never zero (§15)
		func() { kill(relay.WallClock) })
	defer t.Stop()

	// The watchdog (§15): no commit, no file and no CPU for StallSeconds,
	// the group killed at once, the row red, stop: stalled. More than
	// MaxHelpers claudes among the seen descendants at a tick, the group and
	// those killed at once, the row failed, stop: helper-storm (AR25 act 4).
	// Each of its samples records the child's descendants with their start
	// times (seen, the watchdog goroutine's until stalled closes): the sweep
	// kills those that left the group (ADV6-8).
	var (
		stall   StallSample
		stalled = make(chan struct{})
		seen    = map[int]string{}
	)
	if fx.Procs != nil {
		stallFor := time.Duration(opts.StallSeconds) * time.Second
		wd := Watchdog{Git: fx.Git, Procs: fx.Procs, Clock: fx.Clock, Dir: st.Path, Pgid: proc.Pid(), Stall: stallFor, Helpers: MaxHelpers,
			Seen: func(pids []int) {
				for _, pid := range pids {
					if _, ok := seen[pid]; !ok {
						if at, err := fx.Procs.Started(pid); err == nil {
							seen[pid] = at
						}
					}
				}
			}}
		go func() {
			defer close(stalled)
			smp, why := wd.Run(done)
			if why == "" {
				return
			}
			mu.Lock()
			first := reason == ""
			if first {
				reason = why
				if why == relay.Stalled {
					stall = smp
				}
				fx.Procs.KillGroup(wd.Pgid)
			}
			mu.Unlock()
			if first && why == relay.HelperStorm {
				// The storm's seen descendants killed now by their own
				// groups (a helper leads its shell's), not left to spawn
				// on through the Wait (AR25 act 4).
				effects.SweepEscaped(fx.Procs, seen)
			}
		}()
	} else {
		close(stalled)
	}

	lastZone, lastTurns, warned := 0, 0, false
	var (
		tune     Tune   // the stream goroutine's until parsed closes
		softHead string // the head at the soft request
	)
	// held is the current message's live line, rewritten at each of its
	// events and written when the next message begins or at the result: one
	// line per message (UX1), naming the message's own last tool.
	held := ""
	var (
		tally  stream.Tally
		perr   error
		parsed = make(chan struct{})
		hit    time.Time // when the stream first named an account stop (AR63)
	)
	go func() {
		defer close(parsed)
		tally, perr = stream.Parse(out, func(ev stream.Event, t stream.Tally) {
			if t.Account != "" && hit.IsZero() {
				hit = fx.Clock.Now()
			}
			if ev.Type == "rate_limit_event" && t.Account != "" { // a refused limit: the child waits on nothing
				kill(t.Account)
			}
			if ev.Type != "assistant" && ev.Type != "result" {
				return
			}
			if (t.Fresh || ev.Type == "result") && held != "" {
				fmt.Fprintln(log, held)
				held = ""
			}
			if ev.Type == "result" {
				fmt.Fprintln(log, t.Line(opts.Context))
			} else {
				held = t.Line(opts.Context)
			}
			if ev.Type != "assistant" || ev.ParentToolUseID != "" {
				return
			}
			if t.Turns >= 2 && lastTurns < 2 { // the second main-thread message: the row has shown to the band (AR87 ruling 1)
				opts.band.shown(rp.ID)
			}
			z := zone(t.Context, opts.Context)
			if (t.Turns != lastTurns && t.Turns%10 == 0) || z != lastZone {
				put("turn", map[string]any{"turns": t.Turns, "context": t.Context, "cost": t.CostUSD, "usage": t.Usage,
					"last_tool": t.LastTool, "zone": z})
			}
			lastTurns, lastZone = t.Turns, z
			if z >= 2 && !warned {
				warned = true
				put("ceiling", map[string]any{"turns": t.Turns, "context": t.Context, "ceiling": opts.Context})
				notify(fmt.Sprintf("relay %s: context %dk of %dk", rp.ID, t.Context/1000, opts.Context/1000))
			}
			if opts.Context > 0 && tune.SoftTurn == 0 && float64(t.Context) >= SoftLine*float64(opts.Context) {
				tune.SoftTurn = t.Turns
				softHead, _ = fx.Git.Head(st.Path)
				soft := map[string]any{"turns": t.Turns, "context": t.Context, "ceiling": opts.Context,
					"soft": int(SoftLine * float64(opts.Context))}
				writeSoft(fx.FS, softPath, soft)
				put("soft", soft)
			} else if tune.SoftTurn > 0 && tune.CommitTurn == 0 && t.Fresh {
				if h, err := fx.Git.Head(st.Path); err == nil && h != softHead {
					tune.CommitTurn = t.Turns
				}
			}
			if opts.Context > 0 && float64(t.Context) >= HardCap*float64(opts.Context) {
				kill(relay.CeilingKill)
			}
		})
	}()
	// The Wait returns at the child's exit (a kill's too: SIGKILL after
	// KillGrace), whoever still holds its stream.
	exit, werr := proc.Wait()
	opts.band.shown(rp.ID) // the child's exit ends its settle (AR87 ruling 2)
	gone()
	close(done)
	<-stalled
	mu.Lock()
	killed := reason
	mu.Unlock()
	// The orphan sweep (§15, D6): the group's survivors of the Wait killed
	// and read again, three rounds, then the descendants the watchdog saw
	// that left the group (ADV6-8); any is red, one left is stop: orphans.
	var left []int
	if fx.Procs != nil {
		sit.orphans, left = effects.Sweep(fx.Procs, proc.Pid())
		n, escaped := effects.SweepEscaped(fx.Procs, seen)
		sit.orphans, left = sit.orphans+n, append(left, escaped...)
	}
	// The stream ends once its last holder is gone; one still held
	// PipeGrace after the exit and the sweep is closed from the read end
	// and counted: a survivor (ADV6-7).
	cut := false
	select {
	case <-parsed:
		out.Close()
	case <-time.After(effects.PipeGrace):
		cut = true
		out.Close()
		<-parsed
	}
	stuck := cut && perr != nil // the read the close ended: not a stream error
	if stuck {
		sit.orphans++
		perr = nil
	}
	if held != "" { // a stream cut before its result
		fmt.Fprintln(log, held)
	}
	// The account's stop (run 10 ruling 1): the stream's own, or another
	// row's that killed this one; it wins over any other kill, and the gate
	// never runs.
	var as accountCause
	if tally.Account != "" {
		killed = tally.Account
	} else if errors.As(context.Cause(ctx), &as) {
		killed = relay.StopReason(as)
	}
	account := relay.AccountStop(killed)

	line := map[string]any{"child_exit": exit, "turns": tally.Turns, "results": tally.Results, "errors": tally.Errors, "context": tally.Context,
		"usage": tally.Usage, "cost_read": tally.Result != nil, "non_json": tally.NonJSON, "duration_ms": fx.Clock.Now().Sub(sit.start).Milliseconds()}
	if r := tally.Result; r != nil {
		line["subtype"], line["is_error"], line["turns"] = r.Subtype, r.IsError, r.NumTurns
		line["cost_usd"], line["duration_ms"], line["denials"] = r.CostUSD, r.DurationMS, r.Denials
		line["result_tail"] = r.ResultTail
	}
	if killed != "" {
		line["reason"] = string(killed)
	}
	if tally.Account == relay.UsageLimit {
		line["limit"] = tally.Limit(hit).Line() // the kind and reset at the hit (AR63)
	}
	tune.ContextExit, tune.Killed = tally.Context, killed == relay.CeilingKill
	line["tune"] = tune
	if werr != nil {
		// "exit N" only: the stderr after it can echo the prompt (§5); a
		// signal is kept whole, `signal: killed` (run 10 ruling 7).
		line["error"] = exitError(werr)
	} else if perr != nil {
		line["error"] = perr.Error()
	} else if stuck {
		line["error"] = "the stream still held after the exit"
	}
	put("exit", line)
	if tune.SoftTurn > 0 && softPath != "" {
		fx.FS.Remove(softPath)
	}

	transcripts := opts.Transcripts
	if transcripts == "" {
		transcripts = ProjectDir(os.Getenv("HOME"), st.Path)
	}
	builders, source := Builders(filepath.Join(transcripts, rp.SessionID, "subagents"))
	if source == "none" && len(tally.Builders) > 0 {
		builders, source = nil, "stream"
		for id, n := range tally.Builders {
			builders = append(builders, map[string]any{"agent": id, "turns": n})
		}
	}

	// The gate's result steps the machine from exited; no gate, or one that
	// could not gather its facts, is gate-unrun: red, never a pass (§6).
	var g relay.Gate
	var why relay.StopReason
	res, ev := relay.Unrun, relay.GateUnrun
	if opts.Gate != nil && !account && killed != relay.HelperStorm {
		res, why, g = opts.Gate(st, tally.ResultSeen, tally.Result == nil || tally.Result.IsError || exit != 0, opts.wait)
		if g.Orphans = sit.orphans; res == relay.GreenResult {
			res, why = relay.Decide(g) // orphans swept: red
		}
		switch res {
		case relay.GreenResult:
			ev = relay.GateGreen
		case relay.RedResult:
			ev = relay.GateRed
		case relay.FailedResult:
			ev = relay.GateFailed
		}
	}
	s.State, _ = relay.Step(relay.Exited, ev)
	switch ev {
	case relay.GateFailed:
		s.Reason = why
	case relay.GateRed:
		if why == relay.VerifyCut { // AR59 act 3: the row says its verify was cut
			s.Reason = why
		}
	case relay.GateUnrun:
		if s.Reason = relay.GateUnrunReason; why == relay.GitStateMoved || why == relay.GitConfigProgram || why == relay.RefMoved ||
			why == relay.RemoteTimeout || why == relay.RemoteError { // AR88
			s.Reason = why
		}
	}
	if killed == relay.Stalled { // a packet failure: the row red, the night moves on
		s.State, s.Reason = relay.Red, killed
		sit.stall = &stall
	} else if killed != "" {
		s.State, s.Reason = relay.Failed, killed
	} else if perr != nil {
		s.State, s.Reason = relay.Failed, relay.StreamError
	}
	if (len(left) > 0 || stuck) && !account { // the group outlived three kills, or its stream the exit: a sitting failure
		s.State, s.Reason, killed = relay.Failed, relay.Orphans, relay.Orphans
	}
	sit.tally, sit.exit, sit.builders, sit.source, sit.res, sit.g, sit.stop = tally, exit, builders, source, res, g, killed
	sit.end(opts.hold, tally.Limit(hit))
	return s, g
}

// end writes the sitting line, or for a part failed usage-limit under a
// night's hold leaves it to the night (AR65 ruling 3): hold gets the
// Limit and the writer, `limit: paused` on the line when the night pauses.
func (x sitting) end(hold *heldLine, l stream.Limit) {
	if hold == nil || x.s.State != relay.Failed || x.s.Reason != relay.UsageLimit {
		x.write()
		return
	}
	hold.Limit = l
	hold.write = func(paused bool) {
		x.paused = paused
		x.write()
	}
}

// writeSoft writes the soft request at path (0600, its directory 0700);
// a failed write leaves the `soft` line alone to say it.
func writeSoft(fsys effects.FS, path string, soft map[string]any) {
	if path == "" {
		return
	}
	b, err := json.Marshal(soft)
	if err == nil && fsys.MkdirAll(filepath.Dir(path), 0o700) == nil {
		fsys.WriteFile(path, append(b, '\n'), 0o600)
	}
}

// lookupEnv is environ as a getenv: the last entry of a name wins, as
// exec's does.
func lookupEnv(environ []string) func(string) string {
	return func(name string) string {
		v := ""
		for _, kv := range environ {
			if k, val, ok := strings.Cut(kv, "="); ok && k == name {
				v = val
			}
		}
		return v
	}
}

// exitError is the exit line's error for a Wait's err: a signal whole
// (`signal: killed`), an exit status `exit N` and never what follows it.
func exitError(err error) string {
	if msg := err.Error(); strings.HasPrefix(msg, "signal: ") {
		return msg
	}
	head, _, _ := strings.Cut(err.Error(), ": ")
	return head
}

// accountCause is a row context's cause when another row's account stop
// killed it (run 10 ruling 1): the row reads failed with that reason.
type accountCause relay.StopReason

func (a accountCause) Error() string { return "the account's stop: " + string(a) }

// sitting is what §5's `sitting` line summarises: the facts RunRow holds
// when the sitting ends, however it ended.
type sitting struct {
	fx         effects.Effects
	put        func(string, map[string]any)
	s          *relay.Sitting
	st         *Staged
	rp         *RowPlan
	promptHash string
	rulesHash  string
	start      time.Time
	tally      stream.Tally
	exit       int
	builders   []map[string]any
	source     string
	res        relay.Result
	g          relay.Gate
	stop       relay.StopReason
	breaker    BreakerStats
	stall      *StallSample
	deps       map[string]bool
	orphans    int
	paused     bool           // the night paused on this part: `limit: paused` (AR65)
	band       map[string]any // the part's asks, wait and verdict (AR80; nil: it never asked)
}

// live counts the children RunRow holds per tree, process-wide: one from
// the spawn to the Wait (§15's one-child-per-tree invariant reads it).
var live = struct {
	sync.Mutex
	n map[string]int
}{n: map[string]int{}}

// liveChild counts a child in tree until the returned func is called.
func liveChild(tree string) func() {
	live.Lock()
	live.n[tree]++
	live.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			live.Lock()
			if live.n[tree]--; live.n[tree] <= 0 {
				delete(live.n, tree)
			}
			live.Unlock()
		})
	}
}

// LiveChildren is a copy of the live children per tree RunRow holds now.
func LiveChildren() map[string]int {
	live.Lock()
	defer live.Unlock()
	out := make(map[string]int, len(live.n))
	for k, v := range live.n {
		out[k] = v
	}
	return out
}

// facts are the sitting line's invariant facts (§15): the heads, the
// gate's commit and clean against a fresh git status, the dependencies,
// the live children, the verify line and the checks against the result.
func (x sitting) facts(headAfter string) relay.Facts {
	f := relay.Facts{HeadBefore: x.st.HeadBefore, HeadAfter: headAfter, Gathered: x.g.Gathered,
		Commit: x.g.Commit, Clean: x.g.Clean, Deps: x.deps, Live: LiveChildren(),
		Pass: x.s.State == relay.Green}
	if x.g.Gathered {
		if status, err := x.fx.Git.Status(x.st.Path); err == nil {
			clean := len(status) == 0
			f.Fresh = &clean
		}
	}
	if v := x.g.Verify; v != nil {
		f.Verify = v.Status
	}
	for _, c := range x.g.Lines {
		f.Checks = append(f.Checks, c.Status)
	}
	return f
}

// RedChecks reads the names of a gate's red checks, in the order the gate
// marks them (AR45). internal/gate sets it to gate.Red wherever the gate
// is linked (the gate imports the night, never the reverse), so the
// `sitting` line's `gate.red` and the `gate` events are read through one
// function. Nil, no gate is linked, and the line names none.
var RedChecks func(relay.Gate) []string

// RedCause reads a gate's cause, the first fact of each red check (AR62
// act 2): internal/gate sets it to gate.Cause beside RedChecks. Nil, no
// gate is linked, and the cause is "".
var RedCause func(relay.Gate) string

// cause is the `sitting` line's `gate.cause`: "" when none is red or no
// gate is linked, never absent; the orphans the sweep added are in it.
func (x sitting) cause() string {
	if RedCause == nil {
		return ""
	}
	return RedCause(x.g)
}

// red is the `sitting` line's `gate.red`: the gate's red checks by name,
// `[]` and never null when none is red, the gate did not gather or none
// ran.
func (x sitting) red() []string {
	red := []string{}
	if RedChecks != nil {
		red = append(red, RedChecks(x.g)...)
	}
	return red
}

// write puts the `sitting` line: every key of the stand-in's (row, repo the
// root, cwd the worktree, mode, branch, base, the hashes, model, max_turns,
// budget_usd, head_before, head_after, session_id, turns, cost_usd (0
// when unread), duration_ms, wall_s, child_exit, is_error, denials, gate as an object
// holding result, verify, red (the red checks by name, AR45) and cause (their first facts, AR62), stop the kill reason, next_after empty: the
// loop knows it), plus the relay's own: state, reason, context, subtype,
// the builders and their source (none when absent), usage and cost_read
// (false: no result event, AR28).
//
// The invariants (§15) are read first: one broken fails the sitting with
// reason invariant, `stop: invariant` and `invariant: <name>` (the first
// broken) on the line; the night stops invariant.
func (x sitting) write() {
	rp, st, t := x.rp, x.st, x.tally
	repo, _ := rootOf(rp.Worktree)
	headAfter, _ := x.fx.Git.Head(st.Path)
	broken := relay.Invariants(x.facts(headAfter))
	if len(broken) > 0 {
		x.s.State, x.s.Reason, x.stop = relay.Failed, relay.Invariant, relay.Invariant
	}
	gate := map[string]any{"result": string(x.res), "verify": nil, "red": x.red(), "cause": x.cause()}
	if x.g.Gathered {
		gate["clean"], gate["commit"], gate["is_error"] = x.g.Clean, x.g.Commit, !x.g.ExitOK
	}
	if v := x.g.Verify; v != nil {
		gate["verify"] = map[string]any{"line": v.Name, "baseline_exit": v.MainExit, "baseline_last": v.MainLast,
			"exit": v.BranchExit, "last": v.BranchLast, "status": v.Status}
	}
	wall := x.fx.Clock.Now().Sub(x.start)
	m := map[string]any{"repo": repo, "cwd": st.Path, "mode": "worktree", "branch": st.Branch, "base": st.Base,
		"prompt_hash": x.promptHash, "rules_hash": x.rulesHash, "model": flagOf(rp.Spawn, "--model"),
		"max_turns": atoi(flagOf(rp.Spawn, "--max-turns")), "budget_usd": atoi(flagOf(rp.Spawn, "--max-budget-usd")),
		"head_before": st.HeadBefore, "head_after": headAfter, "session_id": rp.SessionID,
		"turns": t.Turns, "cost_usd": t.CostUSD, "usage": t.Usage, "cost_read": t.Result != nil, "duration_ms": wall.Milliseconds(), "wall_s": int(wall.Seconds()),
		"child_exit": x.exit, "is_error": true, "denials": 0, "gate": gate, "stop": string(x.stop), "next_after": "",
		"state": string(x.s.State), "reason": string(x.s.Reason), "context": t.Context,
		"builders": x.builders, "builders_source": x.source, "breaker": x.breaker.line()}
	if x.builders == nil {
		m["builders"] = []any{}
	}
	if x.stall != nil {
		m["stall"] = x.stall.line()
	}
	if len(broken) > 0 {
		m["invariant"] = broken[0]
	}
	m["orphans"] = x.orphans
	if x.band != nil {
		m["band"] = x.band
	}
	if x.paused && len(broken) == 0 { // a broken invariant fails the part as before AR65 (fix round F1)
		m["limit"] = "paused"
	}
	if r := t.Result; r != nil {
		m["duration_ms"], m["is_error"], m["subtype"], m["denials"] = r.DurationMS, r.IsError, r.Subtype, r.Denials
		// AR51 ruling 1: one turn count. The result's num_turns is the
		// number its cost belongs to and the `exit` line's; the tally of
		// top-level messages stays the live count on the `turn` lines, and
		// is this line's only when no result arrived.
		m["turns"] = r.NumTurns
	}
	x.put("sitting", m)
}

// ProjectDir is where claude keeps a cwd's transcripts:
// ~/.claude/projects/<the real path with every byte but [A-Za-z0-9] as ->.
func ProjectDir(home, cwd string) string {
	if real, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = real
	}
	slug := []byte(cwd)
	for i, c := range slug {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			slug[i] = '-'
		}
	}
	return filepath.Join(home, ".claude", "projects", string(slug))
}

// Builders reads each subagents/*.jsonl for its burn and nothing else: per
// file the distinct message ids (turns), the output tokens summed and the
// last context. Source is "subagents", or "none" when the directory is
// absent or empty.
func Builders(dir string) ([]map[string]any, string) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	sort.Strings(files)
	var out []map[string]any
	for _, p := range files {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		output, context := 0, 0
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 16<<20)
		for sc.Scan() {
			var l struct {
				Message *struct {
					ID    string `json:"id"`
					Usage *struct {
						Input       int `json:"input_tokens"`
						CacheCreate int `json:"cache_creation_input_tokens"`
						CacheRead   int `json:"cache_read_input_tokens"`
						Output      int `json:"output_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if json.Unmarshal(sc.Bytes(), &l) != nil || l.Message == nil || l.Message.Usage == nil {
				continue
			}
			u := l.Message.Usage
			if !seen[l.Message.ID] {
				seen[l.Message.ID] = true
				output += u.Output
			}
			context = u.Input + u.CacheCreate + u.CacheRead
		}
		f.Close()
		name := filepath.Base(p)
		out = append(out, map[string]any{"agent": name[:len(name)-len(".jsonl")], "turns": len(seen),
			"output_tokens": output, "context": context})
	}
	if len(out) == 0 {
		return nil, "none"
	}
	return out, "subagents"
}

// flagOf is the value after name in argv, or "".
func flagOf(argv []string, name string) string {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == name {
			return argv[i+1]
		}
	}
	return ""
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

var promptRulesHash = prompt.TemplateHash

// rulesHash is the row's template hash: the one it was rendered under, else
// the template hash with no block.
func (o RunOptions) rulesHash() string {
	if o.RulesHash != "" {
		return o.RulesHash
	}
	return promptRulesHash()
}
