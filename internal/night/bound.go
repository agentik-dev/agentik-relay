package night

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// MemoFile is the memo's name under the state directory (§15).
const MemoFile = "baselines.json"

// MemoMaxAge is an entry's life (R6 §4): a green at a head does not stand
// across a toolchain's upgrade, so an entry older than a day is a miss.
const MemoMaxAge = 24 * time.Hour

// MemoEntry is one measured line on main: its exit, last line, the shell's
// status (green on exit 0, else red; an acceptance line's own status is
// recomputed from exit and last by Holds), the load it ran under (0 until
// the breaker read around it: the higher of before and after) and when.
// Limit is the seconds the line ran under (the budget's cap included;
// AR59's fix round, block 2); 0, an older relay's entry, reads as 900.
// Secs is the seconds the line took on the bound shell's clock (AR61 act
// 1), returned on a hit; 0 (a cut, or an older relay's entry) is
// unmeasured.
type MemoEntry struct {
	Exit   int     `json:"exit"`
	Last   string  `json:"last"`
	Status string  `json:"status"`
	Load   float64 `json:"load"`
	TS     string  `json:"ts"`
	Limit  float64 `json:"limit,omitempty"`
	Secs   float64 `json:"secs,omitempty"`
}

// answers reports whether e answers a run under limit (0: any): an exit
// 124 (a cut) only under a limit no longer than the one it ran under, so a
// cut is never replayed under a larger limit; any other exit is the line's
// own verdict and answers every limit, as before AR59 (its fix round,
// block 2).
func (e MemoEntry) answers(limit time.Duration) bool {
	if limit <= 0 || e.Exit != effects.CutExit {
		return true
	}
	ran := effects.Timeout
	if e.Limit > 0 {
		ran = time.Duration(e.Limit * float64(time.Second))
	}
	return limit <= ran
}

// memoRoot is one root's memo: the head it was measured at, by line.
type memoRoot struct {
	Head  string               `json:"head"`
	Lines map[string]MemoEntry `json:"lines"`
}

// Memo is `<state>/baselines.json` (§15), keyed (root, HEAD, line). One
// head per root: a Put at another head drops the old head's keys. Unrun
// results are never stored; an entry older than MemoMaxAge is a miss. A
// nil *Memo remembers nothing.
type Memo struct {
	fs    effects.FS
	path  string
	now   func() time.Time // the age's clock; nil: entries do not age
	mu    sync.Mutex
	roots map[string]*memoRoot
	puts  map[string]map[string]MemoEntry // root -> line -> entry this memo stored
}

// LoadMemo reads the memo under state; a missing or unreadable file is an
// empty memo (the memo only saves shell lines, it decides nothing). An
// empty state is no memo (nil).
func LoadMemo(fs effects.FS, state string) *Memo {
	if fs == nil || state == "" {
		return nil
	}
	m := &Memo{fs: fs, path: filepath.Join(state, MemoFile), puts: map[string]map[string]MemoEntry{}}
	m.roots = m.read()
	return m
}

// read is the memo file's roots; missing or unreadable, none.
func (m *Memo) read() map[string]*memoRoot {
	roots := map[string]*memoRoot{}
	if b, err := m.fs.ReadFile(m.path); err == nil {
		if json.Unmarshal(b, &roots) != nil || roots == nil {
			roots = map[string]*memoRoot{}
		}
	}
	for k, r := range roots {
		if r == nil || r.Lines == nil {
			delete(roots, k)
		}
	}
	return roots
}

// fresh reports whether e is younger than MemoMaxAge by the memo's clock
// (every reader that measures sets one); an unreadable ts is not.
func (m *Memo) fresh(e MemoEntry) bool {
	if m.now == nil {
		return true
	}
	ts, err := time.Parse(time.RFC3339, e.TS)
	return err == nil && m.now().Sub(ts) <= MemoMaxAge
}

// Get is the entry for line measured in root at head, under any limit.
func (m *Memo) Get(root, head, line string) (MemoEntry, bool) {
	return m.GetUnder(root, head, line, 0)
}

