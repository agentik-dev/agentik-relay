package hooks

import (
	"slices"
	"strings"
	"testing"
)

// TestAR21_3_AnsiCQuote (ADV12-4): `$'…'` is read with its backslash
// escapes, so `\'` inside it closes nothing and the command after the `;`
// is read; an escape that spells the command word is read as that word.
func TestAR21_3_AnsiCQuote(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{"echo $'\\'' ; git push origin main", "push"},
		{"echo $'\\'' ; relay run", "nested-run"},
		{"echo $'it\\'s' ; git push origin main", "push"},
		{"echo $'\\\\' ; git push origin main", "push"},
		{"echo $'a\\\\\\'b' ; git push origin main", "push"},
		{"$'\\x67it' push origin main", "push"},
		{"$'\\147it' push origin main", "push"},
		{"$'\\u0067it' push origin main", "push"},
		{"git $'push' origin main", "push"},
		{"echo $'a b'", ""},
		{"echo $'\\''", ""},
	} {
		if got := hookSays(t, tc.cmd, true); got != tc.want {
			t.Errorf("in a sitting, %q = %q, want %q", tc.cmd, got, tc.want)
		}
		if deny, _ := Verdict(tc.cmd, true); deny != (tc.want != "") {
			t.Errorf("Verdict(%q, true) = %v", tc.cmd, deny)
		}
	}
	// the decoded word: `\'` is a quote, `\\` a backslash, `\n` a newline
	for cmd, want := range map[string]string{
		"echo $'a\\'b'":   "a'b",
		"echo $'a\\\\b'":  "a\\b",
		"echo $'a\\nb'":   "a\nb",
		"echo x$'\\x41'y": "xAy",
		"echo $'\\cA'":    "\x01",
		"echo $'\\q'":     "\\q",
		"echo $'\\x'":     "\\x",
	} {
		segs := Segments(cmd)
		if len(segs) != 1 || len(segs[0]) != 2 || segs[0][1] != want {
			t.Errorf("Segments(%q) = %q, want [echo %q]", cmd, segs, want)
		}
	}
	// AR46: a quote that never closes is a line the parser rejects
	if got := hookSays(t, "echo $'unfinished", true); got != "unreadable" {
		t.Errorf("in a sitting, an unfinished quote = %q, want unreadable", got)
	}
}

// TestAR21_3_ArithmeticShift (ADV12-5): inside `((…))`, `$((…))`, `$[…]`
// and `${…}` a `<<` is a shift, so the lines after it are commands; after
// `let` it is read both ways (sh and zsh open a heredoc there), so the
// lines after it are read as commands and past its would-be body too.
func TestAR21_3_ArithmeticShift(t *testing.T) {
	for _, cmd := range []string{
		"(( 1<<2 ))\ngit push origin main\n2",
		"((1<<2))\ngit push origin main\n2",
		"echo $(( 1<<2 ))\ngit push origin main\n2",
		"echo \"$(( 1<<2 ))\"\ngit push origin main\n2",
		"let x=1<<2\ngit push origin main\n2",
		"let 'x=1'<<2\ngit push origin main\n2",
		"for (( i=0; i<<2; i++ )); do :; done\ngit push origin main\n2",
		"echo $[1<<2]\ngit push origin main\n2",
		"echo ${x[1<<2]}\ngit push origin main\n2",
		// read both ways after let: the shell's heredoc hides the quote
		"let x=1<<EOF\nit's\nEOF\ngit push origin main",
		// a shift then a real heredoc, each read as the shell reads it
		"(( 1<<2 ))\ncat <<EOF\nit's\nEOF\ngit push origin main\n2",
	} {
		if got := hookSays(t, cmd, true); got != "push" {
			t.Errorf("in a sitting, %q = %q, want push", cmd, got)
		}
	}
	// outside arithmetic a heredoc body is still data
	for _, cmd := range []string{
		"(( 1 )); cat <<EOF\ngit push\nEOF\necho ok",
		"echo $(( 1 )) <<EOF\ngit push\nEOF",
		"cat <<EOF\ngit push\nEOF",
	} {
		for _, seg := range Segments(cmd) {
			if slices.Contains(seg, "push") {
				t.Errorf("Segments(%q) reads the heredoc body: %q", cmd, strings.Join(seg, " "))
			}
		}
	}
	// AR46: the parser reads `((` as arithmetic alone, so the two subshells
	// the shells fall back to are a line it rejects: denied in a sitting,
	// and read loosely, body and all
	if got := hookSays(t, "((cat <<EOF\ngit push\nEOF\n) )", true); got == "" {
		t.Errorf("in a sitting, a `((` that is two subshells is not denied")
	}
}
