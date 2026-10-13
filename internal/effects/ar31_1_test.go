package effects

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ownedDir fails t unless dir is a dir (never a link), the account's, mode
// 0700.
func ownedDir(t *testing.T, what, dir string) {
	t.Helper()
	fi, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("%s %s: %v", what, dir, err)
	}
	uid, ok := ownerOf(fi)
	if !fi.IsDir() || !ok || int(uid) != os.Getuid() || fi.Mode().Perm() != 0o700 {
		t.Errorf("%s %s: %s, uid %d; want a dir of the account's, 0700", what, dir, fi.Mode(), uid)
	}
}

// gone fails t unless each path is absent.
func gone(t *testing.T, what string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s: %s is still there (%v)", what, p, err)
		}
	}
}

// TestAR31_1_TmpBudget is AR31 act 1, the owner's acceptance 1: under a
// 200-byte HOME (the state root under it) Pin makes its temp dir in the
// resolved TmpBase, and a scratched line's TMPDIR is its scratch's t/,
// at most TmpMax bytes, resolved as it is, 0700 and the account's, while
// its scratch home is over 200. The budget: tmpWorst is the root plus 41
// and at most TmpMax, and TmpMax, a slash, TmpRoom bytes and the NUL are
// SunPath; under the line's TMPDIR a socket 40 bytes past it binds, and a
// dir padded to TmpMax bytes takes a slash and 40 bytes, never 41. The
// dirs go with the scratch and at Release; a pin whose temp dir would
// give a TMPDIR over TmpMax makes no scratch and leaves nothing in it.
func TestAR31_1_TmpBudget(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(base) >= 199 {
		t.Fatalf("the test's temp dir %s is %d bytes: no room for a 200-byte HOME", base, len(base))
	}
	home := filepath.Join(base, strings.Repeat("h", 200-len(base)-1))
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	old, _ := Current()
	tools := PinTools
	var p Pinned
	t.Cleanup(func() { p.Release(); Use(old); PinTools = tools; identity.once = sync.Once{} })
	PinTools = []string{"go", "gofmt"}
	if p, err = Pin(); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(TmpBase)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p.Tmp) != root || !regexp.MustCompile(`^relay-`+strconv.Itoa(os.Getpid())+`-[0-9]{1,10}$`).MatchString(filepath.Base(p.Tmp)) {
		t.Fatalf("Pinned.Tmp %s: want relay-%d-<n> in %s", p.Tmp, os.Getpid(), root)
	}
	ownedDir(t, "the pin's temp dir", p.Tmp)

	sh, remove, err := Scratched(OSShell{})
	if err != nil {
		t.Fatal(err)
	}
	o := sh.(OSShell)
	exit, tmpdir, err := sh.Run(base, `printf %s "$TMPDIR"`, time.Minute)
	if err != nil || exit != 0 || tmpdir != filepath.Join(o.Tmp, "t") {
		t.Fatalf("the line's TMPDIR %q (exit %d, %v), want its scratch's %s/t", tmpdir, exit, err, o.Tmp)
	}
	if filepath.Dir(o.Tmp) != p.Tmp {
		t.Errorf("the scratch's temp dir %s is not in the pin's %s", o.Tmp, p.Tmp)
	}
	if len(tmpdir) > TmpMax {
		t.Errorf("the line's TMPDIR %s is %d bytes, over %d", tmpdir, len(tmpdir), TmpMax)
	}
	if r, err := filepath.EvalSymlinks(tmpdir); err != nil || r != tmpdir {
		t.Errorf("the line's TMPDIR %s resolves to %s (%v): a socket's length counts the path as given", tmpdir, r, err)
	}
	ownedDir(t, "the scratch's temp dir", o.Tmp)
	ownedDir(t, "the line's TMPDIR", tmpdir)
	if len(o.Scratch) <= 200 {
		t.Errorf("the scratch home %s is %d bytes: not under the 200-byte HOME", o.Scratch, len(o.Scratch))
	}
	t.Logf("TMPDIR %s: %d bytes, under a HOME of %d bytes and a scratch home of %d", tmpdir, len(tmpdir), len(home), len(o.Scratch))

	if w := tmpWorst(root); w != len(root)+41 || w > TmpMax {
		t.Errorf("tmpWorst(%s) = %d: want the root plus 41, at most %d", root, w, TmpMax)
	}
	if TmpMax+1+TmpRoom+1 != SunPath {
		t.Errorf("TmpMax %d, a slash, TmpRoom %d and the NUL are not SunPath %d", TmpMax, TmpRoom, SunPath)
	}
	bind := func(sock string) error {
		l, err := net.Listen("unix", sock)
		if err == nil {
			l.Close()
		}
		return err
	}
	if err := bind(filepath.Join(tmpdir, strings.Repeat("s", TmpRoom))); err != nil {
		t.Errorf("a socket %d bytes past the line's TMPDIR: %v", TmpRoom, err)
	}
	if n := TmpMax - len(tmpdir) - 1; n > 0 {
		pad := filepath.Join(tmpdir, strings.Repeat("p", n))
		if err := os.Mkdir(pad, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := bind(filepath.Join(pad, strings.Repeat("s", TmpRoom))); err != nil {
			t.Errorf("a %d-byte dir, a slash and %d bytes: %v, want it bound", len(pad), TmpRoom, err)
		}
		if err := bind(filepath.Join(pad, strings.Repeat("s", TmpRoom+1))); err == nil {
			t.Errorf("a %d-byte dir, a slash and %d bytes bound: want it refused (%d bytes with the NUL)", len(pad), TmpRoom+1, SunPath+1)
		}
	} else {
		t.Logf("the line's TMPDIR is %d bytes: no room to pad a dir to %d", len(tmpdir), TmpMax)
	}

	long := filepath.Join(base, strings.Repeat("l", TmpMax))
	if err := os.Mkdir(long, 0o700); err != nil {
		t.Fatal(err)
	}
	q := p
	q.Tmp = long
	Use(q)
	if h, tmp, err := newScratch(); err == nil || !strings.Contains(err.Error(), "over 62") || h != "" || tmp != "" {
		t.Errorf("a scratch in a %d-byte temp dir: %q %q, %v; want over 62", len(long), h, tmp, err)
	}
	Use(p)
	if left, _ := os.ReadDir(long); len(left) > 0 {
		t.Errorf("a refused scratch left %d entries in %s", len(left), long)
	}

	remove()
	gone(t, "the scratch's remover", o.Tmp, o.Scratch)
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	gone(t, "Release", p.Tmp)
}

