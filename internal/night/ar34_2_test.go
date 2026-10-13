package night_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// cutLine is TestAR34_2's shell: `echo ok` on main in root adds a group of
// its own to reg, writes the owner's `stop` to the night's control file and
// then runs until its group is signalled (minutes on the fake clock, which
// only the test moves; 3 s of the machine's at most). Every other line is
// the machine's.
type cutLine struct {
	root, control string
	reg           *effects.Groups
	started       chan struct{}
	sigs          chan os.Signal
}

func (c *cutLine) Run(dir, line string, timeout time.Duration) (int, string, error) {
	if dir != c.root || line != "echo ok" {
		return effects.OSShell{}.Run(dir, line, timeout)
	}
	const pgid = 1 << 30 // no process's: the signal is the fake's
	term := make(chan struct{})
	var once sync.Once
	c.reg.Add(effects.Group{Pgid: pgid}, func(s os.Signal) error {
		select {
		case c.sigs <- s:
		default:
		}
		if s == syscall.SIGTERM {
			once.Do(func() { close(term) })
		}
		return nil
	})
	defer c.reg.Done(pgid)
	os.MkdirAll(filepath.Dir(c.control), 0o700)
	night.WriteControl(effects.OSFS{}, c.control, "stop")
	close(c.started)
	select {
	case <-term:
		return -1, "", nil // ended by its signal
	case <-time.After(3 * time.Second):
		return 0, "ok", nil // never signalled: it ran out
	}
}

// TestAR34_2_StopCutsTheMeasure: under run, a `stop` written as the row's
// acceptance line starts on main ends the plan within one poll of the fake
// clock: the line's group gets SIGTERM, its check reads unrun: owner-stop,
// the ledger holds a `measure-stopped` line and plan lines cut by the
// owner, and the night's open stops it owner-stop with no spawn.
func TestAR34_2_StopCutsTheMeasure(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	t0 := time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)
	clock, reg := &effects.StepClock{T: t0}, &effects.Groups{}
	o := w.opts(t, by)
	control := night.ControlPath(o.State, o.PlanOptions.Night)
	line := &cutLine{root: w.root, control: control, reg: reg, started: make(chan struct{}), sigs: make(chan os.Signal, 4)}
	w.fx.Clock, w.fx.Groups, w.fx.Shell = clock, reg, line
	const poll = 2 * time.Second
	o.PlanOptions.Control, o.PlanOptions.Poll = control, poll

	type result struct {
		n   *night.Night
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := night.Run(w.fx, effects.OSSpawner{}, q, o)
		done <- result{n, err}
	}()
	select {
	case <-line.started:
	case <-time.After(3 * time.Second):
		t.Fatal("the row's acceptance line never started on main")
	}
	clock.Advance(poll) // one poll
	var r result
	select {
	case r = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the plan still measures one poll after the owner's stop")
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	n, lines := r.n, w.ledger.Written()
	if got := <-line.sigs; got != syscall.SIGTERM {
		t.Fatalf("the line's group got %v first; want SIGTERM", got)
	}
	if !clock.Now().Equal(t0.Add(poll)) {
		t.Fatalf("the fake clock reads %v; the plan ended past one poll", clock.Now())
	}
	if n.State != relay.Stopped(relay.OwnerStop) || states(n) != "r1=skipped" || count(lines, "spawn") != 0 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:owner-stop, r1 skipped, none", n.State, states(n), count(lines, "spawn"))
	}
	if c := n.Plan.Rows[0].Baseline[0]; c.Status != relay.CheckUnrun || c.MainLast != "owner-stop" {
		t.Fatalf("the cut line's check %+v; want unrun: owner-stop", c)
	}
	ms := lastOf(lines, "measure-stopped")
	if ms == nil || ms["reason"] != "owner-stop" || ms["said"] != "stop" || fmt.Sprint(ms["unrun"]) != "1" {
		t.Fatalf("measure-stopped = %v", ms)
	}
	if pl := lastOf(lines, "plan"); pl == nil || pl["stopped"] != "owner-stop" {
		t.Fatalf("plan line = %v; want stopped owner-stop", pl)
	}
	if why := skipReason(w, "r1"); why != "the night stopped: owner-stop" {
		t.Fatalf("r1's skip reason %q", why)
	}
}
