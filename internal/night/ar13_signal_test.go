package night_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR13_2_SignalReapsGroups (ADV6-2): SIGTERM to the relay, delivered
// through the night's signal channel while a spawned child and a measured
// line (both deaf to TERM) run: both groups are in groups.json, both are
// gone within KillGrace, the night stops owner-kill with the signal as its
// detail, the sitting line is written and groups.json is cleared.
func TestAR13_2_SignalReapsGroups(t *testing.T) {
	pids := t.TempDir() // the child's pid file: written once it ignores TERM
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl",
		env: []string{"FAKECLAUDE_DELAY_MS=20000", "FAKECLAUDE_IGNORE_TERM=1", "FAKECLAUDE_PIDFILE=" + pids}},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	night.KillGrace = 300 * time.Millisecond
	groups := &effects.Groups{}
	w.fx.Groups, w.fx.Shell, w.fx.Procs = groups, effects.OSShell{Groups: groups}, effects.OSProcs{}
	o := w.opts(t, by)
	sigs := make(chan os.Signal, 1)
	o.Signals = sigs
	file := filepath.Join(night.NightDir(o.State, "2026-09-22-1"), "groups.json")

	lineExit := make(chan int, 1)
	var seen []effects.Group
	type doc struct{ Groups []effects.Group }
	go func() {
		for i := 0; i < 500 && count(w.ledger.Written(), "spawn") == 0; i++ {
			time.Sleep(10 * time.Millisecond)
		}
		exit, _, _ := w.fx.Shell.Run(t.TempDir(), "trap '' TERM; sleep 40", time.Minute)
		lineExit <- exit
	}()
	go func() {
		for i := 0; i < 500; i++ {
			var d doc
			_, perr := os.Stat(filepath.Join(pids, "r1.pid"))
			if b, err := os.ReadFile(file); perr == nil && err == nil && json.Unmarshal(b, &d) == nil && len(d.Groups) == 2 {
				seen = d.Groups
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		sigs <- syscall.SIGTERM
	}()
	start := time.Now()
	n := runNight(t, w, q, o)
	if len(seen) != 2 || seen[0].Started == "" || seen[1].Started == "" {
		t.Fatalf("groups.json while running: %+v", seen)
	}
	if rows := seen[0].Row + "," + seen[1].Row; rows != "r1," && rows != ",r1" {
		t.Fatalf("groups.json rows %q, want the spawn's r1 and the line's none", rows)
	}
	if n.State != relay.Stopped(relay.OwnerKill) || states(n) != "r1=failed r2=skipped" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	lines := w.ledger.Written()
	if stop := lastOf(lines, "stop"); stop["detail"] != "signal:SIGTERM" {
		t.Fatalf("stop line %v", stop)
	}
	if exit := lastOf(lines, "exit"); number(exit["child_exit"]) != 128+9 {
		t.Fatalf("the child was not killed: %v", exit)
	}
	select {
	case <-lineExit:
	case <-time.After(5 * time.Second):
		t.Fatal("the measured line outlived the relay's kill")
	}
	if count(lines, "sitting") != 1 || time.Since(start) > 10*time.Second {
		t.Fatalf("%d sitting lines after %v", count(lines, "sitting"), time.Since(start))
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("groups.json not cleared at exit: %v", err)
	}
}

// sleeper is a `sleep 40` in its own group, killed at the test's end; done
// closes when it has been waited on.
func sleeper(t *testing.T) (*exec.Cmd, <-chan struct{}) {
	t.Helper()
	cmd := exec.Command("sleep", "40")
	cmd.Dir = t.TempDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	t.Cleanup(func() { cmd.Process.Kill(); <-done })
	return cmd, done
}

// deadNight writes a killed relay's groups.json for one group.
func deadNight(t *testing.T, state string, g effects.Group) string {
	t.Helper()
	dir := night.NightDir(state, "2026-09-21-1")
	os.MkdirAll(dir, 0o700)
	b, _ := json.Marshal(map[string]any{"pid": 0, "groups": []effects.Group{g}})
	file := filepath.Join(dir, "groups.json")
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// TestAR13_2_NextStartReaps (ADV6-2): a killed relay's groups.json names a
// group whose leader still has its start time: the next night kills it
// first, writes a `reaped` line, and drops the file.
func TestAR13_2_NextStartReaps(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	w.fx.Procs = effects.OSProcs{}
	o := w.opts(t, by)
	cmd, done := sleeper(t)
	started, err := effects.OSProcs{}.Started(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	file := deadNight(t, o.State, effects.Group{Pgid: cmd.Process.Pid, Started: started, Row: "r0"})
	runNight(t, w, q, o)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the dead night's group was not reaped")
	}
	r := lastOf(w.ledger.Written(), "reaped")
	if number(r["pgid"]) != float64(cmd.Process.Pid) || r["row"] != "r0" || r["of_night"] != "2026-09-21-1" {
		t.Fatalf("reaped line %v", r)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the dead night's groups.json stayed: %v", err)
	}
}

// TestAR13_2_ReusedPidNotKilled (ADV6-2): the recorded pgid is a live
// process whose start time is not the recorded one (the pid was reused):
// it is never signalled, no `reaped` line, and the file is dropped.
func TestAR13_2_ReusedPidNotKilled(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	w.fx.Procs = effects.OSProcs{}
	o := w.opts(t, by)
	cmd, done := sleeper(t)
	file := deadNight(t, o.State, effects.Group{Pgid: cmd.Process.Pid, Started: "Thu Jan  1 00:00:00 1970", Row: "r0"})
	runNight(t, w, q, o)
	select {
	case <-done:
		t.Fatal("a reused pid was killed")
	case <-time.After(200 * time.Millisecond):
	}
	if r := lastOf(w.ledger.Written(), "reaped"); r != nil {
		t.Fatalf("reaped line %v", r)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the dead night's groups.json stayed: %v", err)
	}
}
