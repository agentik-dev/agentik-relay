package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// ar37Repos is AR37 fix's world: HOME a resolved temp dir holding repos a
// (one commit, so it can hold a linked worktree) and b, the state under
// another temp home, and toplevel the cwd's own through git (TestMain
// fakes it to an empty tree; each test's cwd is a temp repo). It returns
// HOME, a, b, the state and a run of the relay that returns its exit and
// its output.
func ar37Repos(t *testing.T) (home, a, b, state string, do func(args ...string) (int, string)) {
	t.Helper()
	getenv = fakeEnv(t.TempDir())
	oldTop := toplevel
	t.Cleanup(func() { getenv, toplevel = os.Getenv, oldTop })
	toplevel = func() (string, error) {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		return strings.TrimSpace(string(out)), err
	}
	for k, v := range map[string]string{"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	a, b = filepath.Join(home, "a"), filepath.Join(home, "b")
	for _, argv := range [][]string{
		{"init", "-q", "-b", "main", a}, {"init", "-q", "-b", "main", b},
		{"-C", a, "commit", "-q", "--allow-empty", "-m", "one"},
	} {
		if out, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", argv, err, out)
		}
	}
	os.MkdirAll(filepath.Join(b, "sub"), 0o755)
	state = effects.StateDir(getenv)
	do = func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		return code, out.String() + errb.String()
	}
	return home, a, b, state, do
}

// TestAR37Fix_F1_StopWhileMeasuring (AR37 fix F1): a night still measuring
// (no status.json, its groups.json naming root A) is named in A by `relay
// status` and `relay stop`, with the note from repo B; a half-written
// status.json with no groups.json takes A from the ledger's night-open
// line.
func TestAR37Fix_F1_StopWhileMeasuring(t *testing.T) {
	_, a, b, state, do := ar37Repos(t)
	const id = "2026-09-30-8"
	nd := night.NightDir(state, id)
	if err := os.MkdirAll(nd, 0o700); err != nil {
		t.Fatal(err)
	}
	groups := `{"pid":1,"started":"then","root":"` + a + `","groups":[]}` + "\n"
	if err := os.WriteFile(filepath.Join(nd, night.GroupsFile), []byte(groups), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(b, "sub"))
	note := "note: this night runs in ~/a, not in this repo\n"
	if code, out := do("status"); code != exitOK || !strings.HasPrefix(out, "night "+id+" in ~/a: ; 0 rows\n") {
		t.Fatalf("status while measuring = %d %q; want the night in ~/a", code, out)
	}
	want := "relay stop: night " + id + " in ~/a: \"stop\" written; the night is still measuring: it stops within a few seconds, before any row starts\n"
	if code, out := do("stop"); code != exitOK || out != want+note {
		t.Fatalf("stop while measuring = %d %q; want %q", code, out, want+note)
	}
	os.Remove(filepath.Join(nd, night.GroupsFile))
	if err := os.WriteFile(filepath.Join(nd, "status.json"), []byte(`{"night":"`+id+`","sta`), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := effects.LedgerDir(state)
	os.MkdirAll(ledger, 0o700)
	open := `{"ts":"2026-09-30T21:00:00-07:00","night":"` + id + `","event":"night-open","root":"` + a + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(ledger, "2026-09-30.jsonl"), []byte(open), 0o600); err != nil {
		t.Fatal(err)
	}
	want = "relay stop: night " + id + " in ~/a: \"stop\" written; the loop reads it after its next event\n"
	if code, out := do("stop"); code != exitOK || out != want+note {
		t.Fatalf("stop over half a status.json = %d %q; want %q", code, out, want+note)
	}
	t.Logf("%s%s", want, note)
}

// TestAR37Fix_F2_LinkedWorktreeNoNote (AR37 fix F2): a cwd in a linked
// worktree of the night's root is the night's repo (one git common
// directory): the line names the night and there is no note; from repo B
// the note stays.
func TestAR37Fix_F2_LinkedWorktreeNoNote(t *testing.T) {
	home, a, b, state, do := ar37Repos(t)
	wt := filepath.Join(home, "a-wt")
	if out, err := exec.Command("git", "-C", a, "worktree", "add", "-q", "-b", "wt", wt).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	os.MkdirAll(filepath.Join(wt, "sub"), 0o755)
	const id = "2026-09-30-9"
	nd := night.NightDir(state, id)
	if err := os.MkdirAll(nd, 0o700); err != nil {
		t.Fatal(err)
	}
	st := `{"night":"` + id + `","state":"running","rows":[],"root":"` + a + `"}`
	if err := os.WriteFile(filepath.Join(nd, "status.json"), []byte(st), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "relay stop: night " + id + " in ~/a: \"stop\" written; the loop reads it after its next event\n"
	t.Chdir(filepath.Join(wt, "sub"))
	if code, out := do("stop"); code != exitOK || out != want {
		t.Fatalf("stop from a linked worktree of A = %d %q; want %q, no note", code, out, want)
	}
	t.Chdir(b)
	if code, out := do("stop"); code != exitOK || !strings.HasSuffix(out, "note: this night runs in ~/a, not in this repo\n") {
		t.Fatalf("stop from repo B = %d %q; want the note", code, out)
	}
	t.Logf("%s", want)
}
