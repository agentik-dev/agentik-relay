package gate

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar32bRow stages build row id off main, commits files (path to bytes) in
// it and gates it with may as its packet's may-touch: the result and the
// head the gate judged.
func ar32bRow(t *testing.T, w *world, id string, files map[string]string, may ...string) (relay.Result, string) {
	t.Helper()
	tree := filepath.Join(w.root, ".claude", "worktrees", id)
	st, err := night.Stage(w.fx, &night.RowPlan{ID: id, Worktree: tree, Branch: id, Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Unlock() })
	for f, b := range files {
		write(t, tree, f, b)
	}
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-qm", id+"'s work")
	p := pk(may)
	p.ID = id
	res, _, g, _ := Run(w.fx, st, p, nil, seen, Verify{})
	return res, g.Head
}

// ar32bMerge is an extra step for orchSitting: git merge --no-ff of the
// branches into the sitting's branch, as the sitting runs it.
func ar32bMerge(t *testing.T, branches ...string) func(string) {
	return func(tree string) {
		gitIn(t, tree, append([]string{"merge", "-q", "--no-ff", "-m", "merge " + strings.Join(branches, ", ")}, branches...)...)
	}
}

// gitTry is git in dir, its output and error, for a command meant to fail.
func gitTry(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	return string(out), err
}

const ar32bCharter = "history/sittings/2026-09-29-1-o.md"

// TestAR32B_1_MergedRowTipIsGatedHead is ar32b act 1 (orchestrator r1 §5):
// a green build-row gate notes its head with its row, an orchestrator gate
// notes none, and a merge on the sitting's first-parent line is proven
// only when its second of two parents is a head noted green in this root
// by this process; a helper's branch, a red row's head, main, a head noted
// in another root and an octopus red row-unproven.
func TestAR32B_1_MergedRowTipIsGatedHead(t *testing.T) {
	t.Run("a noted head", func(t *testing.T) {
		w, o := orchWorld(t, ar32aRows)
		res, head := ar32bRow(t, w, "b1", map[string]string{"b1.txt": "b1\n"}, "b1.txt")
		if res != relay.GreenResult || night.GreenHead(w.root, head) != "b1" {
			t.Fatalf("b1: %s, noted %q; want green, noted b1", res, night.GreenHead(w.root, head))
		}
		res, why, g := orchSitting(t, w, o, map[string]string{ar32bCharter: ar32aCharter}, ar32bMerge(t, "b1"))
		if res != relay.GreenResult || why != "" || g.Unproven {
			t.Errorf("%s, %q, unproven %v, %q; want green", res, why, g.Unproven, queueDetail(w))
		}
		if id := night.GreenHead(w.root, g.Head); id != "" {
			t.Errorf("the orchestrator's head is noted as %q; want none", id)
		}
	})
	for _, c := range []struct {
		name  string
		setup func(t *testing.T, w *world) func(string)
	}{
		{"a helper's branch", func(t *testing.T, w *world) func(string) {
			return func(tree string) {
				gitIn(t, tree, "checkout", "-q", "-b", "side", "HEAD~1")
				write(t, tree, "side.txt", "side\n")
				gitIn(t, tree, "add", ".")
				gitIn(t, tree, "commit", "-qm", "a helper's branch")
				gitIn(t, tree, "checkout", "-q", "o.1")
				ar32bMerge(t, "side")(tree)
			}
		}},
		{"a red row's head", func(t *testing.T, w *world) func(string) {
			if res, _ := ar32bRow(t, w, "b2", map[string]string{"b2.txt": "b2\n"}, "other.txt"); res != relay.RedResult {
				t.Fatalf("b2: %s; want red", res)
			}
			return ar32bMerge(t, "b2")
		}},
		{"main", func(t *testing.T, w *world) func(string) {
			return func(tree string) {
				write(t, w.root, "m.txt", "m\n")
				gitIn(t, w.root, "add", ".")
				gitIn(t, w.root, "commit", "-qm", "main moves")
				ar32bMerge(t, "main")(tree)
			}
		}},
		{"a head noted in another root", func(t *testing.T, w *world) func(string) {
			tree := filepath.Join(w.root, ".claude", "worktrees", "b3")
			gitIn(t, w.root, "worktree", "add", "-q", "-b", "b3", tree, "main")
			write(t, tree, "b3.txt", "b3\n")
			gitIn(t, tree, "add", ".")
			gitIn(t, tree, "commit", "-qm", "b3's work")
			night.NoteGreen(t.TempDir(), "b3", gitIn(t, tree, "rev-parse", "HEAD"))
			return ar32bMerge(t, "b3")
		}},
		{"an octopus", func(t *testing.T, w *world) func(string) {
			for _, id := range []string{"b1", "b3"} {
				if res, _ := ar32bRow(t, w, id, map[string]string{id + ".txt": id + "\n"}, id+".txt"); res != relay.GreenResult {
					t.Fatalf("%s: %s; want green", id, res)
				}
			}
			return ar32bMerge(t, "b1", "b3")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, o := orchWorld(t, ar32aRows)
			step := c.setup(t, w)
			res, why, g := orchSitting(t, w, o, map[string]string{ar32bCharter: ar32aCharter}, step)
			if res != relay.RedResult || !g.Unproven || why != relay.RowUnproven && why != relay.MainMoved {
				t.Errorf("%s, %q, unproven %v; want red, row-unproven", res, why, g.Unproven)
			}
		})
	}
}

