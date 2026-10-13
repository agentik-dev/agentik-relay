package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// configAtCommit is OSGit whose commit in root also writes a filter into
// the root's config, as an escaped daemon could after the merged verify.
type configAtCommit struct {
	effects.OSGit
	t    *testing.T
	root string
}

func (g configAtCommit) Commit(dir, msg, name, email string) (string, error) {
	sha, err := g.OSGit.Commit(dir, msg, name, email)
	if err == nil && dir == g.root {
		gitIn(g.t, dir, "config", "filter.fix.smudge", "sed s/.*/a/")
	}
	return sha, err
}

// refusedAs reports whether err names one of reasons.
func refusedAs(err error, reasons ...relay.StopReason) bool {
	for _, r := range reasons {
		if err != nil && strings.Contains(err.Error(), string(r)) {
			return true
		}
	}
	return false
}

// TestAR21_1_ConfigBetweenRows is ADV12's second test: a filter in the
// root's config before stage, git state changed between two rows, and a
// merged verify line (or a daemon after it) that writes a filter: each is
// refused, nothing merges, and a merge already committed is undone to the
// recorded main (AR21 act 1, ADV12-2).
func TestAR21_1_ConfigBetweenRows(t *testing.T) {
	t.Run("filter-before-stage", func(t *testing.T) {
		w := hermeticWorld(t)
		gitIn(t, w.root, "config", "filter.fix.smudge", "sed s/.*/a/")
		tree := filepath.Join(w.root, ".claude", "worktrees", "a1")
		st, err := night.Stage(w.fx, &night.RowPlan{ID: "a1", Worktree: tree, Branch: "a1", Base: "main"})
		if err == nil {
			st.Unlock()
		}
		if !refusedAs(err, relay.GitConfigProgram) {
			t.Fatalf("Stage under a filter = %v; want git-config-program", err)
		}
		if _, err := os.Stat(tree); err == nil {
			t.Fatal("the worktree was checked out under the filter")
		}
	})
	t.Run("between-rows", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stageRow(t, "a1", "main", "b.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s, want green; gate %+v", res, g)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err != nil {
			t.Fatal(err)
		}
		// the previous row's merged verify line, or a daemon, writes a filter
		gitIn(t, w.root, "config", "filter.fix.smudge", "sed s/.*/a/")
		tree := filepath.Join(w.root, ".claude", "worktrees", "a2")
		st2, err := night.Stage(w.fx, &night.RowPlan{ID: "a2", Worktree: tree, Branch: "a2", Base: "main"})
		if err == nil {
			st2.Unlock()
		}
		if !refusedAs(err, relay.GitConfigProgram, relay.GitStateMoved) {
			t.Fatalf("Stage after a filter between rows = %v; want refused", err)
		}
		if _, err := os.Stat(tree); err == nil {
			t.Fatal("the worktree was checked out under the filter")
		}
	})
	t.Run("bound-once", func(t *testing.T) {
		w := hermeticWorld(t)
		w.stageRow(t, "a1", "main", "b.txt")
		common := gitIn(t, w.root, "rev-parse", "--path-format=absolute", "--git-common-dir")
		hook(t, filepath.Join(common, "hooks"), "pre-commit", "true\n")
		tree := filepath.Join(w.root, ".claude", "worktrees", "a2")
		st2, err := night.Stage(w.fx, &night.RowPlan{ID: "a2", Worktree: tree, Branch: "a2", Base: "main"})
		if err == nil {
			st2.Unlock()
		}
		if !refusedAs(err, relay.GitStateMoved) {
			t.Fatalf("Stage after the hooks changed = %v; want git-state-moved", err)
		}
	})
	t.Run("gate-against-baseline", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stageRow(t, "a1", "main", "b.txt")
		gitIn(t, w.root, "config", "filter.fix.smudge", "sed s/.*/a/")
		// a stage line that bound the state after the write (a forged or
		// restaged one): the gate compares the root's baseline, not it
		d, err := effects.GitStateDigest(w.root, st.Path)
		if err != nil {
			t.Fatal(err)
		}
		st.StateDigest = d
		res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.Unrun || (why != relay.GitStateMoved && why != relay.GitConfigProgram) {
			t.Fatalf("Run = %s %q; want unrun, git-state-moved or git-config-program", res, why)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || w.merged() {
			t.Fatalf("merged past a filter: %v", err)
		}
	})
	t.Run("merged-verify-writes", func(t *testing.T) {
		w := hermeticWorld(t)
		// AR22 act 0: the verify line runs in a clone, whose own config a
		// bare `git config` writes; it writes the root's by path.
		write(t, w.root, "Makefile", "check:\n\tgit -C '"+w.root+"' config filter.fix.smudge 'sed s/.*/a/'\n\tgrep -qx a a.txt\n")
		gitIn(t, w.root, "add", ".")
		gitIn(t, w.root, "commit", "-qm", "main's harness")
		st := w.stageRow(t, "a1", "main", "b.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s, want green; gate %+v", res, g)
		}
		main := gitIn(t, w.root, "rev-parse", "HEAD")
		_, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto")
		if !refusedAs(err, relay.GitStateMoved, relay.GitConfigProgram) || w.merged() {
			t.Fatalf("merge after a verify line wrote a filter = %v, merged %v", err, w.merged())
		}
		if now := gitIn(t, w.root, "rev-parse", "HEAD"); now != main {
			t.Fatalf("main is at %s, want the recorded %s", now, main)
		}
	})
	t.Run("undone-after-commit", func(t *testing.T) {
		w := hermeticWorld(t)
		st := w.stageRow(t, "a1", "main", "b.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s, want green; gate %+v", res, g)
		}
		main := gitIn(t, w.root, "rev-parse", "HEAD")
		w.fx.Git = configAtCommit{t: t, root: w.root}
		_, err := MergeIfGreen(w.fx, w.root, g, "a1", "")
		if !refusedAs(err, relay.GitStateMoved, relay.GitConfigProgram) || w.merged() {
			t.Fatalf("merge with a filter written at commit = %v, merged %v", err, w.merged())
		}
		if now := gitIn(t, w.root, "rev-parse", "HEAD"); now != main {
			t.Fatalf("main is at %s, want the recorded %s", now, main)
		}
		if _, err := os.Stat(filepath.Join(w.root, "b.txt")); err == nil {
			t.Fatal("b.txt is in the root: the merge was not undone")
		}
	})
}
