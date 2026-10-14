package effects

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAR76_1_StateDirTilde (session-layer-r1 §2.4; AR76 ruling 1): `~` and
// a leading `~/` in $XDG_STATE_HOME name HOME for the relay's own state
// dir, and the session file lies under the same base.
func TestAR76_1_StateDirTilde(t *testing.T) {
	for state, base := range map[string]string{
		"~/x": "/h/x",
		"~":   "/h",
		"/s":  "/s",
		"~x":  "~x",
		"":    "/h/.local/state",
	} {
		env := map[string]string{"HOME": "/h", "CLAUDE_CODE_SESSION_ID": "s1", "XDG_STATE_HOME": state}
		get := func(k string) string { return env[k] }
		if got, want := StateDir(get), filepath.Join(base, stateName); got != want {
			t.Errorf("XDG_STATE_HOME=%q: StateDir = %q, want %q", state, got, want)
		}
		paths := ConfigPaths(&FakeFS{}, get, "/r")
		if want := filepath.Join(base, "agentik-session", "sessions", "s1", "resolved.yaml"); len(paths) != 4 || paths[3] != want {
			t.Errorf("XDG_STATE_HOME=%q: ConfigPaths = %q, want the session's %q", state, paths, want)
		}
	}
}

// ar76Moved is the session layer's warning in c, "" when there is none.
func ar76Moved(c Config) string {
	for _, w := range c.Warnings {
		if strings.HasPrefix(w, ar75Session+": not read: ") {
			return strings.TrimPrefix(w, ar75Session+": not read: ")
		}
	}
	return ""
}

// TestAR76_2_FolderMovedReason (§2.3; AR76 ruling 2): a layer listing the
// relay's file under the repo's other config folder names the folder move
// and the listed twin, either way; another checkout's files keep their
// reason.
func TestAR76_2_FolderMovedReason(t *testing.T) {
	oldRepo := filepath.Join("/r", oldRepoConfig)
	oldLocal := filepath.Join(filepath.Dir(oldRepo), localConfig)

	moved := &FakeFS{}
	moved.WriteFile(oldRepo, []byte(oldSection+": {verify_seconds: 1800}\n"), 0o600)
	moved.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+inputs(moved, oldRepo, oldLocal)), 0o600)
	moved.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	moved.Remove(oldRepo)
	c, err := LoadConfig(moved, ar75Env, "/r")
	ar75Unread(t, "the new folder read, the old listed", c, err, "resolved before the config folder moved: "+oldRepo)

	back := &FakeFS{}
	back.WriteFile(oldRepo, []byte(oldSection+": {verify_seconds: 1800}\n"), 0o600)
	back.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+inputs(back, ar75Repo, ar75Local)), 0o600)
	c, err = LoadConfig(back, ar75Env, "/r")
	if why := ar76Moved(c); err != nil || c.VerifySeconds != 1800 || why != "resolved before the config folder moved: "+ar75Repo {
		t.Errorf("the old folder read, the new listed: verify_seconds = %d, reason %q, %v; want 1800 and the move naming %s", c.VerifySeconds, why, err, ar75Repo)
	}

	other := &FakeFS{}
	other.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	other.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+
		inputs(other, filepath.Join("/w", oldRepoConfig), filepath.Join("/w", filepath.Dir(oldRepoConfig), localConfig))), 0o600)
	c, err = LoadConfig(other, ar75Env, "/r")
	ar75Unread(t, "another checkout's old folder", c, err, "resolved for another checkout: "+ar75Repo+" not listed")
}

// TestAR76_3_NullInputs (§2.3; AR76 ruling 3): YAML's null for
// provider.inputs is no list; any other scalar is malformed; an empty list
// lists nothing.
func TestAR76_3_NullInputs(t *testing.T) {
	fake := &FakeFS{}
	fake.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	for _, tc := range []struct{ name, block, reason string }{
		{"a flow null", "provider: {inputs: null}\n", "no provider.inputs"},
		{"a tilde", "provider:\n  inputs: ~\n", "no provider.inputs"},
		{"upper-case NULL", "provider:\n  inputs: NULL\n", "no provider.inputs"},
		{"no value", "provider:\n  inputs:\n", "no provider.inputs"},
		{"a word", "provider:\n  inputs: none\n", "malformed provider.inputs"},
		{"an empty list", "provider:\n  inputs: []\n", "resolved for another checkout: " + ar75Repo + " not listed"},
	} {
		fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+tc.block), 0o600)
		c, err := LoadConfig(fake, ar75Env, "/r")
		ar75Unread(t, tc.name, c, err, tc.reason)
	}
}
