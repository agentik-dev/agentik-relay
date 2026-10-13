package effects

import (
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// repoWithRemote is a temp repo on main with one commit, its bare file
// remote as origin, and git's identity and global config pinned in the
// environment, so OSGit's commands run as they would for the relay.
func repoWithRemote(t *testing.T) (root, remote string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	root, remote = filepath.Join(dir, "root"), filepath.Join(dir, "remote.git")
	gitIn(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	gitIn(t, dir, "init", "-q", "-b", "main", root)
	os.WriteFile(filepath.Join(root, "f"), []byte("x\n"), 0o644)
	gitIn(t, root, "add", "f")
	gitIn(t, root, "commit", "-qm", "one")
	gitIn(t, root, "remote", "add", "origin", remote)
	gitIn(t, root, "push", "-q", "origin", "main")
	return root, remote
}

func TestPrePushRefuses(t *testing.T) {
	root, _ := repoWithRemote(t)
	var g OSGit
	tree := filepath.Join(root, ".claude", "worktrees", "a1")
	if err := g.WorktreeAdd(root, tree, "a1", "main"); err != nil {
		t.Fatal(err)
	}
	if err := g.InstallPrePush(root); err == nil {
		t.Fatal("InstallPrePush accepted the main worktree")
	}
	if err := g.InstallPrePush(tree); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", tree, "push", "origin", "a1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("git push from the worktree succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "relay: push refused") {
		t.Fatalf("the refusal does not name the relay:\n%s", out)
	}
	// The root's pushes are the owner's and stay untouched.
	gitIn(t, root, "push", "-q", "origin", "main")
}

func TestOSGitWriteSide(t *testing.T) {
	root, _ := repoWithRemote(t)
	var g OSGit
	var w Git = g
	for i := 0; i < 2; i++ {
		if err := w.Exclude(root, ".claude/worktrees/"); err != nil {
			t.Fatal(err)
		}
	}
	ex, _ := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if n := strings.Count(string(ex), ".claude/worktrees/\n"); n != 1 {
		t.Fatalf("Exclude twice wrote the pattern %d times", n)
	}
	tree := filepath.Join(root, ".claude", "worktrees", "a1")
	if err := w.WorktreeAdd(root, tree, "a1", "main"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tree, "g"), []byte("y\n"), 0o644)
	gitIn(t, tree, "add", "g")
	gitIn(t, tree, "commit", "-qm", "two")
	head := gitIn(t, tree, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(root, "h"), []byte("z\n"), 0o644)
	gitIn(t, root, "add", "h")
	gitIn(t, root, "commit", "-qm", "three on main")

	if names, err := w.DiffNames(root, "main", "a1"); !reflect.DeepEqual(names, []string{"g"}) || err != nil {
		t.Fatalf("DiffNames main...a1 = %q, %v (three-dot: main's own h is not a1's)", names, err)
	}
	wts, err := w.Worktrees(root)
	if err != nil || len(wts) != 2 || wts[0].Path != root || wts[0].Branch != "main" ||
		wts[1].Path != tree || wts[1].Branch != "a1" || wts[1].Head != head {
		t.Fatalf("Worktrees = %+v, %v", wts, err)
	}
	gate := filepath.Join(root, ".claude", "worktrees", "gate-a1")
	if err := w.WorktreeAddDetached(root, gate, "a1"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(gate, "junk"), []byte("j\n"), 0o644)
	if err := w.WorktreeDiscard(root, gate); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gate); !os.IsNotExist(err) {
		t.Fatalf("the discarded worktree is still there: %v", err)
	}

	before := gitIn(t, root, "rev-parse", "HEAD")
	if err := w.MergeNoCommit(root, "a1", "t", "t@t"); err != nil {
		t.Fatal(err)
	}
	if err := w.MergeAbort(root); err != nil {
		t.Fatal(err)
	}
	if after := gitIn(t, root, "rev-parse", "HEAD"); after != before {
		t.Fatalf("abort left HEAD at %s, want %s", after, before)
	}
	if s, _ := g.Status(root); len(s) != 0 {
		t.Fatalf("abort left the root dirty: %q", s)
	}
	if err := w.MergeNoCommit(root, "a1", "t", "t@t"); err != nil {
		t.Fatal(err)
	}
	sha, err := w.Commit(root, "merge a1", "t", "t@t")
	if err != nil || sha == before {
		t.Fatalf("Commit = %s, %v", sha, err)
	}
	if parents := gitIn(t, root, "rev-list", "--parents", "-n1", "HEAD"); len(strings.Fields(parents)) != 3 {
		t.Fatalf("the merge is not a merge commit: %s", parents)
	}

	os.WriteFile(filepath.Join(tree, "dirty"), []byte("d\n"), 0o644)
	if err := w.WorktreeRemove(root, tree); err == nil {
		t.Fatal("WorktreeRemove removed a worktree holding an untracked file")
	}
	os.Remove(filepath.Join(tree, "dirty"))
	if err := w.WorktreeRemove(root, tree); err != nil {
		t.Fatal(err)
	}
	if ok, _ := g.BranchExists(root, "a1"); !ok {
		t.Fatal("WorktreeRemove deleted the branch")
	}
}

