package effects

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The AR75 fixture: the relay reads /r, the session is s1 under /h.
const (
	ar75Repo    = "/r/.claude/agentik/config.yaml"
	ar75Local   = "/r/.claude/agentik/config.local.yaml"
	ar75Session = "/h/.local/state/agentik-session/sessions/s1/resolved.yaml"
)

func ar75Env(k string) string {
	return map[string]string{"HOME": "/h", "CLAUDE_CODE_SESSION_ID": "s1"}[k]
}

func ar75NoSession(k string) string { return map[string]string{"HOME": "/h"}[k] }

// sumIn is path's sha256 in fake as the session plugin records it: 64
// lowercase hex digits, or null for an absent file.
func sumIn(fake *FakeFS, path string) string {
	b, ok := fake.Files[path]
	if !ok {
		return "null"
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// provider is a session file's provider block, its inputs the given path
// and sha256 pairs (AR75 ruling 6).
func provider(pairs ...string) string {
	b := "provider:\n  inputs:\n"
	for i := 0; i+1 < len(pairs); i += 2 {
		b += "    - path: " + pairs[i] + "\n      sha256: " + pairs[i+1] + "\n"
	}
	return b
}

// inputs is a provider block listing each path at its sha256 in fake now.
func inputs(fake *FakeFS, paths ...string) string {
	var pairs []string
	for _, p := range paths {
		pairs = append(pairs, p, sumIn(fake, p))
	}
	return provider(pairs...)
}

// unreadableFS fails every read of path after its first: the repo file the
// layers read turns unreadable before the session layer's check reads it
// (an unreadable layer file is LoadConfig's error at its own read).
type unreadableFS struct {
	*FakeFS
	path  string
	reads int
}

func (f *unreadableFS) ReadFile(p string) ([]byte, error) {
	if p == f.path {
		if f.reads++; f.reads > 1 {
			return nil, &os.PathError{Op: "read", Path: p, Err: os.ErrPermission}
		}
	}
	return f.FakeFS.ReadFile(p)
}

// ar75Unread asserts the session layer was skipped for reason: the repo's
// verify_seconds (1800) in force, one warning naming the layer and the
// reason, and the layer not in Sources.
func ar75Unread(t *testing.T, name string, c Config, err error, reason string) {
	t.Helper()
	if err != nil || c.VerifySeconds != 1800 {
		t.Errorf("%s: verify_seconds = %d, %v; want the repo's 1800", name, c.VerifySeconds, err)
	}
	if want := []string{ar75Session + ": not read: " + reason}; !reflect.DeepEqual(c.Warnings, want) {
		t.Errorf("%s: Warnings = %q, want %q", name, c.Warnings, want)
	}
	for _, s := range c.Sources {
		if s == ar75Session {
			t.Errorf("%s: Sources = %q, the skipped layer listed", name, c.Sources)
		}
	}
}

// TestAR75_1_FreshLayerRead (session-layer-r1 §2.1, §2.2; ruling 1, 2): a
// layer whose inputs list the relay's repo and local files at their sha256
// now is read over both, the local file read between them; a profiles and
// an overlay input at wrong hashes do not matter.
func TestAR75_1_FreshLayerRead(t *testing.T) {
	fake := &FakeFS{}
	profiles := "/h/.config/agentik/session/profiles.yaml"
	overlay := "/h/.local/state/agentik-session/sessions/s1/overlay.yaml"
	fake.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	fake.WriteFile(ar75Local, []byte("relay: {tmux: never}\n"), 0o600)
	fake.WriteFile(profiles, []byte("default: {}\n"), 0o600)
	fake.WriteFile(overlay, []byte("_prompts_since_checkpoint: 1\n"), 0o600)
	fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+provider(
		ar75Repo, sumIn(fake, ar75Repo), ar75Local, sumIn(fake, ar75Local),
		profiles, strings.Repeat("0", 64), overlay, strings.Repeat("f", 64))), 0o600)
	c, err := LoadConfig(fake, ar75Env, "/r")
	want := []string{ar75Repo, ar75Local, ar75Session}
	if err != nil || !reflect.DeepEqual(c.Sources, want) || len(c.Warnings) != 0 {
		t.Fatalf("a fresh layer: Sources = %q, Warnings = %q, %v; want %q and none", c.Sources, c.Warnings, err, want)
	}
	if c.VerifySeconds != 3600 || c.Tmux != "never" {
		t.Errorf("a fresh layer: verify_seconds = %d, tmux = %q; want the layer's 3600 and the local's never", c.VerifySeconds, c.Tmux)
	}
}

