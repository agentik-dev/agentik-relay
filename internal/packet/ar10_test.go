package packet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR10_0_ParseRefusesUnboundedLines: an acceptance line is targeted
// (§15): `agentik-relay plan|run` without --rows, `agentik-relay gate` without a row, `go
// test` without a package and -run, `go test ./...`, and `make check` (the
// verify line is the gate, never an acceptance line) make the packet
// invalid; the bounded forms and the pre-AR10 records parse.
func TestAR10_0_ParseRefusesUnboundedLines(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	const cell = "| acceptance | `cat x \\| wc -l` reads 6 or more |"
	orig := string(src)
	i := strings.Index(orig, "| acceptance |")
	j := i + strings.Index(orig[i:], "\n")
	with := func(id, line string) error {
		s := orig[:i] + strings.Replace(cell, "`cat x \\| wc -l` reads 6", "`"+line+"` reads 0; `cat x \\| wc -l` reads 6", 1) + orig[j:]
		s = strings.Replace(s, "# Packet ok-minimal:", "# Packet "+id+":", 1)
		_, err := Parse("testdata/"+id+".md", []byte(s))
		return err
	}
	for _, line := range []string{
		"make check; echo $?",
		"make check 2>&1 \\| tail -1",
		"go test -race ./... >/dev/null 2>&1; echo $?",
		"go test ./... -run 'TestAR4_' -v 2>&1 \\| grep -c '^--- PASS'",
		"go test ./internal/stream -v 2>&1 \\| grep -c '^--- PASS'",
		"go test -run 'TestX' -v 2>&1 \\| grep -c PASS",
		"go build -o bin/relay ./cmd/relay && bin/relay plan --json \\| jq -r .valid",
		"t=$(mktemp -d) && XDG_STATE_HOME=\"$t\" go run ./cmd/relay run --dry > \"$t/o\"; echo $?",
		"bin/relay gate --json; echo $?",
		"bin/relay gate --json > out; echo $?",
		"make -n check >/dev/null; echo $?", // any flag (AR13 act 7, ADV6-12)
		// off the allowlist since AR20 act 5 (ADV11-10)
		"t=$(mktemp -d) && RELAY_MEASURE_DEPTH=1 XDG_STATE_HOME=\"$t\" go run ./cmd/relay plan --rows ar11-x > \"$t/o\" 2>&1; echo $?",
		"go build -o bin/relay ./cmd/relay && bin/relay gate a3-machine --json \\| jq -r .result",
	} {
		if err := with("ar11-x", line); err == nil || !strings.Contains(err.Error(), "unbounded") {
			t.Errorf("%s: Parse = %v; want an unbounded refusal", line, err)
		}
	}
	for _, line := range []string{
		"go test ./internal/effects -run 'TestAR2_6_' -count=1 -v 2>&1 \\| grep -c '^--- PASS'",
		"go test ./internal/effects -run 'TestX\\|TestY' -v 2>&1 \\| grep -c '^--- PASS'",
		"grep -c 'relay plan' README.md",
	} {
		if err := with("ar11-x", line); err != nil {
			t.Errorf("%s: Parse = %v; want it to parse", line, err)
		}
	}
	if err := with("ar9-queue-and-resync", "make check; echo $?"); err != nil {
		t.Errorf("a pre-AR10 record is refused: %v", err)
	}
	for _, p := range []string{filepath.Join(copyIn(t, "../..", "design/packets/ar10-bounded-night.md"), "design", "packets", "ar10-bounded-night.md")} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(p, b); err != nil {
			t.Errorf("the ar10 packet: %v", err)
		}
	}
}
