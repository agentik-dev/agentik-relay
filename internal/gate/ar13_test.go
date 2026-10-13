package gate

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR13_0_GateBindsBranchSha (ADV6-3, -4): the gate judges the row's
// branch at one sha. A worktree moved off the row's branch is red even when
// its own HEAD passes every line; a rename out of an unlisted path lists
// the source (--no-renames); and the merge lands the sha the gate measured,
// refusing a branch tip that moved after it (`merge` line `moved`).
func TestAR13_0_GateBindsBranchSha(t *testing.T) {
	// Off the branch: a1 fails its line, the checked-out evil passes it.
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	gitIn(t, w.tree, "checkout", "-q", "-b", "evil")
	write(t, w.tree, "a.txt", "evil\n")
	gitIn(t, w.tree, "commit", "-qam", "evil")
	p := pk([]string{"a.txt"}, packet.Line{Shell: "cat a.txt", Expect: "reads `evil`", Kind: packet.Reads, Value: "evil"})
	if res, _, g, _ := Run(w.fx, st, p, nil, seen, Verify{}); res == relay.GreenResult {
		t.Fatalf("a worktree off its row's branch gated green: %+v", g)
	}

	// A rename hides nothing: the source is outside may-touch.
	w = newWorld(t)
	st = w.stage(t, "allowed/b.txt")
	gitIn(t, w.tree, "mv", "a.txt", "allowed/a.txt")
	gitIn(t, w.tree, "commit", "-qm", "move")
	g := Gather(w.fx, st, pk([]string{"allowed/"}), nil, seen)
	if !reflect.DeepEqual(g.Outside, []string{"a.txt"}) {
		t.Fatalf("a rename out of a.txt: outside %v, want [a.txt]", g.Outside)
	}

	// The merge lands the gated sha, never a tip that moved after it.
	w = newWorld(t)
	st = w.stage(t, "a.txt")
	green := Gather(w.fx, st, pk([]string{"a.txt", "b.txt"}), nil, seen)
	gated := gitIn(t, w.root, "rev-parse", "a1")
	if res, _ := relay.Decide(green); res != relay.GreenResult || green.Head != gated {
		t.Fatalf("gate %s bound %q, want green at %s", res, green.Head, gated)
	}
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	write(t, w.tree, "b.txt", "late\n")
	gitIn(t, w.tree, "add", ".")
	gitIn(t, w.tree, "commit", "-qm", "after the gate")
	if sha, err := MergeIfGreen(w.fx, w.root, green, "a1", ""); err == nil {
		t.Fatalf("a moved tip merged as %s", sha)
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("a refused merge moved the root to %s", head)
	}
	ws := w.ledger.Written()
	if l := ws[len(ws)-1]; l["event"] != "merge" || l["result"] != "moved" {
		t.Fatalf("the last line is %v, want a merge line moved", l)
	}
	green = Gather(w.fx, st, pk([]string{"a.txt", "b.txt"}), nil, seen)
	sha, err := MergeIfGreen(w.fx, w.root, green, "a1", "")
	if err != nil {
		t.Fatal(err)
	}
	if parents := strings.Fields(gitIn(t, w.root, "rev-list", "--parents", "-n1", sha)); len(parents) != 3 || parents[2] != green.Head {
		t.Fatalf("the merge %v does not land the gated %s", parents, green.Head)
	}
}

// probeFails is the machine's shell except the verify probe, which cannot
// start.
type probeFails struct{ effects.Shell }

func (s probeFails) Run(dir, line string, timeout time.Duration) (int, string, error) {
	if strings.Contains(line, "make -n check") {
		return -1, "", errors.New("the probe could not start")
	}
	return s.Shell.Run(dir, line, timeout)
}

// TestAR13_0_VerifyResolvedOnMain (ADV6-10): `verify: auto` is resolved on
// main, never in the row's tree: a branch that deletes the Makefile runs
// `make check` and is red; a root with no `check:` is skipped; a probe that
// cannot start is unrun, never a pass.
func TestAR13_0_VerifyResolvedOnMain(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make")
	}
	w := newWorld(t)
	write(t, w.root, "Makefile", "check:\n\t@echo main red; exit 1\n")
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "a check")
	st := w.stage(t, "a.txt")
	gitIn(t, w.tree, "rm", "-q", "Makefile")
	gitIn(t, w.tree, "commit", "-qm", "no check here")
	res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt", "Makefile"}), nil, seen, Verify{Name: "auto"})
	if res == relay.GreenResult || g.Verify == nil || g.Verify.Name != "make check" || g.Verify.Status != relay.CheckRed {
		t.Fatalf("a branch without the Makefile = %s, verify %+v", res, g.Verify)
	}

	w = newWorld(t)
	st = w.stage(t, "a.txt")
	res, _, g, _ = Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{Name: "auto"})
	if res != relay.GreenResult || g.Verify.Status != relay.CheckSkipped {
		t.Fatalf("a root without check: = %s, verify %+v", res, g.Verify)
	}
	w.fx.Shell = probeFails{w.fx.Shell}
	res, _, g, _ = Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{Name: "auto"})
	if res == relay.GreenResult || g.Verify.Status != relay.CheckUnrun {
		t.Fatalf("a failed probe = %s, verify %+v", res, g.Verify)
	}
}