// TestAR31_1_TmpOnePerRun is AR31 act 1 under the package pin: two
// scratches get two temp dirs in one Pinned.Tmp, and what a line of the
// first writes by "$TMPDIR/x" or by "${TMPDIR}x" (which lands in the
// scratch's own dir) is absent from the second's line; each goes at its
// own remover. An unscratched line's goes when Run returns. A Scratch
// without its Tmp runs nothing, and a pin whose Tmp is a link, a 0755 dir
// or gone makes no scratch. A second PinAt gets its own Tmp, and its
// Release leaves the first's.
func TestAR31_1_TmpOnePerRun(t *testing.T) {
	p, ok := Current()
	if !ok {
		t.Fatal("no pin")
	}
	tools := PinTools
	t.Cleanup(func() { Use(p); PinTools = tools })
	dir := t.TempDir()
	sh1, rm1, err := Scratched(OSShell{})
	if err != nil {
		t.Fatal(err)
	}
	sh2, rm2, err := Scratched(OSShell{})
	if err != nil {
		t.Fatal(err)
	}
	o1, o2 := sh1.(OSShell), sh2.(OSShell)
	if filepath.Dir(o1.Tmp) != p.Tmp || filepath.Dir(o2.Tmp) != p.Tmp || o1.Tmp == o2.Tmp {
		t.Fatalf("two scratches' temp dirs %s and %s: want two in %s", o1.Tmp, o2.Tmp, p.Tmp)
	}
	if exit, _, err := sh1.Run(dir, `touch "$TMPDIR/planted" "${TMPDIR}planted"`, time.Minute); err != nil || exit != 0 {
		t.Fatalf("the first scratch's line: exit %d, %v", exit, err)
	}
	for _, f := range []string{filepath.Join(o1.Tmp, "t", "planted"), filepath.Join(o1.Tmp, "tplanted")} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("the first line's plant is not in its scratch's temp dir: %v", err)
		}
	}
	exit, last, err := sh2.Run(dir, `n=0; for f in "$TMPDIR/planted" "${TMPDIR}planted"; do test -e "$f" && n=$((n+1)); done; echo $n`, time.Minute)
	if err != nil || exit != 0 || last != "0" {
		t.Errorf("the second scratch's line found %q of the first's plants (exit %d, %v), want 0", last, exit, err)
	}
	rm1()
	gone(t, "the first scratch's remover", o1.Tmp, o1.Scratch)
	for _, d := range []string{o2.Tmp, o2.Scratch} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("the first scratch's remover took the second's %s: %v", d, err)
		}
	}
	rm2()
	gone(t, "the second scratch's remover", o2.Tmp, o2.Scratch)

	exit, tmpdir, err := OSShell{}.Run(dir, `printf %s "$TMPDIR"`, time.Minute)
	if err != nil || exit != 0 || filepath.Base(tmpdir) != "t" || filepath.Dir(filepath.Dir(tmpdir)) != p.Tmp {
		t.Errorf("an unscratched line's TMPDIR %q (exit %d, %v), want <m>/t in %s", tmpdir, exit, err, p.Tmp)
	} else {
		gone(t, "an unscratched line's Run", filepath.Dir(tmpdir))
	}

	ran := filepath.Join(dir, "ran")
	if _, _, err := (OSShell{Scratch: t.TempDir()}).Run(dir, "touch "+ran, time.Minute); err == nil || !strings.Contains(err.Error(), "without its temp dir") {
		t.Errorf("a Scratch without its Tmp: %v, want refused", err)
	}
	gone(t, "a Scratch without its Tmp", ran)

	target := t.TempDir()
	if err := os.Chmod(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(t.TempDir(), "loose")
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	// Each why is the Lstat's, before any make: these dirs are long, so a
	// scratch made in one would be refused for its length instead.
	for _, c := range []struct{ tmp, why string }{
		{link, "not a dir"},
		{loose, "mode 0755, not 0700"},
		{filepath.Join(dir, "missing"), "no such file or directory"},
	} {
		q := p
		q.Tmp = c.tmp
		Use(q)
		_, rm, err := Scratched(OSShell{})
		Use(p)
		if err == nil || !strings.Contains(err.Error(), "the pin's temp dir "+c.tmp+": ") || !strings.Contains(err.Error(), c.why) {
			rm()
			t.Errorf("a pin whose Tmp is %s: Scratched %v, want refused naming the pin's temp dir and %q", c.tmp, err, c.why)
		}
	}
	for _, d := range []string{target, loose} {
		if left, _ := os.ReadDir(d); len(left) > 0 {
			t.Errorf("a refused scratch left %d entries in %s", len(left), d)
		}
	}

	PinTools = nil
	q, err := PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if q.Tmp == p.Tmp || filepath.Dir(q.Tmp) != filepath.Dir(p.Tmp) {
		t.Errorf("a second PinAt's Tmp %s: want another dir beside %s", q.Tmp, p.Tmp)
	}
	if err := q.Release(); err != nil {
		t.Fatal(err)
	}
	gone(t, "the second pin's Release", q.Tmp)
	ownedDir(t, "the first pin's temp dir after the second's Release", p.Tmp)
	Use(p)
	if c, _ := Current(); c.Tmp != p.Tmp {
		t.Errorf("Use restored a pin whose Tmp is %s, want %s", c.Tmp, p.Tmp)
	}
}

