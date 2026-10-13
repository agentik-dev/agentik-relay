package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ownLogs puts a copy of the package pin whose Logs is a dir of the
// test's own, restored at cleanup, and names it: the package pin's holds
// the red logs of earlier tests (AR31 act 2, D18).
func ownLogs(t *testing.T) string {
	t.Helper()
	old, _ := effects.Current()
	p := old
	p.Logs = filepath.Join(t.TempDir(), "logs")
	effects.Use(p)
	t.Cleanup(func() { effects.Use(old) })
	return p.Logs
}

// seqOut is `seq 1 n`'s output.
func seqOut(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%d\n", i)
	}
	return b.String()
}

// logsIn are the names in dir.
func logsIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestAR31_2_DoctorKeepsLog is AR31 act 2, the owner's acceptance 3: a red
// `relay doctor --verify` line's --json names a 0600 log in the pin's Logs
// holding its whole output (its first line the line's TMPDIR, gone after
// the command), and the clone is gone; the text's fourth line is `log:
// <path>`. A green line and the `auto` probe keep none. A line bounded
// stopped exits 2, naming its log at the end of its error, and --json
// carries both.
func TestAR31_2_DoctorKeepsLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = os.Getenv })
	measuring(t)
	logs := ownLogs(t)
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	g("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "Makefile"), []byte("check:\n\t@echo fine\n"), 0o644)
	g("add", "Makefile")
	g("commit", "-q", "-m", "check")
	do := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := run(append([]string{"doctor", "--verify", repo}, args...), strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	decode := func(out string) verifyReport {
		t.Helper()
		var rep verifyReport
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatalf("doctor --verify --json: %v: %q", err, out)
		}
		return rep
	}

	const red = `printf "%s\n" "$TMPDIR"; seq 1 30000; exit 3`
	code, out, errs := do("--line", red, "--json")
	rep := decode(out)
	if code != exitRed || rep.OK || rep.Exit != 3 || rep.Last != "30000" || rep.Log == "" {
		t.Fatalf("a red line = %d %+v %q; want exit 1 with its log", code, rep, errs)
	}
	if fi, err := os.Lstat(rep.Log); err != nil || filepath.Dir(rep.Log) != logs || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Errorf("the log %s: %v, %v; want a 0600 file in %s", rep.Log, fi, err, logs)
	}
	b, err := os.ReadFile(rep.Log)
	tmpdir, rest, _ := strings.Cut(string(b), "\n")
	if err != nil || !strings.HasPrefix(tmpdir, "/") || rest != seqOut(30000) {
		t.Errorf("the log holds %d bytes (%v), first line %q: want the line's TMPDIR, then seq 1 30000's %d", len(b), err, tmpdir, len(seqOut(30000)))
	}
	if _, err := os.Lstat(tmpdir); !os.IsNotExist(err) {
		t.Errorf("the line's TMPDIR %s outlived the command (%v)", tmpdir, err)
	}
	if left, _ := filepath.Glob(filepath.Join(effects.StateDir(getenv), "gate", "fresh-*")); len(left) > 0 {
		t.Errorf("the clone outlived the verify: %v", left)
	}
	code, out, _ = do("--line", red)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if code != exitRed || len(lines) != 4 || !strings.HasPrefix(lines[3], "log: "+logs+string(filepath.Separator)) {
		t.Errorf("the red text = %d %q; want a fourth line log: <path>", code, out)
	} else if _, err := os.Stat(strings.TrimPrefix(lines[3], "log: ")); err != nil {
		t.Errorf("the text's log: %v", err)
	}
	kept := len(logsIn(t, logs))

	if code, out, _ = do("--line", "seq 1 10", "--json"); code != exitOK || decode(out).Log != "" {
		t.Errorf("a green line = %d %q; want exit 0 and no log", code, out)
	}
	if code, out, _ = do("--json"); code != exitOK || decode(out).Line != "make check" || decode(out).Log != "" {
		t.Errorf("the auto probe and a green check = %d %q; want make check, exit 0, no log", code, out)
	}
	if n := len(logsIn(t, logs)); n != kept {
		t.Errorf("a green line or the probe kept a log: %d files, %d before", n, kept)
	}

	const stopped = "echo 'bounded: stop: wall' >&2; exit 2"
	code, _, errs = do("--line", stopped)
	errs = strings.TrimSuffix(errs, "\n")
	i := strings.LastIndex(errs, "; log: ")
	if code != exitUsage || !strings.HasPrefix(errs, "relay doctor --verify: ") || i < 0 || !strings.HasPrefix(errs[i:], "; log: "+logs+string(filepath.Separator)) {
		t.Errorf("a bounded stop = %d %q; want exit 2, its error ending ; log: <path>", code, errs)
	} else if b, err := os.ReadFile(errs[i+len("; log: "):]); err != nil || string(b) != "bounded: stop: wall\n" {
		t.Errorf("the stopped line's log: %q, %v", b, err)
	}
	code, out, _ = do("--line", stopped, "--json")
	if rep = decode(out); code != exitUsage || rep.Error == "" || rep.Log == "" || !strings.HasSuffix(rep.Error, "; log: "+rep.Log) {
		t.Errorf("a bounded stop --json = %d %+v; want error and log", code, rep)
	}
}

// TestAR31_2_PrintGateNamesLog is AR31 act 2: `relay gate`'s text ends a
// check's line `; log: <path>` when it kept one, and a gate that stopped
// with an error prints `  log: <path>` for each check that kept one.
func TestAR31_2_PrintGateNamesLog(t *testing.T) {
	g := relay.Gate{Gathered: true, ResultSeen: true, ExitOK: true, Lines: []relay.Check{
		{Name: "echo ok", Expect: "reads `ok`", BranchLast: "ok", Status: relay.CheckGreen},
		{Name: "seq 1 30000; exit 1", Expect: "exits 0", BranchExit: 1, BranchLast: "30000", Status: relay.CheckRed, BranchLog: "/x/a1-line2.log"},
	}, Verify: &relay.Check{Name: "make check", Expect: "exit 0", BranchExit: 2, BranchLast: "boom", Status: relay.CheckRed, BranchLog: "/x/a1-verify.log"}}
	var b bytes.Buffer
	printGate(&b, gateReport{Gate: g, Row: "a1", Result: relay.RedResult})
	for _, want := range []string{
		"  1. green: exit 0, last \"ok\"; want reads `ok`\n",
		"  2. red: exit 1, last \"30000\"; want exits 0; log: /x/a1-line2.log\n",
		"  verify make check: exit 2, last \"boom\"; red; log: /x/a1-verify.log\n",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no %q in:\n%s", want, b.String())
		}
	}
	b.Reset()
	stopped := relay.Gate{Lines: []relay.Check{{Name: "seq 1 30000; exit 1", Status: relay.CheckRed, BranchLog: "/x/a1-line1.log"}}}
	printGate(&b, gateReport{Gate: stopped, Row: "a1", Result: relay.Unrun, Error: "ls-remote: gone"})
	if want := "gate a1: unrun: ls-remote: gone\n  log: /x/a1-line1.log\n"; b.String() != want {
		t.Errorf("a stopped gate's text %q, want %q", b.String(), want)
	}
}
