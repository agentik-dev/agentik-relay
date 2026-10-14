package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR28_2_PostToolUseAsksStop: in a sitting whose session has a soft
// file, PostToolUse answers the note that asks the child to land the act
// in flight; no file, a file that does not parse, a helper and no row read
// {}. hooks.json runs PostToolUse on every tool (no matcher).
func TestAR28_2_PostToolUseAsksStop(t *testing.T) {
	state := t.TempDir()
	const session = "5f0c1a2b-0000-4000-8000-000000000001"
	row := map[string]string{"XDG_STATE_HOME": state, "RELAY_ROW": "a1"}
	path := SoftPath(func(k string) string { return row[k] }, session)
	if path != filepath.Join(state, "agentik-relay", "sessions", session, "soft.json") {
		t.Fatalf("SoftPath %q is not beside start.json", path)
	}
	good := []byte(`{"ceiling":130000,"context":118000,"soft":117000,"turns":2}` + "\n")
	note := `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"relay: soft ceiling: context 118k of 130k. ` +
		`Land the act in flight now (its gate, its commit, the charter's exit line), then end your turn; start no new act."}}` + "\n"
	for _, c := range []struct {
		name string
		env  map[string]string
		file []byte
		want string
	}{
		{"the file", row, good, note},
		{"none", row, nil, "{}\n"},
		{"a bad one", row, []byte("{not json"), "{}\n"},
		{"a helper", map[string]string{"XDG_STATE_HOME": state, "RELAY_ROW": "a1", "CLAUDE_SESSION_HELPER": "1"}, good, "{}\n"},
		{"no row", map[string]string{"XDG_STATE_HOME": state}, good, "{}\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fx := &Fake{Files: map[string][]byte{}}
			if c.file != nil {
				fx.Files[path] = c.file
			}
			var out bytes.Buffer
			in := `{"session_id":"` + session + `","hook_event_name":"PostToolUse","tool_name":"Bash"}`
			if code := Run("PostToolUse", strings.NewReader(in), &out, func(k string) string { return c.env[k] }, fx); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if out.String() != c.want {
				t.Fatalf("answer %s, want %s", out.String(), c.want)
			}
		})
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "plugin", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher *string `json:"matcher"`
			Hooks   []struct{ Command string }
		}
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	post := cfg.Hooks["PostToolUse"]
	if len(post) != 1 || post[0].Matcher != nil || len(post[0].Hooks) != 1 ||
		post[0].Hooks[0].Command != `"${CLAUDE_PLUGIN_ROOT}/hooks/run" PostToolUse` {
		t.Fatalf("hooks.json PostToolUse %+v", post)
	}
}
