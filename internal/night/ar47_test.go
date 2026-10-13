package night

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// fakeIdle answers each Idle with the next of series, then its last for
// ever; err makes every read fail.
type fakeIdle struct {
	series []float64
	err    error

	mu sync.Mutex
	n  int
}

func (i *fakeIdle) Idle() (float64, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.n++
	if i.err != nil {
		return 0, i.err
	}
	if len(i.series) == 0 {
		return 0, errors.New("idle: no series")
	}
	return i.series[min(i.n, len(i.series))-1], nil
}

func (i *fakeIdle) reads() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.n
}

// idleWait is one Breaker.Wait over fakes: its stats, its answer, what the
// note heard, and how far the clock moved.
type idleWait struct {
	st    BreakerStats
	ok    bool
	notes string
	loads int
	moved string
}

// waitWith runs DefaultBreaker.Wait with idle as its second key (nil: no
// reader) over the loads.
func waitWith(idle effects.Idle, loads ...float64) idleWait {
	b := DefaultBreaker
	b.Idle = idle
	load, clock := &effects.FakeLoad{Series: loads}, newJump()
	start := clock.Now()
	var notes []string
	st, ok := b.Wait(context.Background(), load, clock, func(open bool, avg float64, cores int) {
		notes = append(notes, fmt.Sprint(open, avg, cores))
	})
	return idleWait{st, ok, fmt.Sprint(notes), load.Reads(), clock.Now().Sub(start).String()}
}

// breakerStates is the ledger's `breaker` lines by state, as breakerLines
// writes them.
func breakerStates(l *effects.FakeLedger, state string) []string {
	var out []string
	for _, w := range l.Written() {
		if w["event"] == "breaker" && w["state"] == state {
			out = append(out, fmt.Sprint(w["state"], " ", w["load"], "/", w["cores"]))
		}
	}
	return out
}

