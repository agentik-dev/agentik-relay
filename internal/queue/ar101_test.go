package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ar101Load writes src as q.yaml in a temp repo and loads it; its packets
// are absent, so Validate's packet problems are read past.
func ar101Load(t *testing.T, src string) (*Queue, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "q.yaml"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(dir, "q.yaml")
}

// ar101EffortProblems are Validate's problems that name the effort key, as
// "<row>: <msg>".
func ar101EffortProblems(q *Queue) []string {
	var out []string
	for _, p := range q.Validate() {
		if strings.Contains(p.Msg, "effort") {
			out = append(out, p.Row+": "+p.Msg)
		}
	}
	return out
}

// TestAR101_1_EffortKey (AR101 ruling 1): `effort` is optional in the
// defaults and in a row; a row's value outranks the defaults', which a row
// inherits; unset reads "" and EffortLevel's default high; the five levels
// are valid and any other value is a row problem (an orchestrator row's
// too, a parked row's never); the key is not a budget key.
func TestAR101_1_EffortKey(t *testing.T) {
	q, err := ar101Load(t, `version: 1
defaults: {model: m, effort: medium}
rows:
  - id: a
    packet: a.md
  - id: b
    packet: b.md
    effort: low
  - id: c
    packet: c.md
    effort: max
`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Defaults.Effort != "medium" {
		t.Fatalf("defaults effort %q", q.Defaults.Effort)
	}
	for i, want := range []string{"medium", "low", "max"} {
		if r := q.Rows[i]; r.Effort != want || r.EffortLevel() != want {
			t.Errorf("row %s: effort %q, level %q; want %s", r.ID, r.Effort, r.EffortLevel(), want)
		}
	}
	if ps := ar101EffortProblems(q); len(ps) != 0 {
		t.Fatalf("effort problems on valid values: %v", ps)
	}

	q, err = ar101Load(t, "version: 1\ndefaults: {model: m}\nrows:\n  - id: a\n    packet: a.md\n")
	if err != nil {
		t.Fatal(err)
	}
	if r := q.Rows[0]; r.Effort != "" || r.EffortLevel() != "high" || DefaultEffort != "high" {
		t.Fatalf("unset: effort %q, level %q, default %q", r.Effort, r.EffortLevel(), DefaultEffort)
	}
	if (Row{}).EffortLevel() != "high" {
		t.Fatalf("a zero row's level is not high")
	}

	var rows strings.Builder
	for _, e := range Efforts {
		rows.WriteString("  - id: r-" + e + "\n    packet: r-" + e + ".md\n    effort: " + e + "\n")
	}
	if q, err = ar101Load(t, "version: 1\nrows:\n"+rows.String()); err != nil {
		t.Fatal(err)
	}
	if ps := ar101EffortProblems(q); len(ps) != 0 || strings.Join(Efforts, " ") != "low medium high xhigh max" {
		t.Fatalf("the five levels %v: problems %v", Efforts, ps)
	}

	q, err = ar101Load(t, `version: 1
defaults: {model: m, merge: merge-on-green, device: never, effort: extreme}
rows:
  - id: a
    packet: a.md
  - id: b
    packet: b.md
    effort: High
  - id: c
    packet: c.md
    effort: low
  - id: d
    packet: d.md
    parked: later
  - id: o
    kind: orchestrator
    prompt: history/brief.md
    sections: [How this runs]
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`a: effort "extreme"; want low, medium, high, xhigh or max`,
		`b: effort "High"; want low, medium, high, xhigh or max`,
		`o: effort "extreme"; want low, medium, high, xhigh or max`,
	}
	if got := ar101EffortProblems(q); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("effort problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	for _, bad := range []string{
		"version: 1\nrows:\n  - id: a\n    packet: a.md\n    budget: {turns: 3, effort: low}\n",
		"version: 1\nrows:\n  - id: a\n    packet: a.md\n    effort: [low]\n",
	} {
		if _, err := ar101Load(t, bad); err == nil || !strings.Contains(err.Error(), "effort") {
			t.Errorf("Load %q: err %v; want a refusal naming effort", bad, err)
		}
	}
}
