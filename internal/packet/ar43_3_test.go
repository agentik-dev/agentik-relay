package packet

import (
	"strings"
	"testing"
)

// TestAR43_3_NoHeredocNoExpansion: an acceptance line holds no heredoc or
// here-string, no `${`, and no `>&` whose fd runs on into a file name; a
// `<<` or a `${` inside single quotes is a literal and passes.
func TestAR43_3_NoHeredocNoExpansion(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{"cat <<'a b' ; make check #'", "`<<`"},
		{"echo \"${x:-\"'\"}\" ; make check #'", "`${`"},
		{"echo \"${x}\"", "`${`"},
		{"cat <<<x", "`<<`"},
		{"echo >&2x", "`>&` without an fd writes a file"},
		{"echo hi 2>&1x", "`>&` without an fd writes a file"},
		// erring toward refusal, as the `$'` scan does
		{"grep -c \"<<\" f", "`<<`"},
		{"echo \\${x}", "`${`"},
		{"echo ${x}", "`${`"},
		{"echo 'a' <<b", "`<<`"},
		{"echo >&2-", "`>&` without an fd writes a file"},
		{"echo >>&2x", "writes a file"},
		// zsh reads `>>&1` as an append to a file named `1`
		{"wc -l x >>&1", "writes a file"},
		{"wc -l x 2>>&1", "writes a file"},
		// a bare `<` is refused, and panics no reader
		{"wc -l x;<", "`<`"},
		{"< ", "`<`"},
	} {
		if why := Unbounded(tc.line); !strings.Contains(why, tc.want) {
			t.Errorf("Unbounded(%q) = %q, want a refusal naming %s", tc.line, why, tc.want)
		}
	}
	for _, l := range []string{
		"go test ./internal/hooks -run \"^TestAR43_\" -count=1 -v 2>&1 | grep -c \"^--- PASS: TestAR43_\"",
		"go test ./internal/packet -run \"^TestAR43_\" -count=1 -v 2>&1 | grep -c \"^--- PASS: TestAR43_\"",
		"grep -c '<<' f",
		"grep -c '${' f",
		"wc -l x 2>&1",
		"wc -l x 2>&1 | grep -c 1",
		"wc -l x >> /dev/null",
		"wc -l x 2>&1; echo ok",
		"(wc -l x 2>&1)",
		"wc -l x 2>&1&& echo ok",
	} {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q", l, why)
		}
	}
}
