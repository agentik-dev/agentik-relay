package relay

import (
	"slices"
	"testing"
)

// TestAR59_3_DecideVerifyCut is AR59 act 3: a gate whose verify line was
// cut is red, verify-cut, even with a red acceptance line beside it; an
// uncut red verify reads no reason; main-moved, a named reason, is kept
// ahead of it; verify-cut is never a night stop.
func TestAR59_3_DecideVerifyCut(t *testing.T) {
	g := Gate{Gathered: true, ResultSeen: true, ExitOK: true, Commit: true, Clean: true, OriginSame: true,
		Verify: &Check{Status: CheckRed, BranchExit: 124, Cut: 900}}
	if res, why := Decide(g); res != RedResult || why != VerifyCut {
		t.Fatalf("a cut verify: Decide = %s %s, want red verify-cut", res, why)
	}
	g.Lines = []Check{{Status: CheckRed}}
	if res, why := Decide(g); res != RedResult || why != VerifyCut {
		t.Fatalf("a cut verify and a red line: Decide = %s %s, want red verify-cut", res, why)
	}
	g.Lines, g.Verify.Cut = nil, 0
	if res, why := Decide(g); res != RedResult || why != "" {
		t.Fatalf("an uncut red verify: Decide = %s %q, want red with no reason", res, why)
	}
	g.Verify.Cut, g.MainMoved = 900, true
	if res, why := Decide(g); res != RedResult || why != MainMoved {
		t.Fatalf("main moved and a cut verify: Decide = %s %s, want red main-moved", res, why)
	}
	if VerifyCut != "verify-cut" || slices.Contains(StopReasons, VerifyCut) || slices.Contains(SittingFailures, VerifyCut) {
		t.Fatal("verify-cut is a stop reason or a sitting failure; it is a red row's")
	}
}
