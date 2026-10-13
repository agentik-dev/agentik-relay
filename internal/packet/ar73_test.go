package packet

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestAR73_1_AttendantReports (AR73 rulings 1 to 4): the attendant owes its
// caller an answer and so sits in the report set, not the exempt set, which
// goes empty (its cap rule stays in TestAR72_2 for a future exempt agent);
// its maxTurns is 35, its bound of 15 tool calls plus AR72's margin of 20.
// TestAR72_1 then holds its bound and its "never skipped" bullet. It reads a
// copy of plugin/agents.
func TestAR73_1_AttendantReports(t *testing.T) {
	dir := filepath.Join(copyIn(t, "../..", "plugin/agents"), "plugin", "agents")
	const name = "attendant"
	switch {
	case slices.Contains(ar72Exempt, name):
		t.Errorf("%s: in the exempt set, want the report set (AR73 ruling 1)", name)
	case !slices.Contains(ar72Report, name):
		t.Errorf("%s: not in the report set (AR73 ruling 1)", name)
	}
	if len(ar72Exempt) != 0 {
		t.Errorf("the exempt set holds %v, want it empty (AR73 ruling 3)", ar72Exempt)
	}
	fm, _ := ar72Agent(t, dir, name)
	if turns := ar72MaxTurns(t, name, fm); turns != 35 {
		t.Errorf("%s.md: maxTurns %d, want 35 (AR73 ruling 2)", name, turns)
	}
}
