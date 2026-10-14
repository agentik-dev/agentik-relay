package night_test

import (
	"fmt"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR80_11_StoppedNightEndsHolds (AR80 fix round, finding 1): a night
// that leaves running ends every unspawned row's wait: r1 failing
// no-commit while r2 holds, `kill r1` spawned while r2 holds, and `kill
// r1` held while r2 queues each end the night with the clock still, r2
// skipped owner-stop, never spawned and never asked again.
func TestAR80_11_StoppedNightEndsHolds(t *testing.T) {
	ns := []effects.FakeAnswer{{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}}
	for _, c := range []struct {
		name, line, holds string
		stall             bool
		r2asks            int
	}{
		{"no-commit", "", "r2", false, 1},
		{"kill spawned", "kill r1", "r2", true, 1},
		{"kill held", "kill r1", "r1", false, 0},
	} {
		band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{c.holds: ns}}
		w, q, o, sp, clock := ar80World(t, band, "r1", "r2")
		o.Parallel = 2
		sp.stall["r1"] = c.stall
		sp.wait["r1"] = func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) > 0 }
		res := ar71Run(w, q, o, sp)
		ar65Until(t, c.name+": r2's band line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) > 0 })
		if c.line != "" {
			if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), c.line); err != nil {
				t.Fatal(err)
			}
		}
		n := ar65End(t, res)
		all := w.ledger.Written()
		sit := ar80One(t, all, "sitting", "r2")
		asks := 0
		for _, a := range band.Asks() {
			if fmt.Sprint(a[len("2026-09-22-1/"):len("2026-09-22-1/r2")]) == "r2" {
				asks++
			}
		}
		if len(sp.spawns("r2")) != 0 || sit["state"] != "skipped" || sit["stop"] != "owner-stop" || asks != c.r2asks || !clock.Now().Equal(ar65T) {
			t.Fatalf("%s: night %s; r2 spawns %v, sitting %v/%v, asks %d (want %d), clock %s", c.name, n.State, sp.spawns("r2"), sit["state"], sit["stop"], asks, c.r2asks, clock.Now())
		}
	}
}
