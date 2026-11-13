package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The DW1R tests (packet design/packets/dw1r-door-writer.md) run
// scripts/install-door with sh over made-up doors in a temp dir. None builds
// or runs a relay binary, and none runs make install.

// dw1Script and dw1Vendored are the copies dw1Copy makes of the two scripts.
var dw1Script, dw1Vendored string

// dw1Copy copies scripts/install-door and the vendored scripts/agentik out of
// the tree (copyIn, AR17D act 4) and points dw1Script and dw1Vendored at the
// copies; each test calls it first.
func dw1Copy(t *testing.T) {
	t.Helper()
	root := copyIn(t, "../..", "scripts/install-door", "scripts/agentik")
	dw1Script = filepath.Join(root, "scripts", "install-door")
	dw1Vendored = filepath.Join(root, "scripts", "agentik")
}

var dw1VersionLine = regexp.MustCompile(`(?m)^VERSION=.*\n`)

// dw1Door writes the vendored door to dir/name with its one VERSION= line
// set to version ("" drops the line) and extra appended, and returns its path.
func dw1Door(t *testing.T, dir, name, version, extra string) string {
	t.Helper()
	b, err := os.ReadFile(dw1Vendored)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(dw1VersionLine.FindAll(b, -1)); n != 1 {
		t.Fatalf("the vendored door has %d VERSION= lines, want 1", n)
	}
	line := ""
	if version != "" {
		line = "VERSION=" + version + "\n"
	}
	b = dw1VersionLine.ReplaceAllLiteral(b, []byte(line))
	b = append(b, extra...)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// dw1Run runs sh scripts/install-door SRC DEST and returns stdout, stderr
// and the exit code.
func dw1Run(t *testing.T, src, dest string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("sh", dw1Script, src, dest)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("sh %s: %v", dw1Script, err)
	}
	return out.String(), errb.String(), code
}

func dw1Read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// dw1Only fails unless dir holds exactly names: no temp file left beside DEST.
func dw1Only(t *testing.T, dir string, names ...string) {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range es {
		got = append(got, e.Name())
	}
	if strings.Join(got, " ") != strings.Join(names, " ") {
		t.Errorf("%s holds %q, want %q", dir, got, names)
	}
}

// dw1Kept runs the script over dest and wants dest's bytes unchanged, the
// given stdout, no stderr and exit 0.
func dw1Kept(t *testing.T, src, dest, stdout string) {
	t.Helper()
	before := dw1Read(t, dest)
	out, errOut, code := dw1Run(t, src, dest)
	if code != 0 || out != stdout || errOut != "" {
		t.Errorf("over %s: exit %d, stdout %q, stderr %q; want 0, %q, none", filepath.Base(dest), code, out, errOut, stdout)
	}
	if !bytes.Equal(dw1Read(t, dest), before) {
		t.Errorf("%s changed", filepath.Base(dest))
	}
}

// dw1Written runs the script over dest and wants dest the source's bytes at
// mode 0755, no output and exit 0.
func dw1Written(t *testing.T, src, dest string) {
	t.Helper()
	out, errOut, code := dw1Run(t, src, dest)
	if code != 0 || out != "" || errOut != "" {
		t.Errorf("over %s: exit %d, stdout %q, stderr %q; want 0 and no output", filepath.Base(dest), code, out, errOut)
	}
	if !bytes.Equal(dw1Read(t, dest), dw1Read(t, src)) {
		t.Errorf("%s is not the source's bytes", filepath.Base(dest))
	}
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o755 {
		t.Errorf("%s mode %v, want a file at 0755", filepath.Base(dest), fi.Mode())
	}
}

// TestDW1_1_NewerKept (R2, R3): a door at 0.2.0, or one at the source's
// 0.1.0 with a line of its own, stays byte for byte, silently.
func TestDW1_1_NewerKept(t *testing.T) {
	dw1Copy(t)
	d := t.TempDir()
	dw1Kept(t, dw1Vendored, dw1Door(t, d, "newer", "0.2.0", ""), "")
	dw1Kept(t, dw1Vendored, dw1Door(t, d, "equal", "0.1.0", "# a line of its own\n"), "")
	dw1Only(t, d, "equal", "newer")
}

// TestDW1_2_Written (R2, R4): no door is written from the source at 0755; a
// door at 0.0.9 becomes the source; the first VERSION= line is the one read.
func TestDW1_2_Written(t *testing.T) {
	dw1Copy(t)
	d := t.TempDir()
	dw1Written(t, dw1Vendored, filepath.Join(d, "none"))
	older := dw1Door(t, d, "older", "0.0.9", "")
	if err := os.Chmod(older, 0o644); err != nil {
		t.Fatal(err)
	}
	dw1Written(t, dw1Vendored, older)
	dw1Written(t, dw1Vendored, dw1Door(t, d, "first", "0.0.1", "VERSION=9.9.9\n"))
	dw1Only(t, d, "first", "none", "older")
}

