package hooks

import (
	"path/filepath"
	"testing"
)

// TestAR76_4_HookStateTilde (session-layer-r1 §2.4; AR76 ruling 1): the
// hooks read $XDG_STATE_HOME as the relay does, `~/` under HOME, so the
// state file, the soft request and the file door's state root are the
// relay's; with neither HOME nor the value, no base is known.
func TestAR76_4_HookStateTilde(t *testing.T) {
	home := t.TempDir()
	env := func(k string) string {
		return map[string]string{"HOME": home, "XDG_STATE_HOME": "~/x", "RELAY_ROW": "ar76"}[k]
	}
	dir := filepath.Join(home, "x", "agentik-relay", "sessions", "s1")
	if got, want := statePath(env, "s1"), filepath.Join(dir, "start.json"); got != want {
		t.Errorf("statePath = %q, want %q", got, want)
	}
	if got, want := SoftPath(env, "s1"), filepath.Join(dir, "soft.json"); got != want {
		t.Errorf("SoftPath = %q, want %q", got, want)
	}
	was := PidDir
	t.Cleanup(func() { PidDir = was })
	PidDir = nil
	ledger := filepath.Join(home, "x", "agentik-relay", "ledger", "a.jsonl")
	if g, ok := fileDoor(ledger, home, env); !ok || g.Name != StateRoot.Name {
		t.Errorf("fileDoor(%q) = %q, %v; want %s", ledger, g.Name, ok, StateRoot.Name)
	}
	if got := statePath(func(string) string { return "" }, "s1"); got != "" {
		t.Errorf("no HOME, no XDG_STATE_HOME: statePath = %q, want none", got)
	}
}
