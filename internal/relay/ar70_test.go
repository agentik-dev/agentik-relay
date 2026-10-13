package relay

import (
	"errors"
	"testing"
)

// TestAR70_1_PausedVerdict: a sibling killed in its gate lands its gate's
// verdict while paused (AR70 ruling 1): paused takes `gate-green` to green,
// `gate-red` and `gate-unrun` to red and `gate-failed` to failed, as exited
// does; `spawn` and `halt` stand; every other event is refused, the state
// kept.
func TestAR70_1_PausedVerdict(t *testing.T) {
	pairs := map[Event]State{
		GateGreen:  Green,
		GateRed:    Red,
		GateUnrun:  Red,
		GateFailed: Failed,
		Spawn:      Running,
		Halt:       Failed,
	}
	for e, want := range pairs {
		if got, err := Step(Paused, e); got != want || err != nil {
			t.Errorf("Step(paused, %s) = %q, %v; want %q", e, got, err, want)
		}
		if e == Spawn || e == Halt {
			continue
		}
		if got, err := Step(Exited, e); got != want || err != nil {
			t.Errorf("Step(exited, %s) = %q, %v; want %q, as paused", e, got, err, want)
		}
	}
	for _, e := range Events {
		if _, ok := pairs[e]; ok {
			continue
		}
		if got, err := Step(Paused, e); got != Paused || !errors.Is(err, ErrIllegal) {
			t.Errorf("Step(paused, %s) = %q, %v; want paused, refused", e, got, err)
		}
	}
}
