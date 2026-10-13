package relay

import (
	"slices"
	"testing"
)

// TestAR17A_0_DecideMainMoved: a moved base is red, main-moved, after the
// sitting failures and before the touch set; never a night stop.
func TestAR17A_0_DecideMainMoved(t *testing.T) {
	g := Gate{Gathered: true, ResultSeen: true, ExitOK: true, Commit: true, Clean: true, OriginSame: true, MainMoved: true}
	if res, why := Decide(g); res != RedResult || why != MainMoved {
		t.Fatalf("Decide = %s %s, want red main-moved", res, why)
	}
	g.OriginSame = false
	if res, why := Decide(g); res != FailedResult || why != OriginMoved {
		t.Fatalf("Decide = %s %s, want failed origin-moved first", res, why)
	}
	if slices.Contains(StopReasons, MainMoved) {
		t.Fatal("main-moved is a stop reason; it is a red row's")
	}
}
