package gate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

func nestedEnv(k string) string {
	return map[string]string{effects.MeasureDepthVar: "1"}[k]
}

// TestAR10_0_GateNestedRunsNothing: a gate under RELAY_MEASURE_DEPTH runs
// no acceptance line and no verify line: each is `unrun: nested`, the
// result is never green, and the line's side effect never happens.
func TestAR10_0_GateNestedRunsNothing(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	w.fx.Getenv = nestedEnv
	p := pk([]string{"a.txt"}, packet.Line{Shell: "touch gate-was-here", Expect: "exits 0", Kind: packet.Exits, Value: "0"})
	res, _, g, _ := Run(w.fx, st, p, nil, seen, Verify{Name: "touch verify-was-here"})
	if res == relay.GreenResult || len(g.Lines) != 1 {
		t.Fatalf("nested gate = %s, %d lines", res, len(g.Lines))
	}
	for _, c := range append(g.Lines, *g.Verify) {
		if c.Status != relay.CheckUnrun || c.BranchLast != effects.NestedReason || c.BranchExit != -1 {
			t.Fatalf("check %+v", c)
		}
	}
	for _, f := range []string{"gate-was-here", "verify-was-here"} {
		if _, err := os.Stat(filepath.Join(w.tree, f)); err == nil {
			t.Fatalf("a nested gate ran a line: %s exists", f)
		}
	}
}

// TestAR10_0_MergeNestedRefuses: MergeIfGreen under the guard merges
// nothing, even for a green gate: the verify line would be measured.
func TestAR10_0_MergeNestedRefuses(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	if res, _ := relay.Decide(green); res != relay.GreenResult {
		t.Fatalf("fixture gate = %s", res)
	}
	w.fx.Getenv = nestedEnv
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	if sha, err := MergeIfGreen(w.fx, w.root, green, "a1", ""); err == nil {
		t.Fatalf("a nested merge landed %s", sha)
	}
	if after := gitIn(t, w.root, "rev-parse", "HEAD"); after != before {
		t.Fatalf("the root moved %s -> %s", before, after)
	}
}

// TestAR10_7_GateInvariantFailsGate: an acceptance line that dirties the
// tree after the gate read it clean breaks clean-is-fresh: the result is
// failed invariant, never green, and the result line carries `stop:
// invariant` and `invariant: clean-is-fresh`. The same gate with a clean
// line is green with no invariant.
func TestAR10_7_GateInvariantFailsGate(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	res, why, _, err := Run(w.fx, st, pk([]string{"a.txt"}, packet.Line{Shell: "true", Expect: "exits 0", Kind: packet.Exits, Value: "0"}), nil, seen, Verify{})
	if res != relay.GreenResult || err != nil {
		t.Fatalf("clean gate = %s %s %v", res, why, err)
	}
	// The lines run in a fresh tree (AR17A act 1): the dirt is written into
	// the row's own tree, by its path.
	p := pk([]string{"a.txt"}, packet.Line{Shell: "touch " + w.tree + "/dirt", Expect: "exits 0", Kind: packet.Exits, Value: "0"})
	res, why, _, err = Run(w.fx, st, p, nil, seen, Verify{})
	if res != relay.FailedResult || why != relay.Invariant || err != nil {
		t.Fatalf("dirtying gate = %s %s %v", res, why, err)
	}
	ws := w.ledger.Written()
	end := ws[len(ws)-1]
	if end["result"] != "failed" || end["reason"] != "invariant" || end["stop"] != "invariant" || end["invariant"] != relay.InvClean {
		t.Fatalf("result line %v", end)
	}
}
