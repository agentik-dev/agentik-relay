package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR25_3_DoctorVerify is ADV15-4's measure: `relay doctor --verify
// <dir>` runs dir's `make check` as a gate runs it, in a fresh clone of
// its HEAD under the pin. The fixture's check prints its env: HOME is a
// scratch dir under the pin's relay home, PYTHONNOUSERSITE=1; a file
// left dirty in dir never reaches the clone, and the clone is gone after.
// A red check exits 1 with its last line; nested, it never runs.
func TestAR25_3_DoctorVerify(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = os.Getenv })
	measuring(t)
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mk := func(body string) {
		os.WriteFile(filepath.Join(repo, "Makefile"), []byte("check:\n\t@"+body+"\n"), 0o644)
		g("add", "Makefile")
		g("commit", "-q", "-m", "check")
	}
	g("init", "-q", "-b", "main")
	mk(`test ! -e dirty && echo "home=$$HOME nous=$$PYTHONNOUSERSITE"`)
	os.WriteFile(filepath.Join(repo, "dirty"), []byte("x"), 0o644)
	do := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := run(append([]string{"doctor", "--verify", repo}, args...), strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	p, ok := effects.Current()
	if !ok {
		t.Fatal("no pin")
	}
	code, out, errs := do("--json")
	var rep verifyReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil || code != exitOK || !rep.OK || rep.Line != "make check" {
		t.Fatalf("doctor --verify = %d %q %q (%v)", code, out, errs, err)
	}
	h, nous, _ := strings.Cut(strings.TrimPrefix(rep.Last, "home="), " nous=")
	if !strings.HasPrefix(h, p.Home+string(filepath.Separator)) || nous != "1" {
		t.Fatalf("the check's env: last %q, want HOME under the relay home %s and PYTHONNOUSERSITE=1", rep.Last, p.Home)
	}
	if _, err := os.Stat(h); err == nil {
		t.Fatalf("the scratch home %s outlived the line", h)
	}
	if left, _ := filepath.Glob(filepath.Join(effects.StateDir(getenv), "gate", "fresh-*")); len(left) > 0 {
		t.Fatalf("the clone outlived the verify: %v", left)
	}
	mk(`echo one; echo boom; exit 3`)
	code, out, _ = do()
	if code != exitRed || !strings.Contains(out, "exit 2 in") && !strings.Contains(out, "exit 3 in") || !strings.Contains(out, "last: boom") {
		t.Fatalf("a red check = %d %q", code, out)
	}
	code, out, _ = do("--line", "echo $$PYTHONNOUSERSITE; echo line")
	if code != exitOK || !strings.Contains(out, "last: line") {
		t.Fatalf("doctor --verify --line = %d %q", code, out)
	}
	getenv = fakeEnv(home) // nested: RELAY_MEASURE_DEPTH set
	if code, _, errs = do(); code != exitUsage || !strings.Contains(errs, "nested") {
		t.Fatalf("nested doctor --verify = %d %q", code, errs)
	}
}
