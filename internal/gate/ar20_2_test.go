package gate

import (
	"reflect"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR20_2_GNUmakefileCannotNeuter (ADV11-4): a branch that breaks a.txt
// and adds a GNUmakefile whose check is `true` (named in may-touch) is
// judged under main's Makefile alone: make would read the branch's
// GNUmakefile first, so the overlay deletes the copy main lacks. The gate's
// verify is red and the merge refused; every make spelling is guarded, so
// `*` never admits one.
func TestAR20_2_GNUmakefileCannotNeuter(t *testing.T) {
	w := harnessWorld(t)
	st := w.stage(t, "a.txt")
	write(t, w.tree, "GNUmakefile", "check:\n\ttrue\n")
	gitIn(t, w.tree, "add", ".")
	gitIn(t, w.tree, "commit", "-qm", "neuter by spelling")
	_, _, g, _ := Run(w.fx, st, pk([]string{"a.txt", "*"}), nil, seen, Verify{})
	if !reflect.DeepEqual(g.Outside, []string{"GNUmakefile"}) {
		t.Fatalf("outside %q under `*`; want GNUmakefile named", g.Outside)
	}
	may := pk([]string{"a.txt", "GNUmakefile"})
	res, _, g, _ := Run(w.fx, st, may, nil, seen, Verify{Name: "auto"})
	if res != relay.RedResult || g.Verify == nil || g.Verify.Status != relay.CheckRed {
		t.Fatalf("Run = %s, verify %+v; want red under main's Makefile", res, g.Verify)
	}
	res, _, g, _ = Run(w.fx, st, may, nil, seen, Verify{})
	if res != relay.GreenResult {
		t.Fatalf("Run with no verify = %s", res)
	}
	if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
		t.Fatalf("merged under the branch's GNUmakefile: %v", err)
	}
}

// TestAR20_2_HarnessListFromQueue (the gate's half): a row whose queue
// names `tests` in its harness is judged under main's tests/: a branch
// that rewrites tests/check.sh to `true` and breaks a.txt is red, `*`
// never admits tests/, and the merge is refused. With the default harness
// the same tests/ is the branch's own, so the row list is what binds it.
func TestAR20_2_HarnessListFromQueue(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "queue.yaml", "version: 1\ndefaults: {verify: auto}\nrows:\n  - id: a1\n    packet: a1.md\n    harness: [Makefile, tests]\n")
	q, err := queue.Load(dir, "queue.yaml")
	if err != nil {
		t.Fatal(err)
	}
	row := q.Rows[0]
	w := newWorld(t)
	write(t, w.root, "Makefile", "check:\n\tsh tests/check.sh\n")
	write(t, w.root, "tests/check.sh", "grep -qx a a.txt\n")
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "main's harness")
	st := w.stage(t, "a.txt")
	write(t, w.tree, "tests/check.sh", "true\n")
	gitIn(t, w.tree, "commit", "-qam", "neuter by tests/")
	v := Verify{Name: row.Verify, Harness: row.Harness}
	_, _, g, _ := Run(w.fx, st, pk([]string{"a.txt", "*"}), nil, seen, v)
	if !reflect.DeepEqual(g.Outside, []string{"tests/check.sh"}) {
		t.Fatalf("outside %q under `*`; want tests/ guarded", g.Outside)
	}
	may := pk([]string{"a.txt", "tests/"})
	res, _, g, _ := Run(w.fx, st, may, nil, seen, v)
	if res != relay.RedResult || g.Verify == nil || g.Verify.Status != relay.CheckRed {
		t.Fatalf("Run = %s, verify %+v; want red under main's tests/", res, g.Verify)
	}
	res, _, g, _ = Run(w.fx, st, may, nil, seen, Verify{Harness: row.Harness})
	if res != relay.GreenResult {
		t.Fatalf("Run with no verify = %s", res)
	}
	if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
		t.Fatalf("merged under the branch's tests/: %v", err)
	}
	// The default harness leaves tests/ the branch's: green (the list binds it).
	res, _, _, _ = Run(w.fx, st, may, nil, seen, Verify{Name: "auto"})
	if res != relay.GreenResult {
		t.Fatalf("default harness Run = %s; want green (tests/ not in it)", res)
	}
}
