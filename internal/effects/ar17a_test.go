package effects

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// ar17aRepo is a temp repo on main with a second commit on branch b, and
// a tag named main at b's tip: `main` is ambiguous.
func ar17aRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "one"},
		{"checkout", "-q", "-b", "b"}, {"commit", "-q", "--allow-empty", "-m", "two"}, {"tag", "main", "HEAD"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return filepath.Clean(dir)
}

// TestAR17A_0_DiffStderrUnrun: a diff git warns about (here, `main` names
// a tag and a branch) is an error, never an answer read past its stderr
// (ADV10-2).
func TestAR17A_0_DiffStderrUnrun(t *testing.T) {
	dir := ar17aRepo(t)
	if names, err := (OSGit{}).DiffNames(dir, "main", "HEAD"); err == nil {
		t.Fatalf("DiffNames past an ambiguous main = %q, nil; want an error", names)
	}
}

// TestAR17A_0_RevParseCommitBindsBranch: the full refname answers the
// branch, whatever a tag of the same name points at.
func TestAR17A_0_RevParseCommitBindsBranch(t *testing.T) {
	dir := ar17aRepo(t)
	g := OSGit{}
	br, err := g.RevParseCommit(dir, "refs/heads/main^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	tag, _ := git(dir, "rev-parse", "refs/tags/main^{commit}")
	if br == tag {
		t.Fatalf("refs/heads/main resolved to the tag's %s", tag)
	}
	if _, err := g.RevParseCommit(dir, "refs/heads/nosuch^{commit}"); err == nil {
		t.Fatal("an absent branch resolved")
	}
}
