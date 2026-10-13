package gate

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// merged reports whether the ledger holds a `merge` line `merged`.
func (w *world) merged() bool {
	for _, l := range w.ledger.Written() {
		if l["event"] == "merge" && l["result"] == "merged" {
			return true
		}
	}
	return false
}

// TestAR17A_0_GateBindsBase is ADV10's first test: a child moves main, then
// tags main; each gate is red with its reason and nothing merges. A main
// that moves after a green gate refuses the merge, main-moved.
func TestAR17A_0_GateBindsBase(t *testing.T) {
	t.Run("update-ref", func(t *testing.T) {
		w := newWorld(t)
		st := w.stage(t, "a.txt")
		gitIn(t, w.tree, "update-ref", "refs/heads/main", "HEAD") // from the linked worktree
		res, why, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
		if res != relay.RedResult || string(why) != "main-moved" {
			t.Fatalf("Run = %s %q, want red main-moved; gate %+v", res, why, g)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || w.merged() {
			t.Fatalf("merged after main moved: %v", err)
		}
	})
	t.Run("tag", func(t *testing.T) {
		w := newWorld(t)
		st := w.stage(t, "a.txt", "secret/a")
		gitIn(t, w.tree, "tag", "main", "HEAD")
		res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
		if res != relay.RedResult || !slices.Contains(g.Outside, "secret/a") {
			t.Fatalf("Run = %s, outside %q; want red naming secret/a", res, g.Outside)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || w.merged() {
			t.Fatalf("merged past a tag named main: %v", err)
		}
	})
	t.Run("after the gate", func(t *testing.T) {
		w := newWorld(t)
		st := w.stage(t, "a.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s, want green; gate %+v", res, g)
		}
		gitIn(t, w.root, "commit", "-q", "--allow-empty", "-m", "main moves")
		_, err := MergeIfGreen(w.fx, w.root, g, "a1", "")
		if err == nil || !strings.Contains(err.Error(), "main-moved") || w.merged() {
			t.Fatalf("merge after main moved = %v", err)
		}
	})
}

// TestAR17A_0_UrlPushSeen: a push by URL moves no tracking ref, so only
// stage's ls-remote snapshot sees it: failed, origin-moved (ADV10-3).
func TestAR17A_0_UrlPushSeen(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	remote := filepath.Join(filepath.Dir(w.root), "remote.git")
	gitIn(t, w.tree, "-c", "core.hooksPath=/dev/null", "push", "-q", remote, "HEAD:refs/heads/x")
	res, why, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
	if res != relay.FailedResult || why != relay.OriginMoved {
		t.Fatalf("Run = %s %s, want failed origin-moved; gate %+v", res, why, g)
	}
}