// GetUnder is Get for a run under limit (0: any): an entry that does not
// answer it (MemoEntry.answers) is a miss.
func (m *Memo) GetUnder(root, head, line string, limit time.Duration) (MemoEntry, bool) {
	if m == nil || head == "" {
		return MemoEntry{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.roots[root]
	if r == nil || r.Head != head {
		return MemoEntry{}, false
	}
	e, ok := r.Lines[line]
	if !ok || !m.fresh(e) || !e.answers(limit) {
		return MemoEntry{}, false
	}
	return e, true
}

// Put stores e for (root, head, line) and saves: never an unrun entry; a
// root's other head is dropped first.
func (m *Memo) Put(root, head, line string, e MemoEntry) error {
	if m == nil || head == "" || e.Status == relay.CheckUnrun {
		return nil
	}
	m.mu.Lock()
	r := m.roots[root]
	if r == nil || r.Head != head {
		r = &memoRoot{Head: head, Lines: map[string]MemoEntry{}}
		m.roots[root] = r
		m.puts[root] = map[string]MemoEntry{} // this memo's stores at the old head go too
	}
	r.Lines[line] = e
	if m.puts[root] == nil {
		m.puts[root] = map[string]MemoEntry{}
	}
	m.puts[root][line] = e
	m.mu.Unlock()
	return m.Save()
}

// Save writes the memo (ADV6-9): under the file's lock it re-reads the
// file, merges this memo's own stores into it by root (a root at the same
// head keeps both writers' lines; at another head, this memo's head wins),
// writes a temp file of its own beside it and renames it over.
func (m *Memo) Save() error {
	if m == nil {
		return nil
	}
	if err := m.fs.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	unlock, err := m.fs.Lock(m.path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	m.mu.Lock()
	roots := m.read()
	for root, lines := range m.puts {
		mine := m.roots[root]
		if mine == nil || len(lines) == 0 {
			continue
		}
		r := roots[root]
		if r == nil || r.Head != mine.Head {
			r = &memoRoot{Head: mine.Head, Lines: map[string]MemoEntry{}}
			roots[root] = r
		}
		for line, e := range lines {
			r.Lines[line] = e
		}
	}
	m.roots = roots
	b, err := json.MarshalIndent(roots, "", "  ")
	m.mu.Unlock()
	if err != nil {
		return err
	}
	tmp, err := m.fs.WriteTemp(filepath.Dir(m.path), MemoFile+".tmp*", append(b, '\n'))
	if err != nil {
		return err
	}
	return m.fs.Rename(tmp, m.path)
}

// memoState is the state directory the gate's baselines memoize under:
// XDG's, read through Effects.Getenv; none (no memo) when Getenv is nil.
func memoState(fx effects.Effects) string {
	if fx.Getenv == nil {
		return ""
	}
	return effects.StateDir(fx.Getenv)
}

// Outcomes of one line through a boundShell.
const (
	outRan = iota + 1 // a shell line ran
	outHit            // the memo answered
	outCut            // past the budget: not started
)

// DefaultMeasureSeconds is `--measure-seconds`' default: the whole plan's
// budget for lines on main (§15).
const DefaultMeasureSeconds = 600

// BudgetReason is the last line of every check the budget left unrun.
const BudgetReason = "budget"

var errBudget = errors.New(BudgetReason)

// MeasureTimeoutDetail is the detail of a `measure-timeout` stop (AR38
// act 2): how many checks the measure left unrun, its time and source, and
// the two ways to raise it.
func MeasureTimeoutDetail(p *Plan) string {
	return fmt.Sprintf("%d checks unrun: the measure ran out of time (%d s, from %s); "+
		"raise measure_seconds in the queue or pass --measure-seconds", p.Budget, p.MeasureSeconds, p.MeasureFrom)
}

// Where the measure's budget came from (AR38 act 1), in its order: the
// flag, the queue's defaults, else DefaultMeasureSeconds.
const (
	MeasureFromFlag    = "flag"
	MeasureFromQueue   = "queue"
	MeasureFromDefault = "default"
)

// measureBudget is the plan's budget and its source: MeasureSeconds (the
// flag), else the queue's `measure_seconds` (§3), else the default.
func (o *Options) measureBudget(q *queue.Queue) (time.Duration, string) {
	switch {
	case o.MeasureSeconds > 0:
		return time.Duration(o.MeasureSeconds) * time.Second, MeasureFromFlag
	case q != nil && q.Defaults.MeasureSeconds > 0:
		return time.Duration(q.Defaults.MeasureSeconds) * time.Second, MeasureFromQueue
	}
	return DefaultMeasureSeconds * time.Second, MeasureFromDefault
}

// rootLocks holds one mutex per root, process-wide: one measurement at a
// time per root (§15).
var rootLocks sync.Map // root -> *sync.Mutex

// sha1Of is the sha1 of s, the locks' names.
func sha1Of(s string) [sha1.Size]byte { return sha1.Sum([]byte(s)) }

// LockRoot takes root's measurement lock and returns its unlock: every
// line on main (a plan's, a baseline's, the merge's verify and the merge
// itself) runs under it. The lock is the process's mutex, then flock on
// `<state>/locks/root-<sha1(root)>` (R6 §4): one measurement per root on
// this machine, across processes. An empty state is the mutex alone.
func LockRoot(state, root string) (func(), error) {
	root = filepath.Clean(root)
	m, _ := rootLocks.LoadOrStore(root, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	if state == "" {
		return mu.Unlock, nil
	}
	p := filepath.Join(state, "locks", fmt.Sprintf("root-%x", sha1Of(root)))
	funlock, err := effects.OSFS{}.Lock(p)
	if err != nil {
		mu.Unlock()
		return nil, fmt.Errorf("the root lock %s: %w", p, err)
	}
	return func() { funlock(); mu.Unlock() }, nil
}

// RootLock is LockRoot under the state the stage locks live in (LockDir's
// parent): the machine's, whatever state the memo reads.
func RootLock(root string) (func(), error) {
	return LockRoot(filepath.Dir(LockDir()), root)
}

// gateLocks holds one slot per cleaned root, process-wide: one gate
// verify at a time per root (verify-cut-r1 §2.1; AR78). No flock: the
// account lock makes the night the only process that gates.
var gateLocks sync.Map // root -> *gateSlot

// gateSlot is one root's gate lock: held, by holder, until its unlock
// closes freed.
type gateSlot struct {
	mu     sync.Mutex
	held   bool
	holder string
	freed  chan struct{}
}

// take holds s for holder when it is free, and returns its unlock; else
// the holder and the channel its unlock closes.
func (s *gateSlot) take(holder string) (unlock func(), behind string, freed <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held {
		return nil, s.holder, s.freed
	}
	ch := make(chan struct{})
	s.held, s.holder, s.freed = true, holder, ch
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.held, s.holder = false, ""
			close(ch)
		})
	}, "", nil
}

// ErrGateKilled is GateLock's end when the owner's kill closed killed
// before the lock was held: the verify line never runs (AR78 ruling 1).
var ErrGateKilled = errors.New("killed waiting for the gate lock")

// GateLock takes root's gate lock for holder (the row) and returns its
// unlock and how long it waited, only once it holds it (verify-cut-r1
// §3). A closed killed returns ErrGateKilled at once; a free lock is
// taken at once, with no call to waiting. Else it calls waiting(the
// holder's row, since), waits on killed and the lock, then, held or
// killed, calls waiting("", since) once. A nil killed never closes; a nil
// waiting is not called. Real time, as the line's deadline.
func GateLock(root, holder string, killed <-chan struct{}, waiting func(behind string, since time.Time)) (unlock func(), waited time.Duration, err error) {
	v, _ := gateLocks.LoadOrStore(filepath.Clean(root), &gateSlot{})
	s := v.(*gateSlot)
	since := time.Now() // real time: the line's deadline is (AR59 act 3)
	select {
	case <-killed: // a kill wins over a free lock (§2.4)
		return nil, 0, ErrGateKilled
	default:
	}
	unlock, behind, freed := s.take(holder)
	if unlock != nil {
		return unlock, 0, nil
	}
	if waiting != nil {
		waiting(behind, since)
	}
	for unlock == nil && err == nil {
		select {
		case <-killed:
			err = ErrGateKilled
		case <-freed: // every wait wakes; one takes it, the rest wait again
			unlock, _, freed = s.take(holder)
		}
	}
	waited = time.Since(since)
	if waiting != nil {
		waiting("", since)
	}
	return unlock, waited, err
}

// boundShell is the shell every measurement on main goes through (§15):
// in the root, one line at a time (RootLock); the memo answers a line
// already measured at head, and a line that ran is stored only when the
// root's HEAD and status read after it equal those before (ADV6-9); with a budget,
// a line starts only while budget remains, under the lesser of its own
// timeout and the remainder, and past it is not started (errBudget).
// Other directories pass straight through.
type boundShell struct {
	sh         effects.Shell
	git        effects.Git
	memo       *Memo
	clock      effects.Clock
	root, head string

	load    effects.Load // nil: the breaker never opens
	breaker Breaker
	ledger  effects.Ledger
	stats   BreakerStats
	tripped bool // the breaker was still open at its Max: no line starts again
	loaded  bool // the last Run's line ran above Close x cores (D4)

	budget   time.Duration // 0: none
	start    time.Time
	deadline <-chan time.Time
	spent    bool // under mu: stop sets it from the watch's goroutine

	// The owner's stop (AR34 act 2), under mu: the reason every line from
	// then reads, what the owner wrote, and the breaker wait's end.
	stopped, said string
	ctx           context.Context
	cancel        context.CancelFunc

	mu   sync.Mutex
	ran  int
	hit  int
	cut  int
	last int // the last Run's outcome

	// secs is the last Run's seconds on the clock, or its memo entry's
	// on a hit (AR61 act 1); 0 when it was cut, failed or never ran.
	secs float64
}

// bound arms the budget: it starts now, and the clock's After marks its end.
func (b *boundShell) bound(d time.Duration) {
	b.budget, b.start, b.deadline = d, b.clock.Now(), b.clock.After(d)
}

// remaining is the budget left, 0 once the deadline fired or the owner
// stopped the measure.
func (b *boundShell) remaining() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.spent {
		select {
		case <-b.deadline:
			b.spent = true
		default:
		}
	}
	if b.spent {
		return 0
	}
	return b.budget - b.clock.Now().Sub(b.start)
}

