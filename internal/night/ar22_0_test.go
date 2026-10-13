package night_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestAR22_0_StageRunsNoHook is ADV13-10: a sitting can `git config
// --global core.hooksPath`; the next row's stage (its `worktree add` and
// the pre-push install) runs under the runner, so a global post-checkout
// never runs as the relay. HOME, XDG_CONFIG_HOME and XDG_STATE_HOME are
// temp dirs, and GIT_CONFIG_GLOBAL is unset so HOME's .gitconfig is read.
func TestAR22_0_StageRunsNoHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	w := newWorld(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	os.Unsetenv("GIT_CONFIG_GLOBAL") // t.Setenv restores it
	hooks, marker := filepath.Join(home, "hooks"), filepath.Join(home, "ran")
	os.MkdirAll(hooks, 0o755)
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\necho \"$RELAY_ROW\" > '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core]\n\thooksPath = "+hooks+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := w.stage(t, "a1")
	if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the stage ran the global post-checkout (%v)", err)
	}
	if _, err := os.Stat(filepath.Join(st.Path, "a.txt")); err != nil {
		t.Fatalf("the stage checked out nothing: %v", err)
	}
}
