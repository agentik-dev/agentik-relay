package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAR21_2_PidDirDoor (ADV12-8): in a sitting, the file tools are denied
// the account's pid dir (PidDir, night.PidDir in the relay) when
// XDG_STATE_HOME points elsewhere; a state root and a path are compared
// after their symlinks resolve (a /tmp root written through /private/tmp,
// and back); and a `~/` path is read under HOME.
func TestAR21_2_PidDirDoor(t *testing.T) {
	tmp := t.TempDir()
	account, xdg, home := filepath.Join(tmp, "account"), filepath.Join(tmp, "xdg"), filepath.Join(tmp, "home")
	pidDir := filepath.Join(account, ".local", "state", "agent-relay")
	old := PidDir
	t.Cleanup(func() { PidDir = old })
	PidDir = func() (string, error) { return pidDir, nil }
	env := func(k string) string {
		return map[string]string{"RELAY_ROW": "ar21-fixes-adv12", "XDG_STATE_HOME": xdg, "HOME": home}[k]
	}
	for _, r := range []struct{ tool, path, want string }{
		{"Write", filepath.Join(pidDir, "relay.pid"), "pid-dir"},
		{"Edit", filepath.Join(account, ".local", "state", "x", "..", "agent-relay", "relay.pid"), "pid-dir"},
		{"Write", filepath.Join(xdg, "agent-relay", "ledger.jsonl"), "state-root"},
		{"Write", "~/.local/state/agent-relay/relay.pid", ""}, // HOME's default is not the root: XDG_STATE_HOME is
		{"Write", "~/notes.md", ""},
		{"Write", filepath.Join(account, "notes.md"), ""},
	} {
		if got := fileSays(t, env, r.tool, tmp, r.path); got != r.want {
			t.Errorf("%s %s = %q, want %q", r.tool, r.path, got, r.want)
		}
	}
	// ~/ expands to HOME: with no XDG_STATE_HOME, HOME's default is the root.
	noXDG := func(k string) string { return map[string]string{"RELAY_ROW": "r", "HOME": home}[k] }
	for _, p := range []string{"~/.local/state/agent-relay/relay.pid", "~/.local/state/agent-relay"} {
		if got := fileSays(t, noXDG, "Write", tmp, p); got != "state-root" {
			t.Errorf("Write %s = %q, want state-root", p, got)
		}
	}
	// A state root under a symlink (the Mac's /tmp is /private/tmp) is the
	// same root by either spelling, whether or not it exists yet.
	real := filepath.Join(tmp, "real")
	os.MkdirAll(real, 0o755)
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ root, path string }{
		{filepath.Join(link, "s"), filepath.Join(real, "s", "agent-relay", "relay.pid")},
		{filepath.Join(real, "s"), filepath.Join(link, "s", "agent-relay", "ledger.jsonl")},
		{filepath.Join(link, "s"), filepath.Join(real, "s", "agent-relay")},
	} {
		e := func(k string) string { return map[string]string{"RELAY_ROW": "r", "XDG_STATE_HOME": c.root}[k] }
		if got := fileSays(t, e, "Write", tmp, c.path); got != "state-root" {
			t.Errorf("root %s, Write %s = %q, want state-root", c.root, c.path, got)
		}
	}
	PidDir = func() (string, error) { return filepath.Join(link, "p"), nil }
	if got := fileSays(t, env, "Write", tmp, filepath.Join(real, "p", "relay.pid")); got != "pid-dir" {
		t.Errorf("the pid dir through its link: %q, want pid-dir", got)
	}
	if tmpDir, err := filepath.EvalSymlinks("/tmp"); err == nil && tmpDir != "/tmp" {
		e := func(k string) string {
			return map[string]string{"RELAY_ROW": "r", "XDG_STATE_HOME": "/tmp/ar21-2-no-such"}[k]
		}
		if got := fileSays(t, e, "Write", tmp, filepath.Join(tmpDir, "ar21-2-no-such", "agent-relay", "relay.pid")); got != "state-root" {
			t.Errorf("a /tmp root written through %s: %q", tmpDir, got)
		}
	}
	// Outside a sitting nothing is denied.
	if got := fileSays(t, func(k string) string { return "" }, "Write", tmp, filepath.Join(pidDir, "relay.pid")); got != "" {
		t.Errorf("outside a sitting: %q", got)
	}
}
