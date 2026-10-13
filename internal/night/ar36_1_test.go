package night_test

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// ar36Port points night.LockPort at a free port of this test's own for
// its life, and returns it.
func ar36Port(t *testing.T) int {
	t.Helper()
	port := freePort(t)
	old := night.LockPort
	t.Cleanup(func() { night.LockPort = old })
	night.LockPort = func() (int, error) { return port, nil }
	return port
}

// portFree is whether 127.0.0.1:port binds now.
func portFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// TestAR36_1_LiveHolderFileIsNotReplaced (AR36 ruling 1): relay.pid names
// a live process other than this one (the test's parent, its real start
// time) while this process's port is free, as a probe linked to a port of
// its own and the account's real pid dir: PidFile refuses naming that
// holder and the path, leaves the file's inode and text as they were, and
// frees the port it took. Its subtest start-unreadable (the fix round's
// adversary F2): the holder live by kill(pid, 0) while PidStarted fails for
// it (a ps that could not run) is refused all the same.
func TestAR36_1_LiveHolderFileIsNotReplaced(t *testing.T) {
	holder := os.Getppid()
	started, err := night.PidStarted(holder)
	if err != nil {
		t.Skipf("the parent's start time: %v", err)
	}
	ar36Refused(t, holder, fmt.Sprintf("%d %s\n", holder, started))
	t.Run("start-unreadable", func(t *testing.T) {
		orig := night.PidStarted
		t.Cleanup(func() { night.PidStarted = orig })
		night.PidStarted = func(pid int) (string, error) {
			if pid == holder {
				return "", fmt.Errorf("ps: fork: resource temporarily unavailable")
			}
			return orig(pid)
		}
		ar36Refused(t, holder, fmt.Sprintf("%d %s\n", holder, started))
	})
}

// ar36Refused writes line as relay.pid in a temp dir, then expects
// PidFile on a free port of its own to refuse naming holder and the path,
// the file's inode and text unchanged, the port free and no lock held.
func ar36Refused(t *testing.T, holder int, line string) {
	t.Helper()
	port := ar36Port(t)
	dir := t.TempDir()
	path := filepath.Join(dir, night.PidFileName)
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	release, err := night.PidFile(dir)
	if err == nil {
		release()
		t.Fatalf("PidFile over a live holder's relay.pid (pid %d) took the lock and wrote its line", holder)
	}
	want := fmt.Sprintf("%s names live relay %d, which holds another lock; this process is on port %d", path, holder, port)
	if err.Error() != want {
		t.Errorf("the refusal %q, want %q", err, want)
	}
	if !strings.HasSuffix(err.Error(), fmt.Sprintf("relay.pid names live relay %d, which holds another lock; this process is on port %d", holder, port)) {
		t.Errorf("the refusal %q is not the ruled text", err)
	}
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("the holder's relay.pid after the refusal: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Errorf("the holder's relay.pid is a new inode after the refusal")
	}
	if b, _ := os.ReadFile(path); string(b) != line {
		t.Errorf("the holder's relay.pid reads %q, want %q", b, line)
	}
	if !portFree(port) {
		t.Errorf("port %d is still held after the refusal", port)
	}
	if m := night.LockMoved(); m != "" {
		t.Errorf("a refused PidFile left a lock held: %q", m)
	}
}

// TestAR36_1_DeadHolderFileIsReplaced (AR36 ruling 1, crash recovery):
// relay.pid names a pid that is not live (a reaped child), or a live pid
// whose start time is not the line's (a reused pid): PidFile takes the
// lock and rewrites the file with its own line.
func TestAR36_1_DeadHolderFileIsReplaced(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	dead := cmd.Process.Pid
	if _, err := night.PidStarted(dead); err == nil {
		t.Skipf("pid %d, reaped, is live again", dead)
	}
	for name, line := range map[string]string{
		"dead":   fmt.Sprintf("%d Thu Jan  1 00:00:00 2015\n", dead),
		"reused": fmt.Sprintf("%d Thu Jan  1 00:00:00 2015\n", os.Getppid()),
	} {
		t.Run(name, func(t *testing.T) {
			port := ar36Port(t)
			dir := t.TempDir()
			path := filepath.Join(dir, night.PidFileName)
			if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
				t.Fatal(err)
			}
			release, err := night.PidFile(dir)
			if err != nil {
				t.Fatalf("PidFile over a %s holder's relay.pid: %v", name, err)
			}
			defer release()
			if portFree(port) {
				t.Errorf("port %d is free while the lock is held", port)
			}
			self, err := night.PidStarted(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(path); string(b) != fmt.Sprintf("%d %s\n", os.Getpid(), self) {
				t.Errorf("relay.pid reads %q, want this process's line", b)
			}
			if m := night.LockMoved(); m != "" {
				t.Errorf("held and unmoved: %q", m)
			}
		})
	}
}
