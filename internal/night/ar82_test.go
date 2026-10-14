package night_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The kinds a refused event names, through a night (AR82 rulings 2, 3):
// a family's weekly cap pauses only that family's rows; an overage limit
// stops the night at once.

// TestAR82_3_FamilyWeekly (AR82 ruling 2; the ledger's entry 12): TestAR71's
// world with r1 on claude-opus-5-5, r2 and r3 on claude-fable-5-1, at
// --parallel 2. r1's part 1 is a refused seven_day_opus event resetting at
// 02:00, exit 143: one pause line [r1], kind model, scope opus, wake 02:02,
// and no kill line. Before the clock moves r2 runs on to green, then r3
// spawns after r2's sitting line and goes green. At 02:02 one wake line
// [r1], r1's part 2, all green, queue-empty, 4 spawns.
func TestAR82_3_FamilyWeekly(t *testing.T) {
	w, q, o, sp, clock := ar71World(t, nil)
	q.Rows[0].Model = "claude-opus-5-5"
	q.Rows[1].Model, q.Rows[2].Model = "claude-fable-5-1", "claude-fable-5-1"
	sp.first["r1"] = effects.FakeRun{Out: ar65Event("seven_day_opus", ar65T.Add(time.Hour)), Exit: 143}
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "the pause line", func() bool { return len(ar65Events(w.ledger.Written(), "pause")) > 0 })
	ar71Pause(t, ar65Events(w.ledger.Written(), "pause")[0], "[r1]", "model", "opus", "2026-10-11T02:02:00Z", 1)
	ar65Until(t, "r3's sitting line, the night paused", func() bool {
		return len(rowLines(w.ledger.Written(), "sitting", "r3")) == 1 && ar65Paused(o, "r1")
	})
	all := w.ledger.Written()
	if ps := ar65Events(all, "pause"); len(ps) != 1 || len(ar65Events(all, "kill")) != 0 || !clock.Now().Equal(ar65T) {
		t.Fatalf("%d pause lines, kill lines %v, clock %s; want one, none, unmoved", len(ps), ar65Events(all, "kill"), clock.Now())
	}
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

// TestAR82_4_OverageStops (AR82 ruling 3): TestAR65's world, r1's child a
// refused overage event resetting an hour on, no --until: the night stops
// usage-limit at once, r1 failed and r2 skipped, no pause line, the stop
// line's detail naming the overage limit, and r1's exit line's limit kind
// overage.
func TestAR82_4_OverageStops(t *testing.T) {
	w, q, o, sp, _ := ar65World(t, ar65Event("overage", ar65T.Add(time.Hour)), "r1", "r2")
	res := ar65Start(w, q, o, sp)
	ar65Until(t, "a stop or a pause line", func() bool {
		all := w.ledger.Written()
		return len(ar65Events(all, "stop")) > 0 || len(ar65Events(all, "pause")) > 0
	})
	if ps := ar65Events(w.ledger.Written(), "pause"); len(ps) != 0 {
		t.Fatalf("pause lines %v; want none, the night stopped at once", ps)
	}
	n := ar65End(t, res)
	all := w.ledger.Written()
	if n.State != relay.Stopped(relay.UsageLimit) || states(n) != "r1=failed r2=skipped" {
		t.Fatalf("night %s, %s; want stopped:usage-limit, r1=failed r2=skipped", n.State, states(n))
	}
	if d := lastOf(all, "stop")["detail"]; d != "the overage limit" {
		t.Errorf("the stop line's detail %q, want %q", d, "the overage limit")
	}
	ex := rowLines(all, "exit", "r1")
	if len(ex) != 1 {
		t.Fatalf("r1's exit lines %v, want one", ex)
	}
	if lm, _ := ex[0]["limit"].(map[string]any); lm == nil || lm["kind"] != "overage" || lm["scope"] != "" {
		t.Errorf("r1's exit line's limit %v; want kind overage, scope \"\"", ex[0]["limit"])
	}
}
