package night

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// landedRepo is a real git root under t.TempDir on main with one commit,
// committing as a fixed author with no global config.
func landedRepo(t *testing.T) (string, func(args ...string) string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := filepath.Join(t.TempDir(), "root")
	run := func(args ...string) string {
		return landedGit(t, root, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	}
	landedGit(t, filepath.Dir(root), "init", "-q", "-b", "main", root)
	run("commit", "-q", "--allow-empty", "-m", "init")
	return root, run
}

// TestAR13_8_FreshBranchNotLanded (§16 gap 1): a fresh branch, its tip its
// cut and so an ancestor of main, is neither landed nor a satisfied
// dependency.
func TestAR13_8_FreshBranchNotLanded(t *testing.T) {
	root, run := landedRepo(t)
	run("branch", "a1-fresh")
	g := effects.OSGit{}
	if DepSatisfied(g, root, nil, "a1-fresh") {
		t.Fatal("a fresh branch (tip = cut, an ancestor of main) read as landed")
	}
	if ok, why := satisfied(g, root, nil, nil, "a1-fresh"); ok || why != "" {
		t.Fatalf("satisfied = %v %q, want pending", ok, why)
	}
}

// TestAR13_8_FirstParentMergeLands: a branch that moved and was merged
// --no-ff onto main's first-parent line lands; a branch cut off that merged
// tip and never moved (its queue base a1-done: the merge-base is its tip;
// AR22 act 3 reads the cut from git, never head_before) does not, nor does
// a branch main fast-forwarded to.
func TestAR13_8_FirstParentMergeLands(t *testing.T) {
	root, run := landedRepo(t)
	run("checkout", "-q", "-b", "a1-done")
	run("commit", "-q", "--allow-empty", "-m", "work")
	tip := run("rev-parse", "HEAD")
	run("checkout", "-q", "main")
	run("merge", "-q", "--no-ff", "-m", "merge a1-done", "a1-done")
	run("branch", "a2-stacked", "a1-done")
	g := effects.OSGit{}
	if !DepSatisfied(g, root, nil, "a1-done") {
		t.Fatal("a branch merged on main's first-parent line did not land")
	}
	stacked := &queue.Queue{Rows: []queue.Row{{ID: "a2-stacked", Base: "a1-done"}}}
	if DepSatisfied(g, root, stacked, "a2-stacked") || tip == "" {
		t.Fatal("a branch whose tip is its cut read as landed")
	}
	run("checkout", "-q", "-b", "a3-ff")
	run("commit", "-q", "--allow-empty", "-m", "ff")
	run("checkout", "-q", "main")
	run("merge", "-q", "--ff-only", "a3-ff")
	if DepSatisfied(g, root, nil, "a3-ff") {
		t.Fatal("a fast-forwarded branch read as a first-parent merge")
	}
}

// TestAR13_8_FakeGitMerged: the fake answers FirstParentMerged from Merged
// and records the call.
func TestAR13_8_FakeGitMerged(t *testing.T) {
	g := &effects.FakeGit{Merged: map[string]bool{"a1..main": true}}
	if ok, _ := g.FirstParentMerged("/r", "a1", "main"); !ok {
		t.Fatal("Merged[a1..main] not read")
	}
	if ok, _ := g.FirstParentMerged("/r", "a2", "main"); ok {
		t.Fatal("an absent key read as merged")
	}
}

func landedGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
