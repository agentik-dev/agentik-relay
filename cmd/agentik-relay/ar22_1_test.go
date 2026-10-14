package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR22_1_UnpinnedToolExit2 is AR22 act 1 (ADV13-2) at the start: with
// git, make, sh and ps in no dir the account cannot write, every verb but
// version and the hook exits 2 naming unpinned-tool before it reads a
// queue or makes a relay home.
func TestAR22_1_UnpinnedToolExit2(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	old, _ := effects.Current()
	dirs, keep := effects.PinDirs, pinFor
	t.Cleanup(func() { effects.PinDirs, pinFor = dirs, keep; effects.Use(old) })
	pinFor = func() (func(), error) {
		p, err := effects.Pin()
		return func() { p.Release() }, err
	}
	effects.PinDirs = []string{t.TempDir()}
	for _, args := range [][]string{{"gate", "a1"}, {"run", "--dry"}, {"plan"}, {"clean"}} {
		var out, errb bytes.Buffer
		if code := run(args, strings.NewReader(""), &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "unpinned-tool") {
			t.Errorf("relay %v = %d %q, want 2 unpinned-tool", args, code, errb.String())
		}
	}
	if _, err := os.Stat(filepath.Join(state, "agentik-relay", "home")); err == nil {
		t.Errorf("a refused start made a relay home")
	}
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, strings.NewReader(""), &out, &errb); code != exitOK {
		t.Errorf("relay version = %d %q: it needs no pin", code, errb.String())
	}
}
