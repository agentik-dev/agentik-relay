package gate

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR53_4_DetailNamesTheEntry: a row is staged, a hook is planted in
// the common dir, and the gate is unrun, git-state-moved, its detail
// naming hooks and no other entry; the next stage's refusal names it too
// (AR53 act 2).
func TestAR53_4_DetailNamesTheEntry(t *testing.T) {
	w := hermeticWorld(t)
	st := w.stageRow(t, "a1", "main", "b.txt")
	common := gitIn(t, w.root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	hook(t, filepath.Join(common, "hooks"), "pre-commit", "true\n")
	res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
	if res != relay.Unrun || why != relay.GitStateMoved {
		t.Fatalf("Run = %s %q; gate %+v; want unrun git-state-moved", res, why, g)
	}
	detail := ""
	for _, l := range w.ledger.Written() {
		if d, ok := l["detail"].(string); ok && l["event"] == "gate" && l["check"] == nil {
			detail = d
		}
	}
	if want := "git-state-moved: hooks changed since stage"; !strings.Contains(detail, want) {
		t.Errorf("the gate's detail = %q; want it to hold %q", detail, want)
	}
	tree := filepath.Join(w.root, ".claude", "worktrees", "a2")
	st2, err := night.Stage(w.fx, &night.RowPlan{ID: "a2", Worktree: tree, Branch: "a2", Base: "main"})
	if err == nil {
		st2.Unlock()
	}
	if want := "git-state-moved: hooks changed since the root's first stage"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("the next stage = %v; want it to hold %q", err, want)
	}
}
