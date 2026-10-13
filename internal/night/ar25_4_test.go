package night

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// stormRow runs one row whose child (4242) has 4243 to 4246 below it, the
// first n of 4243 to 4245 reading `claude` by comm (4246 a zsh), and returns
// the sitting, its line and the kills.
func stormRow(t *testing.T, n int) (*relay.Sitting, map[string]any, []int) {
	t.Helper()
	repo, _ := fixture(t)
	fx, g, _, l := fakes(repo, false)
	sp := &effects.FakeSpawner{PID: 4242}
	comms := map[int]string{4246: "/bin/zsh"}
	for i, c := range []string{"claude", "/opt/elsewhere/claude", "claude"}[:n] {
		comms[4243+i] = c
	}
	for pid := 4243 + n; pid <= 4245; pid++ {
		comms[pid] = "make"
	}
	procs := &effects.FakeProcs{Mtime: time.Now(), Mortal: true, Comms: comms,
		Desc:   map[int][]int{4242: {4243, 4244, 4245, 4246}},
		Starts: map[int]string{4243: "t1", 4244: "t2", 4245: "t3", 4246: "t4"}}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main", HeadBefore: "abc"}
	g.Heads = map[string]string{st.Path: "abc"}
	fx.Procs = procs
	s, _ := RunRow(fx, sp, st, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: time.Hour})
	return s, sittingOf(l, "r1"), procs.Kills()
}

// seqProcs is a FakeProcs whose Descendants answers the next of Seq at each
// read (then the last): a tree that changes between the watchdog's ticks.
type seqProcs struct {
	*effects.FakeProcs
	mu  sync.Mutex
	Seq [][]int
	n   int
}

func (p *seqProcs) Descendants(int) ([]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	return p.Seq[min(p.n, len(p.Seq))-1], nil
}

// TestAR25_4_HelperStorm is ADV15-1's enforced half: each watchdog tick
// counts the `claude` processes (by comm) among the row's seen descendants,
// its own sitting (the spawn root) not among them, so the base is 0. Three,
// more than MaxHelpers, kill the row's group and its seen descendants at
// once and fail the row helper-storm, a sitting failure that stops the
// night, whatever env a script gave them; two do not. A helper that left
// the tree (its parent gone, or setsid) still counts while its start time
// reads the same.
func TestAR25_4_HelperStorm(t *testing.T) {
	if MaxHelpers != 2 || relay.HelperStorm != "helper-storm" {
		t.Fatalf("MaxHelpers %d, HelperStorm %q", MaxHelpers, relay.HelperStorm)
	}
	s, sit, kills := stormRow(t, 3)
	if s.State != relay.Failed || s.Reason != relay.HelperStorm || sit["stop"] != "helper-storm" {
		t.Fatalf("three claudes: sitting %+v, line %v; want failed helper-storm", s, sit)
	}
	for _, pid := range []int{4242, 4243, 4244, 4245, 4246} {
		if !slices.Contains(kills, pid) {
			t.Fatalf("kills %v: %d not killed", kills, pid)
		}
	}
	if got, err := relay.NightStep(relay.NightRunning, relay.OnSittingFailed, relay.HelperStorm); err != nil || got != relay.Stopped(relay.HelperStorm) {
		t.Fatalf("a helper storm steps the night to %v, %v", got, err)
	}
	if s, sit, _ := stormRow(t, 2); s.Reason == relay.HelperStorm || sit["stop"] == "helper-storm" {
		t.Fatalf("two claudes: sitting %+v, line %v; want no storm", s, sit)
	}

	// The seen descendants, not only the tree: 4243 and 4244 at the first
	// tick, then 4243 gone from the tree but alive as itself, 4244 and 4245
	// below: three.
	clock := jumpClock{&effects.StepClock{T: time.Date(2026, 9, 24, 23, 0, 0, 0, time.UTC)}, WatchdogPoll}
	procs := &seqProcs{FakeProcs: &effects.FakeProcs{Mtime: clock.Now(), CPU: []float64{1, 2, 3, 4, 5, 6},
		Comms:  map[int]string{4243: "claude", 4244: "claude", 4245: "claude", 4246: "sh"},
		Starts: map[int]string{4243: "t1", 4244: "t2", 4245: "t3", 4246: "t4"}},
		Seq: [][]int{{4243, 4244, 4246}, {4244, 4246}, {4244, 4245}}}
	wd := Watchdog{Git: &effects.FakeGit{Heads: map[string]string{"/wt": "abc"}}, Procs: procs, Clock: clock,
		Dir: "/wt", Pgid: 4242, Helpers: MaxHelpers}
	if _, why := wd.Run(make(chan struct{})); why != relay.HelperStorm || procs.n != 3 {
		t.Fatalf("an escaped helper: Run %q after %d reads, want helper-storm at the third", why, procs.n)
	}
	// 4243 dead (its start time gone): two, and an ended sitting is no storm.
	delete(procs.Starts, 4243)
	procs.n = 0
	done := make(chan struct{})
	go func() { time.Sleep(50 * time.Millisecond); close(done) }()
	wd.Clock = effects.FakeClock{T: clock.Now()}
	if _, why := wd.Run(done); why != "" {
		t.Fatalf("a dead helper counted: Run %q", why)
	}
}
