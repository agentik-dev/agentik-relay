package effects

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestAgentikR1_3_StateDirNewElseOld (agentik-r1 §6.3 c2; ruling 3): the
// state dir is agentik-relay when it exists, else agent-relay when it
// exists, else agentik-relay, under XDG_STATE_HOME or HOME's .local/state.
func TestAgentikR1_3_StateDirNewElseOld(t *testing.T) {
	base := t.TempDir()
	env := func(k string) string {
		if k == "XDG_STATE_HOME" {
			return base
		}
		return ""
	}
	nw, old := filepath.Join(base, "agentik-relay"), filepath.Join(base, "agent-relay")
	if got := StateDir(env); got != nw {
		t.Errorf("neither: StateDir = %q, want %q", got, nw)
	}
	if err := os.Mkdir(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := StateDir(env); got != old {
		t.Errorf("the old only: StateDir = %q, want %q", got, old)
	}
	if err := os.Mkdir(nw, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := StateDir(env); got != nw {
		t.Errorf("both: StateDir = %q, want %q", got, nw)
	}
	home := t.TempDir()
	if got, want := StateDir(func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}), filepath.Join(home, ".local", "state", "agentik-relay"); got != want {
		t.Errorf("HOME: StateDir = %q, want %q", got, want)
	}
}

// TestAgentikR1_3_LayersNewElseOld (§6.3 c1, c3, c4; rulings 3, 4): each
// of the three layers is read from its new file when it exists, else its
// old one; the repo's old file is flagged in Warnings whenever it exists.
func TestAgentikR1_3_LayersNewElseOld(t *testing.T) {
	env := map[string]string{"HOME": "/h", "CLAUDE_CODE_SESSION_ID": "s1"}
	get := func(k string) string { return env[k] }
	fs := &FakeFS{}
	fs.WriteFile("/h/.config/agent-relay/config.yaml", []byte("tmux: never\n"), 0o600)
	fs.WriteFile("/r/.claude/claude-code-session/config.yaml", []byte("agent-relay: {queue: o.yaml}\n"), 0o600)
	fs.WriteFile("/h/.local/state/claude-code-session/sessions/s1/resolved.yaml", []byte("agent-relay: {allowed_tools: Bash}\n"), 0o600)
	c, err := LoadConfig(fs, get, "/r")
	if err != nil || c.Tmux != "never" || c.Queue != "o.yaml" || c.AllowedTools != "Bash" || len(c.Sources) != 3 {
		t.Fatalf("the old files = %+v, %v", c, err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "/r/.claude/claude-code-session/config.yaml") ||
		!strings.Contains(c.Warnings[0], RepoConfigFile) {
		t.Errorf("the old repo file: Warnings = %q", c.Warnings)
	}
	fs.WriteFile("/h/.config/agentik/relay/config.yaml", []byte("tmux: auto\n"), 0o600)
	fs.WriteFile("/r/.claude/agentik/config.yaml", []byte("relay: {queue: n.yaml}\n"), 0o600)
	fs.WriteFile("/h/.local/state/agentik-session/sessions/s1/resolved.yaml", []byte("relay: {allowed_tools: Edit}\n"), 0o600)
	c, err = LoadConfig(fs, get, "/r")
	want := []string{"/h/.config/agentik/relay/config.yaml", "/r/.claude/agentik/config.yaml",
		"/h/.local/state/agentik-session/sessions/s1/resolved.yaml"}
	if err != nil || c.Tmux != "auto" || c.Queue != "n.yaml" || c.AllowedTools != "Edit" || !reflect.DeepEqual(c.Sources, want) {
		t.Fatalf("the new files = %+v, %v", c, err)
	}
	if len(c.Warnings) != 1 {
		t.Errorf("the old repo file beside the new: Warnings = %q, want it flagged", c.Warnings)
	}
	if RepoConfigFile != ".claude/agentik/config.yaml" {
		t.Errorf("RepoConfigFile = %q", RepoConfigFile)
	}
}

// TestAgentikR1_3_RelaySectionWins (§6.3 c6; ruling 4): a layer's old
// `agent-relay` section alone is read without a word; with `relay` beside
// it, `relay` wins, the old is not merged, and Warnings says so.
func TestAgentikR1_3_RelaySectionWins(t *testing.T) {
	get := func(k string) string { return map[string]string{"HOME": "/h"}[k] }
	fs := &FakeFS{}
	fs.WriteFile("/r/.claude/agentik/config.yaml", []byte("agent-relay: {queue: old.yaml}\n"), 0o600)
	c, err := LoadConfig(fs, get, "/r")
	if err != nil || c.Queue != "old.yaml" || len(c.Warnings) != 0 {
		t.Fatalf("the old section alone = %+v, %v", c, err)
	}
	fs.WriteFile("/r/.claude/agentik/config.yaml", []byte("agent-relay: {queue: old.yaml, tmux: never}\nrelay: {queue: new.yaml}\n"), 0o600)
	c, err = LoadConfig(fs, get, "/r")
	if err != nil || c.Queue != "new.yaml" || c.Tmux != "auto" {
		t.Fatalf("both sections = %+v, %v", c, err)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "relay wins") {
		t.Errorf("both sections: Warnings = %q", c.Warnings)
	}
}
