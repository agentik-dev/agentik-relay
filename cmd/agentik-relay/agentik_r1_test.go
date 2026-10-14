package main

import (
	"reflect"
	"testing"
)

// TestAgentikR1_4_StopLineReadsBoth (agentik-r1 §6.3; ruling 5): a ledger's
// stop line parks its rows whichever spelling its resume lines hold.
func TestAgentikR1_4_StopLineReadsBoth(t *testing.T) {
	got := redSittings(map[string]any{"event": "stop", "resume": []any{"relay resume a", "agentik relay resume b", "no line"}})
	var ids []string
	for _, r := range got {
		ids = append(ids, r[0])
	}
	if !reflect.DeepEqual(ids, []string{"a", "b"}) {
		t.Errorf("redSittings read %q, want a and b", ids)
	}
}

// TestAgentikR1_4_OldNameNote (§4.1; ruling 6): under the old name, on a
// terminal, for any verb but hook, one note; else none.
func TestAgentikR1_4_OldNameNote(t *testing.T) {
	const note = "relay: renamed; use 'agentik relay …' (ends 2026-12-31)"
	for _, tc := range []struct {
		argv0, verb string
		tty         bool
		want        string
	}{
		{"relay", "status", true, note},
		{"/Users/x/go/bin/relay", "run", true, note},
		{"relay", "", true, note},
		{"relay", "hook", true, ""},
		{"relay", "status", false, ""},
		{"agentik-relay", "status", true, ""},
		{"/x/agentik-relay", "hook", false, ""},
	} {
		if got := aliasNote(tc.argv0, tc.verb, tc.tty); got != tc.want {
			t.Errorf("aliasNote(%q, %q, %v) = %q, want %q", tc.argv0, tc.verb, tc.tty, got, tc.want)
		}
	}
}
