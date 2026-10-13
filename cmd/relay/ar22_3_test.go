package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// resumeRepo is a git root under a temp dir (its real path) on main, with
// branch r1 checked out in <root>/.claude/worktrees/r1, made with HOME at
// home, the XDG config and state homes temp and no global git config; the
// test's directory is the root.
func resumeRepo(t *testing.T, home string) (root, tree string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, tree = filepath.Join(dir, "root"), filepath.Join(dir, "root", ".claude", "worktrees", "r1")
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	g("init", "-q", "-b", "main", root)
	g("-C", root, "commit", "-q", "--allow-empty", "-m", "init")
	g("-C", root, "branch", "r1")
	g("-C", root, "worktree", "add", "-q", tree, "r1")
	t.Chdir(root)
	return root, tree
}

// TestAR22_3_ResumeFromGit is ADV13-8: a forged `stage` line's worktree
// /tmp/evil and a `sitting` line's session_id --settings=... are only
// printed. Resume execs `claude --resume <uuid>` in the tree git lists at
// <root>/.claude/worktrees/r1, the uuid the newest transcript Claude Code
// keeps for that dir (a stem that is no UUID is skipped); a row with no
// branch gets the picker, `claude --resume`, in the root (AR22 act 3).
func TestAR22_3_ResumeFromGit(t *testing.T) {
	home := t.TempDir()
	root, tree := resumeRepo(t, home)
	getenv = fakeEnv(home)
	oldExec := execResume
	t.Cleanup(func() { getenv, execResume = os.Getenv, oldExec })
	measuring(t)

	store := filepath.Join(home, ".claude", "projects", regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(tree, "-"))
	os.MkdirAll(store, 0o700)
	const older, newer = "0b5e7c1a-1111-4d2e-9f00-00000000000a", "7d0c9e2b-2222-4a1f-8e00-00000000000b"
	now := time.Now()
	for i, name := range []string{older + ".jsonl", newer + ".jsonl", "--settings=x.jsonl", "not-a-uuid.jsonl"} {
		p := filepath.Join(store, name)
		os.WriteFile(p, []byte("{}\n"), 0o600)
		when := now.Add(time.Duration(i-3) * time.Minute) // the non-UUID stems are newest
		os.Chtimes(p, when, when)
	}

	state := effects.StateDir(getenv)
	l := effects.NewLedger(state, effects.FakeClock{T: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC)}, "2026-09-24-1")
	for _, row := range []string{"r1", "r2"} {
		l.Append("stage", map[string]any{"row": row, "worktree": "/tmp/evil", "branch": row, "base": "main"})
		l.Append("sitting", map[string]any{"row": row, "state": "red", "session_id": "--settings=/tmp/evil/s.json"})
	}
	var dir string
	var argv []string
	execResume = func(d string, a []string) error { dir, argv = d, a; return nil }
	for _, c := range []struct {
		row, dir string
		argv     []string
	}{
		{"r1", tree, []string{"claude", "--resume", newer}},
		{"r2", root, []string{"claude", "--resume"}}, // no branch r2: the picker in the root
	} {
		dir, argv = "", nil
		var out, errb bytes.Buffer
		code := run([]string{"resume", c.row}, strings.NewReader(""), &out, &errb)
		if code != exitOK || dir != c.dir || !reflect.DeepEqual(argv, c.argv) {
			t.Errorf("resume %s = %d, exec in %q argv %q; want %q %q (stdout %q, stderr %q)", c.row, code, dir, argv, c.dir, c.argv, out.String(), errb.String())
		}
		if !strings.Contains(out.String(), strings.Join(c.argv, " ")) || strings.Contains(out.String(), "cd /tmp/evil") ||
			strings.Contains(out.String(), "claude --resume --settings") {
			t.Errorf("resume %s printed %q; want its git line, the ledger's only as a claim", c.row, out.String())
		}
	}
}
