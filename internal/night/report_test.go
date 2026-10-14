package night_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestStatusResumeLines: a red row's resume line is in status.json, in the
// dashboard, in the report, and the `stop` line carries it.
func TestStatusResumeLines(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "no", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	want := []string{"agentik relay resume r1"}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	if !reflect.DeepEqual(st.Resume, want) || st.Rows[0].Resume != want[0] || st.Rows[1].Resume != "" {
		t.Fatalf("status resume %v %+v", st.Resume, st.Rows)
	}
	var b strings.Builder
	night.PrintStatus(&b, st)
	if !strings.Contains(b.String(), "\nagentik relay resume r1\n") {
		t.Fatalf("dashboard:\n%s", b.String())
	}
	r := night.BuildReport(w.ledger.Written(), n.ID)
	if !reflect.DeepEqual(r.Resume, want) {
		t.Fatalf("report resume %v", r.Resume)
	}
	if !strings.Contains(lastOf(w.ledger.Written(), "stop")["resume"].([]any)[0].(string), "agentik relay resume r1") {
		t.Fatal("the stop line has no resume line")
	}
}

// TestReportNoHomePath: a charter committed on a row's branch names the
// root and home by absolute path; the report's markdown holds neither, and
// --write lands it under the repo by a relative path only.
func TestReportNoHomePath(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "no", fixture: "ok.jsonl"})
	home := filepath.Dir(w.root)
	t.Setenv("HOME", home)
	n := runNight(t, w, q, w.opts(t, by))
	wt := filepath.Join(w.root, ".claude", "worktrees", "r1")
	os.MkdirAll(filepath.Join(wt, "history", "sittings"), 0o755)
	charter := "# Sitting\n\n## Handover\n- decide: the tree at " + wt + " and " + home + "/.config\n\n## Next act\n- exit line: done\n"
	os.WriteFile(filepath.Join(wt, "history", "sittings", "2026-09-22-1.md"), []byte(charter), 0o644)
	gitIn(t, wt, "add", ".")
	gitIn(t, wt, "commit", "-qm", "charter")

	lines := w.ledger.Written()
	r := night.BuildReport(lines, n.ID)
	if len(r.Handovers) != 1 || !strings.Contains(r.Handovers[0].Text, "decide: the tree") || strings.Contains(r.Handovers[0].Text, "exit line") {
		t.Fatalf("handovers %+v", r.Handovers)
	}
	if len(r.Sittings) != 2 || !reflect.DeepEqual(r.Waiting, []string{"r1 (off main)"}) || r.Stopped.Reason != "queue-empty" {
		t.Fatalf("report %+v", r)
	}
	md := string(night.Markdown(r, night.RootOf(lines, n.ID), home))
	if strings.Contains(md, home) || strings.Contains(md, w.root) {
		t.Fatalf("a home path in the report:\n%s", md)
	}
	for _, h := range []string{"# Night 2026-09-22-1: stopped:queue-empty", "## Merged", "## Stopped", "## Burn",
		"## Waiting on a hand merge", "## Resume", "## Handovers", "./.claude/worktrees/r1", "~/.config"} {
		if !strings.Contains(md, h) {
			t.Fatalf("no %q in\n%s", h, md)
		}
	}
	if _, err := night.WriteReport(effects.OSFS{}, w.root, "/tmp/x.md", []byte(md)); err == nil {
		t.Fatal("an absolute --write path was taken")
	}
	if _, err := night.WriteReport(effects.OSFS{}, w.root, "../x.md", []byte(md)); err == nil {
		t.Fatal("a --write path outside the repo was taken")
	}
	p, err := night.WriteReport(effects.OSFS{}, w.root, "history/nights/2026-09-22-1.md", []byte(md))
	if got, _ := os.ReadFile(p); err != nil || string(got) != md {
		t.Fatalf("write %s: %v", p, err)
	}
	b, _ := json.Marshal(night.BuildReport(nil, ""))
	if !strings.Contains(string(b), `"sittings":[]`) {
		t.Fatalf("the empty report: %s", b)
	}
}

// TestReportHeadingOrder (UX1): the report's `##` headings come in the
// morning's order: merged, stopped, waiting on a hand merge, resume lines,
// burn, handovers.
func TestReportHeadingOrder(t *testing.T) {
	var got []string
	for _, l := range strings.Split(surfaces(t)["report"], "\n") {
		if strings.HasPrefix(l, "## ") {
			got = append(got, l)
		}
	}
	want := []string{"## Merged", "## Stopped", "## Waiting on a hand merge", "## Resume", "## Burn", "## Handovers"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("headings %q, want %q", got, want)
	}
}

// TestAR5_StopLineKeyValue (UX1): the report's `## Stopped` writes the
// `stop` line as `key: value` lines, keys sorted, `event` dropped, a list
// joined by `, `, no fence; the report's JSON keeps the line whole.
func TestAR5_StopLineKeyValue(t *testing.T) {
	line := `{"event":"stop","state":"stopped","reason":"queue-empty","resume":["agentik relay resume r1","agentik relay resume r2"],"sittings":3}`
	r := night.Report{Night: "2026-09-22-1", Stopped: night.Stopped{Reason: "queue-empty", Line: line}}
	md := string(night.Markdown(r, "", ""))
	sec := md[strings.Index(md, "## Stopped"):]
	sec = sec[:strings.Index(sec, "## Waiting")]
	want := "reason: queue-empty\nresume: agentik relay resume r1, agentik relay resume r2\nsittings: 3\nstate: stopped\n"
	if !strings.Contains(sec, want) || strings.Contains(sec, "```") || strings.Contains(sec, "event") {
		t.Fatalf("## Stopped:\n%s", sec)
	}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"line":`) {
		t.Fatalf("report json %s", b)
	}
}
