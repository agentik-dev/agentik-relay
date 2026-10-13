package effects

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sittingKeys names the keys, never the values, of env lines a sitting's
// marks leave behind: RELAY_ROW and any CLAUDE* (AR24 act 3, ADV14-5).
func sittingKeys(lines []string) []string {
	var bad []string
	for _, kv := range lines {
		if strings.HasPrefix(kv, "CLAUDE") || strings.HasPrefix(kv, "RELAY_ROW=") {
			k, _, _ := strings.Cut(kv, "=")
			bad = append(bad, k)
		}
	}
	return bad
}

// TestAR24_3_TmuxDropsSitting (AR24 act 3, ADV14-5): every tmux command
// OSTmux runs, new-session, new-window and has-session alike, runs under
// DropSitting's env, so a tmux server a depth-1 relay starts carries no
// CLAUDE_SESSION_DEPTH or RELAY_ROW into a later window; the owner's
// RELAY_DEVICE_VERBS passes.
func TestAR24_3_TmuxDropsSitting(t *testing.T) {
	dir := t.TempDir()
	for k, v := range map[string]string{"HOME": filepath.Join(dir, "home"), "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_STATE_HOME": filepath.Join(dir, "state"), "RELAY_ROW": "r1", SessionDepthVar: "1", SessionTurnsVar: "40",
		"CLAUDECODE": "1", "RELAY_DEVICE_VERBS": "status"} {
		t.Setenv(k, v)
	}
	envFile := filepath.Join(dir, "env")
	tmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(tmux, []byte("#!/bin/sh\n/usr/bin/env > '"+envFile+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tm := OSTmux{LookPath: func(string) (string, error) { return tmux, nil }, Socket: filepath.Join(dir, "relay.sock")}
	for name, drive := range map[string]func() error{
		"new-session": func() error { return tm.NewSession("s", dir, []string{"sh"}) },
		"new-window":  func() error { return tm.NewWindow("s", "w", dir, []string{"sh"}) },
		"has-session": func() error { _, err := tm.Alive("s"); return err },
	} {
		os.Remove(envFile)
		if err := drive(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := os.ReadFile(envFile)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		lines := strings.Split(string(b), "\n")
		if bad := sittingKeys(lines); len(bad) > 0 {
			t.Errorf("%s: tmux saw a sitting's env: %v", name, bad)
		}
		found := false
		for _, kv := range lines {
			found = found || kv == "RELAY_DEVICE_VERBS=status"
		}
		if !found {
			t.Errorf("%s: RELAY_DEVICE_VERBS did not pass", name)
		}
	}
}

// TestAR24_3_ChildEnvSameDrop (AR24 act 3, ADV14-5): ChildEnv drops what
// DropSitting drops and nothing else; what ChildEnv adds after the drop
// (RELAY_ROW, the depth, the budget, RELAY_BIN, the push deny) aside.
func TestAR24_3_ChildEnvSameDrop(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "CLAUDECODE=1", SessionDepthVar + "=1", SessionTurnsVar + "=40",
		"CLAUDE_CODE_ENTRYPOINT=cli", "RELAY_ROW=parent", "RELAY_BIN=/x/relay", MeasureDepthVar + "=1",
		"RELAY_DEVICE_VERBS=status", "HOME=/h", "XCLAUDE=kept"}
	drop := DropSitting(environ)
	want := []string{"PATH=/usr/bin", "RELAY_DEVICE_VERBS=status", "HOME=/h", "XCLAUDE=kept"}
	if !reflect.DeepEqual(drop, want) {
		t.Fatalf("DropSitting = %v, want %v", drop, want)
	}
	child := ChildEnv(environ, "r1", "", "", 40, nil, "/y/relay")
	if len(child) < len(drop) || !reflect.DeepEqual(child[:len(drop)], drop) {
		t.Fatalf("ChildEnv's kept lines %v, DropSitting's %v", child, drop)
	}
	added := []string{"RELAY_ROW=r1", MeasureDepthVar + "=1", SessionDepthVar + "=1", SessionTurnsVar + "=40", "RELAY_BIN=/y/relay"}
	if !reflect.DeepEqual(child[len(drop):], added) {
		t.Fatalf("ChildEnv added %v, want %v", child[len(drop):], added)
	}
}
