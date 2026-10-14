package effects

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	env := map[string]string{"HOME": "/h"}
	get := func(k string) string { return env[k] }
	fs := &FakeFS{}
	c, err := LoadConfig(fs, get, "/r")
	if err != nil || c.Tmux != "auto" || c.AllowedTools != DefaultAllowedTools || !c.Caffeinate ||
		c.Queue != DefaultQueue || len(c.Sources) != 0 {
		t.Fatalf("defaults = %+v, %v", c, err)
	}
	fs.WriteFile("/h/.config/agent-relay/config.yaml", []byte(
		"ntfy: {url: https://ntfy.example, topic: night}\ntmux: never\ncaffeinate: false\ncolour: blue\n"), 0o600)
	fs.WriteFile("/r/.claude/claude-code-session/config.yaml", []byte(
		"other: {x: 1}\nagent-relay:\n  queue: q.yaml\n  merge: branch-only\n  tmux: auto\n"), 0o600)
	env["CLAUDE_CODE_SESSION_ID"] = "s1"
	fs.WriteFile("/h/.local/state/claude-code-session/sessions/s1/resolved.yaml", []byte(
		"agent-relay: {queue: s.yaml, allowed_tools: Bash}\n"+
			inputs(fs, "/r/.claude/claude-code-session/config.yaml", "/r/.claude/claude-code-session/config.local.yaml")), 0o600)
	c, err = LoadConfig(fs, get, "/r")
	if err != nil {
		t.Fatal(err)
	}
	if c.Ntfy.URL != "https://ntfy.example" || c.Ntfy.Topic != "night" || c.Caffeinate ||
		c.Tmux != "auto" || c.Merge != "branch-only" || c.Queue != "s.yaml" || c.AllowedTools != "Bash" {
		t.Fatalf("layered = %+v", c)
	}
	if !reflect.DeepEqual(c.Unknown, []string{"/h/.config/agent-relay/config.yaml: colour"}) {
		t.Fatalf("Unknown = %q", c.Unknown)
	}
	if len(c.Sources) != 3 {
		t.Fatalf("Sources = %q", c.Sources)
	}
	fs.WriteFile("/h/.config/agent-relay/config.yaml", []byte("caffeinate: maybe\n"), 0o600)
	if _, err := LoadConfig(fs, get, "/r"); err == nil || !strings.Contains(err.Error(), "caffeinate") {
		t.Fatalf("a bad shape did not fail: %v", err)
	}
}

func TestTmuxMode(t *testing.T) {
	for in, want := range map[string]string{"": "auto", "auto": "auto", "never": "never", "required": "required", "yes": ""} {
		got, err := Config{Tmux: in}.TmuxMode()
		if got != want || (err != nil) != (want == "") {
			t.Errorf("TmuxMode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
