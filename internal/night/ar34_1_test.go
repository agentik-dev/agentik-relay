package night_test

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// saidDuringPlan builds r1's plan, then writes line to the night's control
// file with its modification time at the plan's start plus age (the owner's
// `agentik-relay stop` or `agentik-relay kill` during the measure, AR34 act 1), and returns
// the night's options holding the plan and the control file's path.
func saidDuringPlan(t *testing.T, line string, age time.Duration) (*world, *queue.Queue, night.NightOptions, string) {
	t.Helper()
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	po := o.PlanOptions
	po.State = o.State
	p, err := night.BuildPlan(w.fx, q, po)
	if err != nil {
		t.Fatal(err)
	}
	o.Plan = p
	control := night.ControlPath(o.State, p.Night)
	if err := os.MkdirAll(night.NightDir(o.State, p.Night), 0o700); err != nil { // the trap made it during the measure
		t.Fatal(err)
	}
	if err := night.WriteControl(effects.OSFS{}, control, line); err != nil {
		t.Fatal(err)
	}
	at := w.fx.Clock.Now().Add(age)
	if err := os.Chtimes(control, at, at); err != nil {
		t.Fatal(err)
	}
	return w, q, o, control
}

// skipReason is the reason of row's `skip` line, "" when none.
func skipReason(w *world, row string) string {
	for _, l := range w.ledger.Written() {
		if l["event"] == "skip" && l["row"] == row {
			r, _ := l["reason"].(string)
			return r
		}
	}
	return ""
}

// TestAR34_1_StopBeforeOpenIsKept: a `stop` written after the plan's start
// and before the open is read, not removed: the night stops owner-stop,
// nothing spawns, and r1 is skipped with the night's reason.
func TestAR34_1_StopBeforeOpenIsKept(t *testing.T) {
	w, q, o, _ := saidDuringPlan(t, "stop", time.Minute)
	n := runNight(t, w, q, o)
	lines := w.ledger.Written()
	if n.State != relay.Stopped(relay.OwnerStop) || states(n) != "r1=skipped" || count(lines, "spawn") != 0 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:owner-stop, r1 skipped, none", n.State, states(n), count(lines, "spawn"))
	}
	if why := skipReason(w, "r1"); why != "the night stopped: owner-stop" {
		t.Fatalf("r1's skip reason %q", why)
	}
	if !night.Clean(n) || count(lines, "owner-stop") != 1 {
		t.Fatalf("clean %v, %d owner-stop lines", night.Clean(n), count(lines, "owner-stop"))
	}
}

// TestAR34_1_KillBeforeOpenIsKept: `kill r1` said during the plan stops the
// night owner-kill at its open; r1 never ran, so it is skipped, not failed
// (the day's refusal of a red row never fires on it).
func TestAR34_1_KillBeforeOpenIsKept(t *testing.T) {
	w, q, o, _ := saidDuringPlan(t, "kill r1", time.Minute)
	n := runNight(t, w, q, o)
	lines := w.ledger.Written()
	if n.State != relay.Stopped(relay.OwnerKill) || states(n) != "r1=skipped" || count(lines, "spawn") != 0 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:owner-kill, r1 skipped, none", n.State, states(n), count(lines, "spawn"))
	}
	if why := skipReason(w, "r1"); why != "the night stopped: owner-kill" {
		t.Fatalf("r1's skip reason %q", why)
	}
	if s := n.Sittings["r1"]; s.State == relay.Failed || s.State == relay.Red {
		t.Fatalf("r1 %+v", s)
	}
}

// TestAR34_1_OldControlIsRemoved: a control file older than the plan's
// start is a leftover: the open removes it and the night runs.
func TestAR34_1_OldControlIsRemoved(t *testing.T) {
	w, q, o, control := saidDuringPlan(t, "stop", -time.Hour)
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green" || count(w.ledger.Written(), "spawn") != 1 {
		t.Fatalf("night %s, %s; want stopped:queue-empty, r1 green", n.State, states(n))
	}
	if _, err := os.Stat(control); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the leftover control file: %v; want removed", err)
	}
}
