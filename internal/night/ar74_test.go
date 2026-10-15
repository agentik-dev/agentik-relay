package night_test

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar74World is TestAR14_3_SpawnEnvCarriesRowTurns's world (its fakes and
// its green gate) from outside the package: this file holds the report's
// tests too, and those need the night world's helpers (night_test).
func ar74World(t *testing.T) effects.Effects {
	now := time.Date(2026, 9, 22, 22, 0, 0, 0, time.UTC)
	g := &effects.FakeGit{
		Roots:     map[string]string{t.TempDir(): "/root"},
		Branches:  map[string]string{"/root": "main"},
		Statuses:  map[string][]string{},
		Ancestors: map[string]bool{"ar1-relay-hooks..main": true}, Merged: map[string]bool{"ar1-relay-hooks..main": true},
		Existing: map[string]bool{"main": true, "ar1-relay-hooks": true},
		Remote:   map[string]string{"refs/remotes/origin/main": "0933461"},
	}
	sh := &effects.FakeShell{Results: map[string]effects.ShellResult{"make check; echo $?": {Exit: 0, Last: "0"}}}
	fs := &effects.FakeFS{}
	fs.MkdirAll("/state/nights/2026-09-22-1", 0o700)
	l := &effects.FakeLedger{Clock: effects.FakeClock{T: now}, Night: "2026-09-22-2"}
	return effects.Effects{Git: g, Shell: sh, Clock: effects.FakeClock{T: now}, FS: fs, Ledger: l}
}

func ar74Green(*night.Staged, bool, bool, night.GateWait) (relay.Result, relay.StopReason, relay.Gate) {
	return relay.GreenResult, "", relay.Gate{Gathered: true, ResultSeen: true, ExitOK: true, Commit: true, Clean: true, OriginSame: true}
}

// TestAR74_1_RunPassesModel (relay-r1 §7; AR74 act 1): the spawned child's
// env holds CLAUDE_SESSION_MODEL=<the spawn line's --model>, the queue
// row's model as written, once; a parent's never passes.
func TestAR74_1_RunPassesModel(t *testing.T) {
	const model = "claude-opus-5-5[1m]"
	rp := &night.RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1",
		Spawn: []string{"claude", "-p", "--max-turns", "80", "--model", model}}
	st := &night.Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}
	sp := &effects.FakeSpawner{PID: 77}
	night.RunRow(ar74World(t), sp, st, rp, nil, night.RunOptions{Context: 1000, Wall: time.Hour, Gate: ar74Green,
		Env: []string{"HOME=/h", "CLAUDECODE=1", "CLAUDE_SESSION_MODEL=x"}})
	if len(sp.Spawns()) != 1 {
		t.Fatalf("%d spawns", len(sp.Spawns()))
	}
	var models []string
	for _, kv := range sp.Spawns()[0].Env {
		if strings.HasPrefix(kv, "CLAUDE_SESSION_MODEL=") {
			models = append(models, kv)
		}
	}
	if strings.Join(models, " ") != "CLAUDE_SESSION_MODEL="+model {
		t.Fatalf("the child's CLAUDE_SESSION_MODEL = %q; want %s", models, model)
	}
}

