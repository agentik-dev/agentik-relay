package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// hook writes an executable hook named name into dir.
func hook(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// hermeticWorld is harnessWorld with the state root in temp dirs.
func hermeticWorld(t *testing.T) *world {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return harnessWorld(t)
}

// rootMerge is OSGit whose merge in root also stages evil.txt: the root's
// merge is not the one mergedVerify judged.
type rootMerge struct {
	effects.OSGit
	t    *testing.T
	root string
}

func (g rootMerge) MergeNoCommit(dir, branch, name, email string) error {
	if err := g.OSGit.MergeNoCommit(dir, branch, name, email); err != nil || dir != g.root {
		return err
	}
	write(g.t, dir, "evil.txt", "evil\n")
	gitIn(g.t, dir, "add", "evil.txt")
	return nil
}

// TestAR20_0_GateHermetic is ADV11's first test: a child's post-checkout
// in its own hooks, a filter in the common .git/config, and a pre-commit in
// the common hooks each fail to bend the gate or the merge; and a root
// merge whose tree is not the verified one is undone, merge-tree-differs
// (ADV11-3, -5, -14).
func TestAR20_0_GateHermetic(t *testing.T) {
	t.Run("post-checkout", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stage(t, "a.txt") // breaks main's check
		hooks := gitIn(t, w.tree, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
		hook(t, hooks, "post-checkout", "echo a > a.txt\n")
		res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{Name: "auto"})
		if res == relay.GreenResult {
			t.Fatalf("Run = green: the child's post-checkout repaired the fresh tree; verify %+v", g.Verify)
		}
	})
	t.Run("filter", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stage(t, "a.txt")
		write(t, w.tree, ".gitattributes", "a.txt filter=fix\n")
		gitIn(t, w.tree, "add", ".gitattributes")
		gitIn(t, w.tree, "commit", "-qm", "attributes")
		gitIn(t, w.tree, "config", "filter.fix.smudge", "sed s/.*/a/")
		res, why, g, _ := Run(w.fx, st, pk([]string{"a.txt", ".gitattributes"}), nil, seen, Verify{Name: "auto"})
		if res != relay.Unrun || string(why) != "git-state-moved" {
			t.Fatalf("Run = %s %q, want unrun git-state-moved; verify %+v", res, why, g.Verify)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
			t.Fatalf("merged past a filter: %v", err)
		}
	})
	t.Run("pre-commit", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stage(t, "b.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{Name: "auto"})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s, want green; gate %+v", res, g)
		}
		common := gitIn(t, w.tree, "rev-parse", "--path-format=absolute", "--git-common-dir")
		hook(t, filepath.Join(common, "hooks"), "pre-commit", "echo evil > evil.txt && git add evil.txt\n")
		_, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto")
		if err == nil || !strings.Contains(err.Error(), "git-state-moved") || w.merged() {
			t.Fatalf("merge after a pre-commit = %v, merged %v", err, w.merged())
		}
		if _, err := os.Stat(filepath.Join(w.root, "evil.txt")); err == nil {
			t.Fatal("evil.txt is on main")
		}
	})
	t.Run("merge-tree-differs", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stage(t, "b.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{Name: "auto"})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s, want green; gate %+v", res, g)
		}
		main := gitIn(t, w.root, "rev-parse", "HEAD")
		w.fx.Git = rootMerge{t: t, root: w.root}
		_, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto")
		if err == nil || !strings.Contains(err.Error(), "merge-tree-differs") || w.merged() {
			t.Fatalf("merge of another tree = %v, merged %v", err, w.merged())
		}
		if now := gitIn(t, w.root, "rev-parse", "HEAD"); now != main {
			t.Fatalf("main is at %s, want the recorded %s", now, main)
		}
		if _, err := os.Stat(filepath.Join(w.root, "evil.txt")); err == nil {
			t.Fatal("evil.txt is in the root")
		}
	})
}
