package hooks

import (
	"encoding/json"
	"strings"
	"testing"
)

// helperSays is the PreToolUse answer for a Bash cmd whose payload carries
// agentID and agentType (a helper's call when either is set): the guard's
// name when it denies, "" when the hook prints nothing. row sets RELAY_ROW
// as ChildEnv does; the env is otherwise empty, so no guard sees a row.
func helperSays(t *testing.T, cmd, agentID, agentType string, row bool) string {
	t.Helper()
	env := func(k string) string {
		if k == "RELAY_ROW" && row {
			return "ar40-helper-relay-guard"
		}
		return ""
	}
	p := map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": cmd}}
	if agentID != "" {
		p["agent_id"] = agentID
	}
	if agentType != "" {
		p["agent_type"] = agentType
	}
	in, _ := json.Marshal(p)
	out := preToolUse(in, env)
	if out == nil {
		return ""
	}
	b, _ := json.Marshal(out)
	s := string(b)
	_, why, ok := strings.Cut(s, `"permissionDecisionReason":"relay: `)
	if !ok || !strings.Contains(s, `"permissionDecision":"deny"`) {
		t.Fatalf("preToolUse(%q) = %s, want a deny or nothing", cmd, s)
	}
	name, _, _ := strings.Cut(why, ":")
	return name
}

// TestAR40_1_HelperDeniedWriteVerbs: a helper's Bash call (agent_id and
// agent_type set, no RELAY_ROW) is denied by helper-relay for every relay
// verb that writes, plan --rows included, and for a go build or go install
// of cmd/relay; either field alone makes the call a helper's, and a helper
// in a row is held for the verbs nested-run does not name.
func TestAR40_1_HelperDeniedWriteVerbs(t *testing.T) {
	const id, typ = "a1b2c3d4", "agent-relay:adversary"
	for _, cmd := range []string{
		"relay run",
		"relay run --orchestrate",
		"relay plan",
		"relay plan --rows a",
		"relay plan --rows=a,b --json",
		"relay gate 2026-09-30-1",
		"relay resume 2026-09-30-1",
		"relay stop",
		"relay kill 2026-09-30-1",
		"relay clean",
		"./bin/relay stop --night 2026-09-30-1",
		"go run ./cmd/relay --json kill",
		"cd /tmp && relay clean",
		"go build -o x ./cmd/relay",
		"go install ./cmd/relay",
	} {
		if got := helperSays(t, cmd, id, typ, false); got != "helper-relay" {
			t.Errorf("helper, %q = %q, want helper-relay", cmd, got)
		}
	}
	for _, c := range []struct{ id, typ, cmd string }{
		{id, "", "relay stop"},
		{"", typ, "relay run --orchestrate"},
		{"", "general-purpose", "go build -o bin/relay ./cmd/relay"},
	} {
		if got := helperSays(t, c.cmd, c.id, c.typ, false); got != "helper-relay" {
			t.Errorf("agent_id %q agent_type %q, %q = %q, want helper-relay", c.id, c.typ, c.cmd, got)
		}
	}
	for _, cmd := range []string{"relay stop", "relay kill x", "relay clean", "relay plan --rows a"} {
		if got := helperSays(t, cmd, id, typ, true); got != "helper-relay" {
			t.Errorf("helper in a row, %q = %q, want helper-relay", cmd, got)
		}
	}
}

// TestAR40_1_HelperReadVerbsPass: the read verbs, the hook itself, and the
// harness's own runs pass from a helper: the hook prints nothing.
func TestAR40_1_HelperReadVerbsPass(t *testing.T) {
	for _, cmd := range []string{
		"relay status",
		"relay status --night 2026-09-30-1",
		"relay report",
		"relay report --night 2026-09-30-1",
		"relay attach --print",
		"relay doctor",
		"relay hook",
		"make check",
		"go test ./cmd/relay -run TestAR40_ -count=1",
		"go test ./cmd/relay",
		"go run ./cmd/relay status",
		"grep -n 'relay stop' design/relay-r1.md",
		"git commit -m 'relay run --orchestrate is held'",
		"echo relay kill",
	} {
		if got := helperSays(t, cmd, "a1b2c3d4", "agent-relay:adversary", false); got != "" {
			t.Errorf("helper, %q denied by %s", cmd, got)
		}
	}
}

// TestAR40_1_MainThreadUnchanged: with no agent_id and no agent_type (the
// main thread) the hook is as it was: out of a row it prints nothing for a
// relay verb that writes; in a row nested-run still names its own verbs and
// lets stop, kill and clean through.
func TestAR40_1_MainThreadUnchanged(t *testing.T) {
	for _, cmd := range []string{"relay run --orchestrate", "relay stop", "relay clean", "go build -o x ./cmd/relay"} {
		if got := helperSays(t, cmd, "", "", false); got != "" {
			t.Errorf("main thread, %q denied by %s", cmd, got)
		}
	}
	for _, r := range []struct{ cmd, want string }{
		{"relay run --orchestrate", "nested-run"},
		{"relay plan --rows a", ""},
		{"relay stop", ""},
		{"relay kill x", ""},
		{"go build -o x ./cmd/relay", "nested-run"},
	} {
		if got := helperSays(t, r.cmd, "", "", true); got != r.want {
			t.Errorf("main thread in a row, %q = %q, want %q", r.cmd, got, r.want)
		}
	}
}
