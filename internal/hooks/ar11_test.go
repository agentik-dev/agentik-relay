package hooks

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAR11_4_HookDeniesNestedRun: with RELAY_ROW set (a sitting), the
// nested-run guard denies `relay run` and `relay plan` without --rows in any
// segment, lets `relay plan --rows` and the read verbs through, and is
// absent outside a sitting (relay-r1 §15, AR11; NoNestedSpawn in Night.tla).
func TestAR11_4_HookDeniesNestedRun(t *testing.T) {
	inRow := func(k string) string { return map[string]string{"RELAY_ROW": "ar11-tla-night"}[k] }
	gs := guards(inRow)
	for _, r := range []struct{ cmd, want string }{
		{"relay run", "nested-run"},
		{"relay run --rows a --dry", "nested-run"},
		{"./bin/relay run", "nested-run"},
		{"make build && bin/relay run --parallel 2", "nested-run"},
		{"bash -c 'relay run'", "nested-run"},
		{"env RELAY_ROW= relay run", "nested-run"},
		{"relay plan", "nested-run"},
		{"relay plan --json", "nested-run"},
		{"relay plan --rows a", ""},
		{"relay plan --rows=a,b --json", ""},
		{"relay plan --json --rows a", ""},
		{"relay status", ""},
		{"relay gate a", "nested-run"}, // AR17B: a sitting never gates
		{"echo relay run", ""},
		{"git commit -m 'relay run guard'", ""},
		{"grep -n 'relay run' design/relay-r1.md", ""},
	} {
		g, ok := match(r.cmd, gs)
		if got := map[bool]string{true: g.Name}[ok]; got != r.want {
			t.Errorf("in a row, match(%q) = %q, want %q", r.cmd, got, r.want)
		}
	}
	if _, ok := match("relay run", guards(func(string) string { return "" })); ok {
		t.Fatal("relay run denied outside a sitting")
	}
	in := `{"tool_name":"Bash","tool_input":{"command":"relay run --rows a"}}`
	b, _ := json.Marshal(preToolUse([]byte(in), inRow))
	if s := string(b); !strings.Contains(s, `"permissionDecision":"deny"`) || !strings.Contains(s, "relay: nested-run: ") {
		t.Fatalf("preToolUse = %s", s)
	}
}
