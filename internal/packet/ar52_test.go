package packet

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAR52_1_MakefilePythonTakesB (design/pin-pycache-r1.md §5 item 2): every
// Makefile line that starts python starts it with -B, so a check run here
// writes no bytecode into a tree a relay has pinned. It reads a copy of the
// Makefile and starts nothing.
func TestAR52_1_MakefilePythonTakesB(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(copyIn(t, "../..", "Makefile"), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for i, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, "$(PY)") || strings.HasPrefix(strings.TrimSpace(line), "PY :=") {
			continue
		}
		n++
		if strings.Count(line, "$(PY)") != strings.Count(line, "$(PY) -B ") {
			t.Errorf("Makefile:%d starts python without -B: %s", i+1, strings.TrimSpace(line))
		}
	}
	if n < 4 {
		t.Errorf("Makefile: %d lines hold $(PY), want at least 4 (check, doclint, hygiene, validate)", n)
	}
}

// ar52Launch reads a line that starts a process with the interpreter variable
// py as its program, by either exec constructor; group 1 is the rest of the
// line. ar52First is the one shape that rest may take: the literal "-B" as
// the next argument, or as the first element of the slice the line spreads.
var (
	ar52Launch = regexp.MustCompile(`exec\.Command(?:Context\([^,]+,\s*|\()py\b(.*)$`)
	ar52First  = regexp.MustCompile(`^\s*,\s*(?:"-B"\s*[,)]|append\(\[\]string\{"-B"\s*[,}])`)
)

// TestAR52_2_TestsPythonTakeB (design/pin-pycache-r1.md §5 item 2): every
// test line under internal/ and cmd/ that starts python passes -B as python's
// first argument, so a test run here writes no bytecode into a tree a relay
// has pinned. It reads a copy of the two trees and starts nothing; its own
// source holds no such line.
func TestAR52_2_TestsPythonTakeB(t *testing.T) {
	root := copyIn(t, "../..", "internal", "cmd")
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(string(b), "\n") {
			m := ar52Launch.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			n++
			if !ar52First.MatchString(m[1]) {
				t.Errorf("%s:%d starts python without -B first: %s", rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 8 {
		t.Errorf("%d test lines start python by py, want at least 8", n)
	}
}
