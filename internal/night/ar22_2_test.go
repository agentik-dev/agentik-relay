package night_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// lockPortEnv hands this test binary's lock port to the children it
// re-executes: the tests' plumbing alone, the relay reads no port from
// the environment.
const lockPortEnv = "AR22_2_NIGHT_LOCK_PORT"

// useTestLockPort (TestMain) points night.LockPort at a free 127.0.0.1
// port, or at the parent test binary's: no test binds the account's.
func useTestLockPort() error {
	port, err := strconv.Atoi(os.Getenv(lockPortEnv))
	if err != nil || port <= 0 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("a free lock port: %w", err)
		}
		port = l.Addr().(*net.TCPAddr).Port
		l.Close()
		os.Setenv(lockPortEnv, strconv.Itoa(port))
	}
	night.LockPort = func() (int, error) { return port, nil }
	return nil
}

// TestAR22_2_Takeover (review-adv13 test 3; ADV13-4): the holder live, its
// pid file unlinked, renamed, or its dir renamed, a second taker (a child
// process) is refused each time, naming no pid it cannot prove live.
func TestAR22_2_Takeover(t *testing.T) {
	if pidChild(t) {
		return
	}
	for _, move := range []string{"unlink", "rename", "dir-rename"} {
		dir := t.TempDir()
		state := filepath.Join(dir, "state")
		release, err := night.PidFile(state)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(state, night.PidFileName)
		switch move {
		case "unlink":
			err = os.Remove(path)
		case "rename":
			err = os.Rename(path, path+".old")
		case "dir-rename":
			err = os.Rename(state, state+".moved")
		}
		if err != nil {
			t.Fatal(err)
		}
		got := childAnswer(t, dir, "TestAR22_2_Takeover", false)
		release()
		if !strings.HasPrefix(got, "NestedError 0: nested: ") || strings.Contains(got, "relay 0") {
			t.Errorf("%s under a live holder: the second taker %q, want refused naming no pid", move, got)
		}
	}
}

// holdChildEnv names the dir a re-executed test binary holds the lock
// over until it is killed (TestAR22_2_HolderDeath).
const holdChildEnv = "AR22_2_HOLD_CHILD"

// TestAR22_2_HolderDeath: a real child holds the lock; the next taker is
// refused naming it (live, its start read back); SIGKILLed, the kernel
// frees its port and the next taker holds, its own line in relay.pid.
func TestAR22_2_HolderDeath(t *testing.T) {
	if dir := os.Getenv(holdChildEnv); dir != "" {
		answer := "held"
		if _, err := night.PidFile(filepath.Join(dir, "state")); err != nil {
			answer = err.Error()
		}
		os.WriteFile(filepath.Join(dir, "answer.tmp"), []byte(answer), 0o600)
		os.Rename(filepath.Join(dir, "answer.tmp"), filepath.Join(dir, "answer"))
		if answer == "held" {
			time.Sleep(time.Hour) // until the parent's SIGKILL
		}
		return
	}
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	os.MkdirAll(filepath.Join(dir, "tmp"), 0o700)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := exec.Command(self, "-test.run=^TestAR22_2_HolderDeath$", "-test.count=1")
	c.Env = append(os.Environ(), holdChildEnv+"="+dir, "TMPDIR="+filepath.Join(dir, "tmp"))
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Process.Kill(); c.Wait() })
	var answer []byte
	for end := time.Now().Add(30 * time.Second); len(answer) == 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the holder never answered")
		}
		answer, _ = os.ReadFile(filepath.Join(dir, "answer"))
	}
	if string(answer) != "held" {
		t.Fatalf("the holder: %s", answer)
	}
	_, err = night.PidFile(state)
	var ne night.NestedError
	if !errors.As(err, &ne) || ne.Pid != c.Process.Pid || err.Error() != fmt.Sprintf("nested: relay %d is live", c.Process.Pid) {
		t.Fatalf("under a live holder %d: %v", c.Process.Pid, err)
	}
	c.Process.Signal(syscall.SIGKILL)
	c.Wait()
	release, err := night.PidFile(state)
	if err != nil {
		t.Fatalf("after the holder's SIGKILL: %v", err)
	}
	defer release()
	if b, _ := os.ReadFile(filepath.Join(state, night.PidFileName)); !strings.HasPrefix(string(b), fmt.Sprintf("%d ", os.Getpid())) {
		t.Fatalf("the pid file after the takeover: %q", b)
	}
}

// TestAR22_2_PidFileMovedStops: the holder's poll re-stats relay.pid; gone,
// or a new inode at its path, the night stops lock-moved before its first
// sitting; release leaves a file it did not write.
func TestAR22_2_PidFileMovedStops(t *testing.T) {
	for _, move := range []string{"gone", "new-inode"} {
		t.Run(move, func(t *testing.T) {
			w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
			o := w.opts(t, by)
			pids := t.TempDir()
			release, err := night.PidFile(pids)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if night.LockMoved() != "" {
				t.Fatalf("held and unmoved: %q", night.LockMoved())
			}
			path := filepath.Join(pids, night.PidFileName)
			if move == "gone" {
				err = os.Remove(path)
			} else {
				b, _ := os.ReadFile(path)
				if err = os.WriteFile(path+".new", b, 0o644); err == nil {
					err = os.Rename(path+".new", path)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			n, err := night.Run(w.fx, effects.OSSpawner{}, q, o)
			if err != nil {
				t.Fatalf("night: %v", err)
			}
			if n.State != relay.Stopped(relay.LockMoved) || relay.LockMoved != "lock-moved" ||
				(n.Sittings["r1"] != nil && relay.Terminal(n.Sittings["r1"].State) && n.Sittings["r1"].State != relay.Skipped) {
				t.Fatalf("night %s: %s", n.State, states(n))
			}
			stopped := false
			for _, l := range w.ledger.Written() {
				if l["event"] == "stop" && l["reason"] == "lock-moved" && strings.Contains(fmt.Sprint(l["detail"]), night.PidFileName) {
					stopped = true
				}
				if l["event"] == "sitting" {
					t.Fatalf("a sitting ran after the pid file moved: %v", l)
				}
			}
			if !stopped {
				t.Fatalf("no lock-moved stop line: %v", w.ledger.Written())
			}
			release()
			if _, err := os.Stat(path); (move == "gone") != os.IsNotExist(err) {
				t.Fatalf("%s: after release, the pid file: %v", move, err)
			}
		})
	}
}
