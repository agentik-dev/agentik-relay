package night

import (
	"strings"
	"testing"
)

// TestAR21_5_ForgedGreenNotLanded is ADV12-7's second half: a green ledger
// line for a row whose branch exists and is not landed satisfies no
// dependent (the child can write the ledger); AR22 act 3 (ADV13-7) takes
// AR21's ledger-green away: for a row with no branch it satisfies none
// either, and plan labels the dependency unproven. A green verdict this
// process wrote satisfies it from memory (AR21 act 5).
func TestAR21_5_ForgedGreenNotLanded(t *testing.T) {
	t.Run("branch-not-landed", func(t *testing.T) {
		repo, q := fixture(t)
		fx, g, _, _ := fakes(repo, false)
		g.Existing["a2-relay-design"] = true
		if DepSatisfied(g, "/root", nil, "a2-relay-design") {
			t.Error("a green ledger line satisfied a branch row that is not landed")
		}
		p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a3-machine"}})
		if err != nil {
			t.Fatal(err)
		}
		if r := p.Rows[0]; r.State != StatePending || len(r.Satisfied) != 0 || len(r.Unproven) != 0 {
			t.Errorf("a3 = %s sat %v unproven %v; want pending on a2-relay-design", r.State, r.Satisfied, r.Unproven)
		}
		// This process's own green verdict is memory, not the ledger.
		NoteGreen("/root", "a2-relay-design", "")
		t.Cleanup(func() { greens.Lock(); delete(greens.m, greenKey("/root", "a2-relay-design")); greens.Unlock() })
		if !DepSatisfied(g, "/root", nil, "a2-relay-design") {
			t.Error("a green verdict this process wrote did not satisfy its dependent")
		}
	})
	t.Run("no-branch", func(t *testing.T) {
		repo, q := fixture(t)
		fx, g, _, _ := fakes(repo, false)
		if DepSatisfied(g, "/root", nil, "a2-relay-design") {
			t.Error("a green ledger line satisfied a row with no branch")
		}
		p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a3-machine"}})
		if err != nil {
			t.Fatal(err)
		}
		if r := p.Rows[0]; r.State != StatePending || len(r.Satisfied) != 0 || strings.Join(r.Unproven, ",") != "a2-relay-design" {
			t.Errorf("a3 = %s sat %v unproven %v; want pending, a2-relay-design unproven", r.State, r.Satisfied, r.Unproven)
		}
		var out strings.Builder
		Print(&out, p)
		if !strings.Contains(out.String(), "pending a2-relay-design (unproven)") {
			t.Errorf("plan does not label the dependency unproven:\n%s", out.String())
		}
	})
}
