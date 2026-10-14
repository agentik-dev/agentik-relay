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

// TestAR39_1_StatusDefaultSkipsHand (AR39 act 1): `relay status` with no
// --night, over a state holding a dated night's status.json and the gate's
// `hand` dir, names the dated night in its header, not `hand`; `--night
// hand` still reads the gate's dir.
func TestAR39_1_StatusDefaultSkipsHand(t *testing.T) {
	getenv = fakeEnv(t.TempDir())
	t.Cleanup(func() { getenv = os.Getenv })
	state := effects.StateDir(getenv)
	const id = "2026-09-29-11"
	nd := night.NightDir(state, id)
	if err := os.MkdirAll(nd, 0o700); err != nil {
		t.Fatal(err)
	}
	st := `{"night":"` + id + `","state":"running","rows":[]}`
	if err := os.WriteFile(filepath.Join(nd, "status.json"), []byte(st), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(night.NightDir(state, "hand"), "r1"), 0o700); err != nil {
		t.Fatal(err)
	}
	do := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		return code, out.String() + errb.String()
	}
	code, out := do("status")
	if code != exitOK || !strings.HasPrefix(out, "night "+id+": running") {
		t.Fatalf("status = %d %q; want the header to name night %s", code, out, id)
	}
	t.Logf("status:\n%s", out)
	if code, out := do("status", "--night", "hand"); code != exitOK || !strings.HasPrefix(out, "night hand") {
		t.Fatalf("status --night hand = %d %q; want night hand", code, out)
	}
}
