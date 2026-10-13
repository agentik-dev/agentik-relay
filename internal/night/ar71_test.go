package night_test

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// A model's limit with a reset pauses only its rows (usage-limit-resume
// r1 §2, §4 test 4; AR71): nights on TestAR67's world at --parallel 2,
// r1's queue model claude-fable-5-1, r2 and r3 opus.

// ar71Fable is a part's is_error result naming the Fable limit, 2am.
var ar71Fable = ar65Result("You've reached your Fable limit · resets 2am (UTC)")

// ar71Spawner starts each row's first child from first (none: clean),
// its stream silent until until[row] reads true when set; every later
// child is clean. A part that exits 0 commits a.txt in the tree first.
// r3After is whether
// r3's first spawn found r2's sitting line.
type ar71Spawner struct {
	w       *world
	first   map[string]effects.FakeRun
	until   map[string]func() bool
	mu      sync.Mutex
	n       map[string]int
	r3After bool
}

func (a *ar71Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	row := filepath.Base(sp.Dir)
	a.mu.Lock()
	a.n[row]++
	k := a.n[row]
	if row == "r3" && k == 1 {
		a.r3After = len(rowLines(a.w.ledger.Written(), "sitting", "r2")) > 0
	}
	a.mu.Unlock()
	run, ok := a.first[row]
	if k > 1 || run.Exit == 0 { // a clean part commits a.txt first
		if err := ar69Commit(sp.Dir, row, k+1); err != nil {
			return nil, nil, err
		}
	}
	if k > 1 || !ok {
		return (&effects.FakeSpawner{Out: ar67Clean, PID: 4343}).Start(sp)
	}
	p, out, err := (&effects.FakeSpawner{Outs: []effects.FakeRun{run}, PID: 4242}).Start(sp)
	if u := a.until[row]; u != nil && err == nil {
		out = &ar69Silent{ReadCloser: out, until: u}
	}
	return p, out, err
}

func (a *ar71Spawner) spawns(row string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if row != "" {
		return a.n[row]
	}
	return a.n["r1"] + a.n["r2"] + a.n["r3"]
}

// ar71World is the world: r1 fable, its part 1 the Fable result (exit
// 1); r2's first child r2 (none: clean), silent until r1's pause line.
func ar71World(t *testing.T, r2 *effects.FakeRun) (*world, *queue.Queue, night.NightOptions, *ar71Spawner, *effects.StepClock) {
	w, q, o, _, clock := ar65World(t, nil, "r1", "r2", "r3")
	q.Rows[0].Model = "claude-fable-5-1"
	o.Parallel = 2
	sp := &ar71Spawner{w: w, n: map[string]int{}, first: map[string]effects.FakeRun{"r1": {Out: ar71Fable, Exit: 1}},
		until: map[string]func() bool{"r2": func() bool { return len(ar65Events(w.ledger.Written(), "pause")) > 0 }}}
	if r2 != nil {
		sp.first["r2"] = *r2
	}
	return w, q, o, sp, clock
}

// ar71Run runs the night on sp in the background.
func ar71Run(w *world, q *queue.Queue, o night.NightOptions, sp effects.Spawner) chan *night.Night {
	res := make(chan *night.Night, 1)
	go func() {
		n, err := night.Run(w.fx, sp, q, o)
		if err != nil {
			n = &night.Night{State: relay.NightState("error: " + err.Error())}
		}
		res <- n
	}()
	return res
}

// ar71Pause asserts the pause line p: rows, kind, scope, wake, count.
func ar71Pause(t *testing.T, p map[string]any, rows, kind, scope, wake string, count int) {
	t.Helper()
	s, ok := p["scope"]
	if fmt.Sprint(p["rows"]) != rows || p["kind"] != kind || !ok || s != scope || p["wake"] != wake || number(p["count"]) != float64(count) {
		t.Fatalf("the pause line %v; want rows %s, kind %s, scope %q, wake %s, count %d", p, rows, kind, scope, wake, count)
	}
}

