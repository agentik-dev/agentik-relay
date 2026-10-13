// Package night is the night's logic over effects: the plan (§9), and in
// time the loop.
package night

import (
	"cmp"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// Row states in a plan.
const (
	StateReady   = "ready"   // every dependency satisfied
	StatePending = "pending" // a dependency is not yet satisfied
	StateInvalid = "invalid" // the row, its packet or a dependency is at fault
	StateParked  = "parked"  // listed, never run
	StateLanded  = "landed"  // its own id satisfied per §3: a green line, or merged into main
	// StateRefused is a row Stage would refuse, its worktree or branch
	// left in place (§8; AR58 fix round): never started, and no plan
	// problem, so the plan stays valid and the night runs the other rows.
	StateRefused = "refused"
)

// Plan is what `relay plan` found: the night it would open, the template,
// each selected row, and the queue's problems. Origin is the remote refs
// snapshot (§6's origin check compares against it). Verify is the repo's
// verify line measured once on main in the root (D22's baseline for the
// merge), nil when the repo has none. Nested is the recursion guard (§15):
// the plan ran under RELAY_MEASURE_DEPTH, so nothing was measured.
// Measured is how many checks ran a shell line (the verify line counts),
// Skipped how many did not (unrun, or a landed row's copied from the ledger).
// Budget is how many of the skipped the budget left unrun (§15); Breaker
// what the load breaker cost the measurement.
type Plan struct {
	Night        string            `json:"night"`
	TemplateHash string            `json:"template_hash"`
	QueueHash    string            `json:"queue_hash"`
	Root         string            `json:"root"`
	Until        string            `json:"until"`
	Stops        []string          `json:"stops"`
	Rows         []RowPlan         `json:"rows"`
	Problems     []queue.Problem   `json:"problems"`
	Origin       map[string]string `json:"origin"`
	Verify       *relay.Check      `json:"verify"`
	Valid        bool              `json:"valid"`
	Nested       bool              `json:"nested"`
	Measured     int               `json:"measured"`
	Skipped      int               `json:"skipped"`
	Budget       int               `json:"budget"`
	Breaker      BreakerStats      `json:"breaker"`
	// MeasureSeconds is the measure's budget the plan used, and
	// MeasureFrom where it came from: flag, queue or default (AR38 act 1).
	MeasureSeconds int    `json:"measure_seconds"`
	MeasureFrom    string `json:"measure_from"`
	// RunStop names the stop the measure's budget makes a run on this
	// plan take at once (AR54 act 1): `measure-timeout: ` and its detail
	// when the budget left checks unrun. "" means the budget cut nothing,
	// not that the run will start: a plan with no ready row has "", yet
	// its run stops gate-failed-dependency (AR54 ruling 5).
	RunStop string `json:"run_stop"`
	// Block is the Every session block of CLAUDE.md at main (AR30), nil
	// when none; BlockSource names it: none, none: not rendered, or
	// CLAUDE.md at <sha12>.
	Block       []byte `json:"-"`
	BlockSource string `json:"block"`
	// Stopped is the reason the owner's stop ended the measure under run
	// (AR34 act 2), owner-stop; empty when none, always under `relay plan`.
	Stopped string `json:"-"`
	// VerifySeconds is the verify baseline's run on main in seconds,
	// rounded up, timed by the bound shell's clock or read from its memo
	// entry (AR61 act 1); 0 is unmeasured: a cut, an error, an unrun line,
	// an entry with no secs. RenderPrompt's CONTEXT names it.
	VerifySeconds int `json:"-"`
	// opts are the options the plan was built with, and main the sha of
	// main it read: SittingPlan cuts each orchestrator sitting by them.
	opts Options
	main string
}

// The block sources a plan names when it read no block (AR30), and the
// ledger whose presence at main makes the plan read CLAUDE.md.
const (
	LedgerFile       = "DECISIONS.md"
	BlockNone        = "none"               // no ledger at main: CLAUDE.md unread
	BlockNotRendered = "none: not rendered" // a ledger, and no block in CLAUDE.md at main
)

// readBlock reads the Every session block at refs/heads/main in the root
// (AR30 act 1), once a night, by Hermetic: no ledger, none, and
// CLAUDE.md unread; a ledger, the block of CLAUDE.md verbatim. It returns
// a plan problem: markers not one BEGIN before one END, or a failed read,
// since a half-read block drops invariants. Main resolves to one sha
// first and every read is at that sha, so a commit landing on main
// between the reads never turns a valid state into a problem.
func readBlock(g effects.Git, root string, p *Plan) string {
	p.BlockSource = BlockNone
	sha, err := g.Hermetic(root, "rev-parse", "--verify", "--end-of-options", "refs/heads/main^{commit}")
	if err != nil {
		return "the Every session block: " + err.Error()
	}
	p.main = sha
	ls, err := g.Hermetic(root, "ls-tree", "--full-tree", "--name-only", sha, "--", LedgerFile, "CLAUDE.md")
	if err != nil {
		return "the Every session block: " + err.Error()
	}
	names := strings.Split(ls, "\n")
	if !contains(names, LedgerFile) {
		return ""
	}
	p.BlockSource = BlockNotRendered
	if !contains(names, "CLAUDE.md") {
		return ""
	}
	md, err := g.Hermetic(root, "cat-file", "blob", sha+":CLAUDE.md")
	if err != nil {
		return "the Every session block: " + err.Error()
	}
	b, err := prompt.Block([]byte(md))
	if err != nil {
		return fmt.Sprintf("the Every session block in CLAUDE.md at %s: %v", short(sha), err)
	}
	if b != nil {
		p.Block, p.BlockSource = b, "CLAUDE.md at "+short(sha)
	}
	return ""
}

// RowPlan is one row: its state, its dependencies split, its baseline on
// main, and the exact spawn it would get. A baseline check that was not run
// has status unrun, exit -1, and the reason as its last line.
type RowPlan struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Pending   []string `json:"pending"`
	Satisfied []string `json:"satisfied"`
	// Unproven is the pending dependencies with no branch that a ledger
	// line claims green: the ledger is never a green, so they wait (AR22
	// act 3, ADV13-7). Print's label only: the --json key sets are frozen.
	Unproven   []string      `json:"-"`
	Baseline   []relay.Check `json:"baseline"`
	BaselineMS int64         `json:"baseline_ms"`
	Spawn      []string      `json:"spawn"`
	SessionID  string        `json:"session_id"`
	Worktree   string        `json:"worktree"`
	Branch     string        `json:"branch"`
	Base       string        `json:"base"`
	Problems   []string      `json:"problems"`
	// Warnings never make a problem (AR61 act 3): the state, Valid,
	// RunStop and the exit stay. One per-act-verify warning when the row's
	// packet orders the verify line per act; `[]` when none.
	Warnings []string `json:"warnings"`
	// Row is the queue row a sitting belongs to, RELAY_ROW's value: the
	// orchestrator row's id for its sitting `<id>.<n>`; empty, the ID.
	Row string `json:"-"`
	// Sitting is what an orchestrator sitting began from (AR32A act 4);
	// nil for a build row.
	Sitting *SittingStart `json:"-"`
}