// TestAR75_2_StaleLayerSkipped (§2.1, §2.3; ruling 2, 3): the repo file
// edited after resolve, a recorded-null local now present, and the repo
// file unreadable each leave the layer unread, naming that path.
func TestAR75_2_StaleLayerSkipped(t *testing.T) {
	edited := &FakeFS{}
	edited.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 900}\n"), 0o600)
	edited.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+inputs(edited, ar75Repo, ar75Local)), 0o600)
	edited.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	c, err := LoadConfig(edited, ar75Env, "/r")
	ar75Unread(t, "the repo file edited", c, err, ar75Repo+" changed since the session resolved it")

	local := &FakeFS{}
	local.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	local.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+inputs(local, ar75Repo, ar75Local)), 0o600)
	local.WriteFile(ar75Local, []byte("relay: {tmux: never}\n"), 0o600)
	c, err = LoadConfig(local, ar75Env, "/r")
	ar75Unread(t, "a null local now present", c, err, ar75Local+" changed since the session resolved it")
	if c.Tmux != "never" {
		t.Errorf("a null local now present: tmux = %q, want the local's never", c.Tmux)
	}

	gone := &FakeFS{}
	gone.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	gone.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+inputs(gone, ar75Repo, ar75Local)), 0o600)
	c, err = LoadConfig(&unreadableFS{FakeFS: gone, path: ar75Repo}, ar75Env, "/r")
	ar75Unread(t, "the repo file unreadable", c, err, ar75Repo+" changed since the session resolved it")
}

// TestAR75_3_OtherCheckoutSkipped (§2.1, §2.3; ruling 2): a layer resolved
// in /w is unread while the relay reads /r, for the repo file and the local
// alike; a spelling that cleans to the relay's path is the relay's.
func TestAR75_3_OtherCheckoutSkipped(t *testing.T) {
	fake := &FakeFS{}
	fake.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	fake.WriteFile("/w/.claude/agentik/config.yaml", []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+
		inputs(fake, "/w/.claude/agentik/config.yaml", "/w/.claude/agentik/config.local.yaml")), 0o600)
	c, err := LoadConfig(fake, ar75Env, "/r")
	ar75Unread(t, "another checkout's repo file", c, err, "resolved for another checkout: "+ar75Repo+" not listed")

	fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+
		inputs(fake, ar75Repo, "/w/.claude/agentik/config.local.yaml")), 0o600)
	c, err = LoadConfig(fake, ar75Env, "/r")
	ar75Unread(t, "another checkout's local", c, err, "resolved for another checkout: "+ar75Local+" not listed")

	fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+provider(
		"/r//.claude/./agentik/config.yaml", sumIn(fake, ar75Repo), "/r/.claude/x/../agentik/config.local.yaml", "null")), 0o600)
	if c, err = LoadConfig(fake, ar75Env, "/r"); err != nil || c.VerifySeconds != 3600 || len(c.Warnings) != 0 {
		t.Errorf("an unclean spelling of /r: verify_seconds = %d, Warnings = %q, %v; want 3600 and none", c.VerifySeconds, c.Warnings, err)
	}
}

// TestAR75_4_UnprovenLayerSkipped (§2.3; ruling 2): no provider block, no
// inputs, a malformed list each leave the layer unread; a relative path is
// skipped before any check, and the layer is read.
func TestAR75_4_UnprovenLayerSkipped(t *testing.T) {
	fake := &FakeFS{}
	fake.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 1800}\n"), 0o600)
	good := provider(ar75Repo, sumIn(fake, ar75Repo), ar75Local, "null")
	for _, tc := range []struct{ name, block, reason string }{
		{"no provider", "", "no provider.inputs"},
		{"no inputs", "provider: {resolver: r1}\n", "no provider.inputs"},
		{"inputs a scalar", "provider:\n  inputs: none\n", "malformed provider.inputs"},
		{"a sha256 of abc", provider(ar75Repo, "abc", ar75Local, "null"), "malformed provider.inputs"},
		{"upper-case hex", provider(ar75Repo, strings.Repeat("A", 64), ar75Local, "null"), "malformed provider.inputs"},
		{"an item without path", good + "    - sha256: null\n", "malformed provider.inputs"},
		{"an item without sha256", good + "    - path: /h/x.yaml\n", "malformed provider.inputs"},
		{"an item a scalar", good + "    - /h/x.yaml\n", "malformed provider.inputs"},
	} {
		fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+tc.block), 0o600)
		c, err := LoadConfig(fake, ar75Env, "/r")
		ar75Unread(t, tc.name, c, err, tc.reason)
	}
	fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 3600}\n"+provider(ar75Repo, sumIn(fake, ar75Repo),
		"config.yaml", "abc", ".claude/agentik/config.local.yaml", strings.Repeat("0", 64), ar75Local, "null")), 0o600)
	c, err := LoadConfig(fake, ar75Env, "/r")
	if want := []string{ar75Repo, ar75Session}; err != nil || c.VerifySeconds != 3600 || len(c.Warnings) != 0 || !reflect.DeepEqual(c.Sources, want) {
		t.Errorf("relative paths: verify_seconds = %d, Warnings = %q, Sources = %q, %v; want 3600, none, %q", c.VerifySeconds, c.Warnings, c.Sources, err, want)
	}
}

