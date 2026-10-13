package effects

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFakeGit(t *testing.T) {
	g := &FakeGit{
		Toplevels: map[string]string{"/r/w": "/r/w"},
		Roots:     map[string]string{"/r/w": "/r"},
		Heads:     map[string]string{"/r": "abc"},
		Branches:  map[string]string{"/r": "main"},
		Statuses:  map[string][]string{"/r": {"?? x"}},
		Ancestors: map[string]bool{"a3..main": true},
		Existing:  map[string]bool{"a3": true},
		Remote:    map[string]string{"refs/remotes/origin/main": "abc"},
		Err:       map[string]error{},
	}
	if v, err := g.Toplevel("/r/w"); v != "/r/w" || err != nil {
		t.Fatalf("Toplevel = %q, %v", v, err)
	}
	if v, _ := g.Root("/r/w"); v != "/r" {
		t.Fatalf("Root = %q", v)
	}
	if v, _ := g.Head("/r"); v != "abc" {
		t.Fatalf("Head = %q", v)
	}
	if v, _ := g.Branch("/r"); v != "main" {
		t.Fatalf("Branch = %q", v)
	}
	if v, _ := g.Status("/r"); !reflect.DeepEqual(v, []string{"?? x"}) {
		t.Fatalf("Status = %q", v)
	}
	if ok, _ := g.IsAncestor("/r", "a3", "main"); !ok {
		t.Fatal("IsAncestor a3 main = false")
	}
	if ok, _ := g.IsAncestor("/r", "a4", "main"); ok {
		t.Fatal("IsAncestor a4 main = true")
	}
	if ok, _ := g.BranchExists("/r", "a3"); !ok {
		t.Fatal("BranchExists a3 = false")
	}
	if refs, _ := g.RemoteRefs("/r"); refs["refs/remotes/origin/main"] != "abc" {
		t.Fatalf("RemoteRefs = %v", refs)
	}
	if _, err := g.Head("/elsewhere"); err == nil {
		t.Fatal("Head of an unset dir did not fail")
	}
	g.Err["Branch"] = errors.New("boom")
	if _, err := g.Branch("/r"); err == nil {
		t.Fatal("Err did not make Branch fail")
	}
	want := []string{"Toplevel /r/w", "Root /r/w", "Head /r", "Branch /r", "Status /r",
		"IsAncestor /r a3 main", "IsAncestor /r a4 main", "BranchExists /r a3",
		"RemoteRefs /r", "Head /elsewhere", "Branch /r"}
	if got := g.Calls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Calls =\n%q\nwant\n%q", got, want)
	}
}

func TestFakeFS(t *testing.T) {
	f := &FakeFS{}
	if err := f.MkdirAll("/s/ledger", 0o700); err != nil {
		t.Fatal(err)
	}
	f.WriteFile("/s/nights/2026-09-22-1/status.json", []byte("{}"), 0o600)
	if !f.Exists("/s/ledger") || !f.Exists("/s") || f.Exists("/t") {
		t.Fatal("Exists wrong")
	}
	if names, _ := f.List("/s"); !reflect.DeepEqual(names, []string{"ledger", "nights"}) {
		t.Fatalf("List = %q", names)
	}
	if _, err := f.List("/t"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("List /t err = %v", err)
	}
	if _, err := f.ReadFile("/t"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadFile /t err = %v", err)
	}
}