// Options select and shape a plan. Repo is the working tree the queue came
// from; State the state directory; Night, when empty, is the first free one.
// AllowedTools, Caffeinate come from the config; UUID mints session ids.
// MeasureSeconds bounds the measurement.
type Options struct {
	Rows         []string
	Until        string
	Now          time.Time
	Repo, State  string
	Night        string
	AllowedTools string
	Caffeinate   bool
	UUID         func() string
	// MeasureSeconds is the whole plan's budget for lines on main
	// (`--measure-seconds`; 0: the queue's `measure_seconds`, else
	// DefaultMeasureSeconds), §15 and AR38 act 1.
	MeasureSeconds int
	// VerifyLimit is the verify baseline's limit (the repo's
	// verify_seconds, AR59 act 2; 0: effects.Timeout); the measure's
	// budget still caps it. Acceptance lines keep effects.Timeout.
	VerifyLimit time.Duration
	// Orchestrate selects the queue's orchestrator row (orchestrator r1
	// §2, §8); without it that row is never planned.
	Orchestrate bool
	// Control is the night's control file, which the measure reads every
	// Poll (0: MeasurePoll) on the clock under `relay run` (AR34 act 2): a
	// stop or a kill there ends the measure as its deadline would, and
	// every live group in Groups (nil: the effects' own, the trap's) gets
	// SIGTERM, then SIGKILL after KillGrace. Empty, no poll (`relay plan`).
	Control string
	Poll    time.Duration
	Groups  *effects.Groups
}

// NightName is `<date>-<n>`, the first n with no `nights/<date>-<n>` under
// the state directory.
func NightName(fs effects.FS, state string, now time.Time) string {
	date := now.Format("2006-01-02")
	taken := map[string]bool{}
	names, _ := fs.List(filepath.Join(state, "nights"))
	for _, n := range names {
		taken[n] = true
	}
	for n := 1; ; n++ {
		if name := fmt.Sprintf("%s-%d", date, n); !taken[name] {
			return name
		}
	}
}

