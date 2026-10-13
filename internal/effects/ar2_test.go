package effects

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// AR2-6: a check that backgrounds a child reports its own exit, and the
// child dies with the line's process group. The line writes the child's pid
// to a file in its own temp dir, so the test watches that process only.
func TestAR2_6_ShellReapsBackgroundChild(t *testing.T) {
	dir := t.TempDir()
	exit, last, err := OSShell{}.Run(dir, "echo 0; sleep 31.4159 & echo $! > pid", time.Minute)
	if exit != 0 || last != "0" || err != nil {
		t.Errorf("Run = %d %q %v; want 0 \"0\" nil", exit, last, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		t.Fatalf("pid file %q: %v", b, err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; { // SIGKILL lands asynchronously
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the backgrounded child survived: pid %d", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
