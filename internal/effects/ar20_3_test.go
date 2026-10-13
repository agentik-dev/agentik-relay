package effects

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestAR20_3_LsRemoteTimesOut is ADV11-16: an ls-remote whose transport
// stalls (a fake ssh that sleeps, and a grandchild of it) returns an error
// within its timeout, and its whole process group is gone after. No
// network: lsRemoteSSH is the fake (AR22 act 1), the host never resolved.
func TestAR20_3_LsRemoteTimesOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ssh := filepath.Join(dir, "ssh")
	body := "#!/bin/sh\necho $$ > " + filepath.Join(dir, "child") + "\nsleep 30 &\necho $! > " +
		filepath.Join(dir, "grandchild") + "\nexec sleep 30\n"
	if err := os.WriteFile(ssh, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	was0 := lsRemoteSSH // AR22 act 1: LsRemote's ssh is its own, never the env's
	lsRemoteSSH = ssh
	t.Cleanup(func() { lsRemoteSSH = was0 })
	was := LsRemoteTimeout
	LsRemoteTimeout = 400 * time.Millisecond
	t.Cleanup(func() { LsRemoteTimeout = was })

	// A loaded machine may take the whole timeout to start git's ssh; the
	// timeout then still held, and the attempt is made again (AR35): until
	// the fake writes its grandchild's pid, its last write before it
	// stalls, with a minute's deadline and each attempt's timeout twice the
	// last's, from 400 ms to at most 6.4 s. Each bound on the time is a
	// hang guard, ten times the timeout.
	pidIn := func(name string) (int, error) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(strings.TrimSpace(string(b)))
	}
	deadline := time.Now().Add(time.Minute)
	for attempt := 1; ; attempt++ {
		guard := 10 * LsRemoteTimeout
		done := make(chan error, 1)
		start := time.Now()
		go func() { _, err := (OSGit{}).LsRemote(dir, "ssh://relay.invalid/x.git"); done <- err }()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("LsRemote = %v, want a timeout", err)
			}
			if took := time.Since(start); took > guard {
				t.Fatalf("LsRemote took %s, its timeout %s", took, LsRemoteTimeout)
			}
		case <-time.After(2 * guard):
			t.Fatalf("LsRemote still running after %s: no timeout", 2*guard)
		}
		if pid, err := pidIn("grandchild"); err == nil && pid > 1 {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("the fake ssh never wrote its grandchild in %d attempts over a minute: %v", attempt, err)
		}
		t.Logf("attempt %d: the fake ssh did not stall inside a timeout of %s", attempt, LsRemoteTimeout)
		LsRemoteTimeout = min(2*LsRemoteTimeout, 6400*time.Millisecond)
	}
	for _, name := range []string{"child", "grandchild"} {
		pid, err := pidIn(name)
		if err != nil || pid <= 1 {
			t.Fatalf("the fake ssh never wrote its %s: %d %v", name, pid, err)
		}
		alive := true
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if syscall.Kill(pid, 0) != nil {
				alive = false
				break
			}
		}
		if alive {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the %s (pid %d) outlived the timeout: an orphan", name, pid)
		}
	}
}

// TestAR20_3_LineOutputTail is ADV11-17's buffer: a line's output keeps
// its last OutputCap bytes, and its last line still reads.
func TestAR20_3_LineOutputTail(t *testing.T) {
	b := tail{max: OutputCap}
	for range 3 {
		b.Write([]byte(strings.Repeat("x", 50<<10)))
	}
	b.Write([]byte("\nlast\n"))
	if got := b.String(); len(got) != OutputCap || LastLine(got) != "last" {
		t.Fatalf("tail holds %d bytes, last line %q", len(got), LastLine(got))
	}
	exit, last, err := OSShell{}.Run(t.TempDir(), "head -c 200000 /dev/zero | tr '\\0' x; echo; echo last", 10*time.Second)
	if err != nil || exit != 0 || last != "last" {
		t.Fatalf("Run = %d %q %v", exit, last, err)
	}
}

// TestAR20_3_SuiteUnderPushDeny: the entries PushDenyEnv adds are the ones
// DropEnvConfig (every suite's TestMain) removes, so the suite's own pushes
// into t.TempDir() remotes run under a sitting's or a gate's env.
func TestAR20_3_SuiteUnderPushDeny(t *testing.T) {
	added := 0
	for _, kv := range PushDenyEnv(nil, []string{"origin"}) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
		added++
	}
	if added != 5 {
		t.Fatalf("PushDenyEnv added %d entries, want 5", added)
	}
	DropEnvConfig()
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_CONFIG_COUNT=") || strings.HasPrefix(kv, "GIT_CONFIG_KEY_") || strings.HasPrefix(kv, "GIT_CONFIG_VALUE_") {
			t.Fatalf("%s survived DropEnvConfig", kv)
		}
	}
}
