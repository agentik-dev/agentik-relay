package gate

import (
	"errors"

	"github.com/agentik-dev/agentik-relay/internal/relay"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// AR2-1: a hand gate of a1 never discards row gate-a1's live tree, which
// sits at the path Hand would use for its temporary worktree.
func TestAR2_1_HandKeepsAnotherRowsTree(t *testing.T) {
	w := newWorld(t)
	tree := filepath.Join(w.root, ".claude", "worktrees", "gate-a1")
	st, err := night.Stage(w.fx, &night.RowPlan{ID: "gate-a1", Worktree: tree, Branch: "gate-a1", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Unlock()
	write(t, tree, "wip.txt", "uncommitted\n")
	gitIn(t, w.root, "branch", "a1")
	if _, _, _, err := Hand(w.fx, w.root, Verify{}, queue.Row{ID: "a1", Base: "main", Packet: pk([]string{"a.txt"})}); err == nil {
		t.Errorf("Hand ran over another row's tree")
	}
	if _, err := os.Stat(filepath.Join(tree, "wip.txt")); err != nil {
		t.Fatalf("row gate-a1's tree was discarded: %v", err)
	}
}

// AR2-2: a hand gate never runs its lines inside a tree a sitting holds.
func TestAR2_2_HandRefusesLockedTree(t *testing.T) {
	w := newWorld(t)
	w.stage(t, "a.txt")
	p := pk([]string{"a.txt"}, packet.Line{Shell: "touch hand-was-here", Kind: packet.Exits, Value: "0"})
	if _, _, _, err := Hand(w.fx, w.root, Verify{}, queue.Row{ID: "a1", Base: "main", Packet: p}); err == nil {
		t.Errorf("Hand gated a locked tree")
	}
	if _, err := os.Stat(filepath.Join(w.tree, "hand-was-here")); err == nil {
		t.Fatalf("Hand ran a line inside a locked sitting's tree")
	}
}

// AR2-3: §6's merge needs the root on main; a root on another branch leaves
// the row branch-only.
func TestAR2_3_MergeNeedsRootOnMain(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	gitIn(t, w.root, "checkout", "-q", "-b", "side")
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	if sha, err := MergeIfGreen(w.fx, w.root, green, "a1", ""); err == nil {
		t.Errorf("merged into side at %s", sha)
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("a refused merge moved side to %s", head)
	}
}

// AR2-4: a gate whose ledger lines were not written is never green (§3 reads
// the `gate` result line).
func TestAR2_4_LedgerFailureIsNotGreen(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	w.ledger.Err = errors.New("disk full")
	if res, _, _, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{}); res == relay.GreenResult {
		t.Fatalf("green with no gate line written")
	}
}

// TestAR7_HandGateUnrunNamesCause: a gather that stops short is unrun, and
// Hand hands back its cause (here the commit fact: no such base), so `relay
// gate` prints `unrun: <cause>`; a lost gate line is Run's error too.
func TestAR7_HandGateUnrunNamesCause(t *testing.T) {
	w := newWorld(t)
	gitIn(t, w.root, "branch", "a1")
	res, _, _, err := Hand(w.fx, w.root, Verify{}, queue.Row{ID: "a1", Base: "nosuch", Packet: pk([]string{"a.txt"})})
	if res != relay.Unrun || err == nil || !strings.HasPrefix(err.Error(), "commit: ") {
		t.Fatalf("Hand over a missing base: %s, %v; want unrun naming the commit fact", res, err)
	}
	w = newWorld(t)
	st := w.stage(t, "a.txt")
	w.ledger.Err = errors.New("disk full")
	if res, _, _, err := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{}); res != relay.Unrun || err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("a lost gate line: %s, %v", res, err)
	}
}

// TestAR7_GateFillsVerify: the gate runs the verify line in the worktree
// after the acceptance lines and fills g.Verify: green on exit 0,
// same-as-main against the plan's baseline, red otherwise, skipped with no
// line; each writes a `gate` line `check: verify`.
func TestAR7_GateFillsVerify(t *testing.T) {
	failing := &relay.Check{Name: "echo boom; exit 3", MainExit: 3, MainLast: "boom", Status: relay.CheckRed}
	for _, tc := range []struct {
		v      Verify
		status string
		res    relay.Result
	}{
		{Verify{}, relay.CheckSkipped, relay.GreenResult},
		{Verify{Name: "test -f a.txt"}, relay.CheckGreen, relay.GreenResult},
		{Verify{Name: "echo boom; exit 3", Baseline: failing}, relay.CheckSameAsMain, relay.GreenResult},
		{Verify{Name: "echo boom; exit 3"}, relay.CheckRed, relay.RedResult},
		{Verify{Name: "echo bang; exit 3", Baseline: failing}, relay.CheckRed, relay.RedResult},
	} {
		w := newWorld(t)
		if tc.v.Baseline != nil {
			tc.v.Baseline.Head = gitIn(t, w.root, "rev-parse", "main") // measured at main's head (R6 §4)
		}
		st := w.stage(t, "a.txt")
		res, _, g, err := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, tc.v)
		if err != nil || res != tc.res || g.Verify == nil || g.Verify.Status != tc.status {
			t.Fatalf("verify %q: %s %v, g.Verify %+v; want %s %s", tc.v.Name, res, err, g.Verify, tc.res, tc.status)
		}
		var line map[string]any
		for _, l := range w.ledger.Written() {
			if l["event"] == "gate" && l["check"] == "verify" {
				line = l
			}
		}
		if line == nil || line["status"] != tc.status {
			t.Fatalf("verify %q: gate line %v", tc.v.Name, line)
		}
	}
}