// TestAR71_2_ModelPause (AR71 rulings 3, 4): r1's Fable limit pauses only
// r1: one pause line [r1], kind model, scope fable, wake 02:02, and no
// kill line. Before the clock moves the night is paused, r2 runs on to
// green, then r3 spawns after r2's sitting line and goes green. At 02:02
// one wake line [r1], r1's part 2, all green, queue-empty, 4 spawns.
func TestAR71_2_ModelPause(t *testing.T) {
	w, q, o, sp, clock := ar71World(t, nil)
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r3's sitting line, the night paused", func() bool {
		return len(rowLines(w.ledger.Written(), "sitting", "r3")) == 1 && ar65Paused(o, "r1")
	})
	all := w.ledger.Written()
	ps := ar65Events(all, "pause")
	if len(ps) != 1 || len(ar65Events(all, "kill")) != 0 || !clock.Now().Equal(ar65T) {
		t.Fatalf("%d pause lines, kill lines %v, clock %s; want one, none, unmoved", len(ps), ar65Events(all, "kill"), clock.Now())
	}
	ar71Pause(t, ps[0], "[r1]", "model", "fable", "2026-10-11T02:02:00Z", 1)
	for _, id := range []string{"r2", "r3"} {
		if sl := rowLines(all, "sitting", id); len(sl) != 1 || sl[0]["state"] != "green" {
			t.Fatalf("%s's sitting lines %v, want one green, before the wake", id, sl)
		}
	}
	if !sp.r3After {
		t.Fatal("r3 spawned before r2's sitting line")
	}
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	n := ar65End(t, res)
	all = w.ledger.Written()
	wk := ar65Events(all, "wake")
	if len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[r1]" || len(rowLines(all, "sitting", "r1")) != 2 {
		t.Fatalf("wake lines %v, r1's sitting lines %d; want one [r1], 2", wk, len(rowLines(all, "sitting", "r1")))
	}
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green r3=green" || sp.spawns("") != 4 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:queue-empty, all green, 4", n.State, states(n), sp.spawns(""))
	}
}

// TestAR71_3_ScopeEdges (AR71 rulings 3 to 5; the hold's N1): a window
// hit outside the scope widens it to every row, a second pause; a
// sibling's model hit leaves an every-row scope every row; the owner's
// stop lets r2 finish before the stop line; --max-sittings picks nothing
// while paused.
func TestAR71_3_ScopeEdges(t *testing.T) {
	t.Run("five_hour outside the scope", func(t *testing.T) {
		w, q, o, sp, clock := ar71World(t, &effects.FakeRun{Out: ar65Event("five_hour", ar65T.Add(2*time.Hour)), Exit: 143})
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "the second pause line", func() bool {
			return len(ar65Events(w.ledger.Written(), "pause")) == 2 && ar65Paused(o, "r1", "r2")
		})
		ar71Pause(t, ar65Events(w.ledger.Written(), "pause")[1], "[r2]", "window", "", "2026-10-11T03:02:00Z", 2)
		ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
		time.Sleep(10 * o.Poll)
		if wk := ar65Events(w.ledger.Written(), "wake"); len(wk) != 0 || sp.spawns("r3") != 0 {
			t.Fatalf("at 02:02: wake lines %v, r3 spawned %d; want none, none", wk, sp.spawns("r3"))
		}
		ar69To(clock, ar65T.Add(2*time.Hour+2*time.Minute))
		n := ar65End(t, res)
		wk := ar65Events(w.ledger.Written(), "wake")
		if len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[r1 r2]" || states(n) != "r1=green r2=green r3=green" {
			t.Fatalf("wake lines %v, %s; want one [r1 r2], all green", wk, states(n))
		}
	})
	t.Run("N1: a sibling's model hit on an every-row scope", func(t *testing.T) {
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		q.Rows[1].Model = "claude-fable-5-1"
		o.Parallel = 2
		sp := &ar69Spawner{own: effects.FakeRun{Out: ar71Fable, Exit: 143}, started: make(chan struct{}), n: map[string]int{},
			paused: func() bool { return len(ar65Events(w.ledger.Written(), "pause")) > 0 },
			r1:     &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{{Out: ar67Limited, Exit: 143}, {Out: ar67Clean}}}}
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "the second pause line", func() bool {
			return len(ar65Events(w.ledger.Written(), "pause")) == 2 && ar65Paused(o, "r1", "r2")
		})
		all := w.ledger.Written()
		if len(ar65Events(all, "kill")) != 1 {
			t.Fatalf("kill lines %v, want r2's", ar65Events(all, "kill"))
		}
		ps := ar65Events(all, "pause")
		ar71Pause(t, ps[0], "[r1 r2]", "window", "", "2026-10-11T02:02:00Z", 1)
		ar71Pause(t, ps[1], "[r2]", "model", "", "2026-10-11T02:02:00Z", 1)
		ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
		n := ar65End(t, res)
		wk := ar65Events(w.ledger.Written(), "wake")
		if len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[r1 r2]" || states(n) != "r1=green r2=green" {
			t.Fatalf("wake lines %v, %s; want one [r1 r2], both green", wk, states(n))
		}
	})
	t.Run("relay stop while r2 runs", func(t *testing.T) {
		w, q, o, sp, _ := ar71World(t, &effects.FakeRun{Out: ar67Clean})
		sp.until["r2"] = func() bool { return len(ar65Events(w.ledger.Written(), "owner-stop")) > 0 }
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
		if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), "stop"); err != nil {
			t.Fatal(err)
		}
		n := ar65End(t, res)
		all := w.ledger.Written()
		sit, stop := -1, -1
		for i, l := range all {
			if l["event"] == "sitting" && l["row"] == "r2" {
				sit = i
			}
			if l["event"] == "stop" {
				stop = i
			}
		}
		if n.State != relay.Stopped(relay.OwnerStop) || n.Sittings["r2"].State != relay.Green || sit < 0 || sit > stop || sp.spawns("r3") != 0 {
			t.Fatalf("night %s, r2 %s, r2's sitting line at %d, the stop line at %d, r3 spawned %d; want stopped:owner-stop, green, before, never",
				n.State, n.Sittings["r2"].State, sit, stop, sp.spawns("r3"))
		}
		if s := n.Sittings["r1"]; s.State != relay.Failed || s.Reason != relay.UsageLimit || night.Locked(ar67Tree(w, "r1")) {
			t.Fatalf("r1 %s %s, locked %v; want failed usage-limit, unlocked", s.State, s.Reason, night.Locked(ar67Tree(w, "r1")))
		}
	})
	t.Run("--max-sittings 2", func(t *testing.T) {
		w, q, o, sp, clock := ar71World(t, nil)
		o.MaxSittings = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's sitting line, the night paused", func() bool {
			return len(rowLines(w.ledger.Written(), "sitting", "r2")) == 1 && ar65Paused(o, "r1")
		})
		time.Sleep(10 * o.Poll)
		if sp.spawns("r3") != 0 {
			t.Fatal("r3 picked while paused at --max-sittings 2")
		}
		ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
		n := ar65End(t, res)
		if n.State != relay.Stopped(relay.CountReached) || states(n) != "r1=green r2=green r3=skipped" || sp.spawns("r1") != 2 || sp.spawns("r3") != 0 {
			t.Fatalf("night %s, %s, r1 spawned %d, r3 %d; want stopped:count-reached, r3 skipped, 2, 0", n.State, states(n), sp.spawns("r1"), sp.spawns("r3"))
		}
	})
}