// BuildPlan validates the queue, resolves every selected row's dependencies
// (§3), snapshots origin, measures the baseline on main in the root (§6),
// and writes one `plan` ledger line per row. It spawns nothing.
func BuildPlan(fx effects.Effects, q *queue.Queue, opts Options) (*Plan, error) {
	if opts.Now.IsZero() {
		opts.Now = fx.Clock.Now()
	}
	if opts.UUID == nil {
		opts.UUID = NewUUID
	}
	if opts.AllowedTools == "" {
		opts.AllowedTools = effects.DefaultAllowedTools
	}
	p := &Plan{Night: opts.Night, TemplateHash: prompt.TemplateHash(), QueueHash: q.Hash,
		Until: opts.Until, Rows: []RowPlan{}, Problems: []queue.Problem{}, opts: opts}
	if p.Night == "" {
		p.Night = NightName(fx.FS, opts.State, opts.Now)
	}
	budget, from := opts.measureBudget(q) // the flag, the queue, then 600 (AR38 act 1)
	p.MeasureSeconds, p.MeasureFrom = int(budget/time.Second), from
	p.Problems = append(p.Problems, q.Validate()...)
	root, err := fx.Git.Root(opts.Repo)
	if err != nil {
		return nil, fmt.Errorf("the root: %w", err)
	}
	p.Root = root
	if p.Origin, err = fx.Git.RemoteRefs(root); err != nil {
		return nil, fmt.Errorf("origin: %w", err)
	}
	if why := readBlock(fx.Git, root, p); why != "" {
		p.Problems = append(p.Problems, queue.Problem{Msg: why})
	}
	p.TemplateHash = prompt.TemplateHashFor(p.Block) // the block opens every row's prompt (AR30 act 2)
	p.Stops = []string{"queue-empty", "owner-stop", "origin-moved"}
	if opts.Until != "" {
		if _, err := time.Parse("15:04", opts.Until); err != nil {
			p.Problems = append(p.Problems, queue.Problem{Msg: fmt.Sprintf("--until %q: want HH:MM", opts.Until)})
		} else {
			p.Stops = append(p.Stops, "until-reached "+opts.Until)
		}
	}
	lines, err := fx.Ledger.Lines()
	if err != nil {
		return nil, fmt.Errorf("the ledger: %w", err)
	}
	// Every validation first (§15, fail early): the queue, --rows, --until
	// and each row's dependencies, before any shell line runs.
	byRow := map[string][]string{}
	for _, pr := range p.Problems {
		byRow[pr.Row] = append(byRow[pr.Row], pr.Msg)
	}
	var rows []queue.Row
	for _, r := range selectRows(q, opts.Rows, p) {
		rp := RowPlan{ID: r.ID, Pending: []string{}, Satisfied: []string{}, Baseline: []relay.Check{},
			Problems: append([]string{}, byRow[r.ID]...), Warnings: []string{}, Branch: r.ID, Base: r.Base,
			Worktree: filepath.Join(root, ".claude", "worktrees", r.ID)}
		if rp.Base == "" {
			rp.Base = "main"
		}
		rows = append(rows, r)
		if r.Parked != "" {
			rp.State = StateParked
			p.Rows = append(p.Rows, rp)
			continue
		}
		if r.IsOrchestrator() { // no packet, no edges, never landed: §2, §4
			rp.Row, rp.Base = r.ID, "main"
			if why := promptProblem(fx.Git, root, p.main, r); why != "" {
				rp.Problems = append(rp.Problems, why)
				p.Problems = append(p.Problems, queue.Problem{Row: r.ID, Msg: why})
			}
			rp.State = StateReady // exempt from the refusal: its sittings stage <id>.<n> (AR58 F0)
			if len(rp.Problems) > 0 {
				rp.State = StateInvalid
			}
			p.Rows = append(p.Rows, rp)
			continue
		}
		for _, d := range edgesOf(r) {
			switch how, why := satisfiedHow(fx.Git, root, q, lines, d); {
			case how == Unproven:
				rp.Pending = append(rp.Pending, d)
				rp.Unproven = append(rp.Unproven, d)
			case how != "":
				rp.Satisfied = append(rp.Satisfied, d)
			case why != "":
				rp.Problems = append(rp.Problems, why)
				p.Problems = append(p.Problems, queue.Problem{Row: r.ID, Msg: why})
			default:
				rp.Pending = append(rp.Pending, d)
			}
		}
		if why := verifyRowProblem(r); why != "" {
			rp.Problems = append(rp.Problems, why)
			p.Problems = append(p.Problems, queue.Problem{Row: r.ID, Msg: why})
		}
		landed, _ := satisfied(fx.Git, root, q, lines, r.ID) // §3's rule on its own id
		var refused string
		if len(rp.Problems) == 0 && !landed { // a row to be started: ready or pending (AR58)
			refused = stageRefusal(fx, root, &rp)
		}
		switch {
		case len(rp.Problems) > 0:
			rp.State = StateInvalid
		case landed:
			rp.State = StateLanded
		case refused != "": // a row problem, never the plan's (AR58 fix round)
			rp.State, rp.Problems = StateRefused, append(rp.Problems, refused)
		case len(rp.Pending) > 0:
			rp.State = StatePending
		default:
			rp.State = StateReady
		}
		p.Rows = append(p.Rows, rp)
	}
	p.Valid = len(p.Problems) == 0

	// Then the measurement: one problem anywhere and nothing is measured.
	p.Nested = effects.Nested(fx.Getenv)
	var unrun string
	switch {
	case !p.Valid:
		unrun = InvalidReason
	case p.Nested:
		unrun = effects.NestedReason // started by a measurement or a test: never measures
	default:
		unrun = baselineBlocked(fx.Git, root)
	}
	// Scope (§15): only ready rows are measured, and the verify line only
	// when one is.
	verifyUnrun := unrun
	if verifyUnrun == "" && !anyReady(p.Rows) {
		verifyUnrun = NoReadyReason
	}
	// Every line on main goes through the memo (§15): a line measured at
	// this head is read back, never run again.
	var sh effects.Shell = fx.Shell
	var bs *boundShell
	endWatch := func() {}
	if unrun == "" {
		bs = newBound(fx, opts.State, root)
		bs.bound(budget)
		sh = bs
		if opts.Control != "" { // under run: the owner's stop ends the measure (AR34 act 2)
			groups := opts.Groups
			if groups == nil {
				groups = fx.Groups
			}
			endWatch = bs.watch(fx.FS, opts.Control, opts.Now, cmp.Or(opts.Poll, MeasurePoll), groups)
			defer endWatch() // idempotent: ended below, before the counts
		}
	}
	p.Verify = verifyBaseline(sh, root, q.Defaults.Verify, verifyUnrun, opts.VerifyLimit)
	p.VerifySeconds = verifySeconds(bs, p.Verify)
	switch {
	case p.Verify == nil:
	case verifyUnrun == "" && bs.last == outRan:
		p.Measured++
	default:
		p.Skipped++
	}
	for i, r := range rows {
		rp := &p.Rows[i]
		if rp.State == StateParked {
			continue
		}
		if r.Packet != nil {
			start := time.Now()
			switch {
			case unrun != "":
				rp.Baseline = baseline(fx.Shell, root, r.Packet.Acceptance, unrun)
			case rp.State == StatePending:
				rp.Baseline = baseline(fx.Shell, root, r.Packet.Acceptance, PendingReason)
			case rp.State == StateRefused: // never started: never measured (AR58)
				rp.Baseline = baseline(fx.Shell, root, r.Packet.Acceptance, RefusedReason)
			case rp.State == StateLanded:
				rp.Baseline = landedBaseline(lines, r.ID, r.Packet.Acceptance)
			default:
				ran, hit := bs.counts()
				rp.Baseline = baseline(sh, root, r.Packet.Acceptance, "")
				ran2, hit2 := bs.counts()
				p.Measured += ran2 - ran
				p.Skipped += hit2 - hit
			}
			rp.BaselineMS = time.Since(start).Milliseconds()
			if rp.State != StateReady || unrun != "" {
				p.Skipped += len(rp.Baseline)
			}
		}
		rp.SessionID = opts.UUID()
		tools := opts.AllowedTools
		if r.IsOrchestrator() {
			tools = WithAgent(tools)
		}
		rp.Spawn = SpawnArgv(r, rp.SessionID, tools, opts.Caffeinate)
	}
	endWatch()
	p.Budget = budgetCut(p)
	if bs != nil {
		p.Breaker = bs.stats
		if why, said := bs.stopReason(); why != "" { // AR34 act 2: who ended the measure, and what it left unrun
			p.Stopped = why
			if err := fx.Ledger.Append("measure-stopped", map[string]any{"reason": why, "said": said, "unrun": cutBy(p, why)}); err != nil {
				return nil, fmt.Errorf("the ledger: %w", err)
			}
		}
	}
	for _, rp := range p.Rows { // after the counts, which every line carries
		if rp.State == StateParked {
			continue
		}
		if err := fx.Ledger.Append("plan", planLine(p, rp)); err != nil {
			return nil, fmt.Errorf("the ledger: %w", err)
		}
	}
	if p.Budget > 0 { // AR54 act 1: the plan names the stop `relay run` makes on it
		p.RunStop = string(relay.MeasureTimeout) + ": " + MeasureTimeoutDetail(p)
	}
	warnPerAct(p, rows) // AR61 act 3: after the verify measure, never a problem
	return p, nil
}

