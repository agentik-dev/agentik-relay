package night_test

import (
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// leapClock is a StepClock whose waits of at most leap pass at once (the
// watchdog's polls), so a stall of 2700 s is read in a blink.
type leapClock struct {
	*effects.StepClock
	leap time.Duration
}

func (c leapClock) After(d time.Duration) <-chan time.Time {
	ch := c.StepClock.After(d)
	if d > 0 && d <= c.leap {
		c.StepClock.Advance(d)
	}
	return ch
}

// stallSpawner starts children that write nothing and live until the
// watchdog's KillGroup reaches their group: then the stream closes and
// Wait is 137.
type stallSpawner struct {
	mu    sync.Mutex
	next  int
	procs map[int]*stallProc
}

type stallProc struct {
	pid  int
	w    *io.PipeWriter
	gone chan struct{}
	once sync.Once
}

func (s *stallSpawner) Start(effects.Spawn) (effects.Process, io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, w := io.Pipe()
	s.next++
	p := &stallProc{pid: 5000 + s.next, w: w, gone: make(chan struct{})}
	s.procs[p.pid] = p
	return p, r, nil
}

func (s *stallSpawner) kill(pgid int) {
	s.mu.Lock()
	p := s.procs[pgid]
	s.mu.Unlock()
	if p != nil {
		p.once.Do(func() { p.w.Close(); close(p.gone) })
	}
}

func (p *stallProc) Signal(os.Signal) error { return nil }
func (p *stallProc) Wait() (int, error)     { <-p.gone; return 137, nil }
func (p *stallProc) Pid() int               { return p.pid }

// TestAR13_3_SecondStallStopsNight (R6 §2): a stalled row is red and the
// night moves on, but a second in one night stops it `stopped:stalled-twice`
// (relay.StalledTwice); the third row never runs.
func TestAR13_3_SecondStallStopsNight(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"},
		spec{id: "r3", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	o.Parallel = 1
	clock := leapClock{&effects.StepClock{T: time.Date(2026, 9, 23, 23, 0, 0, 0, time.UTC)}, night.WatchdogPoll}
	sp := &stallSpawner{procs: map[int]*stallProc{}}
	w.fx.Clock = clock
	w.fx.Procs = &effects.FakeProcs{Mtime: clock.Now().Add(-time.Hour), CPU: []float64{5}, OnKill: sp.kill}
	n, err := night.Run(w.fx, sp, q, o)
	if err != nil {
		t.Fatalf("night: %v", err)
	}
	r1, r2 := n.Sittings["r1"], n.Sittings["r2"]
	if n.State != relay.Stopped(relay.StalledTwice) || relay.StalledTwice != "stalled-twice" ||
		r1.State != relay.Red || r1.Reason != relay.Stalled || r2.State != relay.Red || r2.Reason != relay.Stalled ||
		(n.Sittings["r3"] != nil && n.Sittings["r3"].State == relay.Red) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	stops := 0
	for _, l := range w.ledger.Written() {
		if l["event"] == "stop" {
			stops++
			if l["reason"] != "stalled-twice" {
				t.Fatalf("stop line %v", l)
			}
		}
	}
	if stops != 1 {
		t.Fatalf("%d stop lines", stops)
	}
}
