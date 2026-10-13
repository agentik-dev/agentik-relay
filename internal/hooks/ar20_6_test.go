package hooks

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR20_6_PushAllowedOutsideSitting (the owner, 2026-09-24): with no
// RELAY_ROW the push guard is absent, so the owner's own session pushes.
func TestAR20_6_PushAllowedOutsideSitting(t *testing.T) {
	for _, cmd := range []string{"git push", "git push origin main", "git send-pack /tmp/bare.git main"} {
		if got := hookSays(t, cmd, false); got != "" {
			t.Errorf("outside a sitting, %q denied by %q", cmd, got)
		}
		if deny, why := Verdict(cmd, false); deny {
			t.Errorf("Verdict(%q, false) = %q", cmd, why)
		}
	}
}

// TestAR20_6_PushDeniedInSitting: with RELAY_ROW set, as ChildEnv sets it,
// every push door is still denied, by the push guard first.
func TestAR20_6_PushDeniedInSitting(t *testing.T) {
	for _, cmd := range []string{"git push", "git push origin HEAD:main", "make check && git push",
		"git subtree push --prefix=x origin main", "/usr/libexec/git-core/git-send-pack /tmp/bare.git main"} {
		if got := hookSays(t, cmd, true); got != "push" {
			t.Errorf("in a sitting, %q = %q, want push", cmd, got)
		}
		if deny, why := Verdict(cmd, true); !deny || !strings.HasPrefix(why, "relay: push: ") {
			t.Errorf("Verdict(%q, true) = %v, %q", cmd, deny, why)
		}
	}
}

// fileSays is the PreToolUse answer for a file tool: the guard's name when
// it denies, "" when it allows.
func fileSays(t *testing.T, env func(string) string, tool, cwd, path string) string {
	t.Helper()
	key := "file_path"
	if tool == "NotebookEdit" {
		key = "notebook_path"
	}
	in, _ := json.Marshal(map[string]any{"tool_name": tool, "cwd": cwd, "tool_input": map[string]string{key: path}})
	b, _ := json.Marshal(preToolUse(in, env))
	s := string(b)
	if s == "null" {
		return ""
	}
	_, why, ok := strings.Cut(s, `"permissionDecisionReason":"relay: `)
	if !ok || !strings.Contains(s, `"permissionDecision":"deny"`) {
		t.Fatalf("preToolUse(%s %q) = %s", tool, path, s)
	}
	name, _, _ := strings.Cut(why, ":")
	return name
}

// TestAR20_6_FileToolDoor (ADV11-11): inside a sitting, Write, Edit,
// MultiEdit and NotebookEdit are denied a path whose cleaned form has a
// .git element or lies under the state root (XDG_STATE_HOME's or HOME's
// default), relative paths resolved against the hook's cwd; other paths,
// and every path outside a sitting, are let through.
func TestAR20_6_FileToolDoor(t *testing.T) {
	tmp := t.TempDir()
	tree, xdg, home := filepath.Join(tmp, "tree"), filepath.Join(tmp, "xdg"), filepath.Join(tmp, "home")
	state := filepath.Join(xdg, "agent-relay")
	env := func(k string) string {
		return map[string]string{"RELAY_ROW": "ar20-fixes-adv11", "XDG_STATE_HOME": xdg, "HOME": home}[k]
	}
	for _, r := range []struct{ tool, path, want string }{
		{"Write", filepath.Join(tree, ".git", "hooks", "pre-commit"), "git-dir"},
		{"Edit", ".git/config", "git-dir"},
		{"MultiEdit", "sub/../.git/refs/heads/main", "git-dir"},
		{"Write", filepath.Join(tree, ".git"), "git-dir"},
		{"Edit", filepath.Join(tmp, "root", ".git", "worktrees", "x", "HEAD"), "git-dir"},
		{"Write", filepath.Join(tree, ".GIT", "config"), "git-dir"},
		{"NotebookEdit", filepath.Join(state, "ledger", "x.ipynb"), "state-root"},
		{"Write", filepath.Join(state, "ledger", "2026-09-24.jsonl"), "state-root"},
		{"Edit", "../xdg/agent-relay/queue.yaml", "state-root"},
		{"Write", state, "state-root"},
		{"Write", filepath.Join(tree, "a.go"), ""},
		{"Edit", "internal/x.go", ""},
		{"Write", filepath.Join(tree, ".github", "workflows", "ci.yml"), ""},
		{"Write", filepath.Join(tree, ".gitignore"), ""},
		{"Write", filepath.Join(xdg, "agent-relay-other", "x"), ""},
		{"Read", filepath.Join(tree, ".git", "config"), ""},
	} {
		if got := fileSays(t, env, r.tool, tree, r.path); got != r.want {
			t.Errorf("in a sitting, %s %q = %q, want %q", r.tool, r.path, got, r.want)
		}
	}
	// With XDG_STATE_HOME unset, the state root is HOME's default.
	def := func(k string) string { return map[string]string{"RELAY_ROW": "r", "HOME": home}[k] }
	if got := fileSays(t, def, "Write", tree, filepath.Join(home, ".local", "state", "agent-relay", "ledger", "x")); got != "state-root" {
		t.Errorf("default state root: %q, want state-root", got)
	}
	// Outside a sitting the door is open.
	out := func(k string) string { return map[string]string{"XDG_STATE_HOME": xdg, "HOME": home}[k] }
	for _, p := range []string{filepath.Join(tree, ".git", "config"), filepath.Join(state, "ledger", "x")} {
		if got := fileSays(t, out, "Write", tree, p); got != "" {
			t.Errorf("outside a sitting, Write %q = %q", p, got)
		}
	}
}