// TestDW1_3_NumberByNumber (R2): 0.10.0 is above 0.9.0.
func TestDW1_3_NumberByNumber(t *testing.T) {
	dw1Copy(t)
	d := t.TempDir()
	s := t.TempDir()
	dw1Kept(t, dw1Door(t, s, "src9", "0.9.0", ""), dw1Door(t, d, "door10", "0.10.0", ""), "")
	dw1Written(t, dw1Door(t, s, "src10", "0.10.0", ""), dw1Door(t, d, "door9", "0.9.0", ""))
	dw1Kept(t, dw1Door(t, s, "src1", "1.0.0", ""), dw1Door(t, d, "door2", "2.0.0", ""), "")
	dw1Written(t, dw1Door(t, s, "src011", "0.1.1", ""), dw1Door(t, d, "door010", "0.1.0", ""))
	dw1Only(t, d, "door010", "door10", "door2", "door9")
}

// TestDW1_4_LinkLeft (R2, R3): a link at DEST, to an older door or to
// nothing, stays that link, its target untouched, with the link line.
func TestDW1_4_LinkLeft(t *testing.T) {
	dw1Copy(t)
	d := t.TempDir()
	target := dw1Door(t, d, "target", "0.0.1", "")
	before := dw1Read(t, target)
	for _, c := range []struct{ name, to string }{{"link", target}, {"dangling", filepath.Join(d, "nowhere")}} {
		dest := filepath.Join(d, c.name)
		if err := os.Symlink(c.to, dest); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := dw1Run(t, dw1Vendored, dest)
		want := "the door " + dest + " is a link; left alone\n"
		if code != 0 || out != want || errOut != "" {
			t.Errorf("over %s: exit %d, stdout %q, stderr %q; want 0, %q, none", c.name, code, out, errOut, want)
		}
		if to, err := os.Readlink(dest); err != nil || to != c.to {
			t.Errorf("%s reads %q, %v; want the link to %s", c.name, to, err, c.to)
		}
	}
	if !bytes.Equal(dw1Read(t, target), before) {
		t.Error("the link's target changed")
	}
	dw1Only(t, d, "dangling", "link", "target")
}

// TestDW1_5_NoVersion (R2, R3): a door with no VERSION= line, a source with
// none, and a source at a prerelease each leave the door alone, the line
// naming both sides.
func TestDW1_5_NoVersion(t *testing.T) {
	dw1Copy(t)
	d := t.TempDir()
	s := t.TempDir()
	line := func(dest, theirs, ours string) string {
		return "the door " + dest + " has no version to compare (" + theirs + " there, " + ours + " here); left alone\n"
	}
	door := dw1Door(t, d, "bare", "", "")
	dw1Kept(t, dw1Vendored, door, line(door, "none", "0.1.0"))
	door = dw1Door(t, d, "old1", "0.0.9", "")
	dw1Kept(t, dw1Door(t, s, "bare", "", ""), door, line(door, "0.0.9", "none"))
	door = dw1Door(t, d, "old2", "0.1.0", "")
	dw1Kept(t, dw1Door(t, s, "rc", "0.2.0-rc.1", ""), door, line(door, "0.1.0", "0.2.0-rc.1"))
	dw1Only(t, d, "bare", "old1", "old2")
}

// TestDW1_6_SourceMissing (R3, R4): an unreadable source or a failed write
// exits 1 with one stderr line, DEST unchanged, no temp file beside it; two
// arguments or a usage line and exit 2.
func TestDW1_6_SourceMissing(t *testing.T) {
	dw1Copy(t)
	d := t.TempDir()
	dest := dw1Door(t, d, "door", "0.0.9", "")
	before := dw1Read(t, dest)
	fail := func(src, dest, why string) {
		t.Helper()
		out, errOut, code := dw1Run(t, src, dest)
		if code != 1 || out != "" || strings.Count(errOut, "\n") != 1 || !strings.HasSuffix(errOut, "\n") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want 1, none, one line", why, code, out, errOut)
		}
	}
	fail(filepath.Join(d, "missing"), dest, "a missing source")
	fail(d, dest, "a directory for a source")
	if !bytes.Equal(dw1Read(t, dest), before) {
		t.Error("the door changed")
	}
	dw1Only(t, d, "door")

	if os.Geteuid() != 0 {
		if err := os.Chmod(d, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(d, 0o755) })
		fail(dw1Vendored, dest, "a read-only folder")
		if !bytes.Equal(dw1Read(t, dest), before) {
			t.Error("the door changed")
		}
		dw1Only(t, d, "door")
	}

	cmd := exec.Command("sh", dw1Script, dw1Vendored)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 2 || !strings.HasPrefix(string(out), "usage: install-door SRC DEST") {
		t.Errorf("one argument: %v, %q; want exit 2 and the usage line", err, out)
	}
}