// ar71Stopped reads whether the night's status says stopped.
func ar71Stopped(o night.NightOptions) func() bool {
	return func() bool {
		return strings.HasPrefix(night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night).State, "stopped")
	}
}

// ar71Three is the fix round's world: r1 and r3 on Fable, r2 on Opus, at
// --parallel 3, r1's part 1 the Fable result held until r2 and r3 spawn.
func ar71Three(t *testing.T, r2, r3 effects.FakeRun) (*world, *queue.Queue, night.NightOptions, *ar71Spawner, *effects.StepClock) {
	w, q, o, _, clock := ar65World(t, nil, "r1", "r2", "r3")
	q.Rows[0].Model, q.Rows[2].Model = "claude-fable-5-1", "claude-fable-5-1"
	o.Parallel = 3
	sp := &ar71Spawner{w: w, n: map[string]int{}, first: map[string]effects.FakeRun{"r1": {Out: ar71Fable, Exit: 1}, "r2": r2, "r3": r3}}
	sp.until = map[string]func() bool{"r1": func() bool { return sp.spawns("r2") > 0 && sp.spawns("r3") > 0 }}
	return w, q, o, sp, clock
}

// TestAR71_4_SiblingWeekly (the fix round's F1; run 10 ruling 1): r1's
// Fable limit pauses [r1 r3]; r3, killed, comes back weekly and stops the
// night usage-limit, which kills r2, outside the scope, and fails it
// usage-limit: two kill lines.
func TestAR71_4_SiblingWeekly(t *testing.T) {
	w, q, o, sp, _ := ar71Three(t, effects.FakeRun{Out: ar67Clean}, effects.FakeRun{Out: ar65Event("seven_day", ar65T.Add(48*time.Hour)), Exit: 143})
	sp.until["r3"] = func() bool { return len(ar65Events(w.ledger.Written(), "pause")) > 0 }
	sp.until["r2"] = ar71Stopped(o)
	n := ar65End(t, ar71Run(w, q, o, sp))
	kills := ar65Events(w.ledger.Written(), "kill")
	if s := n.Sittings["r2"]; n.State != relay.Stopped(relay.UsageLimit) || s.State != relay.Failed || s.Reason != relay.UsageLimit || len(kills) != 2 {
		t.Fatalf("night %s, r2 %s %s, kill lines %v; want stopped:usage-limit, r2 failed usage-limit, two kill lines", n.State, s.State, s.Reason, kills)
	}
}

