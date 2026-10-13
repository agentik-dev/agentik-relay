package packet

import "testing"

// TestAR21_3_AnsiCQuote (ADV12-4): an acceptance line with `$'` is refused
// outright, whatever its escapes spell.
func TestAR21_3_AnsiCQuote(t *testing.T) {
	for _, l := range []string{
		"echo $'\\'' ; make check",
		"echo $'\\'' ; rm -rf x",
		"echo $'\\'' > /tmp/x",
		"echo $'a'",
		"echo \"$'\"",
	} {
		if Unbounded(l) == "" {
			t.Errorf("Unbounded(%q) passes it", l)
		}
	}
	for _, l := range []string{"wc -l x", "echo 'a' ; wc -l x", "echo \"$HOME'\""} {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q", l, why)
		}
	}
}
