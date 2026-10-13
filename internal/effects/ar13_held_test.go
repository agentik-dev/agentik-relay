package effects

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// holder reads the pid a test's holder wrote to dir/holder and kills its
// group (it left the line's with setsid) at cleanup.
func holder(t *testing.T, dir string) {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "holder"))
	if pid, _ := strconv.Atoi(strings.TrimSpace(string(b))); pid > 1 {
		t.Cleanup(func() { syscall.Kill(-pid, syscall.SIGKILL) })
	}
}

// heldLine forks a setsid perl that writes its pid to ./holder and sleeps
// holding the line's stdout, waits for the pid, then says ok and exits.
const heldLine = `perl -e 'use POSIX; setsid(); open F, ">holder"; print F $$; close F; sleep 30' & ` +
	`while [ ! -s holder ]; do sleep 0.05; done; echo ok`

// TestAR13_3_ShellHeldPipeUnrun (ADV6-8): a line whose setsid descendant
// still holds its stdout PipeGrace after the exit and the sweep is cut off
// and the line's error (the check unrun), never a clean exit 0; Run
// returns within a second of the grace.
func TestAR13_3_ShellHeldPipeUnrun(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("no perl")
	}
	old := PipeGrace
	PipeGrace = 200 * time.Millisecond
	defer func() { PipeGrace = old }()
	dir := t.TempDir()
	start := time.Now()
	exit, last, err := OSShell{}.Run(dir, heldLine, 20*time.Second)
	took := time.Since(start)
	holder(t, dir)
	if err == nil || exit != -1 || !strings.HasPrefix(err.Error(), "orphans") || took > 5*time.Second {
		t.Fatalf("exit %d last %q err %v after %s, want -1 and an orphans error", exit, last, err, took)
	}
}

// TestAR13_3_DescendantsByPpid: OSProcs.Descendants walks the ppid tree,
// so a descendant that left the group is still found; ParsePSTree walks
// breadth first and ignores a self-parented line.
func TestAR13_3_DescendantsByPpid(t *testing.T) {
	if got := ParsePSTree(" 1 0\n 10 1\n 11 10\n 12 11\n 13 1\n 14 14\n", 10); len(got) != 2 || got[0] != 11 || got[1] != 12 {
		t.Fatalf("ParsePSTree %v, want [11 12]", got)
	}
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("no perl")
	}
	dir := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", `perl -e 'use POSIX; setsid(); open F, ">holder"; print F $$; close F; sleep 30' </dev/null >/dev/null 2>&1 & `+
		`while [ ! -s holder ]; do sleep 0.05; done; sleep 30`)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if b, _ := os.ReadFile(filepath.Join(dir, "holder")); len(b) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	holder(t, dir)
	b, _ := os.ReadFile(filepath.Join(dir, "holder"))
	want, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	pids, err := OSProcs{}.Descendants(cmd.Process.Pid)
	found := false
	for _, p := range pids {
		found = found || p == want
	}
	if err != nil || want == 0 || !found {
		t.Fatalf("descendants of %d: %v %v, want %d among them", cmd.Process.Pid, pids, err, want)
	}
}
