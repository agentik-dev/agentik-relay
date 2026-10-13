package effects

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// ar53Write writes body to p at mode 0644 whatever the umask, making its
// directories.
func ar53Write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
}

// ar53Repo is a tree whose .git directory holds config and info/exclude
// only: a repository as GitStateDigest reads it, with no git run.
func ar53Repo(t *testing.T) (root, common string) {
	t.Helper()
	root = t.TempDir()
	common = filepath.Join(root, ".git")
	ar53Write(t, filepath.Join(common, "config"), "x\n")
	ar53Write(t, filepath.Join(common, "info", "exclude"), "e\n")
	return root, common
}

// ar53Digest is GitStateDigest(root, tree), failing t on an error.
func ar53Digest(t *testing.T, root, tree string) string {
	t.Helper()
	d, err := GitStateDigest(root, tree)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestAR53_1_InfoRefsIsNotState: a regular file info/refs, written,
// rewritten at another mode or removed after a digest, leaves it equal;
// and a tree without one digests as it did before AR53 (the stream
// digestTree wrote, hashed here by hand).
func TestAR53_1_InfoRefsIsNotState(t *testing.T) {
	root, common := ar53Repo(t)
	stream := "f config 644 2\x00x\n" + "- config.worktree\x00" + "- hooks\x00" +
		"d info\x00" + "f info/exclude 644 2\x00e\n" + "- objects/info/alternates\x00"
	sum := sha256.Sum256([]byte(stream))
	was := ar53Digest(t, root, "")
	if want := hex.EncodeToString(sum[:]); was != want {
		t.Fatalf("a tree without info/refs digests %s, want %s as before", was, want)
	}
	refs := filepath.Join(common, "info", "refs")
	for _, c := range []struct {
		name   string
		change func()
	}{
		{"written", func() { ar53Write(t, refs, "0123\trefs/heads/main\n") }},
		{"rewritten", func() { ar53Write(t, refs, "4567\trefs/heads/l2ax\n4567\trefs/heads/main\n") }},
		{"mode", func() { os.Chmod(refs, 0o600) }},
		{"removed", func() { os.Remove(refs) }},
	} {
		c.change()
		if now := ar53Digest(t, root, ""); now != was {
			t.Errorf("info/refs %s: the digest moved", c.name)
		}
	}
}

// TestAR53_2_InfoElseStillMoves: everything else under info/ is still git
// state: exclude, attributes, sparse-checkout, grafts, a new file, a refs
// one level down, a directory info/refs holding a file and a symlink
// info/refs; and a refs outside info/ (hooks/refs) as before.
func TestAR53_2_InfoElseStillMoves(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(info string)
	}{
		{"info/exclude", func(info string) { ar53Write(t, filepath.Join(info, "exclude"), "*.o\n") }},
		{"info/attributes", func(info string) { ar53Write(t, filepath.Join(info, "attributes"), "* filter=x\n") }},
		{"info/sparse-checkout", func(info string) { ar53Write(t, filepath.Join(info, "sparse-checkout"), "/*\n") }},
		{"info/grafts", func(info string) { ar53Write(t, filepath.Join(info, "grafts"), "0123\n") }},
		{"info/x", func(info string) { ar53Write(t, filepath.Join(info, "x"), "x\n") }},
		{"info/sub/refs", func(info string) { ar53Write(t, filepath.Join(info, "sub", "refs"), "r\n") }},
		{"info/refs/ directory", func(info string) { ar53Write(t, filepath.Join(info, "refs", "heads"), "r\n") }},
		{"info/refs symlink", func(info string) {
			if err := os.Symlink("exclude", filepath.Join(info, "refs")); err != nil {
				t.Fatal(err)
			}
		}},
		{"hooks/refs", func(info string) { ar53Write(t, filepath.Join(filepath.Dir(info), "hooks", "refs"), "r\n") }},
	} {
		root, common := ar53Repo(t)
		was := ar53Digest(t, root, "")
		c.change(filepath.Join(common, "info"))
		if now := ar53Digest(t, root, ""); now == was {
			t.Errorf("%s: the digest did not move", c.name)
		}
	}
}

// TestAR53_3_UpdateServerInfo: in a temp repository with the real git, a
// row's commit in its linked worktree and `git update-server-info`, which
// writes info/refs as `git gc --auto` does, leave the digest equal.
func TestAR53_3_UpdateServerInfo(t *testing.T) {
	root, _ := repoWithRemote(t) // skips when git is missing
	var g OSGit
	tree := filepath.Join(root, ".claude", "worktrees", "l2ax")
	if err := g.WorktreeAdd(root, tree, "l2ax", "main"); err != nil {
		t.Fatal(err)
	}
	if err := g.InstallPrePush(tree); err != nil {
		t.Fatal(err)
	}
	was, wasRoot := ar53Digest(t, root, tree), ar53Digest(t, root, "")
	ar53Write(t, filepath.Join(tree, "g"), "g\n")
	gitIn(t, tree, "add", "g")
	gitIn(t, tree, "commit", "-qm", "work")
	gitIn(t, tree, "update-server-info")
	refs := filepath.Join(gitIn(t, root, "rev-parse", "--path-format=absolute", "--git-common-dir"), "info", "refs")
	if fi, err := os.Lstat(refs); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("update-server-info wrote no regular info/refs: %v", err)
	}
	if now := ar53Digest(t, root, tree); now != was {
		t.Error("update-server-info moved the row's digest")
	}
	if now := ar53Digest(t, root, ""); now != wasRoot {
		t.Error("update-server-info moved the root's digest")
	}
}

// TestAR53_5_InfoRefsTempIsNotState: `git update-server-info` writes
// info/refs_ plus six characters and renames it onto info/refs, so a
// regular info/refs_Ab3dEf added after a digest leaves it equal; a
// directory of that name (holding a file), a symlink of that name and a
// regular info/refs_ab (another length) each still move it.
func TestAR53_5_InfoRefsTempIsNotState(t *testing.T) {
	root, common := ar53Repo(t)
	was := ar53Digest(t, root, "")
	ar53Write(t, filepath.Join(common, "info", "refs_Ab3dEf"), "0123\trefs/heads/l2ax\n")
	if now := ar53Digest(t, root, ""); now != was {
		t.Error("info/refs_Ab3dEf written: the digest moved")
	}
	for _, c := range []struct {
		name   string
		change func(info string)
	}{
		{"info/refs_Ab3dEf/ directory", func(info string) { ar53Write(t, filepath.Join(info, "refs_Ab3dEf", "x"), "x\n") }},
		{"info/refs_Ab3dEf symlink", func(info string) {
			if err := os.Symlink("exclude", filepath.Join(info, "refs_Ab3dEf")); err != nil {
				t.Fatal(err)
			}
		}},
		{"info/refs_ab", func(info string) { ar53Write(t, filepath.Join(info, "refs_ab"), "r\n") }},
	} {
		root, common := ar53Repo(t)
		was := ar53Digest(t, root, "")
		c.change(filepath.Join(common, "info"))
		if now := ar53Digest(t, root, ""); now == was {
			t.Errorf("%s: the digest did not move", c.name)
		}
	}
}
