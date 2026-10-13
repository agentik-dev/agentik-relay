package night_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// flipLoad reads 1 over one core until high is set, then 99: the plan
// measures under it, a spawn's breaker opens.
type flipLoad struct{ high atomic.Bool }

func (l *flipLoad) Load() (float64, int, error) {
	if l.high.Load() {
		return 99, 1, nil
	}
	return 1, 1, nil
}

// breakerOpen reports whether the ledger holds a `breaker` `open` line.
func breakerOpen(l *effects.FakeLedger) bool {
	for _, w := range l.Written() {
		if w["event"] == "breaker" && w["state"] == "open" {
			return true
		}
	}
	return false
}

// TestAR13_5_StopEndsBreakerWait (R6 §4): a `stop` or a `kill` while a
// row waits on the breaker (the clock never reaching its poll) ends the
// wait at once: stop, the row is skipped, never spawned, and the night
// drains to owner-stop; kill, the row fails owner-kill.
func TestAR13_5_StopEndsBreakerWait(t *testing.T) {
	for line, want := range map[string]struct {
		night relay.NightState
		row   string
	}{
		"stop":    {relay.Stopped(relay.OwnerStop), "r1=skipped"},
		"kill r1": {relay.Stopped(relay.OwnerKill), "r1=failed"},
	} {
		t.Run(line, func(t *testing.T) {
			w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
			load := &flipLoad{}
			w.fx.Load = load
			o := w.opts(t, by)
			row := o.Row
			o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
				load.high.Store(true) // the plan is measured: the spawn's breaker opens
				return row(r, rp)
			}
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 1000 && !breakerOpen(w.ledger); i++ {
					time.Sleep(10 * time.Millisecond)
				}
				night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, "2026-09-22-1"), line)
			}()
			done := make(chan *night.Night, 1)
			go func() { done <- runNight(t, w, q, o) }()
			var n *night.Night
			select {
			case n = <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("the night still waits on the breaker 10 s after the owner's " + line)
			}
			wg.Wait()
			if n.State != want.night || states(n) != want.row || count(w.ledger.Written(), "spawn") != 0 {
				t.Fatalf("night %s: %s, %d spawns", n.State, states(n), count(w.ledger.Written(), "spawn"))
			}
			if line == "kill r1" && n.Sittings["r1"].Reason != relay.OwnerKill {
				t.Fatalf("r1 %+v", n.Sittings["r1"])
			}
		})
	}
}

// TestAR13_5_SpawnsSpaced (ADV6-13): two rows at --parallel 2 under a
// quiet breaker: the second spawns only once the clock has moved
// SpawnSpacing (one breaker poll) past the first.
func TestAR13_5_SpawnsSpaced(t *testing.T) {
	if night.SpawnSpacing != night.DefaultBreaker.Poll {
		t.Fatalf("SpawnSpacing %s, want one breaker poll", night.SpawnSpacing)
	}
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	clock := &effects.StepClock{T: time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)}
	w.fx.Clock, w.fx.Load = clock, &flipLoad{}
	o := w.opts(t, by)
	o.Parallel = 2
	done := make(chan *night.Night, 1)
	go func() { done <- runNight(t, w, q, o) }()
	for i := 0; i < 1000 && count(w.ledger.Written(), "spawn") == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if n := count(w.ledger.Written(), "spawn"); n != 1 {
		t.Fatalf("%d spawns before the clock moved, want 1", n)
	}
	clock.Advance(night.SpawnSpacing - time.Second)
	time.Sleep(200 * time.Millisecond)
	if n := count(w.ledger.Written(), "spawn"); n != 1 {
		t.Fatalf("%d spawns a second short of SpawnSpacing, want 1", n)
	}
	clock.Advance(time.Second)
	var n *night.Night
	select {
	case n = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the second row never spawned")
	}
	if states(n) != "r1=green r2=green" || count(w.ledger.Written(), "spawn") != 2 {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
}