// newBound is the bounded shell for root: the memo under state keyed by
// root's HEAD (no memo when the head is unknown).
func newBound(fx effects.Effects, state, root string) *boundShell {
	b := &boundShell{sh: fx.Shell, git: fx.Git, clock: fx.Clock, root: root, load: fx.Load, breaker: DefaultBreaker, ledger: fx.Ledger}
	b.breaker.Idle = fx.Idle
	b.ctx, b.cancel = context.WithCancel(context.Background())
	if b.clock == nil {
		b.clock = effects.SystemClock{}
	}
	if head, err := fx.Git.Head(root); err == nil && head != "" {
		b.head, b.memo = head, LoadMemo(fx.FS, state)
		if b.memo != nil {
			b.memo.now = b.clock.Now
		}
	}
	return b
}

func (b *boundShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	if dir != b.root {
		return b.sh.Run(dir, line, timeout)
	}
	b.secs = 0
	if why, _ := b.stopReason(); why != "" { // AR34 act 2: nothing starts after the owner's stop, a memo hit neither
		b.note(outCut)
		return -1, "", stopError(why)
	}
	unlock, err := RootLock(b.root)
	if err != nil {
		b.note(outCut)
		return -1, "", err
	}
	defer unlock()
	if why, _ := b.stopReason(); why != "" { // the owner's stop came while this line waited on the root (AR34 fix, F2): owner-stop, not budget
		b.note(outCut)
		return -1, "", stopError(why)
	}
	b.loaded = false
	if e, ok := b.memo.GetUnder(b.root, b.head, line, timeout); ok { // a measure under another limit is no answer (AR59 fix, block 2)
		b.note(outHit) // costs nothing, so past the budget too
		if e.Exit != effects.CutExit {
			b.secs = e.Secs
		}
		return e.Exit, e.Last, nil
	}
	// The breaker (§15): wait out a load above Open x cores, through the
	// clock; still open at its Max, this line and every later one is unrun.
	if b.tripped {
		b.note(outCut)
		return -1, "", errLoad
	}
	if b.budget > 0 && b.remaining() <= 0 {
		b.note(outCut)
		return -1, "", errBudget
	}
	st, ok := b.breaker.wait(b.context(), b.load, b.clock, b.recordBreaker)
	b.stats.add(st)
	if why, _ := b.stopReason(); why != "" { // the owner's stop ended the wait
		b.note(outCut)
		return -1, "", stopError(why)
	}
	if !ok {
		b.tripped = true
		b.note(outCut)
		return -1, "", errLoad
	}
	// D4's read before the line (breaker-signal-r1 §2): the wait's last
	// load with the idle share the wait read beside it. A load between
	// Close and Open x cores has no share yet, so it is read here.
	pre, own := reading{load: st.last, cores: st.cores, idle: st.idle, idleRead: st.idleRead}, false
	if pre.above(b.breaker.Close) && !pre.idleRead {
		pre.idle, pre.idleRead = b.breaker.idleShare()
		own = true // the wait recorded nothing of this read
		// The read takes a second or more, and the owner's stop can land
		// in it (AR47 fix; AR34 fix, F2): the line never starts, and the
		// error is the stop's, not `budget`.
		if why, _ := b.stopReason(); why != "" {
			b.note(outCut)
			return -1, "", stopError(why)
		}
	}
	if b.budget > 0 {
		rem := b.remaining() // the wait spent budget too
		if rem <= 0 {
			b.note(outCut)
			return -1, "", errBudget
		}
		timeout = min(timeout, rem)
	}
	before := b.tree()
	t0 := b.clock.Now()
	exit, last, err := b.sh.Run(dir, line, timeout)
	took := b.clock.Now().Sub(t0).Seconds()
	if why, _ := b.stopReason(); why != "" { // cut by the owner's stop (AR34 act 2): never judged, never stored
		b.note(outCut)
		return -1, "", stopError(why)
	}
	b.note(outRan)
	load, post := st.last, reading{}
	if b.load != nil {
		if after, cores, lerr := b.load.Load(); lerr == nil {
			load = math.Max(load, after)
			post = reading{load: after, cores: cores}
		}
	}
	// D4 on both keys: loaded at the read before the line or at the read
	// after it. The read after samples the second after the line ended:
	// the other work on the machine, not the line's own threads, which
	// the load average still carries.
	b.loaded = b.breaker.loaded(pre)
	if !b.loaded && post.above(b.breaker.Close) {
		post.idle, post.idleRead = b.breaker.idleShare()
		b.loaded = b.breaker.loaded(post)
	}
	// A line the load alone would have called loaded: one `unconfirmed`
	// line for it, the later of its own reads.
	if !b.loaded {
		switch {
		case post.above(b.breaker.Close):
			b.stats.Unconfirmed++
			b.recordBreaker(brkUnconfirmed, post)
		case own && pre.above(b.breaker.Close):
			b.stats.Unconfirmed++
			b.recordBreaker(brkUnconfirmed, pre)
		}
	}
	if err == nil {
		if exit != effects.CutExit { // AR61 act 1: a cut is unmeasured
			b.secs = took
		}
		status := relay.CheckRed
		if exit == 0 {
			status = relay.CheckGreen
		}
		ts := b.clock.Now().UTC().Format(time.RFC3339)
		// D4: a line measured under load is judged for this plan only
		// (a red is no baseline) and never stored: the memo holds only
		// lines neither read found loaded, so a hit is never loaded.
		// ADV6-9: a line during which the root moved (a commit, a write) is
		// judged here but stored under no head.
		if !b.loaded && before != "" && b.tree() == before {
			b.memo.Put(b.root, b.head, line, MemoEntry{Exit: exit, Last: last, Status: status, Load: load, TS: ts, Limit: timeout.Seconds(), Secs: b.secs})
		}
	}
	return exit, last, err
}

