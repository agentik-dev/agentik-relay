package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR32AFix_MergeRefusedParksTheDay is ar32a review round 1 (F1, F2):
// a green sitting whose merge is refused (git-state-moved,
// merge-tree-differs) reads red, though its `sitting` line says green, so
// a later `relay run --orchestrate` that day refuses; so does a `stop`
// line naming the sitting in its resume ids. A merged sitting, a refusal
// yesterday, another row's refusal and a plain merge error do not refuse.
func TestAR32AFix_MergeRefusedParksTheDay(t *testing.T) {
	_, l := planFakes(t, "main")
	measuring(t)
	top, _ := toplevel()
	home := getenv("HOME")
	const q = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nwords: \"\"\nrows:\n" +
		"  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
	for p, text := range map[string]string{
		filepath.Join(home, ".config", "agent-relay", "config.yaml"): "ntfy:\n  url: https://ntfy.invalid\n  topic: relay-test\n",
		filepath.Join(top, "relay", "queue.yaml"):                    q,
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prior := l.Prior[:1]
	sitting := map[string]any{"event": "sitting", "row": "o.1", "state": "green", "ts": "2026-09-22T03:00:00Z"}
	merge := func(row, result, ts string) map[string]any {
		return map[string]any{"event": "merge", "row": row, "result": result, "ts": ts}
	}
	for _, c := range []struct {
		name  string
		lines []map[string]any
		want  string
	}{
		{"git-state-moved", []map[string]any{sitting, merge("o.1", "git-state-moved", "2026-09-22T03:00:01Z")}, "parked for today: o.1 was red"},
		{"merge-tree-differs", []map[string]any{sitting, merge("o.1", "merge-tree-differs", "2026-09-22T03:00:01Z")}, "parked for today: o.1 was red"},
		{"stop resume", []map[string]any{sitting, {"event": "stop", "reason": "orchestrator-red", "resume": []any{"relay resume o.1"},
			"ts": "2026-09-22T03:00:02Z"}}, "parked for today: o.1 was red"},
		{"merged", []map[string]any{sitting, merge("o.1", "merged", "2026-09-22T03:00:01Z")}, ""},
		{"yesterday", []map[string]any{sitting, merge("o.1", "git-state-moved", "2026-09-21T03:00:01Z"),
			{"event": "stop", "resume": []any{"relay resume o.1"}, "ts": "2026-09-21T03:00:02Z"}}, ""},
		{"another row", []map[string]any{merge("b1", "merge-tree-differs", "2026-09-22T03:00:01Z"),
			{"event": "stop", "resume": []any{"relay resume b1"}, "ts": "2026-09-22T03:00:02Z"}}, ""},
		{"plain error", []map[string]any{sitting, merge("o.1", "branch-only", "2026-09-22T03:00:01Z")}, ""},
	} {
		l.Prior = append(append([]map[string]any{}, prior...), c.lines...)
		got := orchestrateRefusal()
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: orchestrateRefusal() = %q; want %q", c.name, got, c.want)
		}
	}
}

