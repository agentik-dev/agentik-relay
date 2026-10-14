package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ar17cPacket is a row whose acceptance, like the fixture's check (a
// `sleep 45`), never ends: a `cat` of a FIFO nobody writes (%s), on the
// allowlist since AR20 act 5. Whatever the verb measures first outlives the
// signal.
const ar17cPacket = "# Packet r: a signal row\n\n| item | value |\n|---|---|\n| from | `design/relay-r1.md` §9 |\n" +
	"| pin | branch `main` |\n| wall | this repo only |\n| may touch | `x`. Nothing else |\n" +
	"| acceptance | `cat %s && echo ok` reads `ok` |\n| budget | 10 turns, 20k context, $5 |\n\n" +
	"## 1. What the source says (quoted)\nQuoted.\n\n## 2. Rulings\nNone.\n\n## 3. Interfaces (signatures, not prose)\nNone.\n\n" +
	"## 4. Findings that name this row\nNone.\n\n## 5. Forbidden reads\nNone.\n\n## 6. Report (capped)\nA table.\n"

// signalWorld is the built relay over a fixture repo under t.TempDir()
// whose check line is `sleep 45`, one row r, and a scratch environment.
// With branch, the fixture also holds branch r (the gate's case); without
// it, plan reads r ready and measures it (AR58: plan refuses a row whose
// branch exists).
type signalWorld struct{ bin, root, state string }

func newSignalWorld(t *testing.T, branch bool) signalWorld {
	t.Helper()
	tmp := t.TempDir()
	w := signalWorld{bin: filepath.Join(tmp, "relay"), root: filepath.Join(tmp, "repo"), state: filepath.Join(tmp, "state")}
	if msg, err := exec.Command("go", "build", "-ldflags", pidHomeFlag(tmp), "-o", w.bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, msg)
	}
	home, fifo := filepath.Join(tmp, "home"), filepath.Join(tmp, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{filepath.Join(w.root, "relay"), filepath.Join(w.root, "design", "packets"), w.state, filepath.Join(home, ".config", "agentik", "relay")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(home, ".gitconfig"):                                 "[user]\n\tname = relay\n\temail = \n",
		filepath.Join(home, ".config", "agentik", "relay", "config.yaml"): "caffeinate: false\n",
		filepath.Join(w.root, "Makefile"):                                 "check: ; @sleep 45\n",
		filepath.Join(w.root, ".gitignore"):                               ".claude/\n",
		filepath.Join(w.root, "design", "packets", "r.md"):                fmt.Sprintf(ar17cPacket, fifo),
		filepath.Join(w.root, "relay", "queue.yaml"): "version: 1\n" +
			"defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 2m,\n" +
			"           merge: merge-on-green, device: never, verify: auto}\n" +
			"words: w\nrows:\n  - id: r\n    packet: design/packets/r.md\n",
	}
	for p, b := range files {
		if err := os.WriteFile(p, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gits := [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "fixture"}}
	if branch {
		gits = append(gits, []string{"branch", "r"})
	}
	for _, args := range gits {
		c := exec.Command("git", args...)
		c.Dir, c.Env = w.root, w.env(home)
		if msg, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, msg)
		}
	}
	return w
}

func (w signalWorld) env(home string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "GIT_CONFIG_NOSYSTEM=1",
		"XDG_STATE_HOME=" + w.state, "TMPDIR=" + os.TempDir()}
}

// signalMid starts the relay verb, waits for groups.json to record a
// measured line, sends SIGTERM, and asserts: exit 1, the line's group dead,
// the stop line naming the signal.
func signalMid(t *testing.T, args ...string) {
	w := newSignalWorld(t, args[0] != "plan")
	cmd := exec.Command(w.bin, args...)
	var out strings.Builder
	cmd.Dir, cmd.Env = w.root, w.env(filepath.Join(filepath.Dir(w.root), "home"))
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			cmd.Process.Kill()
			<-done
		}
	})
	var pgid int
	for end := time.Now().Add(30 * time.Second); pgid == 0; {
		if time.Now().After(end) {
			t.Fatalf("no groups.json recorded a line in 30 s; output:\n%s", out.String())
		}
		files, _ := filepath.Glob(filepath.Join(w.state, "agentik-relay", "nights", "*", "groups.json"))
		for _, f := range files {
			var doc struct {
				Groups []struct {
					Pgid int `json:"pgid"`
				} `json:"groups"`
			}
			if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &doc) == nil && len(doc.Groups) > 0 {
				pgid = doc.Groups[0].Pgid
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Cleanup(func() { syscall.Kill(-pgid, syscall.SIGKILL) })
	cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("the relay did not exit 20 s after SIGTERM; output:\n%s", out.String())
	}
	if code := cmd.ProcessState.ExitCode(); code != 1 {
		t.Errorf("exit = %d, want 1; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "stopped by signal:SIGTERM") {
		t.Errorf("no stop line naming the signal; output:\n%s", out.String())
	}
	for end := time.Now().Add(3 * time.Second); syscall.Kill(-pgid, 0) == nil; {
		if time.Now().After(end) {
			t.Fatalf("the measured line's group %d is alive after the relay's exit", pgid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestAR17C_1_SignalMidPlan (review ADV10's test 3): SIGTERM to `relay
// plan --rows r` mid-baseline: groups.json records the line, the relay
// exits 1 naming the signal, and no descendant is alive.
func TestAR17C_1_SignalMidPlan(t *testing.T) { signalMid(t, "plan", "--rows", "r") }

// TestAR17C_1_SignalMidGate: the same for `relay gate r` mid-measurement.
func TestAR17C_1_SignalMidGate(t *testing.T) { signalMid(t, "gate", "r") }
