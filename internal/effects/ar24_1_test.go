package effects

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// makefilePY is this repo's Makefile's PY line, verbatim (ADV14-1 at
// Makefile:2).
const makefilePY = "PY := $(shell pyenv which python3 2>/dev/null || command -v python3)"

// fakePyenv puts on the start PATH a pyenv that answers `which python3`
// as pyenv does: the start env's pick (no PYENV_ROOT) is interp, else the
// cwd's .python-version, else $PYENV_ROOT/version, under PYENV_ROOT's
// versions/. It restores the pin and PinTools at cleanup.
func fakePyenv(t *testing.T, interp string) {
	t.Helper()
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { Use(old); PinTools = tools })
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "pyenv"), []byte("#!/bin/sh\n[ \"$1 $2\" = \"which python3\" ] || exit 1\n"+
		"[ -z \"$PYENV_ROOT\" ] && { echo "+interp+"; exit 0; }\n"+
		"v=$(cat .python-version 2>/dev/null || cat \"$PYENV_ROOT/version\")\necho \"$PYENV_ROOT/versions/$v/bin/python3\"\n"), 0o755)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	os.Unsetenv("PYENV_ROOT")
	t.Setenv("PYENV_VERSION", "")
	os.Unsetenv("PYENV_VERSION")
}

// TestAR24_1_Pyenv is ADV14-1 (review test 2): under the pinned env,
// the account's PYENV_ROOT/version and a committed .python-version name
// an interpreter the sitting wrote; the Makefile's PY is still the pinned
// bin/python3, the one pyenv named at start.
func TestAR24_1_Pyenv(t *testing.T) {
	dir := t.TempDir()
	interp := filepath.Join(dir, "python3.12")
	os.WriteFile(interp, []byte("#!/bin/sh\n[ \"$1\" = -c ] && { echo "+dir+"; exit 0; }\necho pinned\n"), 0o755)
	fakePyenv(t, interp)
	account := t.TempDir()
	for _, v := range []string{"evil", "evil2"} {
		os.MkdirAll(filepath.Join(account, ".pyenv", "versions", v, "bin"), 0o755)
		os.WriteFile(filepath.Join(account, ".pyenv", "versions", v, "bin", "python3"), []byte("#!/bin/sh\necho "+v+"\n"), 0o755)
	}
	os.WriteFile(filepath.Join(account, ".pyenv", "version"), []byte("evil\n"), 0o644)
	PinTools = slices.DeleteFunc(slices.Clone(PinTools), func(s string) bool { return s != "pyenv" && s != "python3" })
	p, err := PinAt(t.TempDir(), account)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })
	clone := t.TempDir()
	os.WriteFile(filepath.Join(clone, "Makefile"), []byte(makefilePY+"\nall:\n\t@$(PY)\n"), 0o644)
	for _, c := range []struct{ name, pin string }{{"PYENV_ROOT/version", ""}, {"a committed .python-version", "evil2"}} {
		if c.pin != "" {
			os.WriteFile(filepath.Join(clone, ".python-version"), []byte(c.pin+"\n"), 0o644)
		}
		exit, last, err := OSShell{}.Run(clone, "make", 10*time.Second)
		if err != nil || exit != 0 || strings.TrimSpace(last) != "pinned" {
			t.Fatalf("%s: the Makefile's PY ran %q (exit %d, %v), want the pinned bin/python3's `pinned`", c.name, last, exit, err)
		}
	}
}

// TestAR24_1_EnvHasNoPyenv is ADV14-1's fix: pyenv is not a pinned tool
// (no bin/pyenv link, even with pyenv on the start PATH) and the pinned
// env carries no PYENV_ROOT; python3 is still the interpreter pyenv
// named at start.
func TestAR24_1_EnvHasNoPyenv(t *testing.T) {
	dir := t.TempDir()
	interp := filepath.Join(dir, "python3.12")
	os.WriteFile(interp, []byte("#!/bin/sh\necho "+dir+"\n"), 0o755)
	fakePyenv(t, interp)
	if slices.Contains(PinTools, "pyenv") {
		t.Fatalf("PinTools = %v holds pyenv", PinTools)
	}
	PinTools = slices.DeleteFunc(slices.Clone(PinTools), func(s string) bool { return s != "pyenv" && s != "python3" })
	p, err := PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })
	for _, kv := range p.Env {
		if strings.HasPrefix(kv, "PYENV") {
			t.Errorf("the pinned env holds %s", kv)
		}
	}
	if _, err := os.Lstat(filepath.Join(p.Home, "bin", "pyenv")); err == nil {
		t.Errorf("bin/ links pyenv")
	}
	want, _ := filepath.EvalSymlinks(interp)
	if p.Tools["python3"] != want {
		t.Errorf("python3 = %q, want the interpreter pyenv named at start, %q", p.Tools["python3"], want)
	}
}
