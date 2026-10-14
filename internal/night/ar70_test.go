package night_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// A sibling killed in its gate lands its verdict (AR70): nights on
// TestAR67's world, every wait bounded.

// ar70Row is row id's state in the night's status.
func ar70Row(o night.NightOptions, id string) string {
	for _, r := range night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night).Rows {
		if r.ID == id {
			return r.State
		}
	}
	return ""
}

// ar70Gated is TestAR67_5's night: at --parallel 2 r2's gate lets r1's
// stream go, then holds until the ledger has an `event` line (10 s at
// most).
func ar70Gated(t *testing.T, w *world, o *night.NightOptions, release chan struct{}, event string) {
	t.Helper()
	var once sync.Once
	row := o.Row
	o.Parallel = 2
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := row(r, rp)
		if g := ro.Gate; r.ID == "r2" {
			ro.Gate = func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
				once.Do(func() { close(release) })
				for end := time.Now().Add(10 * time.Second); len(ar65Events(w.ledger.Written(), event)) == 0 && time.Now().Before(end); {
					time.Sleep(5 * time.Millisecond)
				}
				return g(st, seen, isError)
			}
		}
		return b, ro, err
	}
}

// at is the index of the first line of event for row ("": any row) in
// lines, -1 when none.
func ar70At(lines []map[string]any, event, row string) int {
	for i, l := range lines {
		if l["event"] == event && (row == "" || l["row"] == row) {
			return i
		}
	}
	return -1
}