// PerActVerify names the warning a row takes when its packet orders the
// verify line per act (AR61 act 3).
const PerActVerify = "per-act-verify"

// warnPerAct gives each ready or pending row whose packet orders the
// plan's verify line per act one PerActVerify warning naming the place:
// RULES run it once where CONTEXT says once, else it is the gate's alone.
// rows are p.Rows' queue rows, in order.
func warnPerAct(p *Plan, rows []queue.Row) {
	line := VerifyName(p)
	if line == "" {
		return
	}
	tail := "it is the gate's alone"
	if p.VerifySeconds > 0 && p.VerifySeconds <= prompt.VerifyOnceSeconds {
		tail = "RULES run it once"
	}
	for i, r := range rows {
		rp := &p.Rows[i]
		if (rp.State != StateReady && rp.State != StatePending) || r.Packet == nil {
			continue
		}
		if place := r.Packet.OrdersPerAct(line); place != "" {
			rp.Warnings = append(rp.Warnings, fmt.Sprintf("%s: %s orders the verify line per act; %s", PerActVerify, place, tail))
		}
	}
}

// stageRefusal runs the two tests Stage runs on a row the plan would
// start (relay-r1 §8; AR58 fix round, ruling 1) and is why it fails, in
// stage's words, "" when it passes: its worktree exists, or its branch
// does. No third test is added; Stage keeps both, since a worktree can
// appear after plan.
func stageRefusal(fx effects.Effects, root string, rp *RowPlan) string {
	if fx.FS.Exists(rp.Worktree) {
		return fmt.Sprintf("the worktree %s exists", rp.Worktree)
	}
	if exists, err := fx.Git.BranchExists(root, rp.Branch); err != nil {
		return fmt.Sprintf("the branch %s exists (%v)", rp.Branch, err)
	} else if exists {
		return fmt.Sprintf("the branch %s exists", rp.Branch)
	}
	return ""
}

// The last lines of the checks scope left unrun (§15).
const (
	PendingReason = "pending"      // a dependency is not yet satisfied
	LandedReason  = "landed"       // landed, and no plan line of it measured anything
	NoReadyReason = "no ready row" // the verify line, when no row is measured
	RefusedReason = StateRefused   // its worktree or branch left in place (AR58)
)

// budgetCut counts the checks the budget left unrun.
func budgetCut(p *Plan) int { return cutBy(p, BudgetReason) }

// cutBy counts the checks left unrun with reason as their last line.
func cutBy(p *Plan, reason string) int {
	n := 0
	cut := func(c relay.Check) bool { return c.Status == relay.CheckUnrun && c.MainLast == reason }
	if p.Verify != nil && cut(*p.Verify) {
		n++
	}
	for _, r := range p.Rows {
		for _, c := range r.Baseline {
			if cut(c) {
				n++
			}
		}
	}
	return n
}

// anyReady reports whether a row is ready.
func anyReady(rows []RowPlan) bool {
	for _, r := range rows {
		if r.State == StateReady {
			return true
		}
	}
	return false
}

// landedBaseline is a landed row's checks from its last `plan` ledger line
// that measured something (a check not unrun) with one check per acceptance
// line; with none, every check `unrun: landed`. No shell line runs.
func landedBaseline(lines []map[string]any, id string, acc []packet.Line) []relay.Check {
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if l["event"] != "plan" || l["row"] != id {
			continue
		}
		cs, _ := l["baseline"].([]any)
		if len(cs) != len(acc) {
			continue
		}
		out := make([]relay.Check, 0, len(acc))
		measured := false
		for j, c := range cs {
			m, _ := c.(map[string]any)
			last, _ := m["last"].(string)
			status, _ := m["status"].(string)
			measured = measured || status != relay.CheckUnrun
			out = append(out, relay.Check{Name: acc[j].Shell, Expect: acc[j].Expect,
				MainExit: int(number(m["exit"])), MainLast: last, Status: status})
		}
		if measured {
			return out
		}
	}
	return baseline(nil, "", acc, LandedReason)
}

// InvalidReason is the last line of every check an invalid plan left unrun
// (§15, fail early).
const InvalidReason = "invalid"

// Exit is the plan verb's exit status: 2 when the plan is invalid, else 0.
func (p *Plan) Exit() int {
	if !p.Valid {
		return 2
	}
	return 0
}

// selectRows keeps the rows --rows names, in queue order; an unknown name is
// a problem. With no names, every row.
func selectRows(q *queue.Queue, names []string, p *Plan) []queue.Row {
	want := map[string]bool{}
	for _, n := range names {
		if !q.Has(n) {
			p.Problems = append(p.Problems, queue.Problem{Msg: fmt.Sprintf("--rows %s: not in the queue", n)})
		}
		want[n] = true
	}
	var out []queue.Row
	for _, r := range q.Rows {
		switch {
		case r.IsOrchestrator() && !p.opts.Orchestrate: // only under --orchestrate (§8)
			if want[r.ID] {
				p.Problems = append(p.Problems, queue.Problem{Row: r.ID, Msg: fmt.Sprintf("--rows %s: an orchestrator row runs only under --orchestrate", r.ID)})
			}
		case len(names) == 0 || want[r.ID]:
			out = append(out, r)
		}
	}
	return out
}

// edgesOf is the row's dependencies with its base (base counts as an edge).
func edgesOf(r queue.Row) []string {
	out := append([]string{}, r.DependsOn...)
	if r.Base != "" && r.Base != "main" && !contains(out, r.Base) {
		out = append(out, r.Base)
	}
	return out
}

