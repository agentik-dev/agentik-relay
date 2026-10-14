package hooks

import (
	"strings"
	"testing"
)

// TestAgentikR1_1_RowDeniesEverySpelling (agentik-r1 §6.3 c12, §9 A6): a row
// is denied the relay's writing verbs and a new sitting under every
// spelling, the new and the old; a reading verb passes; the session's
// charter verb passes under every spelling, since the orchestrator's rules
// tell a row to open its charter (NICE_TO_HAVE's last line, confirmed here).
func TestAgentikR1_1_RowDeniesEverySpelling(t *testing.T) {
	for _, tc := range []struct{ cmd, guard string }{
		{"agentik relay run", "nested-run"},
		{"agentik-relay run", "nested-run"},
		{"relay run", "nested-run"},
		{"agentik relay gate", "nested-run"},
		{"agentik-relay resume ar1", "nested-run"},
		{"/usr/local/bin/agentik-relay plan", "nested-run"},
		{"agentik session start", "nested-session"},
		{"claude-session start", "nested-session"},
		{"agentik-session start", "nested-session"},
		{"agentik-session stop", "nested-session"},
		{"claude-session report", "nested-session"},
		{"go build ./cmd/agentik-relay", "nested-run"},
		{"go install ./cmd/agentik-relay", "nested-run"},
		{"go build -o /tmp/x ./cmd/agentik-relay/main.go", "nested-run"},
		{"cd cmd && go run ./agentik-relay gate", "nested-run"},
	} {
		deny, why := Verdict(tc.cmd, true)
		if !deny || !strings.HasPrefix(why, "relay: "+tc.guard+": ") {
			t.Errorf("Verdict(%q, true) = %v, %q; want denied by %s", tc.cmd, deny, why, tc.guard)
		}
	}
	for _, cmd := range []string{
		"agentik relay status", "agentik-relay status", "relay status", "agentik relay plan --rows ar1",
		"claude-session charter open", "agentik-session charter open", "agentik session charter open",
		"agentik session charter", "echo agentik session start",
	} {
		if deny, why := Verdict(cmd, true); deny {
			t.Errorf("Verdict(%q, true) = %q; want allowed in a row", cmd, why)
		}
	}
}

// TestAgentikR1_1_HelperDeniesEverySpelling (§6.3 c12; AR40): a helper is
// denied the relay's writing verbs under every spelling, in a row or out of
// one; the reading verbs pass.
func TestAgentikR1_1_HelperDeniesEverySpelling(t *testing.T) {
	for _, row := range []bool{true, false} {
		for _, cmd := range []string{
			"agentik relay stop", "agentik-relay kill", "relay clean", "agentik relay plan --rows ar1",
			"agentik-relay gate", "~/go/bin/agentik-relay resume", "go build ./cmd/agentik-relay",
		} {
			if got := helperSays(t, cmd, "a1", "builder", row); got != "helper-relay" && got != "nested-run" {
				t.Errorf("helper %q (row %v) = %q; want denied", cmd, row, got)
			}
		}
		for _, cmd := range []string{"agentik relay status", "agentik-relay report", "relay doctor"} {
			if got := helperSays(t, cmd, "a1", "builder", row); got != "" {
				t.Errorf("helper %q (row %v) = %q; want allowed", cmd, row, got)
			}
		}
	}
}
