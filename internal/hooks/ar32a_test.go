package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ar32aHook runs PreToolUse for tool with input from cwd and reports
// whether it was denied and why.
func ar32aHook(t *testing.T, tool string, input map[string]any, cwd string, env func(string) string) (bool, string) {
	t.Helper()
	in, err := json.Marshal(map[string]any{"tool_name": tool, "cwd": cwd, "tool_input": input})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Run("PreToolUse", bytes.NewReader(in), &out, env, OS{}); code != 0 {
		t.Fatalf("PreToolUse exit %d, want 0", code)
	}
	if out.Len() == 0 {
		return false, ""
	}
	var ans struct {
		HookSpecificOutput denyOutput `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &ans); err != nil {
		t.Fatalf("PreToolUse answer %q: %v", out.String(), err)
	}
	return ans.HookSpecificOutput.PermissionDecision == "deny", ans.HookSpecificOutput.PermissionDecisionReason
}

// TestAR32A_2_RootWriteDenied is ar32a act 2 (orchestrator r1 §3): in a
// row whose project dir is a relay worktree, a git command whose working
// directory resolves inside the root checkout and outside the worktree is
// denied root-write: by the hook's cwd, or a cd or pushd target earlier
// in the same command (a target the guard cannot read denies too). Git in
// the worktree, git elsewhere, git before the cd and a cd with no git
// pass, and outside a row nothing is read.
func TestAR32A_2_RootWriteDenied(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	tree := filepath.Join(root, ".claude", "worktrees", "orient.2")
	for _, d := range []string{tree, filepath.Join(root, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	kv := map[string]string{"RELAY_ROW": "orient", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100",
		"CLAUDE_PROJECT_DIR": tree, "HOME": other}
	row := ar27bEnv(kv)
	for _, tc := range []struct {
		cwd, cmd string
		deny     bool
	}{
		{tree, "git status", false},
		{tree, "git commit -m act", false},
		{root, "git status", true},
		{filepath.Join(root, "sub"), "git log --oneline", true},
		{tree, "cd " + root + " && git status", true},
		{tree, "cd ../../.. && git merge orient.1", true},
		{tree, "pushd " + root + " >/dev/null; git status", true},
		{tree, "pushd ../../../sub && git diff", true},
		{tree, "cd $ROOT && git status", true},
		{tree, "cd - && git status", true},
		{tree, "cd " + other + " && git status", false},
		{tree, "cd && git status", false},
		{tree, "cd " + root + " && ls", false},
		{tree, "git status; cd " + root, false},
		{tree, "cd " + root + "/.claude/worktrees/orient.2 && git status", false},
	} {
		deny, why := ar32aHook(t, "Bash", map[string]any{"command": tc.cmd}, tc.cwd, row)
		if deny != tc.deny || deny && !strings.HasPrefix(why, "relay: root-write: ") {
			t.Errorf("%q from %s = %v, %q; want denied %v (root-write)", tc.cmd, tc.cwd, deny, why, tc.deny)
		}
	}
	delete(kv, "RELAY_ROW")
	if deny, why := ar32aHook(t, "Bash", map[string]any{"command": "git status"}, root, ar27bEnv(kv)); deny {
		t.Errorf("outside a row git in the root = %q, want allowed", why)
	}
}

// TestAR32A_2_BackgroundAgentDenied is ar32a act 2 (§3): in a row an Agent
// call with run_in_background set is denied, as a background Bash command
// is; in the foreground it passes, and outside a row nothing is read. The
// PreToolUse matcher of hooks.json names Agent, so the hook sees it.
func TestAR32A_2_BackgroundAgentDenied(t *testing.T) {
	row := ar27bEnv(map[string]string{"RELAY_ROW": "orient", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100"})
	helper := ar27bEnv(map[string]string{"CLAUDE_SESSION_HELPER": "1", "CLAUDE_SESSION_DEPTH": "2", "CLAUDE_SESSION_MAX_TURNS": "100"})
	owner := ar27bEnv(map[string]string{})
	for _, tc := range []struct {
		env  func(string) string
		bg   any
		deny bool
	}{
		{row, true, true}, {row, "true", true}, {row, 1, true}, {helper, true, true},
		{row, false, false}, {row, nil, false}, {owner, true, false},
	} {
		in := map[string]any{"subagent_type": "general-purpose", "description": "d", "prompt": "p", "run_in_background": tc.bg}
		deny, why := ar32aHook(t, "Agent", in, "", tc.env)
		if deny != tc.deny || deny && !strings.HasPrefix(why, "relay: background-agent: ") {
			t.Errorf("Agent with run_in_background %#v = %v, %q; want denied %v (background-agent)", tc.bg, deny, why, tc.deny)
		}
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "plugin", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
		}
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	pre := cfg.Hooks["PreToolUse"]
	if len(pre) != 1 {
		t.Fatalf("hooks.json PreToolUse %+v", pre)
	}
	m := regexp.MustCompile("^(" + pre[0].Matcher + ")$")
	for _, tool := range []string{"Agent", "Bash", "Write", "Edit", "MultiEdit", "NotebookEdit"} {
		if !m.MatchString(tool) {
			t.Errorf("the PreToolUse matcher %q misses %s", pre[0].Matcher, tool)
		}
	}
}
