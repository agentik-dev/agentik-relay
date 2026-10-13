package gate

import (
	"errors"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

const stopVerify = "echo 'bounded: stop: wall' >&2; exit 124"

// TestAR18_2_VerifyStopUnrun (conventions §5): a verify line bounded stopped
// is unrun with the stop line as its last, never red; the gate is not green.
func TestAR18_2_VerifyStopUnrun(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{Name: stopVerify})
	if g.Verify == nil || g.Verify.Status != relay.CheckUnrun || g.Verify.BranchLast != "bounded: stop: wall" {
		t.Fatalf("verify = %+v, want unrun: bounded: stop: wall", g.Verify)
	}
	if res == relay.GreenResult {
		t.Fatalf("a stopped verify gated green")
	}
}

// TestAR18_2_MergeStopHeld: the merged tree's verify stopped by bounded
// merges nothing and is never red: the error is a *BoundedStop, the root
// does not move.
func TestAR18_2_MergeStopHeld(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	base := &relay.Check{Name: stopVerify, Expect: "exit 0", MainExit: 0, Status: relay.CheckGreen, Head: before}
	sha, err := MergeIfGreen(w.fx, w.root, green, "a1", stopVerify, base)
	var stop *effects.BoundedStop
	if err == nil || !errors.As(err, &stop) || stop.Line != "bounded: stop: wall" {
		t.Fatalf("merge = %q, %v; want no merge, a BoundedStop", sha, err)
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("a stopped verify moved the root to %s", head)
	}
}
