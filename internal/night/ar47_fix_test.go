package night

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// stopIdle answers as fakeIdle does and, during its read number at (from
// 1), calls stop: the owner's stop landing while `top` samples.
type stopIdle struct {
	series []float64
	at     int
	stop   func()
	n      int
}

func (s *stopIdle) Idle() (float64, error) {
	s.n++
	if s.n == s.at && s.stop != nil {
		s.stop()
	}
	return s.series[min(s.n, len(s.series))-1], nil
}

// TestAR47Fix_StopDuringIdleRead (AR47 ruling 6; AR34 fix, F2): the idle
// read before a line takes a second or more, and the owner's stop can
// land in it. The stop is the owner's: the line never starts and its
// error is `owner-stop`, with a budget (never `budget`, the label
// TestAR34Fix_StopDuringRootLockIsOwnerStop forbids) and with none.
func TestAR47Fix_StopDuringIdleRead(t *testing.T) {
	for _, c := range []struct {
		name   string
		budget time.Duration
		load   float64 // 40: the bounded shell's own read before the line; 70: the wait's
	}{
		{"with a budget", time.Hour, 40},
		{"with no budget", 0, 40},
		{"the wait's read, with a budget", time.Hour, 70},
		{"the wait's read, with no budget", 0, 70},
	} {
		_, _, fx, _, sh := memoPlan(t)
		idle := &stopIdle{series: []float64{0.70}, at: 1}
		fx.Load, fx.Idle = &effects.FakeLoad{Series: []float64{c.load}}, idle
		b := newBound(fx, "/state", "/root")
		idle.stop = func() { b.stop("owner-stop", "stop") }
		if c.budget > 0 {
			b.bound(c.budget)
		}
		_, _, err := b.Run("/root", "make check", time.Minute)
		if err == nil || err.Error() != "owner-stop" || len(sh.Calls()) != 0 || idle.n != 1 {
			t.Errorf("%s: a stop during the idle read: error %v, %d shell calls, %d idle reads; want owner-stop and no line started", c.name, err, len(sh.Calls()), idle.n)
			continue
		}
		if ran, _ := b.counts(); ran != 0 {
			t.Fatalf("%s: %d lines ran after the stop", c.name, ran)
		}
		if _, stored := LoadMemo(fx.FS, "/state").Get("/root", "aaa111", "make check"); stored || b.lastLoaded() {
			t.Fatalf("%s: a line never run is stored (%v) or loaded (%v)", c.name, stored, b.lastLoaded())
		}
		// And nothing starts after it.
		if _, _, err := b.Run("/root", "make check", time.Minute); err == nil || err.Error() != "owner-stop" || len(sh.Calls()) != 0 {
			t.Fatalf("%s: the next line after the stop: %v, %d shell calls", c.name, err, len(sh.Calls()))
		}
	}
}