// TestAR47_2_OpenNeedsBothKeys (breaker-signal-r1 §2; AR47 act 2): the
// breaker opens only when the load is above Open x cores and the idle
// share is at or under IdleFloor. Load 70 on 16 cores with the CPU 70%
// idle never opens and waits nothing; 10% idle opens. Open, it closes
// under Close x cores as before, or when two polls in a row read the idle
// share above the floor: one quiet poll does not close it. The idle share
// is read only once the load has turned its key.
func TestAR47_2_OpenNeedsBothKeys(t *testing.T) {
	if DefaultIdleFloor != 0.25 || DefaultBreaker.IdleFloor != DefaultIdleFloor || DefaultBreaker.Idle != nil {
		t.Fatalf("floor %v, DefaultBreaker %+v", DefaultIdleFloor, DefaultBreaker)
	}
	idle := &fakeIdle{series: []float64{0.70}}
	if w := waitWith(idle, 70); !w.ok || w.st.Opens != 0 || w.st.WaitS != 0 || w.st.Load != 70 || w.notes != "[]" || w.moved != "0s" || w.loads != 1 || idle.reads() != 1 {
		t.Fatalf("load 70, idle 0.70: %+v, %d idle reads; want never open, no wait", w, idle.reads())
	}
	idle = &fakeIdle{series: []float64{0.10}}
	if w := waitWith(idle, 70, 70, 20); !w.ok || w.st.Opens != 1 || w.st.WaitS != 60 || w.notes != "[true 70 16 false 20 16]" || idle.reads() != 2 {
		t.Fatalf("load 70, idle 0.10: %+v, %d idle reads; want open, closed by the load at the second poll", w, idle.reads())
	}
	// A reading at the floor counts as under it.
	if w := waitWith(&fakeIdle{series: []float64{DefaultIdleFloor}}, 70, 70, 20); w.st.Opens != 1 {
		t.Fatalf("idle at the floor: %+v, want open", w)
	}
	// Open, the load never falling: one quiet poll does not close it, the
	// second in a row does.
	if w := waitWith(&fakeIdle{series: []float64{0.10, 0.60, 0.60}}, 70); !w.ok || w.st.Opens != 1 || w.st.WaitS != 60 || w.notes != "[true 70 16 false 70 16]" {
		t.Fatalf("two quiet polls: %+v, want closed at the second poll", w)
	}
	if w := waitWith(&fakeIdle{series: []float64{0.10, 0.60, 0.10, 0.60, 0.60}}, 70); !w.ok || w.st.WaitS != 120 || w.notes != "[true 70 16 false 70 16]" {
		t.Fatalf("a busy poll between two quiet ones: %+v, want closed at the fourth poll", w)
	}
	if w := waitWith(&fakeIdle{series: []float64{0.10, 0.60, 0.10}}, 70); w.ok || w.st.WaitS != 600 {
		t.Fatalf("one quiet poll only: %+v, want still open at Max", w)
	}
	// A quiet machine pays nothing: under Open x cores the share is not read.
	idle = &fakeIdle{series: []float64{0.10}}
	if w := waitWith(idle, 50); !w.ok || w.st.Opens != 0 || idle.reads() != 0 {
		t.Fatalf("load 50: %+v, %d idle reads; want none", w, idle.reads())
	}

	// Through a plan and through a spawn: both take the reader from the effects.
	repo, q := fixture(t)
	fx, _, sh, l := fakes(repo, false)
	fx.Clock, fx.Load, fx.Idle = newJump(), &effects.FakeLoad{Series: []float64{70, 3}}, &fakeIdle{series: []float64{0.70}}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Breaker.Opens != 0 || p.Breaker.WaitS != 0 || len(sh.Calls()) == 0 || len(breakerStates(l, "open")) != 0 {
		t.Fatalf("plan at load 70, idle 0.70: breaker %+v, %d calls, lines %v", p.Breaker, len(sh.Calls()), breakerLines(l))
	}
	fx, _, _, l = fakes(repo, false)
	fx.Clock, fx.Load, fx.Idle = newJump(), &effects.FakeLoad{Series: []float64{70, 70, 20}}, &fakeIdle{series: []float64{0.10}}
	if p, err = BuildPlan(fx, q, Options{Repo: repo, State: "/state"}); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(breakerLines(l)); got != "[open 70/16 close 20/16]" || p.Breaker.Opens != 1 {
		t.Fatalf("plan at load 70, idle 0.10: lines %s, breaker %+v", got, p.Breaker)
	}
	// A spawn at load 90, the CPU 70% idle: the row is spawned (AR10 act 5
	// has the same load, no idle reader, stop the night `load`).
	fx, _, _, l = fakes(repo, false)
	fx.Clock, fx.Load, fx.Idle = newJump(), &effects.FakeLoad{Series: []float64{90}}, &fakeIdle{series: []float64{0.70}}
	sp := &effects.FakeSpawner{}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1",
		Spawn: []string{"claude", "-p", "--model", "claude-opus-5-5"}}
	s, _ := RunRow(fx, sp, &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: time.Hour})
	if s.Reason == relay.LoadOpen || len(sp.Spawns()) != 1 || len(breakerStates(l, "open")) != 0 {
		t.Fatalf("spawn at load 90, idle 0.70: sitting %+v, %d spawns, lines %v", s, len(sp.Spawns()), breakerLines(l))
	}
}

// redLine runs a red `make check` through the root's bounded shell with
// idle as the second key over the loads (the read before the line, then
// the read after it): whether D4 found it loaded, and its memo entry.
func redLine(t *testing.T, idle effects.Idle, loads ...float64) (loaded bool, e MemoEntry, stored bool) {
	t.Helper()
	_, _, fx, _, sh := memoPlan(t)
	sh.Results["make check"] = effects.ShellResult{Exit: 2, Last: "FAIL"}
	fx.Load, fx.Idle = &effects.FakeLoad{Series: loads}, idle
	b := newBound(fx, "/state", "/root")
	if exit, last, err := b.Run("/root", "make check", time.Minute); exit != 2 || last != "FAIL" || err != nil {
		t.Fatalf("the red line: %d %q %v", exit, last, err)
	}
	e, stored = LoadMemo(fx.FS, "/state").Get("/root", "aaa111", "make check")
	return b.lastLoaded(), e, stored
}

