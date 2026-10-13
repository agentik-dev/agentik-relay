package night_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR17B_2_AncestorPidRefuses, under AR21 act 2's rule (ADV12-3): the
// lock, not the line, is the holder. Lines no process holds the lock for
// (a reused start time, a dead pid, a live pid that is no relay) are taken
// over; while this process holds the file, the next comer is refused with
// `nested: relay <pid> is live`, ancestor or not; release empties the file.
func TestAR17B_2_AncestorPidRefuses(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, night.PidFileName)
	parent := os.Getppid()
	at, err := night.PidStarted(parent)
	if err != nil {
		t.Skipf("ps: %v", err)
	}
	sleeper := exec.Command("sleep", "30")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sleeper.Process.Kill(); sleeper.Wait() })
	other, _ := night.PidStarted(sleeper.Process.Pid)
	lines := fmt.Sprintf("%d Mon Jan  1 00:00:00 2001\n999999 %s\n%d %s\n", parent, at, sleeper.Process.Pid, other)
	os.WriteFile(path, []byte(lines), 0o644)
	release, err := night.PidFile(state)
	if err != nil {
		t.Fatalf("reused, dead and unlocked lines: err %v", err)
	}
	b, _ := os.ReadFile(path)
	if want := fmt.Sprintf("%d ", os.Getpid()); !strings.HasPrefix(string(b), want) || strings.Count(string(b), "\n") != 1 {
		t.Fatalf("pid file:\n%s\nwant one line starting %q", b, want)
	}
	_, err = night.PidFile(state)
	var ne night.NestedError
	if !errors.As(err, &ne) || ne.Pid != os.Getpid() || err.Error() != fmt.Sprintf("nested: relay %d is live", os.Getpid()) {
		t.Fatalf("a live holder: err %v", err)
	}
	release()
	if b, _ := os.ReadFile(path); len(b) != 0 {
		t.Fatalf("after release:\n%s", b)
	}
}
