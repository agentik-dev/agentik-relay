package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR13_0_GateLoadsFromRoot (ADV6-10): `relay gate` run from the row's
// own worktree reads the queue and the packet in the root (Git.Root), never
// the cwd's toplevel: a child that widened its own may-touch to cover
// README.md is still judged by the root's packet, README.md outside.
func TestAR13_0_GateLoadsFromRoot(t *testing.T) {
	gateFakes(t, true)
	top, _ := toplevel()
	child := t.TempDir()
	for _, f := range []string{"relay/queue.yaml", "design/packets/a3-machine.md"} {
		src, err := os.ReadFile(filepath.Join(top, f))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Replace(string(src), "`cmd/relay/main_test.go`,", "`cmd/relay/main_test.go`, `README.md`,", 1)
		os.MkdirAll(filepath.Dir(filepath.Join(child, f)), 0o755)
		os.WriteFile(filepath.Join(child, f), []byte(text), 0o644)
	}
	fakeTop(t, child)
	faked := effectsFor
	effectsFor = func(state, night string) effects.Effects {
		fx := faked(state, night)
		fx.Git.(*effects.FakeGit).Roots[child] = top
		return fx
	}
	var out, errb bytes.Buffer
	run([]string{"gate", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb)
	var rep struct {
		Result  string
		Outside []string
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Result != "red" || !reflect.DeepEqual(rep.Outside, []string{"README.md"}) {
		t.Fatalf("gate from the row's tree = %s, outside %v; want red, [README.md]: %s", rep.Result, rep.Outside, out.String())
	}
}

// TestAR13_4_CleanStale (R6 §4, §9): `relay clean --stale` removes a
// stage lock whose pid is dead and the worktree it held when that
// worktree's tip equals its cut; a live holder's lock and a worktree with
// commits past its cut stay.
func TestAR13_4_CleanStale(t *testing.T) {
	oldTop, oldEnv, oldFx, oldDir := toplevel, getenv, effectsFor, night.LockDir
	t.Cleanup(func() { toplevel, getenv, effectsFor, night.LockDir = oldTop, oldEnv, oldFx, oldDir })
	fakeTop(t, "/root")
	getenv = func(k string) string { return "" }
	locks := t.TempDir()
	night.LockDir = func() string { return locks }
	wt := "/root/.claude/worktrees/"
	for name, body := range map[string]string{
		"a": "424242\tTue Sep 22 01:00:00 2026\t" + wt + "a", // dead, untouched
		"b": "424243\tTue Sep 22 01:00:00 2026\t" + wt + "b", // dead, a commit past its cut
		"c": "77\tWed Sep 23 21:00:00 2026\t" + wt + "c",     // live
	} {
		os.WriteFile(filepath.Join(locks, name), []byte(body+"\n"), 0o600)
	}
	g := &effects.FakeGit{Roots: map[string]string{"/root": "/root"},
		Heads: map[string]string{wt + "a": "cut-a", wt + "b": "moved", wt + "c": "cut-c"},
		WTs: []effects.Worktree{{Path: "/root", Branch: "main"}, {Path: wt + "a", Branch: "a"},
			{Path: wt + "b", Branch: "b"}, {Path: wt + "c", Branch: "c"}}}
	clock := effects.FakeClock{T: time.Date(2026, 9, 23, 22, 0, 0, 0, time.UTC)}
	var prior []map[string]any
	for _, r := range []string{"a", "b", "c"} {
		prior = append(prior, map[string]any{"event": "stage", "row": r, "worktree": wt + r, "head_before": "cut-" + r})
	}
	l := &effects.FakeLedger{Clock: clock, Night: "n", Prior: prior}
	procs := &effects.FakeProcs{Starts: map[int]string{77: "Wed Sep 23 21:00:00 2026"}}
	effectsFor = func(state, night string) effects.Effects {
		return effects.Effects{Git: g, Ledger: l, Procs: procs, Clock: clock}
	}
	var out, errb bytes.Buffer
	code := run([]string{"clean", "--stale", "--json"}, strings.NewReader(""), &out, &errb)
	var got struct {
		Removed []string `json:"removed"`
		Kept    []struct{ Path string }
		Locks   []string `json:"locks"`
	}
	json.Unmarshal(out.Bytes(), &got)
	if code != exitRed || fmt.Sprint(got.Removed) != "["+wt+"a]" || len(got.Kept) != 1 || got.Kept[0].Path != wt+"b" || len(got.Locks) != 2 {
		t.Fatalf("clean --stale = %d, %s %s", code, out.String(), errb.String())
	}
	left, _ := os.ReadDir(locks)
	if len(left) != 1 || left[0].Name() != "c" {
		t.Fatalf("locks left %v, want the live holder's alone", left)
	}
	reclaimed := 0
	for _, w := range l.Written() {
		if w["event"] == "lock-reclaimed" {
			reclaimed++
		}
	}
	if reclaimed != 2 {
		t.Fatalf("%d lock-reclaimed lines, want 2", reclaimed)
	}
}

// TestAR13_5_ParallelCapped (ADV6-13): `--parallel` over the machine's
// cores is usage, exit 2, before any plan; at the cores it parses.
func TestAR13_5_ParallelCapped(t *testing.T) {
	oldEnv := getenv
	t.Cleanup(func() { getenv = oldEnv })
	getenv = func(k string) string { return "" }
	over := fmt.Sprint(runtime.NumCPU() + 1)
	var out, errb bytes.Buffer
	if code := run([]string{"run", "--parallel", over, "--dry"}, strings.NewReader(""), &out, &errb); code != exitUsage ||
		!strings.HasPrefix(errb.String(), "relay run: --parallel "+over+": over the machine's "+fmt.Sprint(runtime.NumCPU())+" cores") {
		t.Fatalf("run --parallel %s = %d, %q", over, code, errb.String())
	}
}