// TestAR32B_1_EditInsideMergeRed is ar32b act 1 (§5): a merge of a noted
// head must equal its automatic result (git merge-tree of its parents): a
// line changed inside the merge commit, or a conflict resolved by hand,
// reds row-unproven.
func TestAR32B_1_EditInsideMergeRed(t *testing.T) {
	for _, c := range []struct {
		name string
		own  map[string]string
		step func(t *testing.T, tree string)
	}{
		{"a line changed inside the merge", nil, func(t *testing.T, tree string) {
			gitIn(t, tree, "merge", "-q", "--no-ff", "--no-commit", "b1")
			write(t, tree, "a.txt", "a, changed in the merge\n")
			gitIn(t, tree, "add", ".")
			gitIn(t, tree, "commit", "-q", "--no-edit")
		}},
		{"a conflict resolved by hand", map[string]string{"a.txt": "o\n"}, func(t *testing.T, tree string) {
			cmd := []string{"merge", "-q", "--no-ff", "--no-commit", "b1"}
			if out, err := gitTry(tree, cmd...); err == nil {
				t.Fatalf("the merge did not conflict: %s", out)
			}
			write(t, tree, "a.txt", "o and b1\n")
			gitIn(t, tree, "add", ".")
			gitIn(t, tree, "commit", "-q", "--no-edit")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, o := orchWorld(t, ar32aRows)
			if res, _ := ar32bRow(t, w, "b1", map[string]string{"a.txt": "b1\n"}, "a.txt"); res != relay.GreenResult {
				t.Fatalf("b1: %s; want green", res)
			}
			files := map[string]string{ar32bCharter: ar32aCharter}
			for k, v := range c.own {
				files[k] = v
			}
			res, why, g := orchSitting(t, w, o, files, func(tree string) { c.step(t, tree) })
			if res != relay.RedResult || why != relay.RowUnproven || !g.Unproven {
				t.Errorf("%s, %q, unproven %v; want red, row-unproven", res, why, g.Unproven)
			}
		})
	}
}

// TestAR32B_1_TouchExemptsProvenMerges is ar32b act 1 (§5): touch reads
// each first-parent commit against its first parent, the proven merges
// exempt: a row's guarded path that arrives through a proven merge holds,
// and the same path changed in the sitting's own commit is outside.
func TestAR32B_1_TouchExemptsProvenMerges(t *testing.T) {
	for _, own := range []bool{false, true} {
		w, o := orchWorld(t, ar32aRows)
		if res, _ := ar32bRow(t, w, "b1", map[string]string{"scripts/x.sh": "true\n"}, "scripts/x.sh"); res != relay.GreenResult {
			t.Fatalf("b1: %s; want green", res)
		}
		res, why, g := orchSitting(t, w, o, map[string]string{ar32bCharter: ar32aCharter}, ar32bMerge(t, "b1"), func(tree string) {
			if own {
				write(t, tree, "scripts/x.sh", "false\n")
				gitIn(t, tree, "add", ".")
				gitIn(t, tree, "commit", "-qm", "the sitting's own edit")
			}
		})
		want, outside := relay.GreenResult, []string(nil)
		if own {
			want, outside = relay.RedResult, []string{"scripts/x.sh"}
		}
		if res != want || why != "" || g.Unproven || !slices.Equal(g.Outside, outside) && len(g.Outside)+len(outside) > 0 {
			t.Errorf("own edit %v: %s, %q, unproven %v, outside %q; want %s, outside %q", own, res, why, g.Unproven, g.Outside, want, outside)
		}
	}
}