// TestAR31_1_TmpRootRefused is AR31 act 1: a TmpBase that is missing, a
// file, a dir the account owns, one the account cannot write (/usr), or a
// root-owned dir whose worst case passes TmpMax, is refused unpinned-tool
// naming the temp root and why, after the tools and before anything is
// made: no <state>/home, no new temp dir of this process's in the real
// root, and the process's pin kept. Without a root-owned dir of 29 bytes
// the length rule is proved by tmpWorst alone.
func TestAR31_1_TmpRootRefused(t *testing.T) {
	old, _ := Current()
	tmpBase := TmpBase
	t.Cleanup(func() { TmpBase = tmpBase; Use(old) })
	real, err := filepath.EvalSymlinks(tmpBase)
	if err != nil {
		t.Fatal(err)
	}
	ours := func() []string {
		m, _ := filepath.Glob(filepath.Join(real, "relay-"+strconv.Itoa(os.Getpid())+"-*"))
		return m
	}
	before := ours()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	type refusal struct{ base, why string }
	cases := []refusal{
		{filepath.Join(dir, "missing"), "no such file or directory"},
		{file, "not a dir"},
	}
	if os.Getuid() != 0 {
		cases = append(cases, refusal{dir, "not root-owned (uid " + strconv.Itoa(os.Getuid()) + ")"},
			refusal{"/usr", "the account cannot write or enter it"})
	}
	const long = "/System/Library/LaunchDaemons" // 29 bytes: 70 at worst
	rootOwned := false
	if fi, err := os.Lstat(long); err == nil {
		uid, ok := ownerOf(fi)
		rootOwned = fi.IsDir() && ok && uid == 0 && fi.Mode().Perm()&0o022 == 0 && locked(filepath.Dir(long))
	}
	if rootOwned {
		cases = append(cases, refusal{long, "a line's TMPDIR could reach 70 bytes, over 62"})
	} else {
		if tmpWorst(strings.Repeat("x", 22)) != 63 || tmpWorst(strings.Repeat("x", 21)) != 62 {
			t.Errorf("tmpWorst of a 22-byte and a 21-byte root: %d and %d, want 63 and 62", tmpWorst(strings.Repeat("x", 22)), tmpWorst(strings.Repeat("x", 21)))
		}
		t.Logf("no root-owned %s with a locked parent: the length rule by tmpWorst alone", long)
	}
	for _, c := range cases {
		TmpBase = c.base
		state := t.TempDir()
		_, err := PinAt(state, t.TempDir())
		if !errors.Is(err, ErrUnpinnedTool) || !strings.Contains(err.Error(), "the temp root") || !strings.Contains(err.Error(), c.why) {
			t.Errorf("TmpBase %s: PinAt %v; want unpinned-tool naming the temp root and %q", c.base, err, c.why)
		}
		gone(t, "a refused temp root "+c.base, filepath.Join(state, "home"))
		if got := ours(); !slices.Equal(got, before) {
			t.Errorf("a refused temp root %s: this process's temp dirs %q, before %q", c.base, got, before)
		}
		if p, _ := Current(); p.Home != old.Home || p.Tmp != old.Tmp {
			t.Errorf("a refused temp root %s replaced the pin: %q", c.base, p.Home)
		}
	}
}

