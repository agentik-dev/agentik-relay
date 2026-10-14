package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR34_2_StopVerbSaysWhenNotOpen: `agentik-relay stop` and `agentik-relay kill` against
// a night with a directory and no status.json (its plan still measuring)
// say the measure ends and no row starts, not "after its next event"; once
// the night has opened (status.json), the loop's line again.
func TestAR34_2_StopVerbSaysWhenNotOpen(t *testing.T) {
	home := t.TempDir()
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = os.Getenv })
	state := effects.StateDir(getenv)
	const id = "2026-09-30-1"
	dir := night.NightDir(state, id)
	if err := os.MkdirAll(dir, 0o700); err != nil { // the trap's, during the measure
		t.Fatal(err)
	}
	do := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		return code, out.String() + errb.String()
	}
	const measuring = "the night is still measuring: it stops within a few seconds, before any row starts"
	for _, args := range [][]string{{"stop"}, {"kill", "r1"}} {
		code, out := do(args...)
		if code != exitOK || !strings.Contains(out, measuring) || strings.Contains(out, "after its next event") {
			t.Fatalf("%v during the measure = %d %q; want %q", args, code, out, measuring)
		}
		t.Logf("%v: %s", args, strings.TrimSpace(out))
	}
	if stop, kills := night.ReadControl(effects.OSFS{}, night.ControlPath(state, id)); !stop || len(kills) != 1 {
		t.Fatalf("control %v %v", stop, kills)
	}
	os.WriteFile(filepath.Join(dir, "status.json"), []byte(`{"night":"`+id+`","state":"running","rows":[]}`), 0o600)
	if code, out := do("stop"); code != exitOK || !strings.Contains(out, "after its next event") || strings.Contains(out, measuring) {
		t.Fatalf("stop once open = %d %q", code, out)
	}
}
