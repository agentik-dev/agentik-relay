package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/hooks"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// pidHolderEnv names the pid dir a re-executed test binary holds the pid
// file in (TestMain): it takes the lock, then execs sleep with the lock
// kept, as resume execs claude.
const pidHolderEnv = "AR21_2_PID_HOLDER"

// pidHolder is the holder's half, run from TestMain before anything else.
func pidHolder(dir string) {
	// Before RunPinned: the pinned ps alone, as Pin would resolve it.
	for _, d := range effects.PinDirs {
		if _, err := os.Stat(filepath.Join(d, "ps")); err == nil {
			effects.Use(effects.Pinned{Ps: filepath.Join(d, "ps")})
			break
		}
	}
	if _, err := night.PidFile(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	err := night.ExecHolding("/bin/sleep", []string{"sleep", "120"}, os.Environ())
	fmt.Fprintln(os.Stderr, err)
	os.Exit(3)
}

// TestAR21_2_SecondNightRefused (ADV12-3): a second night from another
// shell (a live holder that is no ancestor, here one that has exec'd, as
// resume does, keeping the lock) refuses run, a measuring plan and resume,
// exit 2, naming its pid; the refused resume prints the line it would
// exec. The Write door of this binary denies the account's pid dir.
func TestAR21_2_SecondNightRefused(t *testing.T) {
	home := t.TempDir()
	resumeRepo(t, home) // resume's door is git's (AR22 act 3)
	getenv = fakeEnv(home)
	oldExec := execResume
	t.Cleanup(func() { getenv, execResume = os.Getenv, oldExec })
	measuring(t)
	dir, err := night.PidDir()
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	holder := exec.Command(self, "-test.run=^$")
	holder.Env = append(os.Environ(), pidHolderEnv+"="+dir)
	var herr bytes.Buffer
	holder.Stderr = &herr
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { holder.Process.Kill(); holder.Wait() })
	pid := holder.Process.Pid
	for end := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		b, _ := os.ReadFile(filepath.Join(dir, night.PidFileName))
		comm, _ := exec.Command("ps", "-o", "comm=", "-p", fmt.Sprint(pid)).Output()
		if strings.HasPrefix(string(b), fmt.Sprintf("%d ", pid)) && strings.Contains(string(comm), "sleep") {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("the holder never held across its exec: %q %q %s", b, comm, herr.String())
		}
	}

	state := effects.StateDir(getenv)
	const id = "2026-09-24-1"
	tree := filepath.Join(home, "repo", ".claude", "worktrees", "r1")
	l := effects.NewLedger(state, effects.FakeClock{T: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC)}, id)
	l.Append("stage", map[string]any{"row": "r1", "worktree": tree, "branch": "r1", "base": "main"})
	l.Append("sitting", map[string]any{"row": "r1", "state": "red", "session_id": "s-1", "turns": 3, "cost_usd": 1.5})
	var ran []string
	execResume = func(dir string, argv []string) error { ran = argv; return nil }
	nested := fmt.Sprintf("nested: relay %d is live", pid)
	for _, args := range [][]string{{"run", "--no-tmux"}, {"plan"}, {"resume"}, {"resume", "r1"}} {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		if code != exitUsage || !strings.Contains(errb.String(), nested) {
			t.Errorf("%v under a second night: exit %d, stderr %q", args, code, errb.String())
		}
		// AR22 act 3: the line is git's (the picker, no transcript here),
		// the ledger's s-1 only a claim printed after it.
		if len(args) == 2 && args[0] == "resume" && (!strings.Contains(out.String(), "&& claude --resume\n") ||
			strings.Contains(out.String(), "claude --resume s-1")) {
			t.Errorf("a refused resume printed %q, want its line", out.String())
		}
	}
	if ran != nil {
		t.Errorf("a refused resume exec'd %v", ran)
	}

	// The relay's hook denies the pid dir though XDG_STATE_HOME is elsewhere.
	in, _ := json.Marshal(map[string]any{"tool_name": "Write", "cwd": home, "tool_input": map[string]string{"file_path": filepath.Join(dir, night.PidFileName)}})
	env := func(k string) string {
		return map[string]string{"RELAY_ROW": "r1", "HOME": home, "XDG_STATE_HOME": filepath.Join(home, "xdg")}[k]
	}
	var b bytes.Buffer
	hooks.Run("PreToolUse", bytes.NewReader(in), &b, env, nil)
	if !strings.Contains(b.String(), `"permissionDecision":"deny"`) || !strings.Contains(b.String(), "relay: pid-dir: ") {
		t.Errorf("Write %s in a sitting: %s", filepath.Join(dir, night.PidFileName), b.String())
	}
}