// Unproven is plan's label for a pending dependency with no branch that a
// ledger line claims green (AR21's ledger-green gone, AR22 act 3,
// ADV13-7): a new process cannot prove it from git or memory, so its
// dependant waits. The claim only labels: it satisfies nothing.
const Unproven = "unproven"

// greens is each root's rows this process wrote a green verdict for
// (NoteGreen): memory, never the ledger a child can append to (AR21 act 5,
// ADV12-7). heads is each root's heads those verdicts judged, to their
// rows (ar32b act 1): what an orchestrator's merge may take in.
var greens = struct {
	sync.Mutex
	m     map[string]bool
	heads map[string]string
}{m: map[string]bool{}, heads: map[string]string{}}

func greenKey(root, id string) string {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	return filepath.Clean(root) + "\x00" + id
}

// NoteGreen records that this process wrote a green verdict for row id in
// root (the gate's result line): it satisfies id's dependents while this
// process lives. head is the commit a build row's gate judged, "" for an
// orchestrator's (ar32b act 1): GreenHead reads it back.
func NoteGreen(root, id, head string) {
	greens.Lock()
	defer greens.Unlock()
	greens.m[greenKey(root, id)] = true
	if head != "" {
		greens.heads[greenKey(root, head)] = id
	}
}

// GreenHead is the row whose green gate of this process noted sha as its
// head in root (NoteGreen), "" when none did: an orchestrator sitting's
// merge is proven only over such a head (orchestrator r1 §5).
func GreenHead(root, sha string) (id string) {
	greens.Lock()
	defer greens.Unlock()
	return greens.heads[greenKey(root, sha)]
}

// ForgetHead drops the heads green gates of row id noted in root
// (NoteGreen): a row whose own merge was refused, or that ended red or
// failed, is no head an orchestrator's merge may take in, since a red
// row's commit reds row-unproven (orchestrator r1 §5; ar32b review round
// 1, F2). The row's green verdict for its dependents stays.
func ForgetHead(root, id string) {
	greens.Lock()
	defer greens.Unlock()
	prefix := greenKey(root, "")
	for k, row := range greens.heads {
		if row == id && strings.HasPrefix(k, prefix) {
			delete(greens.heads, k)
		}
	}
}

func wroteGreen(root, id string) bool {
	greens.Lock()
	defer greens.Unlock()
	return greens.m[greenKey(root, id)]
}

// satisfied applies §3: branch id landed in the root (a merged chain; see
// landed), or a green verdict this process wrote (memory). The ledger is
// never a green (AR22 act 3, ADV13-7): a green line for id, of any event,
// satisfies nothing; with no branch, id is pending, labelled Unproven.
// Never ancestry: a fresh branch's tip is its cut, an ancestor of main,
// and a row cut off its dependency's branch would satisfy itself
// (IsAncestor(a, a) is true).
// Otherwise id is pending when the queue or git knows it (or a line claims
// it), and invalid (why is non-empty) when none does.
func satisfied(g effects.Git, root string, q *queue.Queue, lines []map[string]any, id string) (bool, string) {
	how, why := satisfiedHow(g, root, q, lines, id)
	return how != "" && how != Unproven, why
}

// satisfiedHow is satisfied with how: "landed", "green" (this process's
// verdict), or "" when id is not satisfied; Unproven is a pending id with
// no branch that a ledger line claims green (never satisfied).
func satisfiedHow(g effects.Git, root string, q *queue.Queue, lines []map[string]any, id string) (how, why string) {
	if wroteGreen(root, id) {
		return "green", ""
	}
	exists, err := g.BranchExists(root, id)
	if err != nil {
		return "", fmt.Sprintf("depends on %s: git: %v", id, err)
	}
	if exists {
		if landed(g, root, baseOf(q, id), id, orcOf(q)) {
			return "landed", ""
		}
		return "", ""
	}
	for _, l := range lines {
		if effects.Green(l, id) {
			return Unproven, ""
		}
	}
	if q != nil && q.Has(id) {
		return "", ""
	}
	return "", fmt.Sprintf("depends on %s: unknown to the queue, the ledger and git", id)
}

// DepSatisfied is §3's rule for one dependency id: branch id landed in
// the root (its cut read against its row's base in q; nil or not a row:
// main), or a green verdict this process wrote. The ledger is never read
// (AR22 act 3, ADV13-7). gate.Hand's invariants read it (§15).
func DepSatisfied(g effects.Git, root string, q *queue.Queue, id string) bool {
	return wroteGreen(root, id) || landed(g, root, baseOf(q, id), id, orcOf(q))
}

// orcOf is q's orchestrator row's id, "" when q is nil or has none.
func orcOf(q *queue.Queue) string {
	if q != nil {
		if r, ok := q.Orchestrator(); ok {
			return r.ID
		}
	}
	return ""
}

// baseOf is row id's queue base, "main" when q has no such row or it
// names none.
func baseOf(q *queue.Queue, id string) string {
	if q != nil {
		for _, r := range q.Rows {
			if r.ID == id && r.Base != "" {
				return r.Base
			}
		}
	}
	return "main"
}

// landed is §3's merged reading of branch id (AR13 act 8; the model's
// BranchTipMovedBeforeLanded): a merge on main's first-parent line has id's
// tip as a later parent, and that tip is not its cut. The cut is git's,
// never a stage line's head_before (AR22 act 3, ADV13-11): the merge-base
// of the tip with refs/heads/<base>. For a base not main, a tip that is an
// ancestor of the base (the merge-base is the tip) never moved off it: a
// branch cut off another row's merged tip is not landed, and a base git
// cannot read lands nothing. For base main the merge-base is taken with
// the merge's first parent, never the tip: git records no merge of a tip
// main already holds. With orc, the orchestrator row's id, the merge may
// be one hop away (orchestrator r1 §6, ar32b act 3): on the first-parent
// line of a sitting's branch <orc>.<n> that itself landed on main.
func landed(g effects.Git, root, base, id, orc string) bool {
	merged, err := g.FirstParentMerged(root, id, "main")
	if err != nil {
		return false
	}
	if !merged && orc != "" {
		merged = throughSitting(g, root, id, orc)
	}
	if !merged {
		return false
	}
	if base != "main" {
		unmoved, err := g.IsAncestor(root, "refs/heads/"+id, "refs/heads/"+base)
		return err == nil && !unmoved
	}
	return true
}