// MeasurePoll is Options.Poll's default: how often the measure reads the
// night's control file under `agentik-relay run` (AR34 act 2).
var MeasurePoll = 2 * time.Second

// stopError is a line the owner's stop left unrun or cut (AR34 act 2):
// its text, the reason, is the check's last line.
type stopError string

func (e stopError) Error() string { return string(e) }

// stop is the owner's stop from outside the measure (AR34 act 2), safe
// from another goroutine: the budget counts as spent, as its deadline
// would, a breaker wait ends, and every line not yet run, or cut, is
// unrun with reason; said is what the owner wrote. The first stop holds.
func (b *boundShell) stop(reason, said string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped != "" {
		return
	}
	b.stopped, b.said, b.spent = reason, said, true
	if b.cancel != nil {
		b.cancel()
	}
}

// stopReason is the owner's stop's reason and words, "" when none came.
func (b *boundShell) stopReason() (string, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopped, b.said
}

// context is the breaker wait's: done once the owner stopped the measure.
func (b *boundShell) context() context.Context {
	if b.ctx == nil {
		return context.Background()
	}
	return b.ctx
}

// watch reads control every poll on b's clock until the function it
// returns is called (AR34 act 2), a file older than since a leftover
// (OpenControl's rule): a stop or a kill there stops b with
// owner-stop, and every live group in reg gets SIGTERM, then SIGKILL after
// KillGrace (Groups.Kill, the trap's path). Its first wait is armed before
// watch returns, so before the first line starts.
func (b *boundShell) watch(fs effects.FS, control string, since time.Time, poll time.Duration, reg *effects.Groups) func() {
	quit, done := make(chan struct{}), make(chan struct{})
	tick := b.clock.After(poll)
	go func() {
		defer close(done)
		for {
			select {
			case <-quit:
				return
			case <-tick:
			}
			if stop, kills, _ := controlSince(fs, control, since); stop || len(kills) > 0 {
				b.stop(string(relay.OwnerStop), controlLines(stop, kills))
				reg.Kill(KillGrace)
				return
			}
			tick = b.clock.After(poll)
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(quit)
			<-done
		})
	}
}

