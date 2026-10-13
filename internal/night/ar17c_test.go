package night

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR17C_3_StageLockOneHolder (AR17C, ADV10-13): six takers at once on
// one stage lock, a dead holder's lock there first (every taker reads it
// stale), leave exactly one holder; forty runs.
func TestAR17C_3_StageLockOneHolder(t *testing.T) {
	const takers, runs = 6, 40
	for run := range runs {
		dir := t.TempDir()
		path := filepath.Join(dir, "locks", "l")
		os.MkdirAll(filepath.Dir(path), 0o700)
		os.WriteFile(path, []byte("424242\tTue Sep 22 01:00:00 2026\t/r/.claude/worktrees/a\n"), 0o600)
		fx := effects.Effects{Procs: &effects.FakeProcs{Starts: map[int]string{os.Getpid(): "now"}}}
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			held    int
			release []func()
			start   = make(chan struct{})
		)
		for i := range takers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				undo, err := takeOne(fx, fmt.Sprintf("r%d", i), path, "/r/.claude/worktrees/a")
				if err == nil {
					mu.Lock()
					held++
					release = append(release, undo)
					mu.Unlock()
				}
			}()
		}
		close(start)
		wg.Wait()
		for _, f := range release {
			f()
		}
		if held != 1 {
			t.Fatalf("run %d: %d holders of one lock, want 1", run, held)
		}
	}
}

// takeOne takes the stage lock as Stage does, and its release.
func takeOne(fx effects.Effects, row, path, wt string) (func(), error) {
	f, err := takeLock(fx, row, path, wt)
	if err != nil {
		return nil, err
	}
	st := &Staged{Lock: path, lockFile: f}
	return func() { st.Unlock() }, nil
}

// boundedPython is the interpreter scripts/bounded runs under: pyenv's
// python3, else python3 on PATH; the test skips when there is none.
func boundedPython(t *testing.T) string {
	t.Helper()
	if out, err := exec.Command("pyenv", "which", "python3").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			return p
		}
	}
	p, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3 for scripts/bounded")
	}
	return p
}

// runBounded runs a copy of scripts/bounded with args under a 10 s bound: its
// exit code and stderr. The child's environment carries no BOUNDED_DEPTH
// (AR57, ruling 3: under `make check` the outer bounded sets it, and the
// stop line would read `nested stop`).
func runBounded(t *testing.T, py string, args ...string) (int, string) {
	t.Helper()
	return runBoundedEnv(t, py, nil, args...)
}

// runBoundedEnv is runBounded with env (NAME=value) added to the child's
// environment, after the inherited BOUNDED_DEPTH is removed.
func runBoundedEnv(t *testing.T, py string, env []string, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, py, append([]string{"-B", filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")}, args...)...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "BOUNDED_DEPTH=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	cmd.Stderr = &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, stderr.String()
	case errors.As(err, &ee) && ctx.Err() == nil:
		return ee.ExitCode(), stderr.String()
	}
	t.Fatalf("bounded %v: %v (%s)", args, err, stderr.String())
	return -1, ""
}

// boundedLock writes a bounded lock dir at dir naming pid, started and
// touched age ago.
func boundedLock(t *testing.T, dir string, pid int, age time.Duration) {
	t.Helper()
	then := time.Now().Add(-age)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(dir, "owner")
	os.WriteFile(owner, []byte(fmt.Sprintf("%d %d\n", pid, then.Unix())), 0o600)
	os.Chtimes(owner, then, then)
	os.Chtimes(dir, then, then)
}

// TestAR17C_3_BoundedKeepsLiveLock (AR17C, ADV10-14): scripts/bounded
// never takes a lock whose pid is alive, however old (its STALE was the
// gate's own 900 s wall, so a long live gate lost its tree): exit 2, the
// line unrun, the lock untouched. A dead pid's lock is taken over and the
// line runs (the flocked file stays, released; AR18); six takers of one dead lock at once leave one runner.
func TestAR17C_3_BoundedKeepsLiveLock(t *testing.T) {
	py := boundedPython(t)
	dir := t.TempDir()
	lock := filepath.Join(dir, "live.lock")
	boundedLock(t, lock, os.Getpid(), 2000*time.Second)
	ran := filepath.Join(dir, "ran-live")
	code, stderr := runBounded(t, py, "--lock", lock, "--wall", "5", "--", "touch", ran)
	if _, err := os.Stat(ran); err == nil || code != 2 || !strings.Contains(stderr, "held") {
		t.Fatalf("a live pid's lock 2000 s old: exit %d, ran %v, stderr %q; want 2, unrun, held", code, err == nil, stderr)
	}
	if b, _ := os.ReadFile(filepath.Join(lock, "owner")); !strings.HasPrefix(string(b), fmt.Sprintf("%d ", os.Getpid())) {
		t.Fatalf("the live lock's owner became %q", b)
	}

	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	pid := dead.Process.Pid
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Skipf("pid %d is not dead (%v)", pid, err)
	}
	lock = filepath.Join(dir, "dead.lock")
	boundedLock(t, lock, pid, time.Minute)
	ran = filepath.Join(dir, "ran-dead")
	code, stderr = runBounded(t, py, "--lock", lock, "--wall", "5", "--", "touch", ran)
	if _, err := os.Stat(ran); err != nil || code != 0 {
		t.Fatalf("a dead pid's lock: exit %d, ran %v, stderr %q; want 0, run", code, err == nil, stderr)
	}
	// bounded 63cebd3d (AR18 act 1): the dead holder's mkdir-era directory
	// is removed and the lock is a flocked file the kernel releases.
	if fi, err := os.Stat(lock); err == nil && fi.IsDir() {
		t.Fatalf("the dead holder's lock directory outlived the run")
	}

	lock = filepath.Join(dir, "raced.lock")
	boundedLock(t, lock, pid, time.Minute)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			line := fmt.Sprintf("touch %s/raced-%d; sleep 3", dir, i)
			if code, _ := runBounded(t, py, "--lock", lock, "--wall", "8", "--", "sh", "-c", line); code == 0 {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("six takers of one dead lock: %d ran, want 1", wins)
	}
}

// TestAR17C_4_ProbeLeavesNoOrphan (AR17C, ADV10-21): the live-claude probe
// once cancelled is killed with its whole process group: a child the
// fake lsof started in the background is gone too, not orphaned.
func TestAR17C_4_ProbeLeavesNoOrphan(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child")
	script := filepath.Join(dir, "lsof")
	// The pid file lands by rename, whole or not at all, and only after the
	// child exists; the probe is cancelled on that condition, never on a
	// timeout that a loaded machine outruns before the child is started.
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 30 &\necho $! > "+pidFile+".tmp\nmv "+pidFile+".tmp "+pidFile+"\nwait\n"), 0o755)
	oldArgv := lsofArgv
	t.Cleanup(func() { lsofArgv = oldArgv })
	lsofArgv = []string{script}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() {
		defer cancel()
		for ctx.Err() == nil {
			if _, err := os.Stat(pidFile); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	if _, err := probeClaude(ctx, dir); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("the probe cancelled = %v; want unknown", err)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the fake lsof never started its child: %v", err)
	}
	var pid int
	if _, err := fmt.Sscan(string(b), &pid); err != nil || pid <= 0 {
		t.Fatalf("the child's pid %q", b)
	}
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the probe's child %d outlived the probe by 3 s (an orphan)", pid)
		}
	}
}