// throughSitting reports id's tip a later parent of a merge on the
// first-parent line of a branch refs/heads/<orc>.<n>, n a sitting's number,
// whose own tip is a later parent of a merge on main's first-parent line
// (ar32b act 3). A landed branch that is no sitting's is no hop.
func throughSitting(g effects.Git, root, id, orc string) bool {
	prefix := "refs/heads/" + orc + "."
	out, err := g.Hermetic(root, "for-each-ref", "--format=%(refname)", prefix+"*")
	if err != nil {
		return false
	}
	for _, ref := range strings.Fields(out) {
		n := strings.TrimPrefix(ref, prefix)
		if k, err := strconv.Atoi(n); err != nil || k < 1 || strconv.Itoa(k) != n {
			continue
		}
		if in, err := g.FirstParentMerged(root, id, ref); err != nil || !in {
			continue
		}
		if up, err := g.FirstParentMerged(root, ref, "main"); err == nil && up {
			return true
		}
	}
	return false
}

// baselineBlocked is why the baseline cannot run in the root, "" when it can:
// the root must be clean and on main.
func baselineBlocked(g effects.Git, root string) string {
	branch, err := g.Branch(root)
	if err != nil {
		return "the root's branch is unknown: " + err.Error()
	}
	if branch != "main" {
		return fmt.Sprintf("the root is on %q, not main", branch)
	}
	status, err := g.RootStatus(root)
	if err != nil {
		return "the root's status is unknown: " + err.Error()
	}
	if len(status) > 0 {
		return fmt.Sprintf("the root is dirty (%d paths)", len(status))
	}
	return ""
}

// VerifyRowProblem names the row problem `verify-row` (run 10 ruling 6,
// ADV10-24).
const VerifyRowProblem = "verify-row"

// verifyRowProblem is the row's `verify-row` problem when its packet's
// verify row differs from the gate line, "" otherwise. It runs no shell
// line, so an invalid plan still measures nothing (§15): `auto` resolves to
// `make check`, the one line it can name; a repo with no check target is the
// gate's to find (its verify is then none).
func verifyRowProblem(r queue.Row) string {
	if r.Packet == nil || r.Packet.Verify == "" {
		return ""
	}
	gate := r.Verify
	if gate == "auto" {
		gate = "make check"
	}
	if r.Packet.Verify == gate {
		return ""
	}
	if gate == "" {
		gate = "(none)"
	}
	return fmt.Sprintf("%s: packet verify row `%s` differs from the gate line `%s`", VerifyRowProblem, r.Packet.Verify, gate)
}

// VerifyLine is the repo's verify line for a queue's `verify`: `auto` is
// `make check` when `make -n check` succeeds in the root, else none.
func VerifyLine(sh effects.Shell, root, verify string) string {
	line, _ := verifyLine(sh, root, verify)
	return line
}

// ResolveVerify is VerifyLine and the probe's error when it could not
// start: the gate resolves the line once on main with it, so a probe that
// never ran is unrun, never a repo without one (§6, ADV6-10).
func ResolveVerify(sh effects.Shell, root, verify string) (string, error) {
	return verifyLine(sh, root, verify)
}

// verifyLine is VerifyLine and the probe's error when it could not start.
func verifyLine(sh effects.Shell, root, verify string) (string, error) {
	if verify != "auto" {
		return verify, nil
	}
	exit, _, err := sh.Run(root, "make -n check >/dev/null 2>&1", time.Minute)
	if err == nil && exit == 0 {
		return "make check", nil
	}
	return "", err
}

// VerifyName is the plan's verify line (AR61 act 1): its check's name,
// `auto` read as make check; "" when the repo has none.
func VerifyName(p *Plan) string {
	if p == nil || p.Verify == nil {
		return ""
	}
	if p.Verify.Name == "auto" {
		return "make check"
	}
	return p.Verify.Name
}

// verifySeconds is the verify baseline's seconds, rounded up, as the bound
// shell timed its last line or read them from the memo (AR61 act 1); 0 for
// no bound shell, no check, an unrun check or a cut.
func verifySeconds(bs *boundShell, c *relay.Check) int {
	if bs == nil || c == nil || c.Status == relay.CheckUnrun || c.MainExit == effects.CutExit || bs.secs <= 0 {
		return 0
	}
	return int(math.Ceil(bs.secs))
}

// verifyBaseline runs the verify line once on main in the root: green on
// exit 0, else red with its exit and last line (the merge's same-as-main
// compares to them); unrun, exit -1, when the root cannot run it. Nil when
// the repo has no verify line. It runs under limit's first (the repo's
// verify limit, AR59 act 2), else effects.Timeout.
func verifyBaseline(sh effects.Shell, root, verify, unrun string, limit ...time.Duration) *relay.Check {
	if unrun != "" {
		if verify == "" {
			return nil
		}
		return &relay.Check{Name: verify, Expect: "exit 0", MainExit: -1, MainLast: unrun, Status: relay.CheckUnrun}
	}
	line, err := verifyLine(sh, root, verify)
	// The probe never started (§15): the load, the budget, the owner's stop (AR34 act 2).
	var owner stopError
	if errors.Is(err, errLoad) || errors.Is(err, errBudget) || errors.As(err, &owner) {
		return &relay.Check{Name: verify, Expect: "exit 0", MainExit: -1, MainLast: err.Error(), Status: relay.CheckUnrun}
	}
	if line == "" {
		return nil
	}
	c := &relay.Check{Name: line, Expect: "exit 0"}
	timeout := effects.Timeout
	if len(limit) > 0 && limit[0] > 0 {
		timeout = limit[0]
	}
	exit, last, err := sh.Run(root, line, timeout)
	switch {
	case err != nil:
		c.MainExit, c.MainLast, c.Status = -1, effects.LastLine(err.Error()), relay.CheckUnrun
	case exit == 0:
		c.MainExit, c.MainLast, c.Status = exit, last, relay.CheckGreen
	default:
		c.MainExit, c.MainLast, c.Status = exit, last, relay.CheckRed
	}
	loadedRed(sh, c) // D4 (§15)
	stamp(sh, c)
	return c
}