// TestAR31_1_TmpSwept is AR31 act 1: sweepTmp removes a dead pid's
// relay-<pid>-<n>, 0700 and the account's, with what it holds (a link in
// it is removed, never followed), and keeps a link of that name (its
// victim intact), a 0755 dir, a live pid's, a name of another shape, and
// this process's own. In the real root one plant, a dead pid's 0700 dir,
// is swept by the next PinAt, and the package pin's Tmp survives it.
func TestAR31_1_TmpSwept(t *testing.T) {
	dead := exec.Command("/usr/bin/true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	d := strconv.Itoa(dead.Process.Pid)
	// The victim is 0700 and the account's, as a swept dir is: only the
	// Lstat keeps relay-<dead>-1, never the mode rule.
	victim := t.TempDir()
	if err := os.Chmod(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(victim, "keep")
	if err := os.WriteFile(keep, []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := t.TempDir()
	mk := func(name string, mode os.FileMode) string {
		t.Helper()
		p := filepath.Join(r, name)
		if err := os.Mkdir(p, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := os.Symlink(victim, filepath.Join(r, "relay-"+d+"-1")); err != nil {
		t.Fatal(err)
	}
	mk("relay-"+d+"-2", 0o755)
	mk("relay-"+strconv.Itoa(os.Getppid())+"-3", 0o700)
	mk("relay-x-1", 0o700)
	mk("relay-"+strconv.Itoa(os.Getpid())+"-4", 0o700)
	five := mk("relay-"+d+"-5", 0o700)
	if err := os.WriteFile(filepath.Join(five, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(five, "link")); err != nil {
		t.Fatal(err)
	}
	sweepTmp(r)
	var names []string
	entries, _ := os.ReadDir(r)
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"relay-" + d + "-1", "relay-" + d + "-2", "relay-" + strconv.Itoa(os.Getppid()) + "-3", "relay-" + strconv.Itoa(os.Getpid()) + "-4", "relay-x-1"}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Errorf("after sweepTmp: %q, want %q (only the dead pid's 0700 dir gone)", names, want)
	}
	if b, err := os.ReadFile(keep); err != nil || string(b) != "k" {
		t.Errorf("the sweep went through a link: %q, %v", b, err)
	}

	root, err := filepath.EvalSymlinks(TmpBase)
	if err != nil {
		t.Fatal(err)
	}
	plant, err := os.MkdirTemp(root, "relay-"+d+"-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(plant) })
	if err := os.Chmod(plant, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(plant, "link")); err != nil {
		t.Fatal(err)
	}
	old, _ := Current()
	tools := PinTools
	var q Pinned
	t.Cleanup(func() { q.Release(); Use(old); PinTools = tools })
	PinTools = nil
	if q, err = PinAt(t.TempDir(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	gone(t, "a dead pid's temp dir in the real root at the next PinAt", plant)
	if b, err := os.ReadFile(keep); err != nil || string(b) != "k" {
		t.Errorf("the pin's sweep went through a link: %q, %v", b, err)
	}
	ownedDir(t, "the package pin's temp dir (this pid) after the sweep", old.Tmp)
}
