package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ar27cFixture is a claude install on a fixture PATH (review-adv16 test
// 3): versions/2.1.282, a script, with bin/claude linking it, and cwd/c a
// symlink to it; HOME and the XDG dirs are temp dirs. It returns the dir
// and a row's hook env at depth 1 with PATH bin:versions and PWD cwd.
func ar27cFixture(t *testing.T) (string, func(string) string) {
	t.Helper()
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, t.TempDir())
	}
	dir := t.TempDir()
	for _, d := range []string{"versions", "bin", "cwd"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(dir, "versions", "2.1.282")
	if err := os.WriteFile(real, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "bin", "claude")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "cwd", "c")); err != nil {
		t.Fatal(err)
	}
	return dir, ar27bEnv(map[string]string{
		"RELAY_ROW": "ar27c", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100",
		"PATH": filepath.Join(dir, "bin") + string(os.PathListSeparator) + filepath.Join(dir, "versions"),
		"PWD":  filepath.Join(dir, "cwd"),
	})
}

// TestAR27C_1_GuardRealFile (review-adv16 test 3, the guard; ADV16-3): in
// a row a command word whose real file is the hook's claude's is a claude:
// with the helper prefix the version file's path, the bare version name on
// PATH and ./c (a symlink to it) are denied (the prefix rides only a
// claude); without it each, and a wrapper's operand naming it, is denied
// by helper; exec -a is denied in a row; the literal helper line and ./c
// as an argument are allowed.
func TestAR27C_1_GuardRealFile(t *testing.T) {
	dir, row := ar27cFixture(t)
	const pre = "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 "
	const args = " -p hi --max-turns 3 --max-budget-usd 0.5"
	for _, cmd := range []string{
		pre + filepath.Join(dir, "versions", "2.1.282") + args,
		pre + "./c" + args,
		pre + "2.1.282" + args,
		pre + "exec -a zz claude" + args,
		"exec -ca zz sleep 1",
	} {
		if deny, _ := ar25Hook(t, cmd, row); !deny {
			t.Errorf("%q allowed; want denied", cmd)
		}
	}
	for _, cmd := range []string{
		filepath.Join(dir, "versions", "2.1.282") + " -p hi",
		"../versions/2.1.282 -p hi",
		"./c -p hi",
		"2.1.282 -p hi",
		"nohup ./c -p hi",
		"setsid 2.1.282 -p hi",
		"exec -a zz sleep 1",
	} {
		deny, why := ar25Hook(t, cmd, row)
		if !deny || !strings.HasPrefix(why, "relay: helper: ") {
			t.Errorf("%q = %v, %q; want denied by helper", cmd, deny, why)
		}
	}
	for _, cmd := range []string{ar27bLine, "ls -l ./c", "sleep 1", "exec sleep 1"} {
		if deny, why := ar25Hook(t, cmd, row); deny {
			t.Errorf("%q = %q; want allowed", cmd, why)
		}
	}
	// outside a row no guard keys a claude
	if deny, why := ar25Hook(t, "./c -p hi", ar27bEnv(map[string]string{"PWD": filepath.Join(dir, "cwd")})); deny {
		t.Errorf("outside a row ./c = %q; want allowed", why)
	}
}

// ar27cHook is ar25Hook with the Bash tool's run_in_background set to bg.
func ar27cHook(t *testing.T, cmd string, bg any, env func(string) string) (bool, string) {
	t.Helper()
	in, err := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd, "run_in_background": bg}})
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

// TestAR27C_3_BackgroundFlag (ADV16-4): in a row the Bash tool's own
// run_in_background, true (or "true") with a segment whose command word is
// a claude, is denied by helper; the helper line with it false, and ls or
// a claude query with it true, are allowed; outside a row it is not read.
func TestAR27C_3_BackgroundFlag(t *testing.T) {
	_, row := ar27cFixture(t)
	for _, tc := range []struct {
		cmd  string
		bg   any
		deny bool
	}{
		{ar27bLine, true, true},
		{ar27bLine, "true", true},
		{"cd sub && " + ar27bLine, true, true},
		{ar27bLine, false, false},
		{ar27bLine, nil, false},
		{"ls -l", true, false},
		{"command -v claude", true, false},
	} {
		deny, why := ar27cHook(t, tc.cmd, tc.bg, row)
		if deny != tc.deny || deny && !strings.HasPrefix(why, "relay: helper: ") {
			t.Errorf("%q with run_in_background %v = %v, %q; want denied %v (helper)", tc.cmd, tc.bg, deny, why, tc.deny)
		}
	}
	if deny, why := ar27cHook(t, "claude -p hi", true, ar27bEnv(nil)); deny {
		t.Errorf("outside a row a backgrounded claude = %q; want allowed", why)
	}
}

// TestAR27C_3_ClaudeQueries (ADV16-8): in a row `claude --version` or -v
// alone (a redirection may join), `command -v` or -V naming a claude,
// `type claude` and `which claude` ask and start nothing, so they pass;
// a query word beside a run, command without -v, and a refused wrapper
// before one are still denied by helper.
func TestAR27C_3_ClaudeQueries(t *testing.T) {
	_, row := ar27cFixture(t)
	for _, cmd := range []string{
		"claude --version", "claude -v", "claude --version 2>/dev/null | head -1",
		"command -v claude", "command -V claude", "type claude", "which claude",
		"command -v claude >/dev/null && echo yes",
	} {
		if deny, why := ar25Hook(t, cmd, row); deny {
			t.Errorf("%q = %q; want allowed", cmd, why)
		}
	}
	for _, cmd := range []string{
		"claude --version -p hi", "claude -v --max-turns 1", "claude --version --version",
		"command -p claude -p hi", "command claude --version -p hi", "sudo claude --version",
		"xargs claude --version", "claude --version; claude -p hi", "./c --version -p hi",
	} {
		deny, why := ar25Hook(t, cmd, row)
		if !deny || !strings.HasPrefix(why, "relay: helper: ") {
			t.Errorf("%q = %v, %q; want denied by helper", cmd, deny, why)
		}
	}
}
