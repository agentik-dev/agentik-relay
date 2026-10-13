package packet

import (
	"strings"
	"testing"
)

// TestAR42_1_DollarInSingleQuotes: a `$` that is the last byte inside a
// single-quoted word is literal, and the `'` after it closes the word, so
// no ANSI-C quote opens and the line is read on.
func TestAR42_1_DollarInSingleQuotes(t *testing.T) {
	for _, l := range []string{
		"go test ./internal/packet -run '^(TestA|TestB)$' -count=1",
		"grep -cE '^(a|b)$' f",
		"grep -c 'x$' f | wc -l",
		"echo 'a$' 'b$'",
	} {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q", l, why)
		}
	}
}

// TestAR42_1_AnsiCStillRefused: a `$'` whose `$` is outside a single-quoted
// span is refused with the ANSI-C text, erring toward refusal (a backslash
// before the `$` does not excuse it, and neither do double quotes).
func TestAR42_1_AnsiCStillRefused(t *testing.T) {
	for _, l := range []string{
		"echo 'a'$'b'",                // the first span closed: the `$'` is unquoted
		"echo 'a$'$'\\''; make check", // a literal `$` first, then a real one
		"echo \\$'a'",                 // the backslash does not excuse it
		"echo \"$'\"",                 // double quotes do not excuse it
		"echo \"'\"$'x'",              // a `'` inside double quotes opens nothing
		"echo \\'$'x'",                // an escaped `'` opens nothing
		"echo $'a'",
	} {
		if why := Unbounded(l); !strings.Contains(why, "ANSI-C quote") {
			t.Errorf("Unbounded(%q) = %q, want the ANSI-C refusal", l, why)
		}
	}
}