// TestAR47_3_LoadedNeedsBothKeys (breaker-signal-r1 §2; AR47 act 3): D4
// on both keys. A red baseline line at load 40 on 16 cores (above Close,
// under Open) with the CPU 70% idle stays `red` and is stored in the
// memo; the same line 10% idle is `unrun: loaded` and is not stored. A
// line is loaded when the read before it or the read after it turns both
// keys, and the idle share is read only when that read's load is above
// Close x cores.
func TestAR47_3_LoadedNeedsBothKeys(t *testing.T) {
	plan := func(share float64) (*Plan, effects.Effects) {
		repo, q, fx, _, sh := memoPlan(t)
		sh.Results["make check"] = effects.ShellResult{Exit: 2, Last: "FAIL"}
		fx.Load, fx.Idle = &effects.FakeLoad{Series: []float64{40}}, &fakeIdle{series: []float64{share}}
		p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
		if err != nil {
			t.Fatal(err)
		}
		return p, fx
	}
	p, fx := plan(0.70)
	if v := p.Verify; v == nil || v.Status != relay.CheckRed || v.MainExit != 2 || v.MainLast != "FAIL" || p.Breaker.Opens != 0 {
		t.Fatalf("load 40, idle 0.70: verify %+v, breaker %+v; want red", v, p.Breaker)
	}
	for _, c := range p.Rows[1].Baseline {
		if c.Status == relay.CheckUnrun {
			t.Fatalf("load 40, idle 0.70: check %+v, want none unrun", c)
		}
	}
	if e, ok := LoadMemo(fx.FS, "/state").Get("/root", "aaa111", "make check"); !ok || e.Exit != 2 || e.Status != relay.CheckRed || e.Load != 40 {
		t.Fatalf("load 40, idle 0.70: the memo entry %+v %v, want the red stored with its load", e, ok)
	}
	p, fx = plan(0.10)
	if v := p.Verify; v == nil || v.Status != relay.CheckUnrun || v.MainLast != LoadedReason || v.MainExit != -1 {
		t.Fatalf("load 40, idle 0.10: verify %+v, want unrun: loaded", v)
	}
	if b, _ := fx.FS.ReadFile("/state/baselines.json"); strings.Contains(string(b), "make check") {
		t.Fatalf("load 40, idle 0.10: a loaded line stored:\n%s", b)
	}

	// The reads round one line: before, after.
	for _, c := range []struct {
		name   string
		loads  []float64
		idle   []float64
		loaded bool
		reads  int // of the idle share; -1, not pinned
	}{
		{"quiet before and after", []float64{3, 3}, []float64{0.10}, false, 0},
		{"loaded at the read after", []float64{3, 40}, []float64{0.10}, true, 1},
		{"loaded at the read before", []float64{40, 3}, []float64{0.10}, true, 1},
		{"the read after confirms", []float64{40, 40}, []float64{0.70, 0.10}, true, 2},
		{"neither read confirms", []float64{40, 40}, []float64{0.70, 0.70}, false, 2},
		{"at the floor", []float64{3, 40}, []float64{DefaultIdleFloor}, true, 1},
		{"above Open, unconfirmed by the wait", []float64{70, 3}, []float64{0.70}, false, 1},
	} {
		idle := &fakeIdle{series: c.idle}
		loaded, _, stored := redLine(t, idle, c.loads...)
		if loaded != c.loaded || stored == c.loaded || (c.reads >= 0 && idle.reads() != c.reads) {
			t.Fatalf("%s: loaded %v, stored %v, %d idle reads; want loaded %v, %d reads", c.name, loaded, stored, idle.reads(), c.loaded, c.reads)
		}
	}
}

// unconfirmedLines is the ledger's `breaker` lines with `state: unconfirmed`.
func unconfirmedLines(l effects.Ledger) []map[string]any {
	var out []map[string]any
	for _, w := range l.(*effects.FakeLedger).Written() {
		if w["event"] == "breaker" && w["state"] == "unconfirmed" {
			out = append(out, w)
		}
	}
	return out
}

