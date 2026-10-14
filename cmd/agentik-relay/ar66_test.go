package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR66_2_PausedSittingParksNothing (AR66 ruling 3): `redSittings`
// reads no red in a failed usage-limit `sitting` line with `limit: paused`
// (the night paused on it, so a recovered night parks nothing), and reads
// the same line without it failed. In TestAR32AFix_MergeRefusedParksTheDay's
// world the paused line alone refuses nothing; a `stop` line resuming o.1
// (a night that stopped while paused) still parks the row for the day.
// Red on the pin: the paused line read failed.
func TestAR66_2_PausedSittingParksNothing(t *testing.T) {
	sitting := func(limit bool) map[string]any {
		l := map[string]any{"event": "sitting", "row": "o.1", "state": "failed", "reason": "usage-limit", "ts": "2026-09-22T03:00:00Z"}
		if limit {
			l["limit"] = "paused"
		}
		return l
	}
	if got := redSittings(sitting(true)); len(got) != 0 {
		t.Fatalf("redSittings(the paused line) = %v, want none", got)
	}
	if got := fmt.Sprint(redSittings(sitting(false))); got != "[[o.1 failed]]" {
		t.Fatalf("redSittings(the line without limit) = %s, want [[o.1 failed]]", got)
	}
	_, l := planFakes(t, "main")
	measuring(t)
	top, _ := toplevel()
	home := getenv("HOME")
	const q = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nwords: \"\"\nrows:\n" +
		"  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
	for p, text := range map[string]string{
		filepath.Join(home, ".config", "agentik", "relay", "config.yaml"): "ntfy:\n  url: https://ntfy.invalid\n  topic: relay-test\n",
		filepath.Join(top, "relay", "queue.yaml"):                         q,
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prior := l.Prior[:1]
	stop := map[string]any{"event": "stop", "reason": "owner-stop", "resume": []any{"agentik relay resume o.1"}, "ts": "2026-09-22T03:00:02Z"}
	for _, c := range []struct {
		name  string
		lines []map[string]any
		want  string
	}{
		{"the paused line", []map[string]any{sitting(true)}, ""},
		{"the line without limit", []map[string]any{sitting(false)}, "parked for today: o.1 was failed"},
		{"the paused line, then a stop resuming o.1", []map[string]any{sitting(true), stop}, "parked for today: o.1 was red or failed at the night's stop"},
	} {
		l.Prior = append(append([]map[string]any{}, prior...), c.lines...)
		got := orchestrateRefusal()
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: orchestrateRefusal() = %q; want %q", c.name, got, c.want)
		}
	}
}

// TestAR66_3_CleanMergedKeepsHeld (AR66 ruling 4; usage-limit-resume r1
// §2): in TestClean's world with TestAR13_4_CleanStale's locks, each named
// by the sha1 of its worktree's path as the stage names it, `relay clean
// --merged` keeps a merged tree whose lock's holder lives (a paused row's,
// at its cut, an ancestor of main), naming the pid, exit 1; a dead
// holder's merged tree and an unlocked one go as before; no lock is
// touched. Red on the pin: the held tree removed, exit 0.
func TestAR66_3_CleanMergedKeepsHeld(t *testing.T) {
	oldTop, oldEnv, oldFx, oldDir := toplevel, getenv, effectsFor, night.LockDir
	t.Cleanup(func() { toplevel, getenv, effectsFor, night.LockDir = oldTop, oldEnv, oldFx, oldDir })
	fakeTop(t, "/root")
	getenv = func(k string) string { return "" }
	locks := t.TempDir()
	night.LockDir = func() string { return locks }
	wt := "/root/.claude/worktrees/"
	for name, body := range map[string]string{
		"a": "424242\tTue Sep 22 01:00:00 2026\t" + wt + "a", // dead
		"b": "77\tWed Sep 23 21:00:00 2026\t" + wt + "b",     // live
	} {
		lock := filepath.Join(locks, fmt.Sprintf("%x", sha1.Sum([]byte(wt+name))))
		if err := os.WriteFile(lock, []byte(body+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	g := &effects.FakeGit{Roots: map[string]string{"/root": "/root"},
		WTs: []effects.Worktree{{Path: "/root", Branch: "main"}, {Path: wt + "a", Branch: "a"},
			{Path: wt + "b", Branch: "b"}, {Path: wt + "c", Branch: "c"}, {Path: wt + "d", Branch: "d"}},
		Ancestors: map[string]bool{"a..main": true, "b..main": true, "c..main": true, "main..main": true}}
	procs := &effects.FakeProcs{Starts: map[int]string{77: "Wed Sep 23 21:00:00 2026"}}
	effectsFor = func(state, night string) effects.Effects { return effects.Effects{Git: g, Procs: procs} }
	var out, errb bytes.Buffer
	code := run([]string{"clean", "--merged", "--json"}, strings.NewReader(""), &out, &errb)
	var got struct {
		Removed []string `json:"removed"`
		Kept    []struct{ Path, Why string }
	}
	json.Unmarshal(out.Bytes(), &got)
	if code != exitRed || fmt.Sprint(got.Removed) != "["+wt+"a "+wt+"c]" || len(got.Kept) != 1 ||
		got.Kept[0].Path != wt+"b" || got.Kept[0].Why != "its stage lock is held by pid 77" {
		t.Fatalf("clean --merged = %d, %s %s", code, out.String(), errb.String())
	}
	for _, c := range g.Calls() {
		if c == "WorktreeRemove /root "+wt+"b" {
			t.Fatalf("clean removed the held tree: %s", c)
		}
	}
	if left, _ := os.ReadDir(locks); len(left) != 2 {
		t.Fatalf("locks left %v, want both: --merged reaps none", left)
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"clean", "--merged"}, strings.NewReader(""), &out, &errb); code != exitRed ||
		!strings.Contains(errb.String(), "relay clean: kept "+wt+"b: its stage lock is held by pid 77") {
		t.Fatalf("clean --merged (text) = %d, %q", code, errb.String())
	}
}