// TestAR74_2_LastSitting (relay-r1 §9 `report`; AR74 ruling 2): a row's
// verdict is its last sitting's, as `status` reads it. r1 failed at a usage
// limit, then green: no stopped entry, no resume line, waiting once. r2
// green, then red: stopped red with its resume line, not waiting. r3 red,
// then red: one entry, one line. The burn keeps every sitting. A row's
// next part in flight (a `spawn` line after its `sitting` line) reads as
// `status` reads it, running: no stopped entry, no resume line, not
// waiting (the fix round's item 3).
func TestAR74_2_LastSitting(t *testing.T) {
	const n = "2026-10-12-3"
	sitting := func(row, state, reason string) map[string]any {
		return map[string]any{"night": n, "event": "sitting", "row": row, "state": state, "reason": reason}
	}
	lines := []map[string]any{
		sitting("r1", "failed", "usage-limit"), sitting("r2", "green", ""), sitting("r3", "red", "acceptance"),
		sitting("r1", "green", ""), sitting("r2", "red", "acceptance"), sitting("r3", "red", "acceptance"),
	}
	r := night.BuildReport(lines, n)
	var stopped []string
	for _, s := range r.Stopped.Rows {
		stopped = append(stopped, s.Row+" "+s.State+" "+s.Resume)
	}
	if want := []string{"r2 red agentik relay resume r2", "r3 red agentik relay resume r3"}; !reflect.DeepEqual(stopped, want) {
		t.Fatalf("stopped %q; want %q", stopped, want)
	}
	if want := []string{"agentik relay resume r2", "agentik relay resume r3"}; !reflect.DeepEqual(r.Resume, want) {
		t.Fatalf("resume %q; want %q", r.Resume, want)
	}
	if want := []string{"r1 (off main)"}; !reflect.DeepEqual(r.Waiting, want) {
		t.Fatalf("waiting %q; want %q", r.Waiting, want)
	}
	if len(r.Sittings) != 6 {
		t.Fatalf("%d sittings; want 6", len(r.Sittings))
	}
	md := string(night.Markdown(r, "", ""))
	block := md[strings.Index(md, "## Resume"):strings.Index(md, "## Burn")]
	for line, want := range map[string]int{"agentik relay resume r1": 0, "agentik relay resume r2": 1, "agentik relay resume r3": 1} {
		if got := strings.Count(block, line); got != want {
			t.Fatalf("%q %d times in the Resume block; want %d:\n%s", line, got, want, block)
		}
	}
	spawn := func(row string) map[string]any { return map[string]any{"night": n, "event": "spawn", "row": row} }
	r = night.BuildReport([]map[string]any{spawn("r1"), sitting("r1", "failed", "usage-limit"), spawn("r1"),
		spawn("r2"), sitting("r2", "green", ""), spawn("r2")}, n)
	if len(r.Stopped.Rows) != 0 || len(r.Resume) != 0 || len(r.Waiting) != 0 || len(r.Sittings) != 4 {
		t.Fatalf("rows in flight: stopped %+v, resume %q, waiting %q, %d sittings; want none, none, none, 4",
			r.Stopped.Rows, r.Resume, r.Waiting, len(r.Sittings))
	}
}

// TestAR74_3_HandoverOnce (relay-r1 §9 `report`; AR74 ruling 3): on
// TestReportNoHomePath's fixture, a row of two finished sittings on one
// branch with two dated charters gives each path once; a row whose last
// sitting is in flight gives none yet.
func TestAR74_3_HandoverOnce(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	wt := filepath.Join(w.root, ".claude", "worktrees", "r1")
	os.MkdirAll(filepath.Join(wt, "history", "sittings"), 0o755)
	for _, name := range []string{"2026-09-22-1-r1.md", "2026-09-22-2-r1.md"} {
		charter := "# Sitting\n\n## Handover\n- " + name + "\n\n## Next act\n- exit line: done\n"
		os.WriteFile(filepath.Join(wt, "history", "sittings", name), []byte(charter), 0o644)
	}
	gitIn(t, wt, "add", ".")
	gitIn(t, wt, "commit", "-qm", "two charters")

	lines := w.ledger.Written()
	lines = append(lines, maps.Clone(lastOf(lines, "sitting"))) // r1's second sitting, finished
	r := night.BuildReport(lines, n.ID)
	var paths []string
	for _, h := range r.Handovers {
		paths = append(paths, h.Row+" "+h.Path)
	}
	want := []string{"r1 history/sittings/2026-09-22-1-r1.md", "r1 history/sittings/2026-09-22-2-r1.md"}
	if len(r.Sittings) != 2 || !reflect.DeepEqual(paths, want) {
		t.Fatalf("%d sittings, handovers %q; want 2, %q", len(r.Sittings), paths, want)
	}
	spawn := maps.Clone(lastOf(lines, "spawn")) // r1's third sitting, in flight
	if r := night.BuildReport(append(lines, spawn), n.ID); len(r.Handovers) != 0 {
		t.Fatalf("a row in flight gave handovers %+v", r.Handovers)
	}
}
