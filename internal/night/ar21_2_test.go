package night_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// pidChildEnv names the directory a re-executed test binary takes the pid
// file in: it writes PidFile's answer to <dir>/answer ("held" or the
// refusal) and its own pid to <dir>/child first.
const pidChildEnv = "AR21_2_PID_CHILD"

// pidChild is the child's half: true when this process is one.
func pidChild(t *testing.T) bool {
	dir := os.Getenv(pidChildEnv)
	if dir == "" {
		return false
	}
	os.WriteFile(filepath.Join(dir, "child"), []byte(strconv.Itoa(os.Getpid())), 0o600)
	answer := "held"
	if _, err := night.PidFile(filepath.Join(dir, "state")); err != nil {
		answer = err.Error()
		var ne night.NestedError
		if errors.As(err, &ne) {
			answer = fmt.Sprintf("NestedError %d: %s", ne.Pid, err)
		}
	}
	os.WriteFile(filepath.Join(dir, "answer.tmp"), []byte(answer), 0o600)
	os.Rename(filepath.Join(dir, "answer.tmp"), filepath.Join(dir, "answer"))
	return true
}

// childAnswer re-runs this test binary on test's name as a pid-file child
// over dir/state, double-forked (orphaned, so no process here is its
// ancestor) when detached, and returns its answer.
func childAnswer(t *testing.T, dir, test string, detached bool) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-test.run=^" + test + "$", "-test.count=1"}
	var c *exec.Cmd
	if detached {
		c = exec.Command("/bin/sh", append([]string{"-c", `( "$0" "$@" </dev/null >/dev/null 2>&1 & )`, self}, args...)...)
	} else {
		c = exec.Command(self, args...)
	}
	c.Env = append(os.Environ(), pidChildEnv+"="+dir)
	t.Cleanup(func() {
		if _, err := os.Stat(filepath.Join(dir, "answer")); err == nil {
			return
		}
		if b, err := os.ReadFile(filepath.Join(dir, "child")); err == nil {
			if pid, err := strconv.Atoi(string(b)); err == nil && pid > 1 {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("the child: %v\n%s", err, out)
	}
	for end := time.Now().Add(30 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if b, err := os.ReadFile(filepath.Join(dir, "answer")); err == nil {
			return string(b)
		}
	}
	t.Fatal("the child never answered")
	return ""
}

// TestAR21_2_DetachedNestedRelay (review-adv12 test 3; ADV12-3): with a
// live holder, a double-forked child, whose ancestors do not include the
// holder, is refused by PidFile with NestedError naming the holder's pid.
func TestAR21_2_DetachedNestedRelay(t *testing.T) {
	if pidChild(t) {
		return
	}
	dir := t.TempDir()
	release, err := night.PidFile(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	want := fmt.Sprintf("NestedError %d: nested: relay %d is live", os.Getpid(), os.Getpid())
	if got := childAnswer(t, dir, "TestAR21_2_DetachedNestedRelay", true); got != want {
		t.Fatalf("a double-forked child under a live holder: %q, want %q", got, want)
	}
}

// TestAR21_2_TruncatedPidStillRefuses (ADV12-8): the holder's lock (the
// port since AR22 act 2) is the refusal, not its line: with the pid file truncated, the next comer (here
// a direct child) is still refused; once the holder lets go, the lock is
// free and the next PidFile takes the file over.
func TestAR21_2_TruncatedPidStillRefuses(t *testing.T) {
	if pidChild(t) {
		return
	}
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	release, err := night.PidFile(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, night.PidFileName)
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	if got := childAnswer(t, dir, "TestAR21_2_TruncatedPidStillRefuses", false); !strings.HasPrefix(got, "NestedError 0: nested: ") {
		t.Fatalf("a truncated pid file under a live holder: %q", got)
	}
	release()
	release()
	if b, _ := os.ReadFile(path); len(b) != 0 {
		t.Fatalf("after release: %q", b)
	}
	again, err := night.PidFile(state)
	if err != nil {
		t.Fatalf("after the holder let go: %v", err)
	}
	defer again()
	if b, _ := os.ReadFile(path); !strings.HasPrefix(string(b), fmt.Sprintf("%d ", os.Getpid())) {
		t.Fatalf("the pid file: %q", b)
	}
}