// TestAR75_5_LocalLayer (§2.2; ruling 1): config.local.yaml is read over
// the repo's file and under the session's, from the picked repo file's
// folder only.
func TestAR75_5_LocalLayer(t *testing.T) {
	fake := &FakeFS{}
	fake.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 100, queue: r.yaml}\n"), 0o600)
	fake.WriteFile(ar75Local, []byte("relay: {verify_seconds: 200, tmux: never}\n"), 0o600)
	c, err := LoadConfig(fake, ar75NoSession, "/r")
	if want := []string{ar75Repo, ar75Local}; err != nil || c.VerifySeconds != 200 || c.Queue != "r.yaml" || c.Tmux != "never" || !reflect.DeepEqual(c.Sources, want) {
		t.Errorf("local over repo = %+v, %v", c, err)
	}
	fake.WriteFile(ar75Session, []byte("relay: {verify_seconds: 300}\n"+inputs(fake, ar75Repo, ar75Local)), 0o600)
	c, err = LoadConfig(fake, ar75Env, "/r")
	if err != nil || c.VerifySeconds != 300 || c.Queue != "r.yaml" || c.Tmux != "never" || len(c.Sources) != 3 {
		t.Errorf("session over local = %+v, %v", c, err)
	}

	oldRepo := filepath.Join("/r", oldRepoConfig) // the old folder, by config.go's one spelling of it
	oldLocal := filepath.Join(filepath.Dir(oldRepo), localConfig)
	beside := &FakeFS{}
	beside.WriteFile(ar75Repo, []byte("relay: {verify_seconds: 100}\n"), 0o600)
	beside.WriteFile(oldLocal, []byte(oldSection+": {verify_seconds: 999}\n"), 0o600)
	c, err = LoadConfig(beside, ar75NoSession, "/r")
	if want := []string{ar75Repo}; err != nil || c.VerifySeconds != 100 || !reflect.DeepEqual(c.Sources, want) {
		t.Errorf("an old-folder local beside a new-folder config.yaml = %+v, %v; want it unread", c, err)
	}

	pair := &FakeFS{}
	pair.WriteFile(oldRepo, []byte(oldSection+": {verify_seconds: 100}\n"), 0o600)
	pair.WriteFile(oldLocal, []byte(oldSection+": {verify_seconds: 200}\n"), 0o600)
	pair.WriteFile(ar75Local, []byte("relay: {verify_seconds: 777}\n"), 0o600)
	c, err = LoadConfig(pair, ar75NoSession, "/r")
	if want := []string{oldRepo, oldLocal}; err != nil || c.VerifySeconds != 200 || !reflect.DeepEqual(c.Sources, want) {
		t.Errorf("the old folder's pair = %+v, %v; want %q read, the new-folder local not", c, err, want)
	}
}

// TestAR75_6_StateHomeTilde (§2.4; ruling 4): `~` and a leading `~/` in
// $XDG_STATE_HOME name HOME; ConfigPaths has four layers with a session,
// three without.
func TestAR75_6_StateHomeTilde(t *testing.T) {
	for state, want := range map[string]string{
		"~/x": "/h/x/agentik-session/sessions/s1/resolved.yaml",
		"~":   "/h/agentik-session/sessions/s1/resolved.yaml",
		"/s":  "/s/agentik-session/sessions/s1/resolved.yaml",
		"~x":  "~x/agentik-session/sessions/s1/resolved.yaml",
		"":    ar75Session,
	} {
		env := map[string]string{"HOME": "/h", "CLAUDE_CODE_SESSION_ID": "s1", "XDG_STATE_HOME": state}
		got := ConfigPaths(&FakeFS{}, func(k string) string { return env[k] }, "/r")
		if len(got) != 4 || got[3] != want {
			t.Errorf("XDG_STATE_HOME=%q: ConfigPaths = %q, want the session's %q", state, got, want)
		}
	}
	if got := ConfigPaths(&FakeFS{}, ar75NoSession, "/r"); len(got) != 3 || got[2] != ar75Local {
		t.Errorf("no session: ConfigPaths = %q, want three ending %q", got, ar75Local)
	}
}