// tree is the root's HEAD and status as one string, "" when either is
// unreadable or HEAD is not the memo's head.
func (b *boundShell) tree() string {
	if b.memo == nil || b.git == nil {
		return ""
	}
	head, err := b.git.Head(b.root)
	if err != nil || head != b.head {
		return ""
	}
	status, err := b.git.RootStatus(b.root)
	if err != nil {
		return ""
	}
	return head + "\x00" + fmt.Sprint(status)
}

// lastLoaded reports whether the last line ran loaded, the load above
// Close x cores and the idle share at or under the floor (D4: a red
// measured so is `unrun: loaded`).
func (b *boundShell) lastLoaded() bool { return b.loaded }

// recordBreaker writes the `breaker` ledger line at an open, a close and
// a load the idle share did not confirm.
func (b *boundShell) recordBreaker(state string, r reading) {
	breakerLine(b.ledger, "", state, r)
}

// The states of a `breaker` ledger line. An unconfirmed line is a load
// above the threshold with the idle share above the floor: where the
// load alone would have opened the breaker or marked a line loaded.
const (
	brkOpen        = "open"
	brkClose       = "close"
	brkUnconfirmed = "unconfirmed"
)

// breakerLine is one `breaker` ledger line: its state, the load and the
// cores it was read against; `idle`, to four places, when the idle share
// was read; row when a spawn waited.
func breakerLine(l effects.Ledger, row, state string, r reading) {
	if l == nil {
		return
	}
	m := map[string]any{"state": state, "load": r.load, "cores": r.cores}
	if r.idleRead {
		m["idle"] = math.Round(r.idle*1e4) / 1e4
	}
	if row != "" {
		m["row"] = row
	}
	l.Append("breaker", m)
}

// The last lines of the checks the breaker left unrun (§15).
const (
	LoadReason   = "load"   // the breaker still open at its Max: not started
	LoadedReason = "loaded" // red, measured above Close x cores on both keys (D4): no baseline
)

var errLoad = errors.New(LoadReason)

// loadedShell is a shell that knows whether its last line ran loaded.
type loadedShell interface{ lastLoaded() bool }

// loadedRed is D4: a red check measured under load is `unrun: loaded`,
// never a same-as-main anchor; a green one stands.
func loadedRed(sh effects.Shell, c *relay.Check) {
	if l, ok := sh.(loadedShell); ok && c.Status == relay.CheckRed && l.lastLoaded() {
		c.MainExit, c.MainLast, c.Status = -1, LoadedReason, relay.CheckUnrun
	}
}

// Breaker is §15's load breaker, read before every measurement and every
// spawn: the 1-minute load above Open x cores, confirmed by the idle
// share, opens it; open, it reads both every Poll through the clock and
// closes under Close x cores, or at the second poll in a row that reads
// the idle share above the floor; still open at Max, Wait says no.
type Breaker struct {
	Open, Close float64 // multiples of the cores
	Poll, Max   time.Duration
	// Idle is the second key (breaker-signal-r1 §2): a load above a
	// threshold counts only when the idle share of the CPU is at or under
	// IdleFloor. Nil, or a read that fails, leaves the load alone to decide.
	Idle      effects.Idle
	IdleFloor float64 // a share, 0 to 1
}

// DefaultIdleFloor is the idle share at or under which a high load is
// confirmed: background work that fills the efficiency cores leaves more
// idle than this, runnable threads at normal priority leave none.
const DefaultIdleFloor = 0.25