// TestAR70_2_GateOutAtTheWake (AR70 ruling 5): TestAR67_5's r2 gate held
// until the `wake` line, so r2's outcome is still out at the wake: that
// line names r1 and r2, only r1 has a part 2, and r2 lands green after
// it, its stage unlocked; queue-empty, 3 spawns. The probe stays armed:
// on the fallback, r2's verdict landing after the wake is a part spawned
// before it, so r1's part 2 refused again with no reset stops the night.
func TestAR70_2_GateOutAtTheWake(t *testing.T) {
	t.Run("the wake skips it", func(t *testing.T) {
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		release := make(chan struct{})
		sp := ar70New(release, nil)
		ar70Gated(t, w, &o, release, "wake")
		res := ar70Night(w, q, o, sp)
		ar65Until(t, "the pause, r1 and r2 paused", func() bool { return ar65Paused(o, "r1", "r2") })
		clock.Advance(time.Hour + 2*time.Minute)
		n := ar65End(t, res)
		all := w.ledger.Written()
		wk := ar65Events(all, "wake")
		if len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[r1 r2]" {
			t.Fatalf("the wake lines %v; want one, [r1 r2]", wk)
		}
		if len(sp.r1.Spawns()) != 2 || sp.spawns() != 3 || len(rowLines(all, "spawn", "r2")) != 1 {
			t.Fatalf("spawns: r1 %d, r2 %d, all %d, the stop %q; want a part 2 for r1 only (2, 1, 3)",
				len(sp.r1.Spawns()), len(rowLines(all, "spawn", "r2")), sp.spawns(), lastOf(all, "stop")["detail"])
		}
		sl := rowLines(all, "sitting", "r2")
		if len(sl) != 1 || sl[0]["state"] != "green" || ar70At(all, "sitting", "r2") < ar70At(all, "wake", "") {
			t.Fatalf("r2's sitting lines %v (at %d, the wake at %d); want one, green, after the wake", sl, ar70At(all, "sitting", "r2"), ar70At(all, "wake", ""))
		}
		if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" {
			t.Fatalf("night %s, %s; want stopped:queue-empty, both green", n.State, states(n))
		}
		if night.Locked(ar67Tree(w, "r1")) || night.Locked(ar67Tree(w, "r2")) {
			t.Fatalf("a tree still locked")
		}
	})
	t.Run("the probe stays armed", func(t *testing.T) {
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		release := make(chan struct{})
		sp := ar70New(release, nil)
		bare := effects.FakeRun{Out: ar65Result("API Error: 429"), Exit: 143}
		sp.r1 = &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{bare, bare}}
		sp.hold2 = make(chan struct{})
		ar70Gated(t, w, &o, release, "wake")
		res := ar70Night(w, q, o, sp)
		ar65Until(t, "the fallback pause, r1 and r2 paused", func() bool { return ar65Paused(o, "r1", "r2") })
		clock.Advance(30 * time.Minute)
		ar65Until(t, "r2 green after the wake", func() bool { return ar70Row(o, "r2") == "green" })
		close(sp.hold2)
		ar65Until(t, "a stop or a second pause", func() bool {
			all := w.ledger.Written()
			return len(ar65Events(all, "pause")) == 2 || len(ar65Events(all, "stop")) > 0
		})
		if p := ar65Events(w.ledger.Written(), "pause"); len(p) != 1 {
			clock.Advance(time.Hour) // let the second pause wake and end
			ar65End(t, res)
			t.Fatalf("pause lines %v; want one: r2's verdict, from before the wake, passed the probe", p)
		}
		n := ar65End(t, res)
		ar69Stopped(t, w, n, "refused again after the 30-minute wait, with no reset")
		if states(n) != "r1=failed r2=green" {
			t.Fatalf("%s; want r1 failed, r2 green", states(n))
		}
	})
	// Fix round one, F1: TestAR69_6's spawner, r2's stream silent until
	// the `wake` line, then its own refused five_hour event at T+2h. Its
	// held line lands on a running night: no `pause` line after the wake,
	// and the night stops usage-limit naming r2, as main ends.
	t.Run("its own limit lands on the running night", func(t *testing.T) {
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		o.Parallel = 2
		sp := &ar69Spawner{own: effects.FakeRun{Out: ar65Event("five_hour", ar65T.Add(2*time.Hour)), Exit: 143}, started: make(chan struct{}), n: map[string]int{},
			paused: func() bool { return len(ar65Events(w.ledger.Written(), "wake")) > 0 },
			r1:     &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{{Out: ar67Limited, Exit: 143}, {Out: ar67Clean}}}}
		res := ar70Night(w, q, o, sp)
		ar65Until(t, "the pause, r1 and r2 paused", func() bool { return ar65Paused(o, "r1", "r2") })
		clock.Advance(time.Hour + 2*time.Minute)
		n := ar65End(t, res)
		all := w.ledger.Written()
		at := ar70At(all, "wake", "")
		if at < 0 || len(ar65Events(all, "wake")) != 1 {
			t.Fatalf("the wake lines %v; want one", ar65Events(all, "wake"))
		}
		if p := ar65Events(all[at:], "pause"); len(p) != 0 {
			t.Fatalf("pause lines after the wake %v, night %s; want none on a running night", p, n.State)
		}
		stop := lastOf(all, "stop")
		if n.State != relay.Stopped(relay.UsageLimit) || !strings.Contains(fmt.Sprint(stop["resume"]), "agentik relay resume r2") {
			t.Fatalf("night %s, resume %v; want stopped:usage-limit naming r2", n.State, stop["resume"])
		}
		tree := ar67Tree(w, "r2")
		if s := n.Sittings["r2"]; s.State != relay.Failed || s.Reason != relay.UsageLimit || night.Locked(tree) {
			t.Fatalf("r2 %s %s, locked %v; want failed usage-limit, unlocked", s.State, s.Reason, night.Locked(tree))
		}
		if _, err := os.Stat(tree); err != nil {
			t.Fatalf("r2's tree is gone: %v", err)
		}
		sl := rowLines(all, "sitting", "r2")
		if len(sl) != 1 || sl[0]["limit"] == "paused" || ar70At(all, "sitting", "r2") < at || len(rowLines(all, "spawn", "r2")) != 1 {
			t.Fatalf("r2's sitting lines %v (at %d, the wake at %d), %d spawns; want one after the wake, not limit paused, one spawn",
				sl, ar70At(all, "sitting", "r2"), at, len(rowLines(all, "spawn", "r2")))
		}
	})
}

// ar70Spawner is TestAR67's with r2's first child changed: with last, a
// stall whose stream at its kill says last; else a clean result after a
// commit of a.txt in its tree (the gate wants a commit).
type ar70Spawner struct {
	*ar67Spawner
	last []byte
}

func ar70New(release chan struct{}, last []byte) *ar70Spawner {
	return &ar70Spawner{ar67Spawner: ar67New(last != nil, release), last: last}
}

func (a *ar70Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	a.mu.Lock()
	first := filepath.Base(sp.Dir) == "r2" && a.starts["r2"] == 0
	if first && a.last != nil {
		a.n++
		a.starts["r2"]++
	}
	a.mu.Unlock()
	switch {
	case !first:
		return a.ar67Spawner.Start(sp)
	case a.last != nil:
		close(a.release) // r2 has its child: r1's stream goes
		s := &ar70Stall{ar65Stall: ar65Stall{gone: make(chan struct{})}, out: bytes.NewReader(a.last)}
		return s, s, nil
	}
	os.WriteFile(filepath.Join(sp.Dir, "a.txt"), []byte("r2 part 1\n"), 0o644)
	if out, err := exec.Command("git", "-C", sp.Dir, "commit", "-qam", "part 1").CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("commit: %v: %s", err, out)
	}
	return a.ar67Spawner.Start(sp)
}

