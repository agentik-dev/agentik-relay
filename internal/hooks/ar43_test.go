package hooks

import "testing"

// TestAR43_1_QuotedHeredocDelimiter: a quoted heredoc delimiter runs to its
// closing quote, blanks and all, so the quote the old reading left open no
// longer swallows the command after it; an escaped blank is in the
// delimiter too.
func TestAR43_1_QuotedHeredocDelimiter(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{"cat <<'a b' ; git push origin main #'", "push"},
		{"cat <<\"a b\" ; git push origin main #\"", "push"},
		{"bash -c 'cat <<\"a b\" ; git push origin main #\"'", "push"},
		{"cat <<'a b' ; relay run #'", "nested-run"},
		{"cat <<'ab' ; git push origin main #'", "push"},
		// an escaped quote inside a double-quoted delimiter closes nothing
		{"cat <<\"a\\\" b\" ; git push origin main #\"", "push"},
		// the body ends at the delimiter the shell reads, blank included
		{"cat <<'a b'\ngit status\na b\ngit push origin main", "push"},
		{"cat <<a\\ b\ngit status\na b\ngit push origin main", "push"},
		// a delimiter's quote that never closes: no shell runs the line, and
		// the reading before AR43 is kept, so the rest is still read
		{"cat <<\"a b ; git push origin main", "push"},
		{"cat <<'a b ; relay run", "nested-run"},
		// AR46: a heredoc with no body is a line the parser rejects
		{"cat <<'a b' ; echo ok #'", "unreadable"},
		{"cat <<a\\ b ; echo ok", "unreadable"},
		// the body is still data
		{"cat <<'a b'\ngit push origin main\na b\necho ok", ""},
	} {
		if got := hookSays(t, tc.cmd, true); got != tc.want {
			t.Errorf("in a sitting, %q = %q, want %q", tc.cmd, got, tc.want)
		}
		if deny, _ := Verdict(tc.cmd, true); deny != (tc.want != "") {
			t.Errorf("Verdict(%q, true) = %v", tc.cmd, deny)
		}
	}
}

// TestAR43_2_QuoteInExpansion: inside a double-quoted span a `${…}` runs
// to its matching `}`; a `"` inside it opens a nested span, and a `'`
// inside it is read both ways (a literal in zsh and dash, a quote in bash
// 3.2), so the command after the span the shell closes is read.
func TestAR43_2_QuoteInExpansion(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{"echo \"${x:-\"'\"}\" ; git push origin main #'", "push"},
		{"echo \"${x:-\"'\"}\" ; relay run #'", "nested-run"},
		{"echo \"${x:-'}\" ; echo '}\" ; git push origin main", "push"},
		// the nested span holds a `}`, and an expansion nests in an expansion
		{"echo \"${x:-\"}'\"}\" ; git push origin main #'", "push"},
		{"echo \"${x:-${y:-\"'\"}}\" ; git push origin main #'", "push"},
		// a substitution inside the nested span is still read as commands
		{"echo \"${x:-\"$(git push origin main)\"}\"", "push"},
		// a bare `{` nests nothing: the first `}` ends the expansion
		{"echo \"${x:-{}\" ; git push origin main ; echo \"}\"", "push"},
		// AR46: a quote inside `"${…}"` is unreadable, the push no shell runs too
		{"echo \"${x:-\" ; git push origin main ; echo \"}\"", "unreadable"},
		// both readings of a `'` meet a `<<` after let read both ways
		{"echo \"${x:-'}\" '}\"\nlet x=1<<EOF\nit's\nEOF\ngit push origin main", "push"},
		{"echo \"${HOME}/x\"", ""},
		// AR46: the benign lines the rule over-denies
		{"echo \"${x:-\"a\"}\"", "unreadable"},
		{"echo \"${x:-'}\"", "unreadable"},
		{"echo \"${x:-\"a b\"}\" ; echo ok", "unreadable"},
		{"echo \"${x:-'a'}\" ; echo ok", "unreadable"},
	} {
		if got := hookSays(t, tc.cmd, true); got != tc.want {
			t.Errorf("in a sitting, %q = %q, want %q", tc.cmd, got, tc.want)
		}
		if deny, _ := Verdict(tc.cmd, true); deny != (tc.want != "") {
			t.Errorf("Verdict(%q, true) = %v", tc.cmd, deny)
		}
	}
}

// TestAR43_4_BareRedirectNoPanic: a bare `<` or `<<<` that ends a segment
// has no operand to step over, so the command word's index stays inside
// the segment and no line panics the reader; the line before it is read.
func TestAR43_4_BareRedirectNoPanic(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{"git push origin main\n<", "push"},
		{"relay run\n<<<", "nested-run"},
	} {
		if got := hookSays(t, tc.cmd, true); got != tc.want {
			t.Errorf("in a sitting, %q = %q, want %q", tc.cmd, got, tc.want)
		}
		if deny, _ := Verdict(tc.cmd, true); !deny {
			t.Errorf("Verdict(%q, true) = false", tc.cmd)
		}
	}
	// AR46: a bare redirection is a line the parser rejects, so unreadable
	for cmd, want := range map[string]string{"<": "unreadable", "< ": "unreadable", "wc -l x;<": "unreadable",
		"( < )": "unreadable", "<<<": "unreadable", "a | <": "unreadable", "env -u": "", "env -S": ""} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Verdict(%q, true) panicked: %v", cmd, r)
				}
			}()
			if got := hookSays(t, cmd, true); got != want {
				t.Errorf("in a sitting, %q = %q, want %q", cmd, got, want)
			}
			Segments(cmd)
		}()
	}
}