func TestShell(t *testing.T) {
	dir := t.TempDir()
	var s OSShell
	exit, last, err := s.Run(dir, "echo one; echo two >&2; echo; exit 3", time.Minute)
	if exit != 3 || last != "one" || err != nil { // stdout's last, not stderr's (AR7 act 8)
		t.Fatalf("Run = %d, %q, %v", exit, last, err)
	}
	exit, last, _ = s.Run(dir, "pwd", time.Minute)
	if real, _ := filepath.EvalSymlinks(dir); exit != 0 || (last != dir && last != real) {
		t.Fatalf("pwd = %d, %q", exit, last)
	}
	start := time.Now()
	exit, _, _ = s.Run(dir, "sleep 30 & sleep 30", 200*time.Millisecond)
	if exit != 124 || time.Since(start) > 10*time.Second {
		t.Fatalf("timeout exit = %d after %v", exit, time.Since(start))
	}
	if _, last, _ = s.Run(dir, "printf '%0300d\\n' 0", time.Minute); len(last) != LastCap {
		t.Fatalf("last is %d bytes, want the %d cap", len(last), LastCap)
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestOSGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	gitIn(t, root, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(root, "f"), []byte("x"), 0o644)
	gitIn(t, root, "add", "f")
	gitIn(t, root, "commit", "-qm", "one")
	first := gitIn(t, root, "rev-parse", "HEAD")
	tree := filepath.Join(root, ".claude", "worktrees", "a1")
	gitIn(t, root, "worktree", "add", "-q", "-b", "a1", tree)
	os.WriteFile(filepath.Join(tree, "g"), []byte("y"), 0o644)
	gitIn(t, tree, "add", "g")
	gitIn(t, tree, "commit", "-qm", "two")
	gitIn(t, root, "update-ref", "refs/remotes/origin/main", first)

	var g OSGit
	if v, err := g.Toplevel(tree); v != tree || err != nil {
		t.Fatalf("Toplevel = %q, %v", v, err)
	}
	if v, err := g.Root(tree); v != root || err != nil {
		t.Fatalf("Root = %q, %v; want %q", v, err, root)
	}
	if v, _ := g.Head(root); v != first {
		t.Fatalf("Head = %q", v)
	}
	if v, _ := g.Branch(tree); v != "a1" {
		t.Fatalf("Branch = %q", v)
	}
	if v, err := g.Status(root); len(v) != 0 || err != nil {
		t.Fatalf("Status of a root holding only worktrees = %q, %v", v, err)
	}
	os.WriteFile(filepath.Join(root, "h"), []byte("z"), 0o644)
	if v, _ := g.Status(root); !reflect.DeepEqual(v, []string{"?? h"}) {
		t.Fatalf("Status = %q", v)
	}
	if ok, err := g.IsAncestor(root, "main", "a1"); !ok || err != nil {
		t.Fatalf("IsAncestor main a1 = %v, %v", ok, err)
	}
	if ok, err := g.IsAncestor(root, "a1", "main"); ok || err != nil {
		t.Fatalf("IsAncestor a1 main = %v, %v", ok, err)
	}
	if _, err := g.IsAncestor(root, "nosuch", "main"); err == nil {
		t.Fatal("IsAncestor of an unknown ref did not fail")
	}
	if ok, _ := g.BranchExists(root, "a1"); !ok {
		t.Fatal("BranchExists a1 = false")
	}
	if ok, err := g.BranchExists(root, "a2"); ok || err != nil {
		t.Fatalf("BranchExists a2 = %v, %v", ok, err)
	}
	if refs, _ := g.RemoteRefs(root); !reflect.DeepEqual(refs, map[string]string{"refs/remotes/origin/main": first}) {
		t.Fatalf("RemoteRefs = %v", refs)
	}
}

func TestStateDir(t *testing.T) {
	env := map[string]string{"HOME": "/h"}
	get := func(k string) string { return env[k] }
	if d := StateDir(get); d != "/h/.local/state/agent-relay" {
		t.Fatalf("StateDir = %q", d)
	}
	env["XDG_STATE_HOME"] = "/x"
	if d := StateDir(get); d != "/x/agent-relay" {
		t.Fatalf("StateDir = %q", d)
	}
}

// TestMain points XDG_STATE_HOME, the one state root (§5), at a temp
// directory before any test, so no test reads or writes the machine's
// ledger, locks or nights.
func TestMain(m *testing.M) {
	// The tests push only into t.TempDir() remotes; the deny is advisory (relay-r1 §10; ADV11-8 deferred).
	DropEnvConfig()
	state, err := os.MkdirTemp("", "relay-state-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Setenv("XDG_STATE_HOME", state)
	// AR21 act 0 (ADV12-1): HOME and XDG_CONFIG_HOME are temp dirs, so no
	// test reads the owner's ~/.gitconfig or ~/.config/git; the tests' own
	// git commits as t.
	for k, v := range map[string]string{"HOME": filepath.Join(state, "home"), "XDG_CONFIG_HOME": filepath.Join(state, "config"),
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		if strings.HasPrefix(v, state) {
			os.MkdirAll(v, 0o755)
		}
		os.Setenv(k, v)
	}
	// AR22 act 1: every gate line and runner git runs under a pinned env,
	// built under a temp dir of its own.
	code := RunPinned(m)
	os.RemoveAll(state)
	os.Exit(code)
}

// TestAR7_LastIsStdout: last is stdout's last non-empty line even when
// stderr wrote after it; stderr's only when stdout is empty.
func TestAR7_LastIsStdout(t *testing.T) {
	var sh OSShell
	for _, c := range []struct{ line, want string }{
		{"echo out; echo err >&2", "out"},
		{"echo err >&2; echo out; echo later >&2", "out"},
		{"echo one >&2; echo two >&2", "two"},
		{"true", ""},
	} {
		exit, last, err := sh.Run(t.TempDir(), c.line, 10*time.Second)
		if err != nil || exit != 0 || last != c.want {
			t.Errorf("%q: exit %d, last %q, %v; want last %q", c.line, exit, last, err, c.want)
		}
	}
}
