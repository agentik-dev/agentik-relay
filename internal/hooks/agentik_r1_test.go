package hooks

import (
	"os"
	"path/filepath"
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

// TestAgentikR1_3_StateRootNewElseOld (agentik-r1 §6.3 c2; ruling 3): the
// hooks' state root is agentik-relay when it exists, else agent-relay when
// it exists, else agentik-relay; in a row the file door denies both
// spellings of the state root and of the pid dir, whichever exists.
func TestAgentikR1_3_StateRootNewElseOld(t *testing.T) {
	base := t.TempDir()
	env := func(k string) string {
		switch k {
		case "XDG_STATE_HOME":
			return base
		case "RELAY_ROW":
			return "agentik-r1-relay"
		}
		return ""
	}
	nw, old := filepath.Join(base, "agentik-relay"), filepath.Join(base, "agent-relay")
	if got := stateRoot(env); got != nw {
		t.Errorf("neither: stateRoot = %q, want %q", got, nw)
	}
	if err := os.Mkdir(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := stateRoot(env); got != old {
		t.Errorf("the old only: stateRoot = %q, want %q", got, old)
	}
	if got, want := SoftPath(env, "s1"), filepath.Join(old, "sessions", "s1", "soft.json"); got != want {
		t.Errorf("the old only: SoftPath = %q, want %q", got, want)
	}
	if err := os.Mkdir(nw, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := stateRoot(env); got != nw {
		t.Errorf("both: stateRoot = %q, want %q", got, nw)
	}
	home := t.TempDir()
	was := PidDir
	t.Cleanup(func() { PidDir = was })
	PidDir = func() (string, error) { return filepath.Join(home, ".local", "state", "agentik-relay"), nil }
	for _, p := range []string{
		filepath.Join(nw, "x"), filepath.Join(old, "ledger", "x"),
		filepath.Join(home, ".local", "state", "agent-relay", "relay.pid"),
		filepath.Join(home, ".local", "state", "agentik-relay", "relay.pid"),
	} {
		if g, ok := fileDoor(p, base, env); !ok || g.Name != StateRoot.Name && g.Name != PidLock.Name {
			t.Errorf("fileDoor(%q) = %q, %v; want the state root or the pid lock", p, g.Name, ok)
		}
	}
	if g, ok := fileDoor(filepath.Join(base, "agentik-relayx", "y"), base, env); ok {
		t.Errorf("a sibling of the state root is denied by %s", g.Name)
	}
}
