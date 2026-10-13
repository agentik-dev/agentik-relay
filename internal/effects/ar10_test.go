package effects

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestAR10_0_NestedReadsTheVar: Nested is true only when getenv holds
// RELAY_MEASURE_DEPTH; a nil getenv (a fake bundle) is never nested.
func TestAR10_0_NestedReadsTheVar(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if MeasureDepthVar != "RELAY_MEASURE_DEPTH" {
		t.Fatalf("MeasureDepthVar = %q", MeasureDepthVar)
	}
	if Nested(nil) || Nested(env(nil)) || Nested(env(map[string]string{"RELAY_ROW": "x"})) {
		t.Fatal("nested with no depth set")
	}
	if !Nested(env(map[string]string{MeasureDepthVar: "1"})) {
		t.Fatal("not nested with the depth set")
	}
}

// TestAR10_0_OSShellSetsDepth: every line OSShell runs sees
// RELAY_MEASURE_DEPTH=1, whatever the parent's environment held.
func TestAR10_0_OSShellSetsDepth(t *testing.T) {
	t.Setenv(MeasureDepthVar, "7")
	exit, last, err := OSShell{}.Run(t.TempDir(), `echo "depth=$RELAY_MEASURE_DEPTH"`, 10*time.Second)
	if exit != 0 || err != nil || last != "depth=1" {
		t.Fatalf("OSShell = %d %q %v; want depth=1", exit, last, err)
	}
}

// TestAR10_0_ChildEnvAddsDepth: a spawned child is a measurement's child:
// ChildEnv strips the parent's RELAY_* and adds RELAY_MEASURE_DEPTH=1
// beside RELAY_ROW.
func TestAR10_0_ChildEnvAddsDepth(t *testing.T) {
	env := ChildEnv([]string{"HOME=/h", "RELAY_MEASURE_DEPTH=7", "RELAY_ROW=old"}, "r1", "", "", 80, nil, "")
	want := "HOME=/h RELAY_ROW=r1 RELAY_MEASURE_DEPTH=1 CLAUDE_SESSION_DEPTH=1 CLAUDE_SESSION_MAX_TURNS=80"
	if got := strings.Join(env, " "); got != want {
		t.Fatalf("ChildEnv = %s; want %s", got, want)
	}
}

// TestAR10_4_StepClockFiresWaiters: StepClock.After fires only once Advance
// reaches its deadline, a non-positive wait fires at once, and the other
// clocks keep their word (FakeClock's never moves).
func TestAR10_4_StepClockFiresWaiters(t *testing.T) {
	t0 := time.Date(2026, 9, 23, 22, 0, 0, 0, time.UTC)
	c := &StepClock{T: t0}
	var _ Clock = c
	a, b := c.After(10*time.Second), c.After(30*time.Second)
	fired := func(ch <-chan time.Time) bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	if fired(a) || fired(b) {
		t.Fatal("a waiter fired before the clock moved")
	}
	c.Advance(9 * time.Second)
	if fired(a) {
		t.Fatal("fired 1 s early")
	}
	c.Advance(time.Second)
	if !fired(a) || fired(b) {
		t.Fatal("10 s: the first waiter did not fire, or the second did")
	}
	c.Advance(time.Minute)
	if !fired(b) || !c.Now().Equal(t0.Add(70*time.Second)) {
		t.Fatalf("70 s: second waiter, now %v", c.Now())
	}
	if !fired(c.After(0)) || !fired(FakeClock{T: t0}.After(0)) || fired(FakeClock{T: t0}.After(time.Second)) {
		t.Fatal("a non-positive wait does not fire at once, or FakeClock moved")
	}
}

