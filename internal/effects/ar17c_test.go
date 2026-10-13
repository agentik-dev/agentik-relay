package effects

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestAR17C_2_SetsidDaemonSwept: a daemon that forks, setsid()s and closes
// its stdio leaves the line's group and holds none of its pipes (ADV10-12);
// the session sweep still kills it by the time Run returns (or within the
// SIGKILL's landing), and the line reads unrun: orphans, never green, with
// the count in its reason.
func TestAR17C_2_SetsidDaemonSwept(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("no perl")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "daemon.pid")
	var pid int
	t.Cleanup(func() {
		if pid > 1 {
			syscall.Kill(pid, syscall.SIGKILL) // the escapee is the test's to end
		}
	})
	line := `perl -MPOSIX -e 'if (fork) { exit 0 } POSIX::setsid(); open F, ">", "daemon.pid" or die; print F "$$\n"; close F; open STDIN,"</dev/null"; open STDOUT,">/dev/null"; open STDERR,">/dev/null"; exec "sleep", "77"'; sleep 0.3; echo ok`
	exit, last, err := OSShell{}.Run(dir, line, time.Minute)
	b, rerr := os.ReadFile(pidFile)
	if rerr != nil {
		t.Fatalf("no daemon pid: %v (Run = %d %q %v)", rerr, exit, last, err)
	}
	fmt.Sscan(strings.TrimSpace(string(b)), &pid)
	if pid <= 1 {
		t.Fatalf("pid file %q", b)
	}
	if err == nil || exit == 0 || !strings.HasPrefix(err.Error(), "orphans: ") {
		t.Fatalf("Run = %d %q %v, want -1 and an orphans error (the line unrun)", exit, last, err)
	}
	var n int
	if _, serr := fmt.Sscanf(err.Error(), "orphans: %d", &n); serr != nil || n < 1 {
		t.Fatalf("orphans count in %q: %d (%v), want >= 1", err, n, serr)
	}
	for deadline := time.Now().Add(3 * time.Second); ; {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the setsid daemon survived the sweep: pid %d", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestAR17C_2_SessionSweepRule: the session sweep takes a new pid in the
// line's session or holding its mark, never one alive before the start nor
// a new stranger, and reads again until none is left.
func TestAR17C_2_SessionSweepRule(t *testing.T) {
	old := SweepPause
	SweepPause = time.Millisecond
	t.Cleanup(func() { SweepPause = old })
	p := &FakeProcs{
		Mortal:   true,
		All:      []int{50, 60, 61, 62, 63, 64},
		Sessions: map[int]int{50: 40, 61: 40, 62: 62, 63: 63, 64: 40},
		Held:     map[string][]int{"/m": {62}},
	}
	// 50 was there before (session 40 or not); 61 left the group only; 62
	// setsid'd with the mark; 63 is a stranger; 64 predates the line too.
	found, left := SweepSession(p, 40, map[int]bool{50: true, 60: true, 64: true}, "/m")
	if found != 2 || left != nil {
		t.Fatalf("SweepSession = %d %v, want 2 nil (kills %v)", found, left, p.Kills())
	}
	if k := p.Kills(); !slices.Equal(k, []int{61, 62}) {
		t.Fatalf("kills %v, want [61 62]", k)
	}
}
