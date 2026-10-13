package night

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// lockChildEnv names the directory the re-executed test binary holds the
// root lock for (TestAR13_4_RootLockAcrossProcesses).
const lockChildEnv = "AR13_4_LOCK_CHILD"

// TestAR13_4_RootLockAcrossProcesses (R6 §4): LockRoot is held across
// processes: a second process holding the root's lock keeps this one out
// until it lets go.
func TestAR13_4_RootLockAcrossProcesses(t *testing.T) {
	if dir := os.Getenv(lockChildEnv); dir != "" {
		unlock, err := LockRoot(filepath.Join(dir, "state"), "/r")
		if err != nil {
			os.WriteFile(filepath.Join(dir, "error"), []byte(err.Error()), 0o600)
			return
		}
		os.WriteFile(filepath.Join(dir, "held"), nil, 0o600)
		time.Sleep(800 * time.Millisecond)
		os.WriteFile(filepath.Join(dir, "released"), nil, 0o600)
		unlock()
		return
	}
	dir := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=^TestAR13_4_RootLockAcrossProcesses$", "-test.count=1")
	child.Env = append(os.Environ(), lockChildEnv+"="+dir)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Wait()
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if b, err := os.ReadFile(filepath.Join(dir, "error")); err == nil {
			t.Fatalf("the child could not lock: %s", b)
		}
		if _, err := os.Stat(filepath.Join(dir, "held")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the child never held the lock")
		}
	}
	unlock, err := LockRoot(filepath.Join(dir, "state"), "/r")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := os.Stat(filepath.Join(dir, "released")); err != nil {
		t.Fatal("this process took the root's lock while another process held it")
	}
	if _, err := os.Stat(filepath.Join(dir, "state", "locks", fmt.Sprintf("root-%x", sha1Of("/r")))); err != nil {
		t.Fatalf("no lock file at <state>/locks/root-<sha1>: %v", err)
	}
	// Another root is its own lock.
	other, err := LockRoot(filepath.Join(dir, "state"), "/s")
	if err != nil {
		t.Fatal(err)
	}
	other()
}

// movingWorld is a root whose HEAD is h1 and whose status is clean; the
// shell runs move before answering each line.
func movingWorld(t *testing.T, move func(g *effects.FakeGit)) (effects.Effects, *effects.FakeGit, *effects.FakeShell) {
	t.Helper()
	g := &effects.FakeGit{Heads: map[string]string{"/r": "h1"}, Statuses: map[string][]string{}}
	sh := &effects.FakeShell{Results: map[string]effects.ShellResult{"make check": {Exit: 0, Last: "ok"}}}
	fx := effects.Effects{Git: g, Shell: shellFunc(func(dir, line string, to time.Duration) (int, string, error) {
		if move != nil {
			move(g)
		}
		return sh.Run(dir, line, to)
	}), FS: &effects.FakeFS{}, Clock: effects.FakeClock{T: time.Date(2026, 9, 23, 22, 0, 0, 0, time.UTC)}}
	return fx, g, sh
}

// shellFunc is a Shell of one function.
type shellFunc func(dir, line string, timeout time.Duration) (int, string, error)

func (f shellFunc) Run(dir, line string, timeout time.Duration) (int, string, error) {
	return f(dir, line, timeout)
}