// TestAR10_5_OSLoadParses: the breaker's reading parses sysctl's
// `{ 6.28 27.38 139.23 }` and /proc/loadavg's line to the 1-minute load,
// refuses what is not one, and the fake answers its series then its last.
func TestAR10_5_OSLoadParses(t *testing.T) {
	for in, want := range map[string]float64{"{ 6.28 27.38 139.23 }\n": 6.28, "0.52 0.58 0.59 1/467 4242\n": 0.52, "{ 64.00 1 1 }": 64} {
		if got, err := ParseLoadavg(in); err != nil || got != want {
			t.Fatalf("ParseLoadavg(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "{ }", "{ busy 1 2 }", "-1 0 0"} {
		if _, err := ParseLoadavg(in); err == nil {
			t.Fatalf("ParseLoadavg(%q) parsed", in)
		}
	}
	var _ Load = OSLoad{}
	l := &FakeLoad{Series: []float64{70, 10}}
	a, c, _ := l.Load()
	b, _, _ := l.Load()
	d, _, _ := l.Load()
	if a != 70 || b != 10 || d != 10 || c != 16 || l.Reads() != 3 {
		t.Fatalf("fake load %v %v %v over %d cores, %d reads", a, b, d, c, l.Reads())
	}
}

// TestAR10_6_OSProcsReads: the watchdog's readings parse: ps's CPU time in
// each of its shapes, a group from `ps -o pid=,pgid=`, the newest mtime
// under a tree with .git skipped; a group id of 1 or less is never killed.
func TestAR10_6_OSProcsReads(t *testing.T) {
	for in, want := range map[string]float64{"0:01.50\n": 1.5, "  12:03.00": 723, "01:02:03": 3723, "1-00:00:10": 86410} {
		if got, err := ParsePSTime(in); err != nil || got != want {
			t.Fatalf("ParsePSTime(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "x:01", "1:2:3:4", "-1:00"} {
		if _, err := ParsePSTime(in); err == nil {
			t.Fatalf("ParsePSTime(%q) parsed", in)
		}
	}
	if got := fmt.Sprint(ParsePSGroup("  1   1\n 400 400\n 401 400\n 402 7\nbad\n", 400)); got != "[400 401]" {
		t.Fatalf("ParsePSGroup = %s", got)
	}
	dir := t.TempDir()
	old, recent := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, ".git", "objects", "x"), nil, 0o644)
	os.Chtimes(filepath.Join(dir, "a.txt"), old, old)
	os.Chtimes(filepath.Join(dir, ".git", "objects", "x"), recent, recent)
	os.Chtimes(filepath.Join(dir, ".git", "objects"), recent, recent)
	os.Chtimes(filepath.Join(dir, ".git"), recent, recent)
	os.Chtimes(dir, old, old)
	if got, err := (OSProcs{}).Newest(dir); err != nil || !got.Equal(old.Truncate(time.Second)) && !got.Equal(old) {
		t.Fatalf("Newest = %v, %v; want %v (.git skipped)", got, err, old)
	}
	if (OSProcs{}).KillGroup(1) == nil || (OSProcs{}).KillGroup(0) == nil {
		t.Fatal("KillGroup took a group id of 1 or less")
	}
	var _ Procs = OSProcs{}
	var _ Procs = &FakeProcs{}
}

// TestAR10_8_SweepRounds: Sweep counts the group's members other than the
// leader, kills and reads again until none is left, at most three kills;
// what outlives the third is left.
func TestAR10_8_SweepRounds(t *testing.T) {
	old := SweepPause
	SweepPause = 0
	t.Cleanup(func() { SweepPause = old })
	p := &FakeProcs{Groups: map[int][]int{9: {9, 10, 11}}}
	kills := 0
	p.OnKill = func(int) {
		if kills++; kills == 2 {
			p.Groups = nil
		}
	}
	if n, left := Sweep(p, 9); n != 2 || left != nil || len(p.Kills()) != 2 {
		t.Fatalf("swept in two: orphans %d, left %v, kills %v", n, left, p.Kills())
	}
	p = &FakeProcs{Groups: map[int][]int{9: {12}}}
	if n, left := Sweep(p, 9); n != 1 || fmt.Sprint(left) != "[12]" || len(p.Kills()) != SweepRounds || SweepRounds != 3 {
		t.Fatalf("a survivor: orphans %d, left %v, kills %v", n, left, p.Kills())
	}
	p = &FakeProcs{}
	if n, left := Sweep(p, 9); n != 0 || left != nil || len(p.Kills()) != 0 {
		t.Fatalf("the leader alone: orphans %d, left %v, kills %v", n, left, p.Kills())
	}
}

// TestAR10_8_ShellSweepsBackground: a line that backgrounds a sleep with
// its output elsewhere returns at once, and the sweep has killed the sleep
// by the time Run returns (or within the SIGKILL's landing).
func TestAR10_8_ShellSweepsBackground(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	exit, last, err := OSShell{}.Run(dir, "sleep 27.1828 >/dev/null 2>&1 & echo $! > pid; echo ok", time.Minute)
	if exit != 0 || last != "ok" || err != nil || time.Since(start) > 2*time.Second {
		t.Fatalf("Run = %d %q %v in %v", exit, last, err, time.Since(start))
	}
	b, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	fmt.Sscan(strings.TrimSpace(string(b)), &pid)
	if pid <= 1 {
		t.Fatalf("pid file %q", b)
	}
	for deadline := time.Now().Add(time.Second); ; {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the backgrounded sleep survived the sweep: pid %d", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestAR10_9_WaitReturnsWithinASecond: Run returns within a second of the
// shell's exit whatever holds its pipes: a backgrounded sleep in the
// line's group holding stdout (swept), and one that left the group
// (setpgrp) holding stderr (cut off, the output before the exit kept; since
// ADV6-8 the held pipe is the line's error, -1). The second is the claim
// (AR35): each line has three tries, passes on the first under it and logs
// each miss; any other miss fails at once. The escapee writes `left` once
// out of the group and the shell waits for it, so a loaded machine never
// sweeps it as a member of the group before it has left.
func TestAR10_9_WaitReturnsWithinASecond(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("no perl")
	}
	for i, line := range []string{
		"echo 0; sleep 29.9792 & echo $! > pid",
		`echo 0; perl -e 'setpgrp(0, 0); open(F, ">left"); close(F); sleep 30' 1>&2 & p=$!; n=0; ` +
			`while [ ! -e left ] && [ $n -lt 3000 ]; do sleep 0.01; n=$((n+1)); done; echo $p > pid`,
	} {
		held := i == 1
		for try := 1; ; try++ {
			dir := t.TempDir()
			start := time.Now()
			exit, last, err := OSShell{}.Run(dir, line, time.Minute)
			took := time.Since(start)
			if b, rerr := os.ReadFile(filepath.Join(dir, "pid")); rerr == nil {
				var pid int
				fmt.Sscan(strings.TrimSpace(string(b)), &pid)
				if pid > 1 {
					syscall.Kill(pid, syscall.SIGKILL) // the escapee is the test's to end
				}
			}
			if (exit != 0) != held || last != "0" || (err != nil) != held {
				t.Fatalf("%s: Run = %d %q %v in %v, want 0 (held: -1 and an error) \"0\" under 1 s", line, exit, last, err, took)
			}
			if took < time.Second {
				break
			}
			t.Logf("%s: try %d of 3: Run returned in %v, not under 1 s", line, try, took)
			if try == 3 {
				t.Fatalf("%s: Run never returned under 1 s in 3 tries", line)
			}
		}
	}
}