// TestAR13_1_RedMainNeverMerges (ADV6-5; R6 §4): the merge's verify passes
// green only. A verify line red on main at the root's head refuses the merge
// before it starts (a `merge` line noting `main red`), even when the merged
// tree fails it exactly as main does; a stale red anchor merges nothing
// either: the merged tree's verify must exit 0.
func TestAR13_1_RedMainNeverMerges(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	verify := "echo FAIL pkg/a; exit 2"
	red := &relay.Check{Name: verify, MainExit: 2, MainLast: "FAIL pkg/a", Status: relay.CheckRed, Head: before}
	if sha, err := MergeIfGreen(w.fx, w.root, green, "a1", verify, red); err == nil {
		t.Fatalf("a red main merged %s as same-as-main", sha)
	}
	ws := w.ledger.Written()
	if l := ws[len(ws)-1]; l["event"] != "merge" || l["result"] != "branch-only" || !strings.Contains(l["detail"].(string), "main red") {
		t.Fatalf("the last line is %v, want a merge line branch-only noting main red", l)
	}
	stale := &relay.Check{Name: verify, MainExit: 2, MainLast: "FAIL pkg/a", Status: relay.CheckRed, Head: "0000000"}
	if sha, err := MergeIfGreen(w.fx, w.root, green, "a1", verify, stale); err == nil {
		t.Fatalf("a red verify merged %s against a stale anchor", sha)
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("a red main moved the root to %s", head)
	}
}

// TestAR13_1_AnchorCarriesHead (R6 §4): a baseline on main carries the head
// it was measured at and is compared only there. Main moved: the memo at
// main's head answers, and a miss is `unrun: stale-baseline`, never
// same-as-main against the old head.
func TestAR13_1_AnchorCarriesHead(t *testing.T) {
	w := newWorld(t)
	m0 := gitIn(t, w.root, "rev-parse", "HEAD")
	fails := packet.Line{Shell: "echo broken; exit 3", Expect: "as main", Kind: packet.AsMain}
	base := night.BaselineOnMain(w.fx, w.root, []packet.Line{fails})
	if base[0].Head != m0 || base[0].MainExit != 3 {
		t.Fatalf("the baseline %+v, want exit 3 at %s", base[0], m0)
	}
	write(t, w.root, "c.txt", "c\n")
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "main moves")
	m1 := gitIn(t, w.root, "rev-parse", "HEAD")
	st := w.stage(t, "a.txt")
	vbase := &relay.Check{Name: "echo boom; exit 3", MainExit: 3, MainLast: "boom", Status: relay.CheckRed, Head: m0}
	res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt"}, fails), base, seen, Verify{Name: vbase.Name, Baseline: vbase})
	if res == relay.GreenResult || g.Lines[0].Status != relay.CheckUnrun || g.Lines[0].MainLast != night.StaleReason ||
		g.Verify.Status != relay.CheckUnrun || g.Verify.MainLast != night.StaleReason {
		t.Fatalf("a baseline at %s gated at %s: %s, line %+v, verify %+v", m0, m1, res, g.Lines[0], g.Verify)
	}

	// The memo at main's head answers for the stale anchor.
	state := t.TempDir()
	w.fx.Getenv = func(k string) string {
		if k == "XDG_STATE_HOME" {
			return state
		}
		return ""
	}
	if b := night.BaselineOnMain(w.fx, w.root, []packet.Line{fails, {Shell: vbase.Name, Expect: "as main", Kind: packet.AsMain}}); b[0].Head != m1 {
		t.Fatalf("the baseline at the moved main %+v", b[0])
	}
	res, _, g, _ = Run(w.fx, st, pk([]string{"a.txt"}, fails), base, seen, Verify{Name: vbase.Name, Baseline: vbase})
	if res != relay.GreenResult || g.Lines[0].Status != relay.CheckSameAsMain || g.Lines[0].MainExit != 3 ||
		g.Verify.Status != relay.CheckSameAsMain {
		t.Fatalf("the memo at %s: %s, line %+v, verify %+v", m1, res, g.Lines[0], g.Verify)
	}
}
