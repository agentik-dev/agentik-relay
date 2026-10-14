package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR75_7_DoctorWarns (AR75 ruling 5, session-layer-r1 §2.3): under a
// stale session layer, doctor's config row is a warning naming the layer
// not read, and doctor exits as a warning does, 0.
func TestAR75_7_DoctorWarns(t *testing.T) {
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	top, err := toplevel()
	if err != nil {
		t.Fatal(err)
	}
	home := getenv("HOME")
	state := filepath.Join(home, "state")
	env := map[string]string{"HOME": home, "XDG_STATE_HOME": state, "CLAUDE_CODE_SESSION_ID": "s1", effects.MeasureDepthVar: "1"}
	for k, v := range env {
		t.Setenv(k, v)
	}
	getenv = func(k string) string { return env[k] }
	repoFile := filepath.Join(top, ".claude", "agentik", "config.yaml")
	session := filepath.Join(state, "agentik-session", "sessions", "s1", "resolved.yaml")
	for path, body := range map[string]string{
		repoFile: "relay: {verify_seconds: 3600}\n",
		session: "relay: {verify_seconds: 1800}\nprovider:\n  inputs:\n    - path: " + repoFile + "\n      sha256: " +
			strings.Repeat("0", 64) + "\n    - path: " + filepath.Join(top, ".claude", "agentik", "config.local.yaml") + "\n      sha256: null\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	code := run([]string{"doctor", "--json"}, strings.NewReader(""), &out, &errb)
	var got struct {
		Checks []check `json:"checks"`
		OK     bool    `json:"ok"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("doctor --json: %v; %s%s", err, out.String(), errb.String())
	}
	var row *check
	for i := range got.Checks {
		if got.Checks[i].Name == "config" {
			row = &got.Checks[i]
		}
	}
	if row == nil || !row.OK || !row.Warn || !strings.Contains(row.Detail, "warnings: ") ||
		!strings.Contains(row.Detail, session+": not read: "+repoFile+" changed since the session resolved it") {
		t.Fatalf("the config row = %+v, want a warning naming the stale layer not read", row)
	}
	if code != 0 || !got.OK {
		t.Errorf("doctor exit = %d, ok = %v; want a warning's 0 and ok; checks %+v", code, got.OK, got.Checks)
	}
}