// TestAR32AFix_PlanRowReadsPacketsInItsWorktree is ar32a review round 1
// (F2, F3): the RULES' check, `relay plan --queue relay/queue.yaml --rows
// <id>` from a sitting's worktree, reads the packet of a row the sitting
// added from the worktree holding that queue, where the packet is
// committed, not from the root, where it is not yet. Outside a row the
// same plan still reads packets from the root (ADV6-10).
func TestAR32AFix_PlanRowReadsPacketsInItsWorktree(t *testing.T) {
	planFakes(t, "main")
	top, _ := toplevel()
	wt := filepath.Join(top, ".claude", "worktrees", "o.1")
	const q = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nwords: \"\"\nrows:\n" +
		"  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
	pkt, err := os.ReadFile(filepath.Join(top, "internal", "packet", "testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	for p, text := range map[string]string{
		filepath.Join(top, "relay", "queue.yaml"):               q,
		filepath.Join(wt, "relay", "queue.yaml"):                q + "  - {id: ok-minimal, packet: design/packets/ok-minimal.md}\n",
		filepath.Join(wt, "design", "packets", "ok-minimal.md"): string(pkt),
		filepath.Join(top, "brief.md"):                          "# Brief\n\n## How this runs\nRun.\n",
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inner := effectsFor
	effectsFor = func(state, n string) effects.Effects {
		fx := inner(state, n)
		g := fx.Git.(*effects.FakeGit)
		g.Toplevels = map[string]string{filepath.Join(wt, "relay"): wt, filepath.Join(top, "relay"): top}
		return fx
	}
	plan := func() (int, night.Plan, string) {
		var out, errb bytes.Buffer
		code := run([]string{"plan", "--queue", filepath.Join(wt, "relay", "queue.yaml"), "--rows", "ok-minimal", "--json"}, strings.NewReader(""), &out, &errb)
		var p night.Plan
		json.Unmarshal(out.Bytes(), &p)
		return code, p, out.String() + errb.String()
	}
	withEnv(t, map[string]string{night.RowVar: "o"})
	if code, p, text := plan(); code != exitOK || len(p.Rows) != 1 || strings.Contains(text, "missing") {
		t.Errorf("in the row: plan --rows ok-minimal = %d: %s", code, text)
	}
	withEnv(t, map[string]string{night.RowVar: ""})
	if code, _, text := plan(); code != exitUsage || !strings.Contains(text, "packet design/packets/ok-minimal.md: missing") {
		t.Errorf("outside a row: plan --rows ok-minimal = %d, want 2 and the packet missing at the root: %s", code, text)
	}
}

// TestAR32AFix_RefusalIsThisRepos is ar32a review round 2 (F2): the ledger
// is shared by every repo on the account, so the day's refusal counts a
// red or failed sitting of the row only when its line names this root: a
// `sitting` line's repo, a `merge` line's root, a `stop` line's root, or,
// for a stop line written before it carried one, its night's `night-open`
// root. Another repo's red o.1 the same day does not refuse; a line that
// names no root still does (a red row stays parked).
func TestAR32AFix_RefusalIsThisRepos(t *testing.T) {
	_, l := planFakes(t, "main")
	measuring(t)
	top, _ := toplevel()
	home := getenv("HOME")
	const q = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nwords: \"\"\nrows:\n" +
		"  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
	for p, text := range map[string]string{
		filepath.Join(home, ".config", "agent-relay", "config.yaml"): "ntfy:\n  url: https://ntfy.invalid\n  topic: relay-test\n",
		filepath.Join(top, "relay", "queue.yaml"):                    q,
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const other = "/elsewhere/repo"
	prior := l.Prior[:1]
	sitting := func(root string) map[string]any {
		return map[string]any{"event": "sitting", "row": "o.1", "state": "red", "repo": root, "ts": "2026-09-22T03:00:00Z", "night": "2026-09-22-1"}
	}
	merge := func(root string) map[string]any {
		return map[string]any{"event": "merge", "row": "o.1", "result": "merge-tree-differs", "root": root, "ts": "2026-09-22T03:00:01Z", "night": "2026-09-22-1"}
	}
	open := func(root string) map[string]any {
		return map[string]any{"event": "night-open", "root": root, "ts": "2026-09-22T02:59:00Z", "night": "2026-09-22-1"}
	}
	stop := func(root string) map[string]any {
		m := map[string]any{"event": "stop", "reason": "orchestrator-red", "resume": []any{"relay resume o.1"}, "ts": "2026-09-22T03:00:02Z", "night": "2026-09-22-1"}
		if root != "" {
			m["root"] = root
		}
		return m
	}
	for _, c := range []struct {
		name  string
		lines []map[string]any
		want  string
	}{
		{"another repo's red sitting", []map[string]any{open(other), sitting(other)}, ""},
		{"another repo's refused merge", []map[string]any{open(other), merge(other)}, ""},
		{"another repo's stop", []map[string]any{open(other), stop(other)}, ""},
		{"another repo's stop, by its night", []map[string]any{open(other), stop("")}, ""},
		{"this repo's red sitting", []map[string]any{open(top), sitting(top)}, "parked for today: o.1 was red"},
		{"this repo's refused merge", []map[string]any{open(top), merge(top + "/")}, "parked for today: o.1 was red"},
		{"this repo's stop", []map[string]any{open(top), stop(top)}, "parked for today: o.1 was red"},
		{"this repo's stop, by its night", []map[string]any{open(top), stop("")}, "parked for today: o.1 was red"},
		{"a line naming no root", []map[string]any{stop("")}, "parked for today: o.1 was red"},
	} {
		l.Prior = append(append([]map[string]any{}, prior...), c.lines...)
		got := orchestrateRefusal()
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: orchestrateRefusal() = %q; want %q", c.name, got, c.want)
		}
	}
}
