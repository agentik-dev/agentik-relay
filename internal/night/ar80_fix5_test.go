package night_test

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR80_15_ResetUnaskedNoKey (AR80 fix round, finding 5; ruling 7): r1
// held, r2 queued behind it, both at the reset +150 s: r2 never asked, so
// its spawn and sitting lines carry no band key; its band lines still
// mark the hold, 0 asks over 150 s.
func TestAR80_15_ResetUnaskedNoKey(t *testing.T) {
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}}},
		ResetAt: ar65T.Add(150 * time.Second)}
	w, q, o, _, clock := ar80World(t, band, "r1", "r2")
	o.Parallel = 2
	res := ar71Run(w, q, o, ar80Green(w))
	ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
	ar80Ask(t, band, 1, clock, 60*time.Second)
	ar80Ask(t, band, 2, clock, 120*time.Second)
	ar80Ask(t, band, 3, clock, 150*time.Second)
	ar65End(t, res)
	all := w.ledger.Written()
	_, spawnKey := ar80One(t, all, "spawn", "r2")["band"]
	_, sitKey := ar80One(t, all, "sitting", "r2")["band"]
	if got := ar80Band(t, all, "r2"); got != "held queued; reset 0/150s" || spawnKey || sitKey {
		t.Fatalf("r2's band lines %s, the spawn's band key %v, the sitting's %v", got, spawnKey, sitKey)
	}
}
