package effects

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ownLogs puts a copy of the package pin whose Logs is a dir of the
// test's own, restored at cleanup, and names it: the package pin's holds
// the red logs of earlier tests (AR31 act 2, D18).
func ownLogs(t *testing.T) string {
	t.Helper()
	old, _ := Current()
	p := old
	p.Logs = filepath.Join(t.TempDir(), "logs")
	Use(p)
	t.Cleanup(func() { Use(old) })
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

// logNames are the names in dir.
func logNames(t *testing.T, dir string) []string {
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

// TestAR31_2_LogWhole is AR31 act 2: RunLog keeps a line's whole output,
// past OutputCap, in a new 0600 file named by the time and its label in
// a 0700 Logs, and returns Run's exit and last line; a line that writes
// only stderr leaves its stderr. KeepLog removes or keeps it; RunLogged
// over a fake runs with no log, and a line that cannot start leaves none.
func TestAR31_2_LogWhole(t *testing.T) {
	logs := ownLogs(t)
	dir := t.TempDir()
	const line = "seq 1 30000; exit 3"
	exit, last, log, err := OSShell{}.RunLog(dir, line, time.Minute, "t")
	if err != nil || exit != 3 || last != "30000" {
		t.Fatalf("RunLog = %d %q %q %v; want exit 3, last 30000", exit, last, log, err)
	}
	if rexit, rlast, rerr := (OSShell{}).Run(dir, line, time.Minute); rerr != nil || rexit != exit || rlast != last {
		t.Errorf("Run = %d %q %v; RunLog gave %d %q", rexit, rlast, rerr, exit, last)
	}
	if filepath.Dir(log) != logs || !regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-t-[0-9]+\.log$`).MatchString(filepath.Base(log)) {
		t.Errorf("the log %s: want <stamp>-t-<n>.log in %s", log, logs)
	}
	ownedDir(t, "the logs dir", logs)
	if fi, err := os.Lstat(log); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Errorf("the log %s: %v, %v; want a 0600 file", log, fi.Mode(), err)
	}
	want := seqOut(30000)
	if len(want) <= OutputCap {
		t.Fatalf("seq 1 30000 is %d bytes, not past OutputCap", len(want))
	}
	if b, err := os.ReadFile(log); err != nil || string(b) != want {
		t.Errorf("the log holds %d bytes (%v), want seq 1 30000's %d", len(b), err, len(want))
	}

	_, _, errLog, err := OSShell{}.RunLog(dir, "echo only-err >&2; exit 4", time.Minute, "e")
	if b, rerr := os.ReadFile(errLog); err != nil || rerr != nil || string(b) != "only-err\n" {
		t.Errorf("a stderr-only line's log %s: %q (%v, %v), want only-err", errLog, b, err, rerr)
	}
	if got := KeepLog(errLog, false); got != "" {
		t.Errorf("KeepLog(%s, false) = %q, want \"\"", errLog, got)
	}
	gone(t, "KeepLog(false)", errLog)
	if got := KeepLog(log, true); got != log {
		t.Errorf("KeepLog(%s, true) = %q", log, got)
	}
	if got := KeepLog("", true); got != "" {
		t.Errorf("KeepLog(\"\", true) = %q", got)
	}

	fake := &FakeShell{Results: map[string]ShellResult{line: {Exit: 3, Last: "30000"}}}
	if exit, last, log, err := RunLogged(fake, dir, line, time.Minute, "f"); exit != 3 || last != "30000" || log != "" || err != nil {
		t.Errorf("RunLogged over a fake = %d %q %q %v; want its result and no log", exit, last, log, err)
	}
	if _, _, log, err := RunLogged(OSShell{}, filepath.Join(dir, "missing"), "echo x", time.Minute, "m"); err == nil || log != "" {
		t.Errorf("RunLogged in a missing dir = %q, %v; want an error and no log", log, err)
	}
	if names := logNames(t, logs); len(names) != 1 || names[0] != filepath.Base(log) {
		t.Errorf("the logs dir holds %q, want only %s", names, filepath.Base(log))
	}
}

// TestAR31_2_LogCutAndSwept is AR31 act 2: past LogCap a log keeps its
// first LogCap bytes, the cut line naming the bytes not kept, and both
// tails. A Logs that is a link or a file makes no log and writes nothing
// through it, and the line runs as Run runs it; a 0755 Logs is set back to
// 0700. PinAt removes a .log older than LogKeep and keeps a newer one, a
// link named .log (its target intact), a dir named .log and any other name.
func TestAR31_2_LogCutAndSwept(t *testing.T) {
	logs := ownLogs(t)
	logCap := LogCap
	t.Cleanup(func() { LogCap = logCap })
	LogCap = 1024
	dir := t.TempDir()
	a := strings.Repeat("a", 10240)
	_, _, log, err := OSShell{}.RunLog(dir, `head -c 10240 /dev/zero | tr '\0' a`, time.Minute, "cut")
	if err != nil {
		t.Fatal(err)
	}
	want := a[:1024] + "\nrelay: log cut at 1024 bytes; 9216 more bytes read, not kept; the last 65536 bytes of stdout and of stderr follow\n" +
		"relay: stdout's tail\n" + a + "\nrelay: stderr's tail\n"
	if b, err := os.ReadFile(log); err != nil || string(b) != want {
		t.Errorf("the cut log %s: %d bytes (%v); want %d:\n%.300q", log, len(b), err, len(want), b)
	}
	LogCap = logCap

	p, _ := Current()
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "logs")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "logs")
	if err := os.WriteFile(file, []byte("f"), 0o600); err != nil {
		t.Fatal(err)
	}
	const line = "echo one; echo two >&2; exit 5"
	rexit, rlast, rerr := OSShell{}.Run(dir, line, time.Minute)
	if rerr != nil || rexit != 5 || rlast != "one" {
		t.Fatalf("Run = %d %q %v", rexit, rlast, rerr)
	}
	for _, bad := range []string{link, file} {
		q := p
		q.Logs = bad
		Use(q)
		exit, last, log, err := OSShell{}.RunLog(dir, line, time.Minute, "bad")
		Use(p)
		if err != nil || exit != rexit || last != rlast || log != "" {
			t.Errorf("Logs %s: RunLog = %d %q %q %v; want Run's %d %q and no log", bad, exit, last, log, err, rexit, rlast)
		}
	}
	if names := logNames(t, target); len(names) > 0 {
		t.Errorf("a log was written through the link: %q", names)
	}
	if b, _ := os.ReadFile(file); string(b) != "f" {
		t.Errorf("the file named as Logs was changed: %q", b)
	}

	loose := filepath.Join(t.TempDir(), "logs")
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	q := p
	q.Logs = loose
	Use(q)
	_, _, log, err = OSShell{}.RunLog(dir, line, time.Minute, "loose")
	Use(p)
	if err != nil || filepath.Dir(log) != loose {
		t.Errorf("a 0755 Logs: log %q, %v; want one in %s", log, err, loose)
	}
	ownedDir(t, "a 0755 Logs after a log", loose)

	state := t.TempDir()
	swept := filepath.Join(state, "logs")
	if err := os.Mkdir(swept, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old, fresh := now.Add(-15*24*time.Hour), now.Add(-13*24*time.Hour)
	put := func(name string, at time.Time) string {
		t.Helper()
		f := filepath.Join(swept, name)
		if err := os.WriteFile(f, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, at, at); err != nil {
			t.Fatal(err)
		}
		return f
	}
	put("old.log", old)
	put("new.log", fresh)
	put("notes.txt", old)
	// The link's target is old too, so only the Lstat keeps the link; the
	// dir is empty and old, so only the regular-file rule keeps it.
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(victim, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(swept, "x.log")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(swept, "d.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(swept, "d.log"), old, old); err != nil {
		t.Fatal(err)
	}
	tools := PinTools
	var pinned Pinned
	t.Cleanup(func() { pinned.Release(); Use(p); PinTools = tools })
	PinTools = nil
	if pinned, err = PinAt(state, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if pinned.Logs != swept {
		t.Errorf("PinAt's Logs %s, want %s", pinned.Logs, swept)
	}
	if got, want := logNames(t, swept), []string{"d.log", "new.log", "notes.txt", "x.log"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("after PinAt's sweep: %q, want %q (only old.log gone)", got, want)
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != "v" {
		t.Errorf("the sweep went through the link: %q, %v", b, err)
	}
	if names := logNames(t, logs); len(names) != 1 {
		t.Errorf("the test's own logs dir holds %q, want the one cut log", names)
	}
}