// stamp marks a measured check with the head of main it was measured at
// (R6 §4): the bound shell's, which read it before the first line. An
// unrun check, or one run outside a bound shell, carries none.
func stamp(sh effects.Shell, c *relay.Check) {
	if b, ok := sh.(*boundShell); ok && c.Status != relay.CheckUnrun {
		c.Head = b.head
	}
}

// baseline runs every acceptance line in the root, in order, or records each
// as unrun with the reason.
func baseline(sh effects.Shell, root string, lines []packet.Line, unrun string) []relay.Check {
	out := make([]relay.Check, 0, len(lines))
	if unrun == "" { // AR25 act 1: one fresh scratch home for the baseline's lines
		var done func()
		sh, done, _ = effects.Scratched(sh) // an error: each line makes its own
		defer done()
	}
	for _, l := range lines {
		c := relay.Check{Name: l.Shell, Expect: l.Expect}
		if unrun != "" {
			c.MainExit, c.MainLast, c.Status = -1, unrun, relay.CheckUnrun
			out = append(out, c)
			continue
		}
		exit, last, err := sh.Run(root, l.Shell, effects.Timeout)
		c.MainExit, c.MainLast = exit, last
		switch {
		case err != nil:
			c.MainExit, c.MainLast, c.Status = -1, effects.LastLine(err.Error()), relay.CheckUnrun
		case Holds(l, exit, last):
			c.Status = relay.CheckGreen
		default:
			c.Status = relay.CheckRed
		}
		loadedRed(sh, &c) // D4 (§15)
		stamp(sh, &c)
		out = append(out, c)
	}
	return out
}

// Holds reports whether an acceptance line's expectation holds for an exit
// status and a last line.
func Holds(l packet.Line, exit int, last string) bool {
	n, numErr := strconv.Atoi(strings.TrimSpace(last))
	switch l.Kind {
	case packet.Reads:
		return strings.TrimSpace(last) == l.Value
	case packet.ReadsOrMore:
		return numErr == nil && n >= l.N
	case packet.Under:
		return numErr == nil && n < l.N
	case packet.Exits:
		return exit == l.N
	case packet.AsMain:
		return true // on main a line is its own baseline
	}
	return false
}

// SpawnArgv is §7's spawn line for a row (D1): headless, streamed, the
// pre-allowed working tools, the driver's session id. Stdin is the prompt.
func SpawnArgv(r queue.Row, session, tools string, caffeinate bool) []string {
	var argv []string
	if caffeinate {
		argv = append(argv, "caffeinate", "-i")
	}
	return append(argv, "claude", "-p", "--permission-mode", "auto", "--allowedTools", tools,
		"--model", r.Model, "--max-turns", strconv.Itoa(r.Turns),
		"--max-budget-usd", strconv.Itoa(r.USD), "--output-format", "stream-json", "--verbose",
		"--session-id", session)
}

// planLine is the `plan` ledger line: state, dependencies, the template
// hash, per acceptance line its index, baseline exit, last line and status,
// and the verify line's exit, last line and status on main. Never the shell text or anything else of the packet's body.
func planLine(p *Plan, rp RowPlan) map[string]any {
	checks := make([]any, 0, len(rp.Baseline))
	for i, c := range rp.Baseline {
		checks = append(checks, map[string]any{"n": i + 1, "exit": c.MainExit, "last": c.MainLast, "status": c.Status})
	}
	m := map[string]any{"row": rp.ID, "state": rp.State, "rules_hash": p.TemplateHash,
		"queue_hash": p.QueueHash, "session_id": rp.SessionID, "pending": rp.Pending,
		"satisfied": rp.Satisfied, "baseline": checks, "baseline_ms": rp.BaselineMS, "verify": nil,
		"measured": p.Measured, "skipped": p.Skipped, "budget": p.Budget,
		"measure_seconds": p.MeasureSeconds, "measure_from": p.MeasureFrom,
		"breaker": p.Breaker.line()}
	if p.Stopped != "" { // a plan the owner cut (AR34 act 2)
		m["stopped"] = p.Stopped
	}
	if rp.State == StateRefused { // why plan refused the row, in stage's words (AR58)
		m["reason"] = RefusedWhy(rp)
	}
	if v := p.Verify; v != nil {
		m["verify"] = map[string]any{"exit": v.MainExit, "last": v.MainLast, "status": v.Status}
	}
	return m
}

// RefusedWhy is a refused row's reason in stage's words, "" for any other
// row (AR58 fix round): its last problem.
func RefusedWhy(rp RowPlan) string {
	if rp.State != StateRefused || len(rp.Problems) == 0 {
		return ""
	}
	return rp.Problems[len(rp.Problems)-1]
}

// NewUUID is a random version-4 UUID.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Quote is argv as one shell line. A word that begins with `=` is quoted,
// since zsh reads a leading `=` as a command lookup (`attach -t
// =relay-<night>` would paste as `relay-<night> not found`, AR33 fix 1).
func Quote(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && !strings.HasPrefix(a, "=") && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:@,") == "" {
			out[i] = a
		} else {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(out, " ")
}

