package packet

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAR21_4_RunProbes (ADV12-6): a -run that matches every test is
// refused, the review's two among them; a -run that picks a packet's tests
// passes.
func TestAR21_4_RunProbes(t *testing.T) {
	for _, l := range []string{
		"go test ./internal/packet -run 'Test[A-Z]' -count=1 2>&1 | grep -c FAIL",
		"go test ./internal/gate -run '^(Test|Example)[A-Z0-9_]' -count=1",
		"go test ./internal/gate -run=Test_ -count=1",
		"go test ./internal/gate -run 'Test[0-9A-Z_]' -count=1",
		"go test ./internal/gate -run 'Example[A-Z]' -count=1",
		"go test ./internal/gate -run 'Fuzz[A-Z]' -count=1",
		"go test ./internal/gate -run 'Test.' -count=1",
	} {
		if Unbounded(l) == "" {
			t.Errorf("Unbounded(%q) passes it", l)
		}
	}
	for _, l := range []string{
		"go test ./internal/packet -run TestAR21_ -count=1 2>&1 | grep -c FAIL",
		"go test ./internal/packet -run 'TestAR21_4_|TestAR20_5_' -count=1",
		"go test ./internal/gate -run '^TestGate' -count=1",
	} {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q", l, why)
		}
	}
}

// TestAR21_4_OnePackage (ADV12-6): a `go test` line names exactly one
// package; AR20's own lines, which named several, are refused and its
// stem is spent.
func TestAR21_4_OnePackage(t *testing.T) {
	for _, l := range []string{
		"go test ./internal/gate ./internal/effects ./internal/night -run TestAR20_ -count=1 2>&1 | grep -c FAIL",
		"go test ./internal/packet ./internal/stream ./internal/hooks ./internal/queue -run TestAR20_ -count=1 2>&1 | grep -c FAIL",
		"go test -run TestX ./cmd/relay ./internal/gate",
		"go test ./internal/a -run TestX -count=1 ./internal/b",
		"go test ./internal/prompt ./internal/effects -run 'TestX|TestY' -v 2>&1 | grep -c '^--- PASS'",
	} {
		if Unbounded(l) == "" {
			t.Errorf("Unbounded(%q) passes it", l)
		}
	}
	if why := Unbounded("go test -run TestX ./cmd/relay -count=1"); why != "" {
		t.Errorf("one package refused: %q", why)
	}
	if !spent["ar20-fixes-adv11"] {
		t.Errorf("ar20-fixes-adv11 is not spent")
	}
}

// TestAR21_4_EndlessRead (ADV12-10): a read verb's operand under /dev/
// (but /dev/null) is refused, however it is spelled; /dev/null and a
// relative dev/ in the tree pass.
func TestAR21_4_EndlessRead(t *testing.T) {
	for _, l := range []string{
		"cat /dev/zero",
		"head -c 1 /dev/urandom",
		"tail -n 1 /dev/random | wc -c",
		"wc -c < /dev/zero",
		"wc -c </dev/zero",
		"grep -c x /dev/zero",
		"grep -f/dev/zero x",
		"grep --file=/dev/zero x",
		"sed -n 1p /dev/zero",
		"cut -c1 '/dev/zero'",
		"paste /dev/fd/0",
		"bc /dev/zero",
		"cat //dev/zero",
		"cat /dev/null/../zero",
		"cat ../../../../../../../../dev/zero",
		"cat /d?v/zero",
		"cat /*/zero",
		"cat $NOPE/dev/zero",
		"git diff --no-index /dev/zero x",
	} {
		if Unbounded(l) == "" {
			t.Errorf("Unbounded(%q) passes it", l)
		}
	}
	for _, l := range []string{
		"cat /dev/null", "wc -l /dev/null", "grep -c x f 2>/dev/null",
		"cat x >/dev/null", "grep -c x dev/notes.md", "head -1 ./dev/x",
		"wc -l < f", "test -e history/sessions/NEXT; echo $?",
	} {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q", l, why)
		}
	}
}

// TestAR21_4_OwnLinesPass: this packet's acceptance cell, read from a copy
// of design/packets and parsed, admits every line.
func TestAR21_4_OwnLinesPass(t *testing.T) {
	p := filepath.Join(copyIn(t, "../..", "design/packets/ar21-fixes-adv12.md"), "design", "packets", "ar21-fixes-adv12.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := Parse(p, b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if spent[pk.ID] || legacy[pk.ID] {
		t.Fatalf("%s is grandfathered", pk.ID)
	}
	if len(pk.Acceptance) < 10 {
		t.Errorf("acceptance has %d lines, want 10", len(pk.Acceptance))
	}
	for _, l := range pk.Acceptance {
		if why := Unbounded(l.Shell); why != "" {
			t.Errorf("Unbounded(%q) = %q", l.Shell, why)
		}
	}
}
