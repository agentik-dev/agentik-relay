package night

import (
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR13_1_RedMainNeverMerges (ADV6-5): a verify line red on main is
// named in the plan: merge-on-green rows land branch-only tonight.
func TestAR13_1_RedMainNeverMerges(t *testing.T) {
	repo, q, fx, _, sh := memoPlan(t)
	sh.Results["make check"] = effects.ShellResult{Exit: 2, Last: "FAIL pkg/a"}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if v := p.Verify; v == nil || v.Status != relay.CheckRed {
		t.Fatalf("verify %+v, want red", v)
	}
	var out strings.Builder
	Print(&out, p)
	if !strings.Contains(out.String(), "main is red: merge-on-green rows land branch-only") {
		t.Fatalf("the plan does not name the red main:\n%s", out.String())
	}
	sh.Results["make check"] = effects.ShellResult{Exit: 0, Last: "ok"}
	out.Reset()
	p, _ = BuildPlan(fx, q, Options{Repo: repo, State: "/state2"})
	if Print(&out, p); strings.Contains(out.String(), "main is red") {
		t.Fatalf("a green main printed red:\n%s", out.String())
	}
}

// TestAR13_1_AnchorCarriesHead (R6 §4): every check the plan measured on
// main carries the head it was measured at; an unrun one carries none.
func TestAR13_1_AnchorCarriesHead(t *testing.T) {
	repo, q, fx, _, _ := memoPlan(t)
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if v := p.Verify; v == nil || v.Head != "aaa111" {
		t.Fatalf("verify %+v, want head aaa111", v)
	}
	n := 0
	for _, r := range p.Rows {
		for _, c := range r.Baseline {
			switch {
			case c.Status == relay.CheckUnrun && c.Head != "":
				t.Fatalf("row %s: an unrun check carries head %q", r.ID, c.Head)
			case c.Status != relay.CheckUnrun && c.Head != "aaa111":
				t.Fatalf("row %s: check %+v, want head aaa111", r.ID, c)
			case c.Status != relay.CheckUnrun:
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("no check measured")
	}
	// The second plan reads the memo: the heads stand.
	p, _ = BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if v := p.Verify; v == nil || v.Head != "aaa111" {
		t.Fatalf("verify from the memo %+v", v)
	}
}