// Print writes the plan for a person: first the verdict (the night it
// would open, its rows, valid or not), a verify line red on main (no
// merge-on-green row merges over it, ADV6-5) and the queue's own problems;
// then per row its state, dependencies, baseline, spawn line and resume
// door. The spawn line wraps with ` \` continuations (WrapSpawn), so it
// still pastes as one command.
func Print(w io.Writer, p *Plan) {
	verdict := "valid"
	if !p.Valid {
		verdict = "invalid"
	}
	fmt.Fprintf(w, "plan: night %s would open with %s; %s\n", p.Night, plural(len(p.Rows), "row"), verdict)
	fmt.Fprintf(w, "  stops: %s; template sha256 %s; block: %s\n", strings.Join(p.Stops, ", "), short(p.TemplateHash), p.BlockSource)
	if p.Nested {
		fmt.Fprintf(w, "  nested (%s set): nothing measured\n", effects.MeasureDepthVar)
	}
	if v := p.Verify; v != nil && v.Status == relay.CheckRed {
		// ADV6-5: the merge's verify passes green only.
		fmt.Fprintf(w, "  main is red: merge-on-green rows land branch-only (%s: exit %d, last %q)\n",
			v.Name, v.MainExit, clip(v.MainLast, 40))
	}
	for _, pr := range p.Problems {
		if pr.Row == "" {
			fmt.Fprintf(w, "  queue problem: %s\n", pr.Msg)
		}
	}
	if p.RunStop != "" { // AR54 act 1
		fmt.Fprintf(w, "  a run on this plan stops at once: %s\n", p.RunStop)
	}
	home := os.Getenv("HOME")
	for _, r := range p.Rows {
		fmt.Fprintf(w, "row %s: %s; branch %s off %s\n", r.ID, r.State, r.Branch, r.Base)
		fmt.Fprintf(w, "  worktree %s\n", TildeQuote([]string{r.Worktree}, home))
		if r.State == StateParked {
			continue
		}
		pend := make([]string, 0, len(r.Pending))
		for _, d := range r.Pending {
			if contains(r.Unproven, d) {
				d += " (" + Unproven + ")"
			}
			pend = append(pend, d)
		}
		fmt.Fprintf(w, "  deps: satisfied %s; pending %s\n", list(r.Satisfied), list(pend))
		for _, msg := range r.Problems {
			fmt.Fprintf(w, "  problem: %s\n", msg)
		}
		for _, msg := range r.Warnings { // AR61 act 3
			fmt.Fprintf(w, "  warning: %s\n", msg)
		}
		counts := map[string]int{}
		for _, c := range r.Baseline {
			counts[c.Status]++
		}
		fmt.Fprintf(w, "  baseline on main: %s (%d green, %d red, %d unrun) in %.1fs\n", plural(len(r.Baseline), "line"),
			counts[relay.CheckGreen], counts[relay.CheckRed], counts[relay.CheckUnrun], float64(r.BaselineMS)/1000)
		for i, c := range r.Baseline {
			fmt.Fprintf(w, "    %d. %s: exit %d, last %q; want %s\n", i+1, c.Status, c.MainExit, clip(c.MainLast, 40), c.Expect)
		}
		for _, l := range WrapSpawn(r.Worktree, r.Spawn, home) {
			fmt.Fprintln(w, l)
		}
		for _, l := range WrapResume(r.Worktree, r.SessionID, home) {
			fmt.Fprintln(w, l)
		}
	}
	fmt.Fprintln(w, MeasureLine(p))
	fmt.Fprintf(w, "measured %s, skipped %d", plural(p.Measured, "line"), p.Skipped) // scope (§15)
	if p.Budget > 0 {
		fmt.Fprintf(w, ", %d past the budget", p.Budget) // bounded (§15)
	}
	if b := p.Breaker; b.Opens > 0 {
		fmt.Fprintf(w, ", the breaker opened %s (%d s, load %.2f)", plural(b.Opens, "time"), b.WaitS, b.Load) // §15
	}
	fmt.Fprintln(w)
}

// MeasureLine is the measure's budget and its source as `plan` and `run`
// print it (AR38 act 1): `measure: 3000 s (queue)`.
func MeasureLine(p *Plan) string {
	return fmt.Sprintf("measure: %d s (%s)", p.MeasureSeconds, p.MeasureFrom)
}

// spawnWidth is the column every printed piece of the spawn line stays under.
const spawnWidth = 100

// WrapSpawn is the printed spawn line (UX1): `  spawn: cd <worktree> &&
// <argv> < prompt` in pieces under 100 columns, every piece but the last
// ending ` \`, continuations indented six, home written `~`.
// A flag stays on the piece of its value. Joined, the pieces are one shell
// command.
func WrapSpawn(worktree string, argv []string, home string) []string {
	units := []string{"cd " + TildeQuote([]string{worktree}, home) + " &&"}
	for i := 0; i < len(argv); i++ {
		u := Quote(argv[i : i+1])
		if strings.HasPrefix(argv[i], "--") && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
			u += " " + Quote(argv[i+1:i+2])
			i++
		}
		units = append(units, u)
	}
	return wrap("  spawn:", append(units, "< prompt"))
}

// WrapResume is the printed resume door, wrapped as WrapSpawn is.
func WrapResume(worktree, session, home string) []string {
	return wrap("  resume:", []string{"cd " + TildeQuote([]string{worktree}, home) + " &&", "claude --resume " + session})
}

// wrap packs units after label into pieces under spawnWidth columns, a
// piece ending ` \` when another follows, continuations indented six.
func wrap(label string, units []string) []string {
	const indent = "      "
	var out []string
	cur := label
	for _, u := range units {
		if cur != indent && cur != label && len(cur)+1+len(u)+2 >= spawnWidth {
			out = append(out, cur+" \\")
			cur = indent
		}
		if cur == indent {
			cur += u
		} else {
			cur += " " + u
		}
	}
	return append(out, cur)
}

// TildeQuote is Quote with home written `~`: an argument under home reads
// `~/<rest>`, which a shell still expands.
func TildeQuote(argv []string, home string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if rest, ok := strings.CutPrefix(a, home+"/"); ok && home != "" && rest != "" {
			out[i] = "~/" + Quote([]string{rest})
		} else {
			out[i] = Quote([]string{a})
		}
	}
	return strings.Join(out, " ")
}

// plural is `1 row`, `2 rows`.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// clip is s cut to n runes, `...` marking the cut.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

func list(ids []string) string {
	if len(ids) == 0 {
		return "none"
	}
	return strings.Join(ids, ", ")
}

func contains(ids []string, id string) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}