// goTokens scans every non-generated .go file under the module root and
// calls f with each file's repo-relative path and each token's text; comments
// are not tokens here.
func goTokens(t *testing.T, f func(rel, tok string)) {
	t.Helper()
	top, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(top, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".claude" || d.Name() == "bin") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(top, p)
		fset := token.NewFileSet()
		var s scanner.Scanner
		s.Init(fset.AddFile(rel, -1, len(src)), src, nil, 0)
		for {
			_, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if lit != "" {
				f(filepath.ToSlash(rel), lit)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// refusers hold the hooks' own refusals: the guard table that denies a
// sitting's push, amend and rebase, and the start hook's rules line that
// tells the sitting not to. Neither is a path that runs one.
var refusers = map[string]bool{"internal/hooks/guards.go": true, "internal/hooks/start.go": true}

// refusal names the pre-push installer and its hook text, and ChildEnv's
// denied pushurl (ADV6-6) and the lines' (AR20 act 3); a caller names them
// to refuse a push, never to make one.
var refusal = map[string]bool{"InstallPrePush": true, "PrePushHook": true,
	"DeniedPushURL": true, "PushDenyEnv": true, "DenyPush": true, "DenyingPush": true, "relay-denies-push": true, "GIT_CONFIG_KEY_%d=remote.%s.pushurl": true}

// prePushText is a git.go token that names the pre-push hook or is its text
// (AR2-7): the file's exemption covers the hook, never another push.
func prePushText(tok string) bool {
	return strings.Contains(tok, "pre-push") || strings.Contains(tok, "push refused")
}

// TestNoPushPath: §11, the string push appears in Go code only in the
// guards, the pre-push hook text in this package's git.go, and tests.
func TestNoPushPath(t *testing.T) {
	seen := map[string]bool{}
	goTokens(t, func(rel, tok string) {
		if !strings.Contains(strings.ToLower(tok), "push") || refusal[strings.Trim(tok, "\"`")] || strings.HasSuffix(rel, "_test.go") ||
			refusers[rel] || rel == "internal/effects/git.go" && prePushText(tok) {
			return
		}
		if !seen[rel] {
			t.Errorf("%s holds %q: the relay has no push path", rel, tok)
			seen[rel] = true
		}
	})
}

// TestNoRewrite: D22, no history rewrite exists in the binary.
func TestNoRewrite(t *testing.T) {
	words := []string{"reset --" + "hard", "re" + "base", "--" + "amend"}
	goTokens(t, func(rel, tok string) {
		if strings.HasSuffix(rel, "_test.go") || refusers[rel] {
			return
		}
		for _, w := range words {
			if strings.Contains(strings.ToLower(tok), w) {
				t.Errorf("%s holds %q", rel, w)
			}
		}
	})
}

// TestAR7_StatusSeesClaudeDir: content under .claude/ other than the
// worktrees directory is dirt; .claude/worktrees/ never is.
func TestAR7_StatusSeesClaudeDir(t *testing.T) {
	root, _ := repoWithRemote(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no user excludes file
	var g OSGit
	wt := filepath.Join(root, ".claude", "worktrees", "a1")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wt, "junk"), []byte("j\n"), 0o644)
	if s, err := g.Status(root); err != nil || len(s) != 0 {
		t.Fatalf("worktrees only: Status = %q, %v; want clean", s, err)
	}
	os.WriteFile(filepath.Join(root, ".claude", "settings.local.json"), []byte("{}\n"), 0o644)
	s, err := g.Status(root)
	if err != nil || !reflect.DeepEqual(s, []string{"?? .claude/settings.local.json"}) {
		t.Fatalf("Status = %q, %v; want the settings file as dirt", s, err)
	}
}
