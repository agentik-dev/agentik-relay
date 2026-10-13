package packet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR17C_0_UnboundedSpellings: every spelling of the suite the review
// ran (ADV10-4) and its neighbours are refused by shape: a suite make
// target by make or gmake, the default goal, a command word that is not
// literal, a shell fed from stdin.
func TestAR17C_0_UnboundedSpellings(t *testing.T) {
	for _, line := range []string{
		"make test", "make check-unlocked", "make e2e", "make vet", "make tla", "gmake check",
		"make", "make -j4", "make -C . check", "/usr/bin/make test", "make fmt test",
		"env FOO=1 make -f Makefile check-unlocked", "cd x && make",
		"$(echo make) check", "`echo make` check", "m=make; $m check", "${M} check",
		"echo 'make check' | sh", "echo 'make check' | bash -s", "sh <<< 'make check'",
		"bash < script.sh", "printf x | sh -c", "cat x | zsh --",
		"bash -c 'make test'", "timeout 60 gmake vet",
		// off the allowlist since AR20 act 5 (ADV11-10)
		"make fmt", "make hygiene", "make doclint", "make -C sub build", "make -j 4 fmt",
		"bash scripts/x", "sh -c 'echo hi'",
	} {
		if Unbounded(line) == "" {
			t.Errorf("Unbounded(%q) = \"\", want a refusal", line)
		}
	}
}

// TestAR17C_0_NarrowTargetsAllowed: targeted tests and ordinary read-only
// commands stay acceptance lines (make targets left the list at AR20 act 5).
func TestAR17C_0_NarrowTargetsAllowed(t *testing.T) {
	for _, line := range []string{
		"go test ./internal/packet -run TestAR17C_ -count=1", "test -e history/sessions/NEXT; echo $?",
		"grep -c x design/relay-r1.md | bc",
		"[ -f x ] && echo ok", "echo $HOME",
	} {
		if why := Unbounded(line); why != "" {
			t.Errorf("Unbounded(%q) = %q, want \"\"", line, why)
		}
	}
}

// TestAR17C_0_LivePacketsParse: the packets from ar10-* on still parse
// under the closed rule, read from copies under t.TempDir().
func TestAR17C_0_LivePacketsParse(t *testing.T) {
	src, err := filepath.Glob(filepath.Join(copyIn(t, "../..", "design/packets"), "design", "packets", "ar*.md"))
	if err != nil {
		t.Fatal(err)
	}
	dir, n := t.TempDir(), 0
	for _, p := range src {
		name := filepath.Base(p)
		id := strings.TrimSuffix(name, ".md")
		if !strings.HasPrefix(id, "ar1") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		cp := filepath.Join(dir, name)
		if err := os.WriteFile(cp, b, 0o644); err != nil {
			t.Fatal(err)
		}
		c, _ := os.ReadFile(cp)
		if _, err := Parse("design/packets/"+name, c); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		n++
	}
	if n < 8 {
		t.Fatalf("parsed %d packets from ar10-* on, want 8 or more", n)
	}
}
