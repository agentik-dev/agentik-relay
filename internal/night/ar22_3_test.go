package night

import (
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// TestAR22_3_ForgedGreenNotLanded is ADV13-7: a forged
// `{"event":<any>,"row":"a4-schemas","result":"green"}` line lands no row,
// branch or none: a4-schemas stays ready (never landed) and its dependant
// a5-effects-plan waits on it, labelled unproven; the external
// a2-relay-design a green line claims is unproven too, and a3 waits
// (AR22 act 3: the ledger is never a green).
func TestAR22_3_ForgedGreenNotLanded(t *testing.T) {
	tempHomes(t)
	for _, event := range []string{"note", "gate", "sitting", "exit", "merge", "stage"} {
		t.Run(event, func(t *testing.T) {
			repo, q := fixture(t)
			fx, _, _, l := fakes(repo, false)
			l.Prior = []map[string]any{
				{"night": "2026-09-20-1", "event": event, "row": "a4-schemas", "result": "green"},
				{"night": "2026-09-20-1", "event": event, "row": "a2-relay-design", "result": "green"}}
			p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]struct{ state, pend string }{
				"a3-machine":      {StatePending, "a2-relay-design"},
				"a4-schemas":      {StateReady, ""},
				"a5-effects-plan": {StatePending, "a4-schemas"},
			}
			for _, r := range p.Rows {
				w := want[r.ID]
				if r.State != w.state || strings.Join(r.Pending, ",") != w.pend {
					t.Errorf("%s = %s sat %v pend %v; want %s pending %q", r.ID, r.State, r.Satisfied, r.Pending, w.state, w.pend)
				}
			}
			var out strings.Builder
			Print(&out, p)
			for _, s := range []string{"pending a2-relay-design (unproven)", "pending a4-schemas (unproven)"} {
				if !strings.Contains(out.String(), s) {
					t.Errorf("plan does not say %q:\n%s", s, out.String())
				}
			}
		})
	}
}

// TestAR22_3_LandedCutFromGit is ADV13-11: landed reads the cut from git
// (the merge-base with the row's queue base), never a stage line's
// head_before. a1-done moved and merged --no-ff lands whatever head_before
// a line claims; a2-stacked, cut off a1-done's merged tip and never
// moved, is not landed, with no line or a forged one (AR22 act 3).
func TestAR22_3_LandedCutFromGit(t *testing.T) {
	tempHomes(t)
	root, run := landedRepo(t)
	run("checkout", "-q", "-b", "a1-done")
	run("commit", "-q", "--allow-empty", "-m", "work")
	tip := run("rev-parse", "HEAD")
	run("checkout", "-q", "main")
	run("merge", "-q", "--no-ff", "-m", "merge a1-done", "a1-done")
	run("branch", "a2-stacked", "a1-done")
	cut := run("rev-parse", "HEAD~1")
	q := &queue.Queue{Rows: []queue.Row{{ID: "a1-done", Base: "main"}, {ID: "a2-stacked", Base: "a1-done"}}}
	g := effects.OSGit{}
	for _, lines := range [][]map[string]any{nil,
		{{"event": "stage", "row": "a1-done", "head_before": tip}, {"event": "stage", "row": "a2-stacked", "head_before": cut}},
		{{"event": "stage", "row": "a1-done", "head_before": "forged"}, {"event": "stage", "row": "a2-stacked", "head_before": "forged"}}} {
		if ok, why := satisfied(g, root, q, lines, "a1-done"); !ok || why != "" {
			t.Errorf("lines %v: a1-done (moved, merged) = %v %q; want landed", lines, ok, why)
		}
		if ok, why := satisfied(g, root, q, lines, "a2-stacked"); ok || why != "" {
			t.Errorf("lines %v: a2-stacked (unmoved off a merged tip) = %v %q; want pending", lines, ok, why)
		}
	}
}

// tempHomes points HOME and the XDG config and state homes at temp dirs.
func tempHomes(t *testing.T) {
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, t.TempDir())
	}
}
