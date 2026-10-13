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

// TestAR37_2_StopNamesRepo (AR37 act 2): a night whose status names root A,
// stopped and killed from a cwd in repo B: each verb's line names the night
// in A (home written `~`), a second line notes the night runs in A, not in
// this repo, and the control file is written all the same (one relay per
// account). From a cwd in A itself there is no note.
func TestAR37_2_StopNamesRepo(t *testing.T) {
	home := t.TempDir()
	getenv = fakeEnv(home)
	oldTop := toplevel
	t.Cleanup(func() { getenv, toplevel = os.Getenv, oldTop })
	// TestMain fakes the top level to an empty tree; this test reads the
	// cwd's own, as the verb does, and its cwd is always a temp repo.
	toplevel = func() (string, error) {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		return strings.TrimSpace(string(out)), err
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, r := range []string{a, b} {
		if out, err := exec.Command("git", "init", "-q", "-b", "main", r).CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v\n%s", r, err, out)
		}
	}
	os.MkdirAll(filepath.Join(b, "sub"), 0o755)
	state := effects.StateDir(getenv)
	const id = "2026-09-30-7"
	nd := night.NightDir(state, id)
	if err := os.MkdirAll(nd, 0o700); err != nil {
		t.Fatal(err)
	}
	st := `{"night":"` + id + `","state":"running","rows":[],"root":"` + a + `"}`
	if err := os.WriteFile(filepath.Join(nd, "status.json"), []byte(st), 0o600); err != nil {
		t.Fatal(err)
	}
	do := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		return code, out.String() + errb.String()
	}
	note := "note: this night runs in ~/a, not in this repo\n"
	t.Chdir(filepath.Join(b, "sub"))
	for _, c := range []struct {
		args []string
		line string
	}{
		{[]string{"stop"}, `"stop"`},
		{[]string{"kill", "r1"}, `"kill r1"`},
	} {
		code, out := do(c.args...)
		want := "relay " + c.args[0] + ": night " + id + " in ~/a: " + c.line + " written; the loop reads it after its next event\n"
		if code != exitOK || out != want+note {
			t.Fatalf("%v from repo B = %d %q; want %q", c.args, code, out, want+note)
		}
		t.Logf("%v:\n%s", c.args, out)
	}
	if stop, kills := night.ReadControl(effects.OSFS{}, night.ControlPath(state, id)); !stop || len(kills) != 1 || kills[0] != "r1" {
		t.Fatalf("control %v %v; want stop and kill r1", stop, kills)
	}
	t.Chdir(a)
	if code, out := do("stop"); code != exitOK || !strings.HasPrefix(out, "relay stop: night "+id+" in ~/a: ") || strings.Contains(out, "note:") {
		t.Fatalf("stop from repo A = %d %q; want the line, no note", code, out)
	}
}