// DefaultBreaker opens above 4x the cores, closes under 2x, polls every
// 30 s for up to 10 minutes; its second key turns at DefaultIdleFloor,
// once a caller gives it an Idle to read.
var DefaultBreaker = Breaker{Open: 4, Close: 2, Poll: 30 * time.Second, Max: 10 * time.Minute, IdleFloor: DefaultIdleFloor}

// BreakerStats is what the breaker cost: how often it opened, the seconds
// it waited, and the highest 1-minute load it read. `breaker: {opens,
// wait_s, load}` on the sitting and plan lines. Unconfirmed counts the
// loads above a threshold that the idle share did not confirm, each an
// `unconfirmed` ledger line; the key is left out when it is zero.
type BreakerStats struct {
	Opens       int     `json:"opens"`
	WaitS       int     `json:"wait_s"`
	Load        float64 `json:"load"`
	Unconfirmed int     `json:"unconfirmed,omitempty"`

	last  float64 // the last load read
	cores int
	// The idle share read with the last load, when it was read: D4's read
	// before a line is this one.
	idle     float64
	idleRead bool
}

func (s *BreakerStats) add(o BreakerStats) {
	s.Opens += o.Opens
	s.WaitS += o.WaitS
	s.Load = math.Max(s.Load, o.Load)
	s.Unconfirmed += o.Unconfirmed
}

// line is the stats as a ledger value.
func (s BreakerStats) line() map[string]any {
	m := map[string]any{"opens": s.Opens, "wait_s": s.WaitS, "load": s.Load}
	if s.Unconfirmed > 0 {
		m["unconfirmed"] = s.Unconfirmed
	}
	return m
}

// idleShare reads the second key: the idle share, and whether it was
// read. No reader, or one that fails, is not read, and the load alone
// decides.
func (b Breaker) idleShare() (float64, bool) {
	if b.Idle == nil {
		return 0, false
	}
	share, err := b.Idle.Idle()
	return share, err == nil
}

// quiet reports a read whose idle share is above the floor: the CPU does
// not confirm the load.
func (b Breaker) quiet(share float64, read bool) bool { return read && share > b.IdleFloor }

// reading is one read of the breaker's keys: the load over its cores and,
// where the load called for it, the idle share.
type reading struct {
	load     float64
	cores    int
	idle     float64
	idleRead bool
}

// above reports the load key turned at k x cores.
func (r reading) above(k float64) bool { return r.cores > 0 && r.load > k*float64(r.cores) }

// loaded is D4 for one read: the load above Close x cores, and the idle
// share at or under the floor or not read.
func (b Breaker) loaded(r reading) bool { return r.above(b.Close) && !b.quiet(r.idle, r.idleRead) }

// Wait reads the load and, above Open x cores with the idle share at or
// under IdleFloor, waits for the load to fall under Close x cores or for
// two polls in a row to read the idle share above the floor: true once
// closed (or never open), false when still open at Max or ctx is done.
// The idle share is read only after the load has turned its key. An
// unreadable load (or none) never opens it: the breaker guards, it decides
// nothing about the work. An unreadable idle share (or none) leaves the
// load alone to decide. An idle read takes a second or more: a ctx that
// ended during one ends the wait false, whatever the share read, so
// nothing starts after the owner's stop. note, when given, hears the open
// and the close.
func (b Breaker) Wait(ctx context.Context, load effects.Load, clock effects.Clock, note ...func(open bool, avg1 float64, cores int)) (BreakerStats, bool) {
	return b.wait(ctx, load, clock, nil, note...)
}

// wait is Wait with rec, which hears what the ledger records: the open
// and the close, as note does, and a load above Open x cores the idle
// share did not confirm (once a wait), each with the idle share when it
// was read.
func (b Breaker) wait(ctx context.Context, load effects.Load, clock effects.Clock, rec func(state string, r reading), note ...func(open bool, avg1 float64, cores int)) (BreakerStats, bool) {
	var st BreakerStats
	if load == nil {
		return st, true
	}
	tell := func(state string, avg float64, cores int) {
		if rec != nil {
			rec(state, reading{load: avg, cores: cores, idle: st.idle, idleRead: st.idleRead})
		}
		for _, f := range note {
			if f != nil && state != brkUnconfirmed {
				f(state == brkOpen, avg, cores)
			}
		}
	}
	avg, cores, err := load.Load()
	if err != nil || cores <= 0 {
		return st, true
	}
	st.Load, st.last, st.cores = avg, avg, cores
	if avg <= b.Open*float64(cores) {
		return st, true
	}
	// The second key (breaker-signal-r1 §2): a load the CPU does not
	// confirm (background work, threads that wait) opens nothing.
	st.idle, st.idleRead = b.idleShare()
	if b.Idle != nil && ctx.Err() != nil {
		return st, false // the stop landed during the read (AR47 fix)
	}
	if b.quiet(st.idle, st.idleRead) {
		st.Unconfirmed = 1
		tell(brkUnconfirmed, avg, cores)
		return st, true
	}
	st.Opens = 1
	tell(brkOpen, avg, cores)
	start := clock.Now()
	quiet := 0 // polls in a row that read the idle share above the floor
	for {
		waited := clock.Now().Sub(start)
		st.WaitS = int(waited.Seconds())
		if waited >= b.Max {
			return st, false
		}
		select {
		case <-ctx.Done():
			return st, false
		case <-clock.After(min(b.Poll, b.Max-waited)):
		}
		if avg, cores, err = load.Load(); err != nil || cores <= 0 {
			quiet = 0
			continue
		}
		st.Load, st.last, st.cores = math.Max(st.Load, avg), avg, cores
		st.idle, st.idleRead = 0, false
		if avg >= b.Close*float64(cores) {
			// The load holds: one quiet second does not close it, two
			// polls in a row do.
			st.idle, st.idleRead = b.idleShare()
			if b.Idle != nil && ctx.Err() != nil {
				st.WaitS = int(clock.Now().Sub(start).Seconds())
				return st, false // the stop landed during the read: no close
			}
			if !b.quiet(st.idle, st.idleRead) {
				quiet = 0
				continue
			}
			if quiet++; quiet < 2 {
				continue
			}
		}
		st.WaitS = int(clock.Now().Sub(start).Seconds())
		tell(brkClose, avg, cores)
		return st, true
	}
}

