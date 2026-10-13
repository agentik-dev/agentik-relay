package packet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR14_0_VerifyRowParses: an optional seventh row, `verify`, after
// budget, holds one code span kept as Packet.Verify (conventions §5, §9);
// a packet without it parses with Verify empty; two spans, a bare cell or
// the row out of place are refused.
func TestAR14_0_VerifyRowParses(t *testing.T) {
	parse := func(name string) (*Packet, error) {
		src, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return Parse("testdata/"+name, src)
	}
	pk, err := parse("ok-verify.md")
	if err != nil {
		t.Fatal(err)
	}
	if pk.Verify != "make check" {
		t.Errorf("Verify = %q, want make check", pk.Verify)
	}
	pk, err = parse("ok-minimal.md")
	if err != nil {
		t.Fatal(err)
	}
	if pk.Verify != "" {
		t.Errorf("Verify without the row = %q, want empty", pk.Verify)
	}
	src, err := os.ReadFile(filepath.Join("testdata", "ok-verify.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ old, new, want string }{
		"two spans": {"| verify | `make check` |", "| verify | `make check` or `make test` |", "row verify: want one code span"},
		"bare cell": {"| verify | `make check` |", "| verify | make check |", "row verify: want one code span"},
		"out of place": {"| budget | 10 turns, 20k context, $5 |\n| verify | `make check` |",
			"| verify | `make check` |\n| budget | 10 turns, 20k context, $5 |", "row verify: out of order"},
	} {
		s := strings.Replace(string(src), c.old, c.new, 1)
		if s == string(src) {
			t.Fatalf("%s: the fixture edit did not apply", name)
		}
		if _, err := Parse("testdata/ok-verify.md", []byte(s)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
	}
}