// TestAR13_4_MemoStoresOnlyUnmovedHead (ADV6-9): a line during which the
// root's HEAD moved, or its status changed, is not stored (under either
// head); a line with both unchanged is.
func TestAR13_4_MemoStoresOnlyUnmovedHead(t *testing.T) {
	cases := map[string]struct {
		move   func(g *effects.FakeGit)
		stored bool
	}{
		"unmoved": {nil, true},
		"commit":  {func(g *effects.FakeGit) { g.Heads["/r"] = "h2" }, false},
		"dirty":   {func(g *effects.FakeGit) { g.Statuses["/r"] = []string{" M a.go"} }, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx, _, _ := movingWorld(t, c.move)
			b := newBound(fx, "/state", "/r")
			if exit, _, err := b.Run("/r", "make check", time.Minute); err != nil || exit != 0 {
				t.Fatalf("run: %d %v", exit, err)
			}
			m := LoadMemo(fx.FS, "/state")
			_, at1 := m.Get("/r", "h1", "make check")
			_, at2 := m.Get("/r", "h2", "make check")
			if at1 != c.stored || at2 {
				t.Fatalf("stored at h1 %v, at h2 %v; want h1 %v, h2 false", at1, at2, c.stored)
			}
		})
	}
}

// TestAR13_4_MemoSaveKeepsBothWriters (ADV6-9): two memos loaded from one
// file each store a line; both lines survive, and writers at once never
// fail a save nor leave the file unreadable.
func TestAR13_4_MemoSaveKeepsBothWriters(t *testing.T) {
	state := t.TempDir()
	fs := effects.OSFS{}
	e := MemoEntry{Exit: 0, Last: "ok", Status: relay.CheckGreen, TS: "2026-09-23T22:00:00Z"}
	a, b := LoadMemo(fs, state), LoadMemo(fs, state)
	if err := a.Put("/r", "h1", "make check", e); err != nil {
		t.Fatal(err)
	}
	if err := b.Put("/r", "h1", "go test ./internal/x", e); err != nil {
		t.Fatal(err)
	}
	if err := b.Put("/s", "s1", "make check", e); err != nil {
		t.Fatal(err)
	}
	m := LoadMemo(fs, state)
	for _, k := range [][3]string{{"/r", "h1", "make check"}, {"/r", "h1", "go test ./internal/x"}, {"/s", "s1", "make check"}} {
		if _, ok := m.Get(k[0], k[1], k[2]); !ok {
			t.Fatalf("lost %v", k)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 400)
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := LoadMemo(fs, state)
			for i := range 50 {
				if err := m.Put("/r", "h1", fmt.Sprintf("w%d-%d", w, i), e); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(state, MemoFile))
	if err != nil {
		t.Fatal(err)
	}
	var roots map[string]*memoRoot
	if err := json.Unmarshal(raw, &roots); err != nil {
		t.Fatalf("the memo is corrupt: %v", err)
	}
	if n := len(roots["/r"].Lines); n != 202 {
		t.Fatalf("/r holds %d lines, want 202", n)
	}
	if left, _ := filepath.Glob(filepath.Join(state, "*.tmp*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

// TestAR13_4_MemoAgeIsAMiss (R6 §4): an entry older than MemoMaxAge is a
// miss, so the line runs again; a younger one answers.
func TestAR13_4_MemoAgeIsAMiss(t *testing.T) {
	for _, age := range []time.Duration{time.Hour, MemoMaxAge + time.Minute} {
		fx, _, sh := movingWorld(t, nil)
		now := fx.Clock.Now()
		e := MemoEntry{Exit: 0, Last: "ok", Status: relay.CheckGreen, TS: now.Add(-age).Format(time.RFC3339)}
		LoadMemo(fx.FS, "/state").Put("/r", "h1", "make check", e)
		b := newBound(fx, "/state", "/r")
		b.Run("/r", "make check", time.Minute)
		ran := len(sh.Calls()) == 1
		if ran != (age > MemoMaxAge) {
			t.Fatalf("an entry %s old: ran %v", age, ran)
		}
	}
	if MemoMaxAge != 24*time.Hour {
		t.Fatalf("MemoMaxAge %s, want 24h", MemoMaxAge)
	}
}

// TestAR13_4_StaleLockReclaimed (R6 §4): a stage lock holds `pid started
// worktree`; a lock whose pid is dead, or alive but started at another
// time, is reclaimed with a `lock-reclaimed` line; a live holder's refuses.
func TestAR13_4_StaleLockReclaimed(t *testing.T) {
	cases := map[string]struct {
		pid      int
		started  string
		reclaim  bool
		ownStart string
	}{
		"dead":      {pid: 424242, started: "Tue Sep 22 01:00:00 2026", reclaim: true},
		"restarted": {pid: 77, started: "Tue Sep 22 01:00:00 2026", reclaim: true},
		"live":      {pid: 77, started: "Wed Sep 23 21:00:00 2026", reclaim: false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fx, _, l, rp := stageFakes(t)
			fx.Procs = &effects.FakeProcs{Starts: map[int]string{77: "Wed Sep 23 21:00:00 2026",
				os.Getpid(): "Wed Sep 23 22:00:00 2026"}}
			lock := lockPath(rp.Worktree)
			os.MkdirAll(filepath.Dir(lock), 0o700)
			os.WriteFile(lock, []byte(fmt.Sprintf("%d\t%s\t%s\n", c.pid, c.started, rp.Worktree)), 0o600)
			st, err := Stage(fx, rp)
			if (err == nil) != c.reclaim {
				t.Fatalf("Stage err %v, want reclaimed %v", err, c.reclaim)
			}
			var reclaimed map[string]any
			for _, w := range l.Written() {
				if w["event"] == "lock-reclaimed" {
					reclaimed = w
				}
			}
			if (reclaimed != nil) != c.reclaim {
				t.Fatalf("lock-reclaimed line %v, want %v", reclaimed, c.reclaim)
			}
			if !c.reclaim {
				return
			}
			if reclaimed["pid"] != float64(c.pid) || reclaimed["row"] != rp.ID {
				t.Fatalf("lock-reclaimed %v", reclaimed)
			}
			b, _ := os.ReadFile(lock)
			if got := strings.TrimSpace(string(b)); got != fmt.Sprintf("%d\tWed Sep 23 22:00:00 2026\t%s", os.Getpid(), rp.Worktree) {
				t.Fatalf("the new lock holds %q", got)
			}
			st.Unlock()
		})
	}
	// Stale reads the same: a lock with no started (the old shape) is stale
	// only when its pid is dead.
	procs := &effects.FakeProcs{Starts: map[int]string{77: "x"}}
	dir := t.TempDir()
	for body, want := range map[string]bool{"77 /r/.claude/worktrees/a": false, "424242 /r/.claude/worktrees/a": true} {
		p := filepath.Join(dir, "l")
		os.WriteFile(p, []byte(body+"\n"), 0o600)
		if h, stale := StaleLock(procs, p); stale != want {
			t.Fatalf("%q: stale %v (%+v), want %v", body, stale, h, want)
		}
	}
}

// spentClock is a clock whose every deadline has passed: a budget armed
// on it is spent before the first line.
type spentClock struct{ effects.FakeClock }

func (c spentClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.T
	return ch
}

// TestAR13_4_BudgetBeforeProbe (§16 gap 3): the budget is read before the
// verify line's `make -n check` probe: spent before it, the probe never
// runs and the verify check is `unrun: budget`, never empty.
func TestAR13_4_BudgetBeforeProbe(t *testing.T) {
	repo, q := fixture(t)
	fx, g, sh, _ := fakes(repo, false)
	g.Heads = map[string]string{"/root": "m1"}
	fx.Clock = spentClock{effects.FakeClock{T: time.Date(2026, 9, 23, 22, 0, 0, 0, time.UTC)}}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", MeasureSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range sh.Calls() {
		if strings.HasPrefix(c, "/root: ") {
			t.Fatalf("a line ran in the root past the budget: %q", sh.Calls())
		}
	}
	if v := p.Verify; v == nil || v.Status != relay.CheckUnrun || v.MainLast != BudgetReason || v.Name != "auto" {
		t.Fatalf("verify %+v, want unrun: budget", p.Verify)
	}
}
