package hooks

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// ar25Hook answers cmd as the real PreToolUse hook does under env: deny and
// the reason, "" when allowed.
func ar25Hook(t *testing.T, cmd string, env func(string) string) (bool, string) {
	t.Helper()
	in, err := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": cmd}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Run("PreToolUse", bytes.NewReader(in), &out, env, OS{}); code != 0 {
		t.Fatalf("PreToolUse exit %d, want 0", code)
	}
	var ans struct {
		HookSpecificOutput denyOutput `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &ans); err != nil {
		t.Fatalf("PreToolUse answer %q: %v", out.String(), err)
	}
	why := ans.HookSpecificOutput.PermissionDecisionReason
	return ans.HookSpecificOutput.PermissionDecision == "deny", why
}

// ar25Env is a hook's env: the row and the depth only, never the process's.
func ar25Env(row, depth string) func(string) string {
	return func(k string) string {
		return map[string]string{"RELAY_ROW": row, "CLAUDE_SESSION_DEPTH": depth}[k]
	}
}

// TestAR25_0_MarkersKept (review-adv15 test 1; ADV15-1, -2, -5): in a row a
// segment that drops, blanks, reassigns or lowers the sitting's markers is
// denied (markers), as are a detached server (detached-server) and a new
// sitting (claude-session). Outside a row each is allowed: no guard that
// runs outside a row (amend, rebase, no-verify, alias, interpreter, device)
// reads any of these lines.
func TestAR25_0_MarkersKept(t *testing.T) {
	for _, tc := range []struct{ cmd, guard string }{
		{"env -u RELAY_ROW claude -p hi", "markers"},
		{"env -u RELAY_ROW -u CLAUDE_SESSION_DEPTH claude -p hi", "markers"},
		{"env -u HOME -u CLAUDE_SESSION_DEPTH true", "markers"},
		{"env -uRELAY_ROW true", "markers"},
		{"env --unset=CLAUDE_SESSION_MAX_TURNS true", "markers"},
		{"RELAY_ROW= CLAUDE_SESSION_DEPTH=0 claude -p hi", "markers"},
		{"CLAUDE_SESSION_DEPTH=0 true", "markers"},
		{"CLAUDE_SESSION_DEPTH= true", "markers"},
		{"CLAUDE_SESSION_MAX_TURNS=999 true", "markers"},
		{"unset CLAUDE_SESSION_MAX_TURNS", "markers"},
		{"export RELAY_ROW=x", "markers"},
		{"export -n CLAUDE_SESSION_DEPTH", "markers"},
		{"env -i claude -p hi", "markers"},
		{"env -i PATH=/bin true", "markers"},
		{"env - PATH=/bin true", "markers"},
		{"env -S '-u RELAY_ROW true'", "markers"},
		{"exec -c claude -p hi", "markers"},
		{"make RELAY_ROW= check", "markers"},
		{"CLAUDE_SESSION_DEPTH=3 true", "markers"}, // AR27B act 1: a marker's name is only read
		{"tmux new-window -d claude", "detached-server"},
		{"screen -dm x", "detached-server"},
		{"claude-session start", "claude-session"},
		{"command tmux ls", "detached-server"},
		{"git push origin HEAD &", "push"},
	} {
		deny, why := Verdict(tc.cmd, true)
		if !deny || !strings.HasPrefix(why, "relay: "+tc.guard+": ") {
			t.Errorf("Verdict(%q, true) = %v, %q; want denied by %s", tc.cmd, deny, why, tc.guard)
		}
		if deny, why := Verdict(tc.cmd, false); deny && tc.guard != "push" {
			t.Errorf("Verdict(%q, false) = %q; want allowed outside a row", tc.cmd, why)
		}
	}
	// the markers ride: what keeps them is allowed
	for _, cmd := range []string{
		"env -u HOME true", "export PATH=/bin", "unset FOO",
		"echo tmux screen claude-session", "git status", "make check 2>&1 | tail -1",
		"sleep 1 & wait", "make check > log 2>&1 &",
	} {
		if deny, why := Verdict(cmd, true); deny {
			t.Errorf("Verdict(%q, true) = %q; want allowed in a row", cmd, why)
		}
	}
}

// TestAR25_0_HelperLine (ADV15-1, -7): in a row at depth 1 the one helper
// line is allowed; each part missing or wrong, a row at depth 2, or a
// helper backgrounded is denied (helper).
func TestAR25_0_HelperLine(t *testing.T) {
	const line = "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 3 --max-budget-usd 1" // AR27B act 2
	row := ar27bEnv(map[string]string{"RELAY_ROW": "r25", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100"})
	for _, cmd := range []string{
		line,
		"CLAUDE_SESSION_HELPER=1 CLAUDE_SESSION_DEPTH=2 claude --print hi --max-turns=1 --max-budget-usd=1",
		line + " > out.txt 2>&1 | tail -1",
		"cd sub && " + line,
	} {
		if deny, why := ar25Hook(t, cmd, row); deny {
			t.Errorf("at depth 1 %q = %q; want allowed", cmd, why)
		}
	}
	for _, tc := range []struct {
		name, cmd string
		env       func(string) string
	}{
		{"no -p", "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude hi --max-turns 3", row},
		{"no --max-turns", "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi", row},
		{"--max-turns 0", "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 0", row},
		{"--max-turns=x", "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns=x", row},
		{"no helper marker", "CLAUDE_SESSION_DEPTH=2 claude -p hi --max-turns 3", row},
		{"depth 1", "CLAUDE_SESSION_DEPTH=1 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 3", row},
		{"depth 3", "CLAUDE_SESSION_DEPTH=3 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 3", row},
		{"a helper at depth 2", "CLAUDE_SESSION_DEPTH=3 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 3", ar25Env("r25", "2")},
		{"backgrounded", line + " &", row},
		{"backgrounded, then more", line + " & wait", row},
		{"a subshell backgrounded", "(" + line + ") &", row},
		{"nohup", "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 nohup claude -p hi --max-turns 3", row},
		{"setsid", "CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 setsid claude -p hi --max-turns 3", row},
		{"xargs", "echo x | CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 xargs claude -p hi --max-turns 3", row},
		{"parallel", "parallel -j 2 claude -p {} --max-turns 3 ::: a b", row},
		{"plain", "claude -p hi", row},
	} {
		deny, why := ar25Hook(t, tc.cmd, tc.env)
		if !deny || !strings.HasPrefix(why, "relay: helper: ") {
			t.Errorf("%s: %q = %v, %q; want denied by helper", tc.name, tc.cmd, deny, why)
		}
		if d2, w2 := VerdictEnv(tc.cmd, tc.env); d2 != deny || w2 != why {
			t.Errorf("%s: VerdictEnv = %v, %q; the hook says %v, %q", tc.name, d2, w2, deny, why)
		}
	}
	if deny, why := ar25Hook(t, "claude -p hi", ar25Env("", "")); deny {
		t.Errorf("outside a row `claude -p hi` = %q; want allowed", why)
	}
	// the background mark is the guards' alone: Segments never shows it
	for _, seg := range Segments(line + " & wait") {
		if seg[0] != "CLAUDE_SESSION_DEPTH=2" && seg[0] != "wait" {
			t.Errorf("Segments(%q) keeps the mark: %q", line+" & wait", seg)
		}
	}
}

// TestAR25_0_StopSkipsHelper (ADV15-1; s15-resync): a helper keeps
// RELAY_ROW, so its PreToolUse guards stay on, but CLAUDE_SESSION_HELPER
// makes it no sitting: Stop on a dirty tree answers {} and SessionStart
// writes no start.json. The same env without the marker is blocked.
func TestAR25_0_StopSkipsHelper(t *testing.T) {
	helper := map[string]string{"RELAY_ROW": "ar25-helper", "CLAUDE_SESSION_DEPTH": "2", "CLAUDE_SESSION_HELPER": "1"}
	repo, env := ar23Env(t, helper)
	if got := ar23Hook(t, "Stop", repo, env); got != "{}" {
		t.Fatalf("Stop in a helper = %s, want {}", got)
	}
	if got := ar23Hook(t, "SessionStart", repo, env); got != "{}" {
		t.Fatalf("SessionStart in a helper = %s, want {}", got)
	}
	if w := ar23Written(t, env); len(w) != 0 {
		t.Fatalf("a helper wrote %v", w)
	}
	if deny, why := ar25Hook(t, "git push", env); !deny || !strings.HasPrefix(why, "relay: push: ") {
		t.Fatalf("git push in a helper = %v, %q; want denied by push", deny, why)
	}
	delete(helper, "CLAUDE_SESSION_HELPER")
	repo, env = ar23Env(t, helper)
	if got := ar23Hook(t, "Stop", repo, env); got != ar23Block {
		t.Fatalf("Stop in the same row without the marker = %s, want %s", got, ar23Block)
	}
}