func (b *boundShell) note(out int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last = out
	switch out {
	case outRan:
		b.ran++
	case outHit:
		b.hit++
	case outCut:
		b.cut++
	}
}

// counts is (ran, not run: a memo hit or past the budget) so far.
func (b *boundShell) counts() (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ran, b.hit + b.cut
}

// StaleReason is the last line of an anchor measured at another head of
// main with no memo entry at main's head now (R6 §4): unrun, never compared.
const StaleReason = "stale-baseline"

// MemoCheck is the verify line's check on main at the root's HEAD from the
// memo, nil when the memo has none: the merge's red-main anchor when the
// caller brought no baseline (§15). limit, when given, is the run it must
// answer (Memo.GetUnder; AR59's fix round).
func MemoCheck(fx effects.Effects, root, line string, limit ...time.Duration) *relay.Check {
	head, err := fx.Git.Head(root)
	if err != nil || line == "" {
		return nil
	}
	return memoAt(fx, root, head, line, limit...)
}

// memoAt is line's check on main at head from the memo, nil on a miss.
func memoAt(fx effects.Effects, root, head, line string, limit ...time.Duration) *relay.Check {
	m := LoadMemo(fx.FS, memoState(fx))
	if m != nil {
		m.now = effects.SystemClock{}.Now
		if fx.Clock != nil {
			m.now = fx.Clock.Now
		}
	}
	under := time.Duration(0)
	if len(limit) > 0 {
		under = limit[0]
	}
	e, ok := m.GetUnder(root, head, line, under)
	if !ok {
		return nil
	}
	return &relay.Check{Name: line, Expect: "exit 0", MainExit: e.Exit, MainLast: e.Last, Status: e.Status, Head: head}
}

// Anchor is b, a baseline of line measured on main, where it may be
// compared now (R6 §4): b itself when it was measured at main's head in
// root; else the memo's measure of line at that head; else unrun:
// stale-baseline (StaleReason), never compared. A nil or unrun b is
// returned as it is. limit, when given, is the run the memo's entry must
// answer (Memo.GetUnder; AR59's fix round).
func Anchor(fx effects.Effects, root string, b *relay.Check, line string, limit ...time.Duration) *relay.Check {
	if b == nil || b.Status == relay.CheckUnrun {
		return b
	}
	head, err := fx.Git.Tip(root, "main")
	if err == nil && head != "" && head == b.Head {
		return b
	}
	if err == nil && head != "" {
		if m := memoAt(fx, root, head, line, limit...); m != nil {
			m.Name, m.Expect = b.Name, b.Expect
			return m
		}
	}
	return &relay.Check{Name: b.Name, Expect: b.Expect, MainExit: -1, MainLast: StaleReason, Status: relay.CheckUnrun}
}

// Stale reports whether c is an anchor Anchor could not place at main's head.
func Stale(c *relay.Check) bool {
	return c != nil && c.Status == relay.CheckUnrun && c.MainLast == StaleReason
}

// MemoShell is fx's shell with the memo in front for lines run in root:
// what the gate's probes on main go through.
func MemoShell(fx effects.Effects, root string) effects.Shell {
	return newBound(fx, memoState(fx), root)
}

// DefaultStallSeconds is `--stall-seconds`' default (§15): 45 minutes with
// no commit, no file written and no CPU spent is a stalled child.
const DefaultStallSeconds = 2700

// WatchdogPoll is how often the watchdog samples, through the clock.
const WatchdogPoll = 60 * time.Second

// DefaultWall is a row's wall clock when none reached RunRow (the queue's
// default): a row's wall is never zero (§15).
const DefaultWall = 4 * time.Hour

// StallSample is one reading of a sitting's progress: the worktree's HEAD,
// its newest mtime outside .git, the child's group's CPU seconds; At is
// when it was read.
type StallSample struct {
	Head  string
	Mtime time.Time
	CPUS  float64
	At    time.Time
}

