package effects

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR17B_1_RepoCommitHooksRun: the relay worktree's own hooks directory
// links every hook of the root's (ADV10-11), so a pre-commit that refused
// before still refuses, and the relay's pre-push stays the refusing one.
func TestAR17B_1_RepoCommitHooksRun(t *testing.T) {
	root, _ := repoWithRemote(t)
	hook := "#!/bin/sh\necho 'repo: pre-commit refused' >&2; exit 1\n"
	os.WriteFile(filepath.Join(root, ".git", "hooks", "pre-commit"), []byte(hook), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "hooks", "pre-push"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	var g OSGit
	tree := filepath.Join(root, ".claude", "worktrees", "a1")
	if err := g.WorktreeAdd(root, tree, "a1", "main"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // a second install (a resumed row) is the same
		if err := g.InstallPrePush(tree); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(tree, "g"), []byte("y\n"), 0o644)
	gitIn(t, tree, "add", "g")
	out, err := exec.Command("git", "-C", tree, "commit", "-qm", "two").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "repo: pre-commit refused") {
		t.Fatalf("the repo's pre-commit did not run in the relay worktree (err %v):\n%s", err, out)
	}
	out, err = exec.Command("git", "-C", tree, "push", "origin", "a1").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "relay: push refused") {
		t.Fatalf("the relay's pre-push is not the one that runs (err %v):\n%s", err, out)
	}
}

// TestAR17B_3_ChildEnvSetsBin: the child's RELAY_BIN is the bin ChildEnv
// is given, never a parent's (ADV10-15); "" sets none.
func TestAR17B_3_ChildEnvSetsBin(t *testing.T) {
	in := []string{"HOME=/h", "RELAY_BIN=/parent/relay"}
	got := strings.Join(ChildEnv(in, "r1", "", "", 80, nil, "/abs/relay"), "\n")
	if !strings.Contains(got, "\nRELAY_BIN=/abs/relay") || strings.Contains(got, "/parent/relay") {
		t.Fatalf("ChildEnv:\n%s", got)
	}
	if got := strings.Join(ChildEnv(in, "r1", "", "", 80, nil, ""), "\n"); strings.Contains(got, "RELAY_BIN") {
		t.Fatalf("ChildEnv with no bin:\n%s", got)
	}
	if b := RelayBin(); !filepath.IsAbs(b) {
		t.Fatalf("RelayBin = %q", b)
	}
}
