package hooks

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const ar23Block = `{"decision":"block","reason":"relay: dirty-tree: 1 paths; commit the act or restore them, then stop"}`

// ar23Env points HOME, XDG_CONFIG_HOME and XDG_STATE_HOME at temp dirs and
// makes a git repo holding one uncommitted path, the owner's work. env reads
// those three and extra only, never the process's own: a sitting runs this
// suite with CLAUDE_SESSION_DEPTH set.
func ar23Env(t *testing.T, extra map[string]string) (repo string, env func(string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	repo = t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "owner.txt"), []byte("the owner's work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"HOME": home, "XDG_CONFIG_HOME": config, "XDG_STATE_HOME": state}
	for k, v := range extra {
		vars[k] = v
	}
	return repo, func(k string) string { return vars[k] }
}

// ar23Hook runs one event through the real effects and returns its answer.
func ar23Hook(t *testing.T, event, repo string, env func(string) string) string {
	t.Helper()
	in, err := json.Marshal(map[string]any{"session_id": "s23", "cwd": repo, "source": "startup", "stop_hook_active": false})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Run(event, bytes.NewReader(in), &out, env, OS{}); code != 0 {
		t.Fatalf("%s exit %d, want 0", event, code)
	}
	return strings.TrimSpace(out.String())
}

// ar23Written lists every file under the temp HOME and XDG_STATE_HOME.
func ar23Written(t *testing.T, env func(string) string) []string {
	t.Helper()
	var files []string
	for _, root := range []string{env("HOME"), env("XDG_STATE_HOME")} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files = append(files, p)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// TestAR23_StopOutsideSitting: the owner's own session on a dirty tree gets
// no block, no message and no state write, even over a start.json an older
// SessionStart left behind.
func TestAR23_StopOutsideSitting(t *testing.T) {
	repo, env := ar23Env(t, map[string]string{"RELAY_ROW": ""})
	if got := ar23Hook(t, "Stop", repo, env); got != "{}" {
		t.Fatalf("Stop outside a sitting = %s, want {}", got)
	}
	if w := ar23Written(t, env); len(w) != 0 {
		t.Fatalf("Stop outside a sitting wrote %v", w)
	}
	path := statePath(env, "s23")
	old := startJSON("abc", 0)
	if err := (OS{}).WriteFile(path, old); err != nil {
		t.Fatal(err)
	}
	if got := ar23Hook(t, "Stop", repo, env); got != "{}" {
		t.Fatalf("Stop outside a sitting over an old start.json = %s, want {}", got)
	}
	if b, err := os.ReadFile(path); err != nil || !bytes.Equal(b, old) {
		t.Fatalf("start.json = %q, %v; want it untouched", b, err)
	}
}

// TestAR23_StopInRow: a relay row (RELAY_ROW set) is blocked as before.
func TestAR23_StopInRow(t *testing.T) {
	repo, env := ar23Env(t, map[string]string{"RELAY_ROW": "ar23-stop-scope"})
	if got := ar23Hook(t, "Stop", repo, env); got != ar23Block {
		t.Fatalf("Stop in a row = %s, want %s", got, ar23Block)
	}
}

// TestAR23_StopInClaudeSession: CLAUDE_SESSION_DEPTH an integer >= 1 is a
// sitting; 0, empty, negative and non-numeric are not.
func TestAR23_StopInClaudeSession(t *testing.T) {
	for _, depth := range []string{"1", "2", "+1"} {
		repo, env := ar23Env(t, map[string]string{"CLAUDE_SESSION_DEPTH": depth})
		if got := ar23Hook(t, "Stop", repo, env); got != ar23Block {
			t.Errorf("Stop at depth %q = %s, want %s", depth, got, ar23Block)
		}
	}
	for _, depth := range []string{"0", "", "-1", "one", "1.0", " 1", "1 "} {
		repo, env := ar23Env(t, map[string]string{"CLAUDE_SESSION_DEPTH": depth})
		if got := ar23Hook(t, "Stop", repo, env); got != "{}" {
			t.Errorf("Stop at depth %q = %s, want {}", depth, got)
		}
		if w := ar23Written(t, env); len(w) != 0 {
			t.Errorf("Stop at depth %q wrote %v", depth, w)
		}
	}
}

// TestAR23_StartOutsideSitting: SessionStart outside a sitting writes no
// state file; RELAY_PACKET is still injected, without the Stop guard's
// clause. In a sitting it writes start.json, so the test is not vacuous.
func TestAR23_StartOutsideSitting(t *testing.T) {
	repo, env := ar23Env(t, map[string]string{"CLAUDE_SESSION_DEPTH": "0"})
	if got := ar23Hook(t, "SessionStart", repo, env); got != "{}" {
		t.Fatalf("SessionStart outside a sitting = %s, want {}", got)
	}
	if w := ar23Written(t, env); len(w) != 0 {
		t.Fatalf("SessionStart outside a sitting wrote %v", w)
	}

	repo, env = ar23Env(t, map[string]string{"RELAY_PACKET": "owner.txt"})
	got := ar23Hook(t, "SessionStart", repo, env)
	want := "relay: this sitting builds owner.txt; commit per act; never push, amend or rebase; never a device\nthe owner's work\n"
	var out struct {
		HookSpecificOutput contextOutput `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil || out.HookSpecificOutput.AdditionalContext != want {
		t.Fatalf("SessionStart with a packet outside a sitting = %s, want the packet without the Stop clause", got)
	}
	if w := ar23Written(t, env); len(w) != 0 {
		t.Fatalf("SessionStart with a packet outside a sitting wrote %v", w)
	}

	repo, env = ar23Env(t, map[string]string{"CLAUDE_SESSION_DEPTH": "1"})
	if got := ar23Hook(t, "SessionStart", repo, env); got != "{}" {
		t.Fatalf("SessionStart in a sitting = %s, want {}", got)
	}
	if w := ar23Written(t, env); len(w) != 1 || w[0] != statePath(env, "s23") {
		t.Fatalf("SessionStart in a sitting wrote %v, want only %s", w, statePath(env, "s23"))
	}
}