// same reports no progress between two samples.
func (s StallSample) same(o StallSample) bool {
	return s.Head == o.Head && s.Mtime.Equal(o.Mtime) && s.CPUS == o.CPUS
}

// line is the sample as the sitting line's `stall`.
func (s StallSample) line() map[string]any {
	return map[string]any{"head": s.Head, "mtime": s.Mtime.UTC().Format(time.RFC3339),
		"cpu_s": s.CPUS, "at": s.At.UTC().Format(time.RFC3339)}
}

// MaxHelpers is how many `claude` processes a row's tree may hold at once
// (AR25 act 4, ADV15-1's enforced half): the sitting is the spawn root, not
// its own descendant, so the base is 0 (measured: a headless sitting's own
// descendants are its caffeinate and its tool calls' shells); a row runs one
// helper at a time (hooks.MaxHelperDepth), and one exiting as the next
// starts is two.
const MaxHelpers = 2

// Watchdog is §15's stall guard over one sitting: every Poll (WatchdogPoll)
// through Clock it samples Dir's HEAD, Dir's newest mtime outside .git and
// the CPU seconds of Pgid's group; when all three stay unchanged for Stall
// (DefaultStallSeconds) the child is stalled. Each sample also hands Seen
// (when set) Pgid's descendants by ppid: a descendant that leaves the group
// is still the sweep's (ADV6-8). With Helpers above 0 each sample is also
// the helper breaker's tick (AR25 act 4): more than Helpers `claude`
// processes among the descendants it has seen is a storm.
type Watchdog struct {
	Git     effects.Git
	Procs   effects.Procs
	Clock   effects.Clock
	Dir     string
	Pgid    int
	Stall   time.Duration
	Poll    time.Duration
	Seen    func(pids []int)
	Helpers int

	claudes map[int]string // pid to start time: each descendant read a claude (IsClaude)
}

// sample reads the three; a reading that fails reads as its zero, so a
// child whose tree and group cannot be read at all is stalled in time. n
// is the breaker's count (0 with Helpers 0).
func (w Watchdog) sample() (s StallSample, n int) {
	s = StallSample{At: w.Clock.Now()}
	s.Head, _ = w.Git.Head(w.Dir)
	s.Mtime, _ = w.Procs.Newest(w.Dir)
	if w.Seen != nil || w.Helpers > 0 {
		if pids, err := w.Procs.Descendants(w.Pgid); err == nil {
			if w.Seen != nil {
				w.Seen(pids)
			}
			n = w.helpers(pids)
		}
	}
	pids, err := w.Procs.Group(w.Pgid)
	if err != nil || len(pids) == 0 {
		pids = []int{w.Pgid}
	}
	for _, pid := range pids {
		if c, err := w.Procs.CPUSeconds(pid); err == nil {
			s.CPUS += c
		}
	}
	return s, n
}

// helpers is the breaker's count over the seen descendants, not only the
// tree (AR25 act 4): each of pids not yet known a claude has its Comm read
// (a tool call's shell leads its own group, so no Group read would do),
// and its Exe when the comm is no claude (AR27C act 2; ADV16-3: comm is
// argv[0]); one IsClaude is kept with its start time, and the count is those kept
// whose start time still reads the same, so a helper that left the tree
// (its parent gone, or setsid) still counts and a reused pid never does.
func (w Watchdog) helpers(pids []int) int {
	if w.claudes == nil {
		return 0
	}
	for _, pid := range pids {
		if _, ok := w.claudes[pid]; ok {
			continue
		}
		c, err := w.Procs.Comm(pid)
		if err != nil {
			continue
		}
		exe := ""
		if !effects.IsClaude(c, "") {
			exe, _ = w.Procs.Exe(pid)
		}
		if effects.IsClaude(c, exe) {
			if at, err := w.Procs.Started(pid); err == nil {
				w.claudes[pid] = at
			}
		}
	}
	n := 0
	for pid, at := range w.claudes {
		if s, err := w.Procs.Started(pid); err == nil && s == at {
			n++
		} else {
			delete(w.claudes, pid)
		}
	}
	return n
}

// Run samples until done closes (""), the child stalls (relay.Stalled,
// with the last sample: the one read at the stall) or a tick counts more
// than Helpers claudes (relay.HelperStorm, with that tick's sample).
func (w Watchdog) Run(done <-chan struct{}) (StallSample, relay.StopReason) {
	if w.Poll <= 0 {
		w.Poll = WatchdogPoll
	}
	if w.Stall <= 0 {
		w.Stall = DefaultStallSeconds * time.Second
	}
	if w.Helpers > 0 {
		w.claudes = map[int]string{}
	}
	last, n := w.sample()
	if w.Helpers > 0 && n > w.Helpers {
		return last, relay.HelperStorm
	}
	since := last.At
	for {
		select {
		case <-done:
			return last, ""
		case <-w.Clock.After(w.Poll):
		}
		s, n := w.sample()
		if w.Helpers > 0 && n > w.Helpers {
			return s, relay.HelperStorm
		}
		if !s.same(last) {
			last, since = s, s.At
			continue
		}
		last.At = s.At
		if s.At.Sub(since) >= w.Stall {
			return last, relay.Stalled
		}
	}
}