// TestAR47_4_UnconfirmedLine (breaker-signal-r1 §3; AR47 act 4): a load
// above the threshold that the idle share did not confirm writes one
// `breaker` line with `state: unconfirmed`, `load`, `cores` and `idle`,
// once per wait and once per line's D4 read, and counts in the plan's
// `breaker` value as `unconfirmed`, a key left out when it is zero. An
// open or close line gains `idle` when it was read.
func TestAR47_4_UnconfirmedLine(t *testing.T) {
	var sh *effects.FakeShell
	plan := func(loads, idle []float64) (*Plan, *effects.FakeLedger) {
		repo, q, fx, _, s := memoPlan(t)
		sh = s
		fx.Clock, fx.Load, fx.Idle = newJump(), &effects.FakeLoad{Series: loads}, &fakeIdle{series: idle}
		p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
		if err != nil {
			t.Fatal(err)
		}
		return p, fx.Ledger.(*effects.FakeLedger)
	}
	planLine := func(l *effects.FakeLedger) map[string]any {
		for _, w := range l.Written() {
			if w["event"] == "plan" {
				b, _ := w["breaker"].(map[string]any)
				return b
			}
		}
		return nil
	}

	// Ruling 2's first case: load 70 on 16 cores, the CPU 70% idle.
	p, l := plan([]float64{70, 3}, []float64{0.70})
	u := unconfirmedLines(l)
	if len(u) != 1 || number(u[0]["load"]) != 70 || number(u[0]["cores"]) != 16 || number(u[0]["idle"]) != 0.7 || u[0]["row"] != nil || len(breakerLines(l)) != 1 {
		t.Fatalf("load 70, idle 0.70: unconfirmed lines %v of %v, want exactly one with load 70, cores 16, idle 0.7", u, breakerLines(l))
	}
	if b := planLine(l); b == nil || number(b["unconfirmed"]) != 1 || number(b["opens"]) != 0 || number(b["load"]) != 70 {
		t.Fatalf("the plan line's breaker %v, want unconfirmed: 1", b)
	}
	if b, _ := json.Marshal(p); !strings.Contains(string(b), `"breaker":{"opens":0,"wait_s":0,"load":70,"unconfirmed":1}`) {
		t.Fatalf("--json lacks the count: %s", b)
	}

	// An open confirmed by the share carries it; a close on the load read
	// no share and carries none; a close on two quiet polls carries the last.
	p, l = plan([]float64{70, 70, 20}, []float64{0.10})
	var open, closed map[string]any
	for _, w := range l.Written() {
		if w["event"] == "breaker" && w["state"] == "open" {
			open = w
		} else if w["event"] == "breaker" && w["state"] == "close" {
			closed = w
		}
	}
	if _, has := closed["idle"]; open == nil || closed == nil || number(open["idle"]) != 0.1 || has || len(unconfirmedLines(l)) != 0 {
		t.Fatalf("load 70, idle 0.10: open %v, close %v", open, closed)
	}
	if b, has := planLine(l)["unconfirmed"]; has || p.Breaker.Opens != 1 {
		t.Fatalf("no unconfirmed load: the plan line's breaker carries unconfirmed %v, breaker %+v", b, p.Breaker)
	}
	if b, _ := json.Marshal(p); !strings.Contains(string(b), `"breaker":{"opens":1,"wait_s":60,"load":70}`) {
		t.Fatalf("--json with no unconfirmed load: %s", b)
	}
	_, l = plan([]float64{70, 70, 70, 3}, []float64{0.10, 0.60, 0.60})
	for _, w := range l.Written() {
		if w["event"] == "breaker" && w["state"] == "close" && (number(w["idle"]) != 0.6 || number(w["load"]) != 70) {
			t.Fatalf("a close on two quiet polls: %v, want idle 0.6", w)
		}
	}
	if got := fmt.Sprint(breakerLines(l)); got != "[open 70/16 close 70/16]" {
		t.Fatalf("a close on two quiet polls: lines %s", got)
	}

	// D4: one line per line run whose load the share did not confirm.
	_, l = plan([]float64{40}, []float64{0.70})
	u = unconfirmedLines(l)
	if len(u) == 0 || len(u) != len(sh.Calls()) || number(planLine(l)["unconfirmed"]) != float64(len(u)) {
		t.Fatalf("load 40, idle 0.70: %d unconfirmed lines over %d lines run, plan line %v", len(u), len(sh.Calls()), planLine(l))
	}
	for _, w := range u {
		if number(w["load"]) != 40 || number(w["cores"]) != 16 || number(w["idle"]) != 0.7 {
			t.Fatalf("a D4 unconfirmed line %v", w)
		}
	}
	// A line the share confirms loaded writes none.
	if _, l = plan([]float64{40}, []float64{0.10}); len(unconfirmedLines(l)) != 0 || len(breakerLines(l)) != 0 {
		t.Fatalf("load 40, idle 0.10: breaker lines %v, want none", breakerLines(l))
	}

	// A spawn: the line names its row, the sitting line carries the count.
	repo, _ := fixture(t)
	fx, _, _, l := fakes(repo, false)
	fx.Clock, fx.Load, fx.Idle = newJump(), &effects.FakeLoad{Series: []float64{90}}, &fakeIdle{series: []float64{0.70}}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1",
		Spawn: []string{"claude", "-p", "--model", "claude-opus-5-5"}}
	RunRow(fx, &effects.FakeSpawner{}, &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: time.Hour})
	if u = unconfirmedLines(l); len(u) != 1 || u[0]["row"] != "r1" || number(u[0]["load"]) != 90 || number(u[0]["idle"]) != 0.7 {
		t.Fatalf("a spawn at load 90, idle 0.70: unconfirmed lines %v", u)
	}
	if b, _ := sittingOf(l, "r1")["breaker"].(map[string]any); b == nil || number(b["unconfirmed"]) != 1 || number(b["opens"]) != 0 {
		t.Fatalf("the sitting line's breaker %v", b)
	}
}

