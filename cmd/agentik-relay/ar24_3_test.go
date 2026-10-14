package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR24_3_DoctorProbeDropsSitting (AR24 act 3, ADV14-5): doctor's
// `claude --max-turns 1 --version` probe runs under DropSitting's env: a
// fake claude on the probe's lookup path, run from a depth-1 sitting's
// env, sees no RELAY_ROW and no CLAUDE* key.
func TestAR24_3_DoctorProbeDropsSitting(t *testing.T) {
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	dir := t.TempDir()
	for k, v := range map[string]string{"HOME": filepath.Join(dir, "home"), "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_STATE_HOME": filepath.Join(dir, "state"), "RELAY_ROW": "r1", "CLAUDE_SESSION_DEPTH": "1"} {
		t.Setenv(k, v)
	}
	envFile := filepath.Join(dir, "env")
	claude := filepath.Join(dir, "claude")
	if err := os.WriteFile(claude, []byte("#!/bin/sh\n/usr/bin/env > '"+envFile+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return claude, nil
		}
		return "/fake/" + name, nil
	}
	gitWorktree = func(string) error { return nil }
	var out, errb bytes.Buffer
	run([]string{"doctor", "--json"}, strings.NewReader(""), &out, &errb)
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("the probe never ran the fake claude: %v; %s", err, out.String())
	}
	for _, kv := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(kv, "CLAUDE") || strings.HasPrefix(kv, "RELAY_ROW=") {
			t.Errorf("the probe's claude saw %s", kv)
		}
	}
}