// ar70Night runs the night on sp in the background.
func ar70Night(w *world, q *queue.Queue, o night.NightOptions, sp effects.Spawner) chan *night.Night {
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

// ar70Stall says nothing until its kill, then out.
type ar70Stall struct {
	ar65Stall
	out *bytes.Reader
}

func (s *ar70Stall) Read(p []byte) (int, error) { <-s.gone; return s.out.Read(p) }

// TestAR70_3_SiblingStops (AR70 rulings 3, 4): r2, a stall the pause
// kills, the clock never moved. Its stream at the kill an is_error result
// `API Error: 401`: the night stops auth, r2 failed auth. Its Deps
// unsatisfied (`zz`): the night stops invariant, r2's one sitting line
// invariant with no `limit`. Each: r1 failed usage-limit by the halt, both
// stages unlocked, both trees kept, no `wake` line.
func TestAR70_3_SiblingStops(t *testing.T) {
	for _, c := range []struct {
		name string
		want relay.StopReason
	}{{"auth", relay.Auth}, {"invariant", relay.Invariant}} {
		t.Run(c.name, func(t *testing.T) {
			w, q, o, _, _ := ar65World(t, nil, "r1", "r2")
			release := make(chan struct{})
			var sp effects.Spawner
			o.Parallel = 2
			if c.want == relay.Auth {
				sp = ar70New(release, ar65Result("API Error: 401 authentication_error · Please run /login"))
			} else {
				sp = ar67New(true, release)
				row := o.Row
				o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
					b, ro, err := row(r, rp)
					if r.ID == "r2" {
						ro.Deps = map[string]bool{"zz": false}
					}
					return b, ro, err
				}
			}
			n := ar65End(t, ar70Night(w, q, o, sp))
			all := w.ledger.Written()
			if n.State != relay.Stopped(c.want) || len(ar65Events(all, "wake")) != 0 {
				t.Fatalf("night %s, %d wake lines; want stopped:%s, none", n.State, len(ar65Events(all, "wake")), c.want)
			}
			if s := n.Sittings["r2"]; s.State != relay.Failed || s.Reason != c.want {
				t.Fatalf("r2 %s %s; want failed %s", s.State, s.Reason, c.want)
			}
			if s := n.Sittings["r1"]; s.State != relay.Failed || s.Reason != relay.UsageLimit {
				t.Fatalf("r1 %s %s; want failed usage-limit", s.State, s.Reason)
			}
			if c.want == relay.Invariant {
				if sl := rowLines(all, "sitting", "r2"); len(sl) != 1 || sl[0]["reason"] != "invariant" || sl[0]["limit"] != nil {
					t.Fatalf("r2's sitting lines %v; want one, invariant, no limit", sl)
				}
			}
			for _, id := range []string{"r1", "r2"} {
				tree := ar67Tree(w, id)
				if night.Locked(tree) {
					t.Fatalf("%s's stage still locked", id)
				}
				if _, err := os.Stat(tree); err != nil {
					t.Fatalf("%s's tree is gone: %v", id, err)
				}
			}
		})
	}
}

// TestAR70_4_SpawnedApart (usage-limit-resume r1 §4 test 4, "both
// continue, spawned apart"): TestAR67_3's world under a quiet breaker:
// r2's first part spawns SpawnSpacing after r1's; after the wake one part
// 2 spawns, the second SpawnSpacing later and not a second sooner; both
// green, queue-empty.
func TestAR70_4_SpawnedApart(t *testing.T) {
	w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
	w.fx.Load = &flipLoad{}
	sp := ar67New(true, make(chan struct{}))
	o.Parallel = 2
	res := ar67Night(w, q, o, sp)
	ar65Until(t, "r1's spawn", func() bool { return sp.spawns() == 1 })
	clock.Advance(night.SpawnSpacing)
	ar65Until(t, "the pause, both paused", func() bool {
		return len(rowLines(w.ledger.Written(), "sitting", "r2")) == 1 && ar65Paused(o, "r1", "r2")
	})
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	ar65Until(t, "one part 2's spawn", func() bool { return sp.spawns() == 3 })
	time.Sleep(300 * time.Millisecond)
	if n := sp.spawns(); n != 3 {
		t.Fatalf("%d spawns before the clock moved past the wake, want 3", n)
	}
	clock.Advance(night.SpawnSpacing - time.Second)
	time.Sleep(300 * time.Millisecond)
	if n := sp.spawns(); n != 3 {
		t.Fatalf("%d spawns a second short of SpawnSpacing, want 3", n)
	}
	clock.Advance(time.Second)
	n := ar65End(t, res)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" || sp.spawns() != 4 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:queue-empty, both green, 4", n.State, states(n), sp.spawns())
	}
}
