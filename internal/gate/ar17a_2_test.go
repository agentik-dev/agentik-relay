package gate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// harnessWorld is a world whose main's `make check` holds a.txt to `a`.
func harnessWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld(t)
	write(t, w.root, "Makefile", "check:\n\tgrep -qx a a.txt\n")
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "main's harness")
	return w
}

// TestAR17A_2_BranchCannotNeuterVerify: a branch that breaks a.txt and
// rewrites its Makefile's check to `true` (named in may-touch) is judged
// under main's Makefile: the gate's verify is red and the merge refused. A
// branch that changes the harness harmlessly merges, its merge line noting
// `harness changed` (ADV10-10). Makefile and scripts/ are guarded: `*`
// never admits them.
func TestAR17A_2_BranchCannotNeuterVerify(t *testing.T) {
	w := harnessWorld(t)
	st := w.stage(t, "a.txt")
	write(t, w.tree, "Makefile", "check:\n\ttrue\n")
	gitIn(t, w.tree, "commit", "-qam", "neuter")
	may := pk([]string{"a.txt", "Makefile"})
	res, _, g, _ := Run(w.fx, st, may, nil, seen, Verify{Name: "auto"})
	if res != relay.RedResult || g.Verify == nil || g.Verify.Status != relay.CheckRed {
		t.Fatalf("Run = %s, verify %+v; want red under main's Makefile", res, g.Verify)
	}
	res, _, g, _ = Run(w.fx, st, may, nil, seen, Verify{})
	if res != relay.GreenResult {
		t.Fatalf("Run with no verify = %s", res)
	}
	if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
		t.Fatalf("merged a neutered verify: %v", err)
	}

	w = harnessWorld(t)
	st = w.stage(t, "b.txt")
	write(t, w.tree, "Makefile", "# a comment\ncheck:\n\tgrep -qx a a.txt\n")
	write(t, w.tree, "scripts/x", "x\n")
	gitIn(t, w.tree, "add", ".")
	gitIn(t, w.tree, "commit", "-qm", "harmless")
	_, _, g, _ = Run(w.fx, st, pk([]string{"b.txt", "*"}), nil, seen, Verify{Name: "auto"})
	if !reflect.DeepEqual(g.Outside, []string{"Makefile", "scripts/x"}) {
		t.Fatalf("outside %q under `*`; want the harness named", g.Outside)
	}
	res, _, g, _ = Run(w.fx, st, pk([]string{"b.txt", "Makefile", "scripts/"}), nil, seen, Verify{Name: "auto"})
	if res != relay.GreenResult {
		t.Fatalf("Run = %s, gate %+v", res, g)
	}
	if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err != nil || !w.merged() {
		t.Fatalf("merge = %v", err)
	}
	for _, l := range w.ledger.Written() {
		if l["event"] == "merge" && !strings.Contains(l["detail"].(string), "harness changed") {
			t.Fatalf("merge line %v; want harness changed", l)
		}
	}
}
