package gate

import (
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// stageRow stages row id off base through night.Stage and commits touch in
// it; stage is stageRow for a1 off main.
func (w *world) stageRow(t *testing.T, id, base string, touch ...string) *night.Staged {
	t.Helper()
	tree := filepath.Join(w.root, ".claude", "worktrees", id)
	st, err := night.Stage(w.fx, &night.RowPlan{ID: id, Worktree: tree, Branch: id, Base: base})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Unlock() })
	for _, f := range touch {
		write(t, tree, f, id+" "+f+"\n")
	}
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-qm", id+"'s work")
	return st
}

// TestAR20_1_MainBoundEveryRow is ADV11's second test: a forged `merged`
// ledger line, and a row whose base is not main, each leave a moved main
// main-moved, and nothing merges (ADV11-1, -2). The relay's own merge moves
// main for a row staged in the same process; one read back from the
// ledger binds main now (AR21 act 5); merge lines carry the root (ADV11-17).
func TestAR20_1_MainBoundEveryRow(t *testing.T) {
	t.Run("forged-merged-line", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stage(t, "a.txt")
		gitIn(t, w.tree, "update-ref", "refs/heads/main", "HEAD")
		evil := gitIn(t, w.root, "rev-parse", "main")
		w.ledger.Append("merge", map[string]any{"row": "x", "result": "merged", "sha": evil})
		res, why, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
		if res != relay.RedResult || string(why) != "main-moved" {
			t.Fatalf("Run = %s %q, want red main-moved; gate %+v", res, why, g)
		}
		gitIn(t, w.root, "reset", "-q", "--hard", evil)
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || w.mergedRow("a1") {
			t.Fatalf("merged past a forged ledger line: %v", err)
		}
	})
	t.Run("base-not-main", func(t *testing.T) {
		w := hermeticWorld(t)
		gitIn(t, w.root, "branch", "b0", "main")
		st := w.stageRow(t, "a1", "b0", "b.txt")
		if st.MainSha == "" || st.MainSha != gitIn(t, w.root, "rev-parse", "main") {
			t.Fatalf("MainSha %q not recorded at stage", st.MainSha)
		}
		tree := filepath.Join(w.root, ".claude", "worktrees", "a1")
		gitIn(t, tree, "update-ref", "refs/heads/main", "HEAD")
		gitIn(t, w.root, "reset", "-q", "--hard", "main")
		res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.RedResult || string(why) != "main-moved" {
			t.Fatalf("Run = %s %q, want red main-moved; gate %+v", res, why, g)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || w.merged() {
			t.Fatalf("merged after main moved under a base not main: %v", err)
		}
	})
	t.Run("own-merge", func(t *testing.T) {
		w := hermeticWorld(t)
		a1 := w.stageRow(t, "a1", "main", "b.txt")
		a2 := w.stageRow(t, "a2", "main", "c.txt")
		res, _, g, _ := Run(w.fx, a1, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("a1 = %s; gate %+v", res, g)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err != nil {
			t.Fatal(err)
		}
		for _, l := range w.ledger.Written() {
			if l["event"] == "merge" && l["root"] != w.root {
				t.Fatalf("merge line %v: root %v, want %s", l, l["root"], w.root)
			}
		}
		if res, why, g, _ := Run(w.fx, a2, pk([]string{"c.txt"}), nil, seen, Verify{}); res != relay.GreenResult {
			t.Fatalf("a2 after the relay's own merge = %s %q; gate %+v", res, why, g)
		}
		// The same row read back from the ledger, as an earlier process
		// staged it: its main_sha is a claim, never a binding; main is
		// bound now, so the relay's merge since is no move (AR21 act 5).
		back := night.StagedFromLedger(w.ledger.Written(), "a2")
		back.Path = a2.Path
		if res, why, g, _ := Run(w.fx, back, pk([]string{"c.txt"}), nil, seen, Verify{}); res != relay.GreenResult || g.MainMoved {
			t.Fatalf("a2 from the ledger = %s %q, want green, main bound now; gate %+v", res, why, g)
		}
	})
}

// mergedRow reports whether the ledger holds a `merge` line `merged` for row.
func (w *world) mergedRow(row string) bool {
	for _, l := range w.ledger.Written() {
		if l["event"] == "merge" && l["result"] == "merged" && l["row"] == row {
			return true
		}
	}
	return false
}
