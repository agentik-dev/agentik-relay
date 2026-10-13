package effects

import (
	"strings"
	"testing"
)

// TestAR14_3_ChildCarriesSessionBudget: after the CLAUDE* drop, ChildEnv
// adds CLAUDE_SESSION_DEPTH=1 and CLAUDE_SESSION_MAX_TURNS=<turns> (S10's
// handover; §7); a parent's values never pass.
func TestAR14_3_ChildCarriesSessionBudget(t *testing.T) {
	in := []string{"HOME=/h", "CLAUDECODE=1", SessionTurnsVar + "=999", SessionDepthVar + "=3"}
	got := ChildEnv(in, "r1", "", "", 80, nil, "")
	want := []string{"HOME=/h", "RELAY_ROW=r1", MeasureDepthVar + "=1", SessionDepthVar + "=1", SessionTurnsVar + "=80"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ChildEnv =\n%q\nwant\n%q", got, want)
	}
	if SessionDepthVar != "CLAUDE_SESSION_DEPTH" || SessionTurnsVar != "CLAUDE_SESSION_MAX_TURNS" {
		t.Fatalf("the names are %s and %s", SessionDepthVar, SessionTurnsVar)
	}
}
