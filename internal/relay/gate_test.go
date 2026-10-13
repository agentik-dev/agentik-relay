package relay

import "testing"

// TestDecide has one row per branch of Decide, in §6's order; each row
// flips one fact of an otherwise green gate.
func TestDecide(t *testing.T) {
	green := func() Gate {
		return Gate{
			ExitOK: true, ResultSeen: true, Commit: true, Clean: true, OriginSame: true,
			Lines: []Check{
				{Name: "check", Status: CheckGreen},
				{Name: "as-main", Status: CheckSameAsMain},
				{Name: "skipped", Status: CheckSkipped},
			},
			Verify:   &Check{Name: "verify", Status: CheckSameAsMain},
			Gathered: true,
		}
	}
	table := []struct {
		name   string
		flip   func(*Gate)
		want   Result
		reason StopReason
	}{
		{"not gathered", func(g *Gate) { g.Gathered = false }, Unrun, ""},
		{"no result event", func(g *Gate) { g.ResultSeen = false }, FailedResult, StreamError},
		{"is_error", func(g *Gate) { g.ExitOK = false }, FailedResult, StreamError},
		{"no commit", func(g *Gate) { g.Commit = false }, FailedResult, NoCommit},
		{"dirty tree", func(g *Gate) { g.Clean = false }, FailedResult, DirtyTree},
		{"orphans swept", func(g *Gate) { g.Orphans = 2 }, RedResult, ""},
		{"origin moved", func(g *Gate) { g.OriginSame = false }, FailedResult, OriginMoved},
		{"touched outside", func(g *Gate) { g.Outside = []string{"README.md"} }, RedResult, ""},
		{"line red", func(g *Gate) { g.Lines[0].Status = CheckRed }, RedResult, ""},
		{"line unrun", func(g *Gate) { g.Lines[0].Status = CheckUnrun }, RedResult, ""},
		{"verify red", func(g *Gate) { g.Verify.Status = CheckRed }, RedResult, ""},
		{"verify lost", func(g *Gate) { g.Verify.Status = CheckLost }, RedResult, ""},
		{"no verify line", func(g *Gate) { g.Verify = nil }, GreenResult, ""},
		{"green", func(g *Gate) {}, GreenResult, ""},
	}
	for _, r := range table {
		t.Run(r.name, func(t *testing.T) {
			g := green()
			r.flip(&g)
			if got, reason := Decide(g); got != r.want || reason != r.reason {
				t.Fatalf("Decide = %q, %q; want %q, %q", got, reason, r.want, r.reason)
			}
		})
	}
	// Order: an ungathered gate is unrun whatever else it holds, and an
	// earlier failure outranks a later one.
	if got, _ := Decide(Gate{}); got != Unrun {
		t.Errorf("Decide(Gate{}) = %q; want unrun", got)
	}
	g := green()
	g.Commit, g.Clean, g.Outside = false, false, []string{"x"}
	if got, reason := Decide(g); got != FailedResult || reason != NoCommit {
		t.Errorf("no commit and dirty = %q, %q; want failed, no-commit", got, reason)
	}
}
