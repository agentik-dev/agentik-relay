package effects

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAR20_0_StateDigest: the digest moves with the common config, a
// common hook, info/ and the linked tree's config.worktree; a commit, a
// second worktree and the gate's own reads leave it alone.
func TestAR20_0_StateDigest(t *testing.T) {
	root, _ := repoWithRemote(t)
	var g OSGit
	tree := filepath.Join(root, ".claude", "worktrees", "a1")
	if err := g.WorktreeAdd(root, tree, "a1", "main"); err != nil {
		t.Fatal(err)
	}
	if err := g.InstallPrePush(tree); err != nil {
		t.Fatal(err)
	}
	digest := func() string {
		t.Helper()
		d, err := GitStateDigest(root, tree)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	was := digest()
	os.WriteFile(filepath.Join(tree, "g"), []byte("g\n"), 0o644)
	gitIn(t, tree, "add", "g")
	gitIn(t, tree, "commit", "-qm", "work")
	if err := g.WorktreeAddDetached(root, filepath.Join(t.TempDir(), "fresh"), "a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Status(tree); err != nil {
		t.Fatal(err)
	}
	if now := digest(); now != was {
		t.Fatal("a commit, a worktree or a read moved the digest")
	}
	common := filepath.Join(root, ".git")
	for _, c := range []struct {
		name   string
		change func()
	}{
		{"config", func() { gitIn(t, tree, "config", "filter.x.smudge", "cat") }},
		{"hook", func() { os.WriteFile(filepath.Join(common, "hooks", "pre-commit"), []byte("#!/bin/sh\n"), 0o755) }},
		{"hook mode", func() { os.Chmod(filepath.Join(common, "hooks", "pre-commit"), 0o644) }},
		{"info", func() { os.WriteFile(filepath.Join(common, "info", "attributes"), []byte("* filter=x\n"), 0o644) }},
		{"config.worktree", func() { gitIn(t, tree, "config", "--worktree", "core.fsmonitor", "true") }},
	} {
		name := c.name
		c.change()
		if now := digest(); now == was {
			t.Errorf("%s: the digest did not move", name)
		} else {
			was = now
		}
	}
}

// TestAR20_0_HermeticNoHooks: Hermetic runs no hook, whatever the
// worktree's core.hooksPath names.
func TestAR20_0_HermeticNoHooks(t *testing.T) {
	root, _ := repoWithRemote(t)
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	os.WriteFile(filepath.Join(root, "h"), []byte("h\n"), 0o644)
	gitIn(t, root, "add", "h")
	if _, err := (OSGit{}).Hermetic(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "h"); err != nil {
		t.Fatalf("Hermetic commit ran the pre-commit hook: %v", err)
	}
}