// TestAR71_5_FallbackOutside (the fix round's F2): r2's refusal naming no
// reset, outside the scope and before any wake, trips no probe: a second
// pause line [r2], count 2, scope "", the wake 02:02 kept; at the wake
// one wake line [r1 r2], all green, queue-empty.
func TestAR71_5_FallbackOutside(t *testing.T) {
	w, q, o, sp, clock := ar71World(t, &effects.FakeRun{Out: ar65Result("API Error: 429"), Exit: 1})
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "the second pause line or a stop", func() bool {
		all := w.ledger.Written()
		return len(ar65Events(all, "pause")) == 2 && ar65Paused(o, "r1", "r2") || len(ar65Events(all, "stop")) > 0
	})
	if stop := lastOf(w.ledger.Written(), "stop"); stop != nil {
		t.Fatalf("stopped %v %q before the wake; want a second pause", stop["reason"], stop["detail"])
	}
	ar71Pause(t, ar65Events(w.ledger.Written(), "pause")[1], "[r2]", "unknown", "", "2026-10-11T02:02:00Z", 2)
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	n := ar65End(t, res)
	wk := ar65Events(w.ledger.Written(), "wake")
	if len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[r1 r2]" || n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green r3=green" {
		t.Fatalf("wake lines %v, night %s, %s; want one [r1 r2], stopped:queue-empty, all green", wk, n.State, states(n))
	}
}

// TestAR71_6_SiblingCount (the fix round's F3; AR69 ruling 5): r1's
// Fable limit pauses [r1 r3] (count 1); r2, outside it, is refused
// five_hour at T+2h (count 2); then r3, the first pause's sibling, comes
// back with no reset: its count is its pause's less one, so it pauses and
// the night waits for 03:02, then all green.
func TestAR71_6_SiblingCount(t *testing.T) {
	w, q, o, sp, clock := ar71Three(t, effects.FakeRun{Out: ar65Event("five_hour", ar65T.Add(2*time.Hour)), Exit: 143},
		effects.FakeRun{Out: ar65Result("API Error: 429"), Exit: 143})
	sp.until["r2"] = func() bool { return len(ar65Events(w.ledger.Written(), "pause")) >= 1 }
	sp.until["r3"] = func() bool { return len(ar65Events(w.ledger.Written(), "pause")) >= 2 }
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r3's sitting line", func() bool { return len(rowLines(w.ledger.Written(), "sitting", "r3")) == 1 })
	time.Sleep(20 * o.Poll)
	if !ar65Paused(o, "r1", "r2", "r3") {
		n := ar65End(t, res)
		t.Fatalf("night %s, detail %q; want still paused until 03:02", n.State, lastOf(w.ledger.Written(), "stop")["detail"])
	}
	ar69To(clock, ar65T.Add(2*time.Hour+2*time.Minute))
	if n := ar65End(t, res); n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green r3=green" {
		t.Fatalf("night %s, %s; want stopped:queue-empty, all green", n.State, states(n))
	}
}

// TestAR71_7_SiblingSameReset (the fix round's F4; AR69 ruling 5, "else
// as today"): TestAR69_6's night, r2's own five_hour at r1's reset: one
// pause line, and at 02:02 one wake line, both green.
func TestAR71_7_SiblingSameReset(t *testing.T) {
	w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
	o.Parallel = 2
	sp := &ar69Spawner{own: effects.FakeRun{Out: ar65Event("five_hour", ar65T.Add(time.Hour)), Exit: 143}, started: make(chan struct{}), n: map[string]int{},
		paused: func() bool { return len(ar65Events(w.ledger.Written(), "pause")) > 0 },
		r1:     &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{{Out: ar67Limited, Exit: 143}, {Out: ar67Clean}}}}
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r2's sitting line", func() bool { return len(rowLines(w.ledger.Written(), "sitting", "r2")) == 1 })
	time.Sleep(10 * o.Poll)
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	n := ar65End(t, res)
	all := w.ledger.Written()
	if ps, wk := ar65Events(all, "pause"), ar65Events(all, "wake"); len(ps) != 1 || len(wk) != 1 || states(n) != "r1=green r2=green" {
		t.Fatalf("pause lines %v, wake lines %d, %s; want one each, both green", ps, len(wk), states(n))
	}
}
