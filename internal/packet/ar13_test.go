package packet

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// withLine is ok-minimal with line as a further acceptance item, parsed
// as the packet id.
func withLine(t *testing.T, id, line string) error {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(src), "| acceptance | ", "| acceptance | `"+line+"` reads 0; ", 1)
	s = strings.Replace(s, "# Packet ok-minimal:", "# Packet "+id+":", 1)
	_, err = Parse("testdata/"+id+".md", []byte(s))
	return err
}

// TestAR13_7_ParseUnwrapsWrappers: Unbounded reads a line as the hook does
// (hooks.Segments, hooks.Normalise; ADV6-12, R6 C6): `go test` with any
// `...` or `-run .`, `make` with check among its targets under any flag,
// `agentik-relay plan|run|gate` unbounded inside any wrapper or `sh -c`, and a
// segment whose command is claude are refused; the bounded forms parse.
func TestAR13_7_ParseUnwrapsWrappers(t *testing.T) {
	for _, line := range []string{
		"sh -c 'make check'; echo $?",
		"make -k check; echo $?",
		"make -n check >/dev/null; echo $?",
		"make -C . lint check; echo $?",
		"bash -c \"go test ./...\"; echo $?",
		"go test -run . ./internal/...; echo $?",
		"go test ./internal/... -run TestX; echo $?",
		"go test -run . ./internal/packet; echo $?",
		"go test -run=. ./internal/packet; echo $?",
		"timeout 60 go test ./cmd/... -run TestX; echo $?",
		"sh -c 'relay run'; echo $?",
		"timeout 9 bin/relay plan --json; echo $?",
		"env A=1 relay gate --json; echo $?",
		"claude -p hello; echo $?",
		"timeout 60 claude -p hello; echo $?",
		"echo x && /usr/local/bin/claude --version",
		// off the allowlist since AR20 act 5 (ADV11-10)
		"sh -c \"go test ./internal/hooks -run TestX\"; echo $?",
		"make lint; echo $?",
		"make -n lint >/dev/null; echo $?",
		"bin/relay plan --rows ar14-x --json; echo $?",
		"sh -c 'bin/relay gate ar14-x'; echo $?",
	} {
		if err := withLine(t, "ar14-x", line); err == nil || !strings.Contains(err.Error(), "unbounded") {
			t.Errorf("%s: Parse = %v; want an unbounded refusal", line, err)
		}
	}
	for _, line := range []string{
		"go test ./internal/packet -run 'TestAR13_7_' -count=1; echo $?",
		"echo claude; echo $?",
		"grep -c 'make check' README.md",
	} {
		if err := withLine(t, "ar14-x", line); err != nil {
			t.Errorf("%s: Parse = %v; want it to parse", line, err)
		}
	}
}

// TestAR13_7_LegacyIsExplicit: the records parsed as written are the
// seventeen packets cut before AR10, by their stems (ADV6-12): a future id
// sharing a prefix and a stem after the spent ones are bound by the rule
// (ar10 and ar13 joined spent at AR20 act 5, ar20 at AR21 act 4).
func TestAR13_7_LegacyIsExplicit(t *testing.T) {
	if len(legacy) != 17 {
		t.Fatalf("%d legacy stems; want 17", len(legacy))
	}
	for _, id := range []string{"a3-machine", "ap1-plugin", "ar2-adversarial", "ar9-queue-and-resync", "ux1-review"} {
		if err := withLine(t, id, "make check; echo $?"); err != nil {
			t.Errorf("the record %s is refused: %v", id, err)
		}
	}
	for _, id := range []string{"a1-x", "a3-future", "ar2-x", "ar9-x", "ux9-x", "ap2-x", "ar10-x", "ar21-fixes-adv12"} {
		if err := withLine(t, id, "make check; echo $?"); err == nil {
			t.Errorf("%s is grandfathered", id)
		}
	}
}

// TestAR13_7_CeilingMatchesDoclint: a packet of Ceiling bytes is refused,
// one byte less parses (ADV6-16), and doclint's ceiling and comparison are
// the same.
func TestAR13_7_CeilingMatchesDoclint(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	pad := func(n int) []byte {
		return append(append([]byte(nil), src...), []byte(strings.Repeat("x", n-len(src)))...)
	}
	if _, err := Parse("testdata/ok-minimal.md", pad(Ceiling)); err == nil {
		t.Errorf("a %d-byte packet parses", Ceiling)
	}
	if _, err := Parse("testdata/ok-minimal.md", pad(Ceiling-1)); err != nil {
		t.Errorf("a %d-byte packet: %v", Ceiling-1, err)
	}
	lint, err := os.ReadFile(filepath.Join("..", "..", "scripts", "doclint"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^PACKET_CEILING = (\d+)$`).FindSubmatch(lint)
	if m == nil {
		t.Fatal("doclint has no PACKET_CEILING")
	}
	if n, _ := strconv.Atoi(string(m[1])); n != Ceiling || !strings.Contains(string(lint), "len(data) >= PACKET_CEILING") {
		t.Fatalf("doclint's ceiling is %s, or not `>=`; Ceiling is %d", m[1], Ceiling)
	}
}
