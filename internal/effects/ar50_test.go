package effects

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// killLeft kills, at t's cleanup, the process whose pid a fake wrote to
// pidFile, when it still runs and is a sleep (a pid is never killed on
// its number alone); a fake that wrote none fails the test.
func killLeft(t *testing.T, pidFile string) {
	t.Helper()
	t.Cleanup(func() {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			t.Errorf("the fake left no pid: %v", err)
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || pid <= 1 {
			t.Errorf("the fake's pid file holds %q", b)
			return
		}
		out, _ := exec.Command("/bin/ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
		if strings.HasPrefix(strings.TrimSpace(string(out)), "sleep ") {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	})
}

// TestAR50_1_AsksBounded is AR50 act 1: each ask of a pinned tool ends on
// time whoever holds its output. On a real pin the timeout is 30 s and
// the wait over 0, at most 5 s. With the asks' timeout and wait both 1 s:
// a fake python3 whose prefix ask waits on a `sleep 20` it started, one
// whose prefix ask starts the sleep, answers and exits 0, and one that
// execs `sleep 300` on the ask each give a pin with no error and a note
// (the ask is not honored); one whose roots answer starts the sleep,
// answers and exits 0 gives ErrUnpinnedTool, its roots unread, and one
// whose roots answer execs the sleep gives it too. Each of those two
// errors says the ask timed out and names its bound: the wait after the
// exit, or the timeout. Each PinAt returns in under 10 s, and each case
// kills the sleep it left.
func TestAR50_1_AsksBounded(t *testing.T) {
	timeout, wait := askTimeout, askWait
	if timeout != 30*time.Second || wait <= 0 || wait > 5*time.Second {
		t.Errorf("on a real pin askTimeout = %s and askWait = %s, want 30s and a wait over 0, at most 5s", timeout, wait)
	}
	t.Cleanup(func() { askTimeout, askWait = timeout, wait })
	askTimeout, askWait = time.Second, time.Second
	root := tree(t, map[string]string{"lib/python3.12/os.py": "# os\n"})
	onAsk := func(do string) string {
		return `case "$2" in *pycache_prefix*) ` + do + `;; *) echo ` + root + `;; esac`
	}
	for _, c := range []struct {
		name string
		body func(pid string) string
		says string // what the pin's error says; "" when the pin succeeds
	}{
		{"the prefix ask waits on a child that holds its output", func(pid string) string {
			return onAsk(`sleep 20 & echo $! > ` + pid + `; wait`)
		}, ""},
		{"the prefix ask leaves a child holding its output, answers and exits 0", func(pid string) string {
			return onAsk(`sleep 20 & echo $! > ` + pid + `; echo "$PYTHONPYCACHEPREFIX"; exit 0`)
		}, ""},
		{"the roots answer leaves a child holding its output, answers and exits 0", func(pid string) string {
			return `case "$2" in *pycache_prefix*) echo "$PYTHONPYCACHEPREFIX";; *) sleep 20 & echo $! > ` + pid + `; echo ` + root + `; exit 0;; esac`
		}, "timed out, its output still held 1s after its exit"},
		{"the roots answer execs a sleep", func(pid string) string {
			return `case "$2" in *pycache_prefix*) echo "$PYTHONPYCACHEPREFIX";; *) echo $$ > ` + pid + `; exec sleep 300;; esac`
		}, "timed out after 1s"},
		{"the prefix ask execs a sleep", func(pid string) string {
			return onAsk(`echo $$ > ` + pid + `; exec sleep 300`)
		}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			pid := filepath.Join(t.TempDir(), "pid")
			fakeTool(t, "python3", c.body(pid))
			killLeft(t, pid)
			start := time.Now()
			p, err := PinAt(t.TempDir(), t.TempDir())
			took := time.Since(start)
			if err == nil {
				t.Cleanup(func() { p.Release() })
			}
			if took >= 10*time.Second {
				t.Errorf("PinAt took %s, want under 10 s: the ask's child held the pin", took.Round(time.Millisecond))
			}
			if c.says != "" {
				if !errors.Is(err, ErrUnpinnedTool) {
					t.Fatalf("PinAt = %v, want ErrUnpinnedTool: the roots of an ask that ended at the wait are unread", err)
				}
				if !strings.Contains(err.Error(), c.says) {
					t.Errorf("PinAt = %v, want an error that says %q", err, c.says)
				}
				t.Logf("PinAt = %v", err)
				return
			}
			if err != nil {
				t.Fatalf("PinAt = %v, want a pin: an ask that does not end is not honored, and no pin error", err)
			}
			if note := PycacheNote(p); note == "" {
				t.Errorf("PycacheNote is empty, want the note: the ask did not end with the prefix in time")
			}
		})
	}
}

// TestAR50_2_LongTmpdir is AR50 act 2: the scratched check of
// TestAR49_1_ScratchPrefix (scratchedPrefix) reads the file it means
// whatever the temp dir. TMPDIR is a dir of this test's own, 160 bytes
// long, set before any temp dir is made, so the root python mirrors under
// the prefix is longer than a line's last-line cap (LastCap) by itself;
// the real python3 is pinned as TestAR49_1_ScratchPrefix pins it. Skipped
// without a python3 the pin reaches.
func TestAR50_2_LongTmpdir(t *testing.T) {
	const long = 160
	base, err := os.MkdirTemp("", "ar50-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	tmp := filepath.Join(base, strings.Repeat("t", max(1, long-len(base)-1)))
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { PinTools = tools; Use(old) })
	PinTools = []string{"python3"}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, tmp+"/") || len(tmp) < long {
		t.Fatalf("the temp dir %s is not under the %d-byte TMPDIR %s", dir, long, tmp)
	}
	p, err := PinAt(filepath.Join(dir, "state"), filepath.Join(dir, "account"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()
	if _, ok := p.Tools["python3"]; !ok {
		t.Skip("no python3 the pin reaches")
	}
	scratchedPrefix(t, dir)
}

// TestAR50_3_PredicateEdges is AR50 act 3: two edges of AR49's predicate
// (pyOwn) that no test held, under the honoring fake, each case added
// after a fresh pin and named python3 by Moved and, after a second fresh
// pin, by MovedStrict. A root that is itself named __pycache__ (the
// fake's only root) is an entry and so is a file directly in it: a
// cache-tagged .pyc added there is moved, and so is the root when its
// mode alone changes. A cache tag's digits are ASCII
// digits alone: in a pinned __pycache__ below an ordinary root, a name
// whose tag is fullwidth digits, one whose tag is Arabic-Indic digits,
// one whose opt level is a fullwidth digit and one whose tag holds a
// character below '0' (a hyphen) are each an entry, and moved.
// The control: the same names in ASCII digits are not moved there.
func TestAR50_3_PredicateEdges(t *testing.T) {
	moved := func(p Pinned, strict bool) []string {
		if strict {
			return p.MovedStrict()
		}
		return p.Moved()
	}
	t.Run("a tagged file directly in a root named __pycache__", func(t *testing.T) {
		for _, strict := range []bool{false, true} {
			root := filepath.Join(t.TempDir(), "__pycache__")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(root, "os.py"))
			p := pinFake(t, "python3", honoring(root))
			if note := PycacheNote(p); note != "" || len(p.Roots) != 1 || p.Roots[root] != "python3" {
				t.Fatalf("PycacheNote %q, Roots %v: want the prefix honored and %s python3's only root", note, p.Roots, root)
			}
			time.Sleep(10 * time.Millisecond)
			write(t, filepath.Join(root, "x.cpython-312.pyc"))
			if got := moved(p, strict); !slices.Equal(got, []string{"python3"}) {
				t.Errorf("strict %v: moved = %v, want [python3]", strict, got)
			}
		}
	})
	t.Run("a root named __pycache__, its mode changed", func(t *testing.T) {
		for _, strict := range []bool{false, true} {
			root := filepath.Join(t.TempDir(), "__pycache__")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(root, "os.py"))
			p := pinFake(t, "python3", honoring(root))
			if note := PycacheNote(p); note != "" || len(p.Roots) != 1 || p.Roots[root] != "python3" {
				t.Fatalf("PycacheNote %q, Roots %v: want the prefix honored and %s python3's only root", note, p.Roots, root)
			}
			time.Sleep(10 * time.Millisecond)
			if err := os.Chmod(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if got := moved(p, strict); !slices.Equal(got, []string{"python3"}) {
				t.Errorf("strict %v: moved = %v, want [python3]: the root is an entry whatever its name", strict, got)
			}
		}
	})
	// fresh is a fresh pyTree under a fresh pin of the honoring fake.
	fresh := func(t *testing.T) (Pinned, string) {
		root, cache := pyTree(t)
		p := pinFake(t, "python3", honoring(root, root+"/lib/python3.12"))
		if note := PycacheNote(p); note != "" || p.Roots[root] != "python3" {
			t.Fatalf("PycacheNote %q, Roots %v: want the prefix honored and %s bound to python3", note, p.Roots, root)
		}
		time.Sleep(10 * time.Millisecond)
		return p, cache
	}
	t.Run("the control: ASCII digits in the tag and the opt level", func(t *testing.T) {
		for _, strict := range []bool{false, true} {
			p, cache := fresh(t)
			write(t, filepath.Join(cache, "x.cpython-312.pyc"), filepath.Join(cache, "x.cpython-312.opt-2.pyc"))
			if got := moved(p, strict); len(got) != 0 {
				t.Errorf("strict %v: moved = %v, want none", strict, got)
			}
		}
	})
	for _, c := range []struct{ name, file string }{
		{"fullwidth digits in the tag", "x.cpython-３１２.pyc"},
		{"Arabic-Indic digits in the tag", "x.cpython-٣١٢.pyc"},
		{"a fullwidth digit in the opt level", "x.cpython-312.opt-２.pyc"},
		{"a character below '0' in the tag", "x.cpython-3-2.pyc"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, strict := range []bool{false, true} {
				p, cache := fresh(t)
				write(t, filepath.Join(cache, c.file))
				if got := moved(p, strict); !slices.Equal(got, []string{"python3"}) {
					t.Errorf("strict %v: %s: moved = %v, want [python3]", strict, c.file, got)
				}
			}
		})
	}
}
