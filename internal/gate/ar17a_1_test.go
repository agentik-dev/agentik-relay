package gate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR17A_1_SkipWorktreeCannotFake: the child's tree holds `faked` in
// a.txt behind `update-index --skip-worktree`, so status is clean; the
// commit holds the real text. The gate reads the commit: red (ADV10-7).
func TestAR17A_1_SkipWorktreeCannotFake(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	write(t, w.tree, "a.txt", "faked\n")
	gitIn(t, w.tree, "update-index", "--skip-worktree", "a.txt")
	if s := gitIn(t, w.tree, "status", "--porcelain"); s != "" {
		t.Fatalf("the trick left status %q", s)
	}
	p := pk([]string{"a.txt"}, packet.Line{Shell: "cat a.txt", Expect: "reads `faked`", Kind: packet.Reads, Value: "faked"})
	res, _, g, _ := Run(w.fx, st, p, nil, seen, Verify{})
	if res != relay.RedResult || g.Lines[0].BranchLast != "a1 a.txt" {
		t.Fatalf("Run = %s, line %+v; want red reading the commit's text", res, g.Lines)
	}
}

// TestAR17A_1_ExcludeCannotHide: a file the child keeps out of status by
// info/exclude is not in the commit, so no line the gate runs sees it; and
// since AR20 act 0 the write to info/ is itself unrun, git-state-moved.
func TestAR17A_1_ExcludeCannotHide(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	write(t, w.tree, "extra.sh", "echo helped\n")
	common := gitIn(t, w.tree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	f, err := os.OpenFile(filepath.Join(common, "info", "exclude"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("extra.sh\n")
	f.Close()
	p := pk([]string{"a.txt"}, packet.Line{Shell: "test -e extra.sh", Expect: "exits 0", Kind: packet.Exits, Value: "0"})
	res, why, g, _ := Run(w.fx, st, p, nil, seen, Verify{})
	if res != relay.Unrun || why != relay.GitStateMoved {
		t.Fatalf("Run = %s %q, lines %+v; want unrun git-state-moved", res, why, g.Lines)
	}
}
