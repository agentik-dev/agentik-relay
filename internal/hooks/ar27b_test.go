package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ar27bEnv is a hook's env from kv alone, never the process's.
func ar27bEnv(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

// TestAR27B_1_MarkerBuiltins (review-adv16 test 1; ADV16-1): at depth 1
// every builtin that assigns a marker by name, and zsh -c running one, is
// denied (markers); a $NAME read and a search naming one are allowed.
func TestAR27B_1_MarkerBuiltins(t *testing.T) {
	row := ar27bEnv(map[string]string{"RELAY_ROW": "ar27b", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100"})
	for _, cmd := range []string{
		"read RELAY_ROW </dev/null",
		"read RELAY_ROW </dev/null; CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 1",
		"for RELAY_ROW in ''; do true; done",
		"print -v RELAY_ROW ''",
		"printf -v CLAUDE_SESSION_DEPTH 0",
		"(( CLAUDE_SESSION_DEPTH = 0 ))",
		"getopts ab RELAY_ROW",
		"vared CLAUDE_SESSION_MAX_TURNS",
		"mapfile RELAY_ROW </dev/null",
		": ${RELAY_ROW::=}",
		": ${RELAY_ROW:=x}",
		"zsh -c 'read RELAY_ROW'",
		"declare -n ref=RELAY_ROW",
		"CLAUDE_SESSION_HELPER= true",
		"env -u CLAUDE_SESSION_HELPER true",
		"CLAUDE_SESSION_DEPTH=3 true",
		"grep x f > RELAY_ROW",
	} {
		deny, why := ar25Hook(t, cmd, row)
		if !deny || !strings.HasPrefix(why, "relay: markers: ") {
			t.Errorf("at depth 1 %q = %v, %q; want denied by markers", cmd, deny, why)
		}
	}
	for _, cmd := range []string{
		"echo $RELAY_ROW", "echo ${CLAUDE_SESSION_DEPTH} \"$CLAUDE_SESSION_HELPER\"",
		"grep -n RELAY_ROW f", "rg -n 'CLAUDE_SESSION_(DEPTH|HELPER)' internal",
		"echo MY_RELAY_ROW RELAY_ROWS", "git status",
	} {
		if deny, why := ar25Hook(t, cmd, row); deny {
			t.Errorf("at depth 1 %q = %q; want allowed", cmd, why)
		}
	}
	// outside a row no guard reads a marker's name
	if deny, why := ar25Hook(t, "read RELAY_ROW </dev/null", ar27bEnv(nil)); deny {
		t.Errorf("outside a row `read RELAY_ROW` = %q; want allowed", why)
	}
}

// TestAR27B_1_HelperIsRow (ADV16-1): CLAUDE_SESSION_HELPER alone, RELAY_ROW
// and the depth blank, is a row: the push, nested-run, claude-session and
// helper guards and the file door stay on, and the unreadable depth reads
// MaxHelperDepth, so no helper line passes. Its Stop is still no sitting's.
func TestAR27B_1_HelperIsRow(t *testing.T) {
	for _, env := range []map[string]string{
		{"CLAUDE_SESSION_HELPER": "1"},
		{"CLAUDE_SESSION_HELPER": "1", "CLAUDE_SESSION_DEPTH": "x", "CLAUDE_SESSION_MAX_TURNS": "100"},
		{"CLAUDE_SESSION_HELPER": "1", "CLAUDE_SESSION_DEPTH": "0", "CLAUDE_SESSION_MAX_TURNS": "100"},
	} {
		h := ar27bEnv(env)
		for _, tc := range []struct{ cmd, guard string }{
			{"claude -p hi &", "helper"},
			{"git push origin HEAD", "push"},
			{"relay run --rows x", "nested-run"},
			{"claude-session start", "nested-session"},
			{"CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 1 --max-budget-usd 1", "helper"},
			{"CLAUDE_SESSION_DEPTH=3 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 1 --max-budget-usd 1", "helper"},
			{"tmux new -d claude -p hi", "detached-server"},
		} {
			deny, why := ar25Hook(t, tc.cmd, h)
			if !deny || !strings.HasPrefix(why, "relay: "+tc.guard+": ") {
				t.Errorf("%v: %q = %v, %q; want denied by %s", env, tc.cmd, deny, why, tc.guard)
			}
		}
		if g, ok := fileDoor("/w/.git/config", "/w", h); !ok || g.Name != GitDir.Name {
			t.Errorf("%v: the file door on .git = %v, %v; want git-dir", env, g.Name, ok)
		}
	}
	if inSitting(ar27bEnv(map[string]string{"CLAUDE_SESSION_HELPER": "1", "RELAY_ROW": "r"})) {
		t.Error("a helper reads as a sitting; Stop and start.json skip it")
	}
}

// ar27bLine is the helper line at depth 1 under a 100-turn hook.
const ar27bLine = "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 3 --max-budget-usd 0.5"

// TestAR27B_2_HelperAllowlist (review-adv16 test 2; ADV16-2): the helper
// line is an allowlist. Before claude only the two prefix words, the
// command word literally claude, and after it only -p, --max-turns N
// (1 <= N <= the hook's CLAUDE_SESSION_MAX_TURNS), --max-budget-usd X
// (required, above 0, finite), --model, --output-format, --verbose and one
// prompt word; each other word, and a hook whose turns are unreadable,
// denies it (helper).
func TestAR27B_2_HelperAllowlist(t *testing.T) {
	const pre = "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 "
	row := ar27bEnv(map[string]string{"RELAY_ROW": "ar27b", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100"})
	for _, cmd := range []string{
		ar27bLine,
		pre + "claude --print --max-turns=100 --max-budget-usd=2 --model=claude-opus-5-5 --output-format=json --verbose 'a prompt'",
		pre + "claude -p --model claude-opus-5-5 --output-format stream-json --verbose --max-turns 1 --max-budget-usd 1e-2",
		"CLAUDE_SESSION_HELPER=1 CLAUDE_SESSION_DEPTH=2 claude -p --max-turns 1 --max-budget-usd 1 < prompt.txt > out.txt 2>&1 | tail -1",
		"cd sub && " + ar27bLine,
	} {
		if deny, why := ar25Hook(t, cmd, row); deny {
			t.Errorf("%q = %q; want allowed", cmd, why)
		}
	}
	for _, tc := range []struct {
		name, cmd string
		env       func(string) string
	}{
		{"--settings", ar27bLine + ` --settings '{"disableAllHooks":true}'`, row},
		{"--settings=", ar27bLine + ` --settings={}`, row},
		{"--setting-sources", ar27bLine + " --setting-sources project", row},
		{"CLAUDE_CONFIG_DIR", "CLAUDE_CONFIG_DIR=/tmp/x " + ar27bLine, row},
		{"--dangerously-skip-permissions", ar27bLine + " --dangerously-skip-permissions", row},
		{"--agents", ar27bLine + ` --agents '{}'`, row},
		{"--resume", ar27bLine + " --resume abc", row},
		{"exec -a zz", pre + "exec -a zz claude -p hi --max-turns 3 --max-budget-usd 0.5", row},
		{"exec", pre + "exec claude -p hi --max-turns 3 --max-budget-usd 0.5", row},
		{"command", pre + "command claude -p hi --max-turns 3 --max-budget-usd 0.5", row},
		{"a path", pre + "/usr/local/bin/claude -p hi --max-turns 3 --max-budget-usd 0.5", row},
		{"a prefix twice", pre + "CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 3 --max-budget-usd 0.5", row},
		{"--max-turns 99999999", pre + "claude -p hi --max-turns 99999999 --max-budget-usd 0.5", row},
		{"--max-turns 101", pre + "claude -p hi --max-turns=101 --max-budget-usd 0.5", row},
		{"--max-turns 0", pre + "claude -p hi --max-turns 0 --max-budget-usd 0.5", row},
		{"no --max-turns", pre + "claude -p hi --max-budget-usd 0.5", row},
		{"no --max-budget-usd", pre + "claude -p hi --max-turns 3", row},
		{"--max-budget-usd 0", pre + "claude -p hi --max-turns 3 --max-budget-usd 0", row},
		{"--max-budget-usd -1", pre + "claude -p hi --max-turns 3 --max-budget-usd=-1", row},
		{"--max-budget-usd Inf", pre + "claude -p hi --max-turns 3 --max-budget-usd Inf", row},
		{"--max-budget-usd NaN", pre + "claude -p hi --max-turns 3 --max-budget-usd NaN", row},
		{"--max-budget-usd last", pre + "claude -p hi --max-turns 3 --max-budget-usd", row},
		{"--output-format xml", ar27bLine + " --output-format xml", row},
		{"--model -x", pre + "claude -p hi --max-turns 3 --max-budget-usd 0.5 --model --verbose", row},
		{"two prompts", ar27bLine + " there", row},
		{"--", ar27bLine + " -- -x", row},
		{"no -p", pre + "claude hi --max-turns 3 --max-budget-usd 0.5", row},
		{"turns unreadable", ar27bLine, ar27bEnv(map[string]string{"RELAY_ROW": "ar27b", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "x"})},
		{"turns absent", ar27bLine, ar25Env("ar27b", "1")},
	} {
		deny, why := ar25Hook(t, tc.cmd, tc.env)
		if !deny || !strings.HasPrefix(why, "relay: helper: ") {
			t.Errorf("%s: %q = %v, %q; want denied by helper", tc.name, tc.cmd, deny, why)
		}
	}
}

// TestAR27B_3_ClaudeConfigDoor (ADV16-2, ADV14-7): in a row a write or read
// under a .claude element is denied (claude-config), by the file door and
// by Bash: below the project dir by its part below it, so a worktree under
// <root>/.claude/worktrees/ passes; outside it, whole; a word with $ or a
// tilde by its own elements. A search's operands are exempt.
func TestAR27B_3_ClaudeConfigDoor(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, ".claude", "worktrees", "w")
	home := filepath.Join(root, "home")
	for _, d := range []string{proj, filepath.Join(home, ".claude")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	kv := map[string]string{"RELAY_ROW": "ar27b", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100",
		"HOME": home, "XDG_STATE_HOME": filepath.Join(root, "state"), "CLAUDE_PROJECT_DIR": proj}
	row := ar27bEnv(kv)
	file := func(tool, path, cwd string) (bool, string) {
		in, _ := json.Marshal(map[string]any{"tool_name": tool, "cwd": cwd, "tool_input": map[string]string{"file_path": path}})
		var out bytes.Buffer
		Run("PreToolUse", bytes.NewReader(in), &out, row, OS{})
		var ans struct {
			HookSpecificOutput denyOutput `json:"hookSpecificOutput"`
		}
		_ = json.Unmarshal(out.Bytes(), &ans)
		return ans.HookSpecificOutput.PermissionDecision == "deny", ans.HookSpecificOutput.PermissionDecisionReason
	}
	bash := func(cmd, cwd string) (bool, string) {
		in, _ := json.Marshal(map[string]any{"tool_name": "Bash", "cwd": cwd, "tool_input": map[string]string{"command": cmd}})
		var out bytes.Buffer
		Run("PreToolUse", bytes.NewReader(in), &out, row, OS{})
		var ans struct {
			HookSpecificOutput denyOutput `json:"hookSpecificOutput"`
		}
		_ = json.Unmarshal(out.Bytes(), &ans)
		return ans.HookSpecificOutput.PermissionDecision == "deny", ans.HookSpecificOutput.PermissionDecisionReason
	}
	const guard = "relay: claude-config: "
	for _, tc := range []struct{ tool, path, cwd string }{
		{"Write", filepath.Join(proj, ".claude", "settings.local.json"), proj},
		{"Edit", ".claude/settings.json", proj},
		{"Write", "~/.claude/settings.json", proj},
		{"MultiEdit", filepath.Join(root, ".claude", "worktrees", "other", "x.go"), proj},
		{"Write", filepath.Join(proj, "sub", ".CLAUDE", "settings.json"), proj},
	} {
		if deny, why := file(tc.tool, tc.path, tc.cwd); !deny || !strings.HasPrefix(why, guard) {
			t.Errorf("%s %s = %v, %q; want denied by claude-config", tc.tool, tc.path, deny, why)
		}
	}
	for _, tc := range []struct{ tool, path string }{
		{"Write", filepath.Join(proj, "x.go")}, {"Edit", "internal/x.go"}, {"Write", filepath.Join(root, "y.go")},
	} {
		if deny, why := file(tc.tool, tc.path, proj); deny {
			t.Errorf("%s %s = %q; want allowed", tc.tool, tc.path, why)
		}
	}
	for _, cmd := range []string{
		"printf x > .claude/settings.local.json",
		"printf x >.claude/settings.local.json",
		"cat ~/.claude/settings.json",
		"cat $HOME/.claude/settings.json",
		"cp x " + filepath.Join(proj, ".claude") + "/",
		"mkdir .claude",
		"cd .c* && printf x > settings.local.json",
		"cp x --target-directory=.claude/",
		"echo x | tee ../other/.claude/settings.json",
		"cat ../../settings.json",
		"grep -n x f > .claude/settings.local.json",
		"ls .{claude,x}/",
	} {
		if deny, why := bash(cmd, proj); !deny || !strings.HasPrefix(why, guard) {
			t.Errorf("Bash %q = %v, %q; want denied by claude-config", cmd, deny, why)
		}
	}
	for _, cmd := range []string{
		"go test ./internal/hooks", "cat " + filepath.Join(proj, "x.go"), "ls internal/*/", "grep -rn .claude design/",
		"rg -n '.claude/settings' internal", "cd " + proj + " && git status", "wc -l internal/{hooks,gate}/*.go",
	} {
		if deny, why := bash(cmd, proj); deny {
			t.Errorf("Bash %q = %q; want allowed", cmd, why)
		}
	}
	// outside a row neither door reads .claude
	delete(kv, "RELAY_ROW")
	if deny, why := bash("printf x > .claude/settings.local.json", proj); deny {
		t.Errorf("outside a row = %q; want allowed", why)
	}
	if deny, why := file("Write", filepath.Join(proj, ".claude", "settings.local.json"), proj); deny {
		t.Errorf("outside a row the file door = %q; want allowed", why)
	}
}
