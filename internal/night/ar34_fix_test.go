package night_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// onEvent is a ledger that calls on with each event's name before it
// appends the line.
type onEvent struct {
	effects.Ledger
	on func(event string)
}

func (l onEvent) Append(event string, m map[string]any) error {
	l.on(event)
	return l.Ledger.Append(event, m)
}

// TestAR34Fix_RereadNeverWatches is ar34 review F1: under --orchestrate the
// queue re-read's plan copied the first plan's options, Control and Groups
// with them, so a `kill b` the loop had read as a no-op (b not yet running)
// stopped the re-read's measure and killed the night's groups for good,
// and b, spawned after, died of SIGTERM. The re-read never watches.
func TestAR34Fix_RereadNeverWatches(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil)
	y := orchHead + orchRow + buildRow("b")
	sit(t, files, "o.1", 1, "- act 2", map[string]string{"queue.yaml": y,
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")})
	sit(t, files, "o.2", 2, "DONE: b is in", nil)
	o := w.orchOpts(t, q, files)
	w.fx.Clock = effects.SystemClock{} // the watch's poll is the machine's: 1 ms
	control := night.ControlPath(o.State, o.PlanOptions.Night)
	o.PlanOptions.Control, o.PlanOptions.Poll = control, time.Millisecond
	inner := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		if rp.ID == "o.1" { // after the open, b not yet in the queue: the owner says kill b
			if err := night.WriteControl(effects.OSFS{}, control, "kill b"); err != nil {
				t.Fatal(err)
			}
		}
		return inner(r, rp)
	}
	n := runNight(t, w, q, o)
	if c := count(w.ledger.Written(), "measure-stopped"); c != 0 {
		t.Errorf("%d measure-stopped lines; the re-read's measure is never the owner's to stop", c)
	}
	got := sittings(w)
	if n.Reason != relay.Done || len(got) != 3 || got[1] != "b=green" {
		t.Fatalf("night %s, sittings %v; want done, b green", n.State, got)
	}
}

// TestAR34Fix_StopBeforeFirstStatusIsRead is ar34 review F2 (adversary 1):
// a `stop` written after the open's read and before the first status.json
// (the verb then prints "before any row starts") was read only after the
// loop's first event, so r1 ran to its end. The loop reads it before its
// first pick.
func TestAR34Fix_StopBeforeFirstStatusIsRead(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	control := night.ControlPath(o.State, o.PlanOptions.Night)
	status := filepath.Join(night.NightDir(o.State, o.PlanOptions.Night), "status.json")
	statusThen := true
	w.fx.Ledger = onEvent{Ledger: w.ledger, on: func(e string) {
		if e == "night-open" {
			_, err := os.Stat(status)
			statusThen = err == nil
			if err := night.WriteControl(effects.OSFS{}, control, "stop"); err != nil {
				t.Fatal(err)
			}
		}
	}}
	n := runNight(t, w, q, o)
	if statusThen {
		t.Fatal("status.json existed at night-open: the window this test writes in is gone")
	}
	lines := w.ledger.Written()
	if n.State != relay.Stopped(relay.OwnerStop) || states(n) != "r1=skipped" || count(lines, "spawn") != 0 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:owner-stop, r1 skipped, none", n.State, states(n), count(lines, "spawn"))
	}
	if why := skipReason(w, "r1"); why != "the night stopped: owner-stop" {
		t.Fatalf("r1's skip reason %q", why)
	}
	if s := lastOf(lines, "stop"); s == nil || s["detail"] != "said before the first row: stop" {
		t.Fatalf("the stop line %v", s)
	}
}

// TestAR34Fix_KillBeforeFirstStatusIsRead: the same window for `kill r1`
// (the verb said "before any row starts" for a kill too): owner-kill by act
// 1's rule, r1 skipped, never failed.
func TestAR34Fix_KillBeforeFirstStatusIsRead(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	control := night.ControlPath(o.State, o.PlanOptions.Night)
	w.fx.Ledger = onEvent{Ledger: w.ledger, on: func(e string) {
		if e == "night-open" {
			if err := night.WriteControl(effects.OSFS{}, control, "kill r1"); err != nil {
				t.Fatal(err)
			}
		}
	}}
	n := runNight(t, w, q, o)
	lines := w.ledger.Written()
	if n.State != relay.Stopped(relay.OwnerKill) || states(n) != "r1=skipped" || count(lines, "spawn") != 0 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:owner-kill, r1 skipped, none", n.State, states(n), count(lines, "spawn"))
	}
}