// brokenIdle fails every read and says 90% idle beside its error: a
// share that must not be believed.
type brokenIdle struct{ n int }

func (b *brokenIdle) Idle() (float64, error) {
	b.n++
	return 0.90, errors.New("idle: top -l 2: exit status 1")
}

// TestAR47_5_NilIdleAsToday (breaker-signal-r1 §2; AR47 act 5): an
// unreadable idle share leaves the load alone to decide. With no reader,
// and again with one that fails, load 70 opens the breaker and a red line
// at load 40 is `unrun: loaded`, with the ledger lines and the counts of
// AR10 act 5: no `idle`, no `unconfirmed`.
func TestAR47_5_NilIdleAsToday(t *testing.T) {
	broken := &brokenIdle{}
	for _, c := range []struct {
		name string
		idle effects.Idle
	}{{"no reader", nil}, {"a failing reader", broken}} {
		if w := waitWith(c.idle, 70, 70, 20); !w.ok || w.st.Opens != 1 || w.st.WaitS != 60 || w.st.Load != 70 || w.st.Unconfirmed != 0 || w.notes != "[true 70 16 false 20 16]" {
			t.Fatalf("%s, load 70: %+v, want open and closed as AR10 act 5", c.name, w)
		}
		if w := waitWith(c.idle, 90); w.ok || w.st.Opens != 1 || w.st.WaitS != 600 {
			t.Fatalf("%s, load 90: %+v, want still open at Max", c.name, w)
		}
		if loaded, _, stored := redLine(t, c.idle, 40); !loaded || stored {
			t.Fatalf("%s, a red line at load 40: loaded %v, stored %v", c.name, loaded, stored)
		}

		repo, q, fx, _, sh := memoPlan(t)
		sh.Results["make check"] = effects.ShellResult{Exit: 2, Last: "FAIL"}
		fx.Load, fx.Idle = &effects.FakeLoad{Series: []float64{40}}, c.idle
		p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
		if err != nil {
			t.Fatal(err)
		}
		if v := p.Verify; v == nil || v.Status != relay.CheckUnrun || v.MainLast != LoadedReason || v.MainExit != -1 || p.Breaker.Unconfirmed != 0 {
			t.Fatalf("%s, a plan at load 40: verify %+v, breaker %+v; want unrun: loaded", c.name, v, p.Breaker)
		}
		if b, _ := fx.FS.ReadFile("/state/baselines.json"); strings.Contains(string(b), "make check") || len(breakerLines(fx.Ledger.(*effects.FakeLedger))) != 0 {
			t.Fatalf("%s, a plan at load 40: a loaded line stored, or a breaker line:\n%s", c.name, b)
		}

		repo, q, fx, _, _ = memoPlan(t)
		fx.Clock, fx.Load, fx.Idle = newJump(), &effects.FakeLoad{Series: []float64{70, 70, 20}}, c.idle
		if p, err = BuildPlan(fx, q, Options{Repo: repo, State: "/state"}); err != nil {
			t.Fatal(err)
		}
		l := fx.Ledger.(*effects.FakeLedger)
		if got := fmt.Sprint(breakerLines(l)); got != "[open 70/16 close 20/16]" {
			t.Fatalf("%s, a plan at load 70: lines %s", c.name, got)
		}
		for _, w := range l.Written() {
			if _, has := w["idle"]; has && w["event"] == "breaker" {
				t.Fatalf("%s: a breaker line carries an idle share never read: %v", c.name, w)
			}
		}
		if b, _ := json.Marshal(p); !strings.Contains(string(b), `"breaker":{"opens":1,"wait_s":60,"load":70}`) {
			t.Fatalf("%s: --json %s", c.name, b)
		}
	}
	if broken.n == 0 {
		t.Fatal("the failing reader was never asked")
	}
}
