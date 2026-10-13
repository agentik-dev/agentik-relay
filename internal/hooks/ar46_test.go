package hooks

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// TestAR46_1_TreeReads: in a sitting the guards read a command through the
// shell parser (design/guard-parser-r1.md §2). A `case` inside a
// substitution is read, where the hand reader could only call it
// unreadable; the strings a command hands another reader are still read;
// and an ordinary line is not denied.
func TestAR46_1_TreeReads(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		{`echo "$(case a in a) git push origin main;; esac)"`, "push"},
		{`echo $(case a in a) git push origin main;; esac)`, "push"},
		{`echo "$(true; case a in a) relay run;; esac)"`, "nested-run"},
		{`sh -c 'git push origin main'`, "push"},
		{`find . -exec git push origin main \;`, "push"},
		{`echo "$(date)"`, ""},
		{`n="$(git log --oneline | wc -l)"`, ""},
		{`for f in a b; do echo "$f"; done`, ""},
	} {
		if got := hookSays(t, c.cmd, true); got != c.want {
			t.Errorf("in a sitting %q: guard %q, want %q", c.cmd, got, c.want)
		}
	}
}

// TestAR46_2_QuoteInBraces: a quote character, `'` or `"`, inside a
// `"${…}"` word makes the line unreadable in a sitting (rule 3): the shells
// split on where such a span ends, and one tree holds one reading. A named
// guard still answers first on what the tree does hold. Outside a sitting
// the benign lines the rule over-denies are not denied.
func TestAR46_2_QuoteInBraces(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		// zsh and dash run the push; bash 3.2 does not
		{`echo "${x:-'}" ; git push origin main ; echo "'}"`, "unreadable"},
		// no shell runs the push, and the reader before AR46 denied it
		{`echo "${x:-" ; git push origin main ; echo "}"`, "unreadable"},
		{`echo "${x:-"'"}" ; git push origin main #'`, "push"},
		// the over-denials
		{`echo "${x:-'a'}" ; echo ok`, "unreadable"},
		{`echo "${x:-"a b"}"`, "unreadable"},
		{`echo "${x:-a}"`, ""},
		{`echo "${x}" 'it' "s"`, ""},
	} {
		if got := hookSays(t, c.cmd, true); got != c.want {
			t.Errorf("in a sitting %q: guard %q, want %q", c.cmd, got, c.want)
		}
	}
	// bash 3.2 runs the push
	if got := hookSays(t, `echo "${x:-'}" ; echo '}" ; git push origin main #'`, true); got == "" {
		t.Errorf("in a sitting, the line bash 3.2 runs the push of is not denied")
	}
	for _, cmd := range []string{`echo "${x:-'a'}" ; echo ok`, `echo "${x:-"a b"}"`} {
		if got := hookSays(t, cmd, false); got != "" {
			t.Errorf("outside a sitting %q: guard %q, want none", cmd, got)
		}
	}
}

// oddCommand is a command kind the walker does not know.
type oddCommand struct{ syntax.Command }

// TestAR46_3_FailsClosed: what the reader cannot read is unreadable in a
// sitting, after what did parse: a line a dialect rejects, a substitution
// past maxDepth, a line over 64 KB (never handed to the parser), a node
// kind the walker does not know, and a panic under the reader. Outside a
// sitting none of it is denied, and a guard every session meets still
// answers on the same line.
func TestAR46_3_FailsClosed(t *testing.T) {
	deep := "echo $(echo $(echo $(echo $(echo $(git push origin main)))))"
	long := "echo " + strings.Repeat("a", 70<<10)
	lines := []struct{ cmd, want string }{
		{`echo "$(`, "unreadable"},
		{`wc -l x;<`, "unreadable"},
		{`cat <<'a b' ; echo ok #'`, "unreadable"},
		{"git push origin main\n<", "push"},
		{deep, "unreadable"},
		{long, "unreadable"},
	}
	check := func(what string) {
		t.Helper()
		for _, c := range lines {
			name := c.cmd
			if len(name) > 80 {
				name = name[:80] + "…"
			}
			if got := hookSays(t, c.cmd, true); got != c.want {
				t.Errorf("%sin a sitting %q: guard %q, want %q", what, name, got, c.want)
			}
			if got := hookSays(t, c.cmd, false); got != "" {
				t.Errorf("%soutside a sitting %q: guard %q, want none", what, name, got)
			}
			for _, in := range []bool{true, false} {
				if got := hookSays(t, "git commit --amend ; "+c.cmd, in); got != "amend" {
					t.Errorf("%s(in a sitting: %v) an amend before %q: guard %q, want amend", what, in, name, got)
				}
			}
		}
	}
	check("")
	// the amend past what the parser rejected, or in a line it was never
	// handed, is read loosely
	for _, cmd := range []string{"wc -l x;< ; git commit --amend", `echo "$(git commit --amend`, long + " ; git commit --amend"} {
		if got := hookSays(t, cmd, false); got != "amend" {
			t.Errorf("outside a sitting, an amend past the unread part (%.40q): guard %q, want amend", cmd, got)
		}
	}
	if segs := Segments(long); len(segs) == 0 || segs[len(segs)-1][0] != unreadMark {
		t.Errorf("a 70 KB line carries no mark")
	}

	// the seam: the walk of a parsed tree, under the reader's own recover
	defer func() { walkTree = nil }()
	lines = []struct{ cmd, want string }{{"echo ok", "unreadable"}, {"ls -la | head -3", "unreadable"}}

	// a walker that panics, before it reads a word and after it has read all
	walkTree = func(*walker, *syntax.File) { panic("boom") }
	check("a walker that panics: ")
	walkTree = func(w *walker, f *syntax.File) { w.stmts(f.Stmts); panic("boom") }
	check("a walker that panics at the end: ")
	if got := hookSays(t, "git push origin main", true); got != "push" {
		t.Errorf("a walker that panics at the end: a push it had read = %q, want push", got)
	}

	// a node kind the walker does not know: the rest of the tree is read
	walkTree = func(w *walker, f *syntax.File) {
		w.stmts(append([]*syntax.Stmt{{Cmd: oddCommand{}}}, f.Stmts...))
	}
	check("an unknown node kind: ")
	if got := hookSays(t, "git push origin main", true); got != "push" {
		t.Errorf("an unknown node kind: the push beside it = %q, want push", got)
	}
	var b strings.Builder
	part, arith, loop := &walker{}, &walker{}, &walker{}
	part.part(&b, struct{ syntax.WordPart }{})
	arith.arithm(struct{ syntax.ArithmExpr }{})
	loop.stmt(&syntax.Stmt{Cmd: &syntax.ForClause{Loop: struct{ syntax.Loop }{}}}, nil)
	if !part.unread || !arith.unread || !loop.unread {
		t.Errorf("an unknown kind is read: word part %v, arithmetic %v, loop %v", !part.unread, !arith.unread, !loop.unread)
	}
}

// verdictSays is Verdict's answer for cmd: the guard's name when it denies,
// "" when it allows.
func verdictSays(cmd string, inRow bool) string {
	deny, why := Verdict(cmd, inRow)
	if !deny {
		return ""
	}
	name, _, _ := strings.Cut(strings.TrimPrefix(why, "relay: "), ":")
	return name
}

// TestAR46_4_ReviewRound1: what the review of the tree reader found, one
// subtest an item. "In a row" is Verdict(cmd, true), "outside a row" is
// Verdict(cmd, false), and "the helper's door" is preToolUse with an
// agent_id and no RELAY_ROW.
func TestAR46_4_ReviewRound1(t *testing.T) {
	type line struct{ cmd, want string }
	row := func(t *testing.T, lines ...line) {
		t.Helper()
		for _, c := range lines {
			if got := verdictSays(c.cmd, true); got != c.want {
				t.Errorf("in a row %q: guard %q, want %q", c.cmd, got, c.want)
			}
		}
	}
	outside := func(t *testing.T, lines ...line) {
		t.Helper()
		for _, c := range lines {
			if got := verdictSays(c.cmd, false); got != c.want {
				t.Errorf("outside a row %q: guard %q, want %q", c.cmd, got, c.want)
			}
		}
	}
	door := func(t *testing.T, lines ...line) {
		t.Helper()
		for _, c := range lines {
			if got := helperSays(t, c.cmd, "a1", "", false); got != c.want {
				t.Errorf("at the helper's door, outside a row, %q: guard %q, want %q", c.cmd, got, c.want)
			}
		}
	}
	const (
		deep     = "echo $(echo $(echo $(echo $(echo $(git push origin main)))))"
		quoted   = `echo "${x:-'a'}" ; echo ok`
		evalDeep = "eval eval eval eval eval echo hi"
		shDeep   = "sh -c 'eval eval eval eval echo hi'"
	)

	// item 1: every redirection that feeds a command is the `<` word, so a
	// shell fed by `<&` or `<>` reads its commands from stdin
	t.Run("1 input redirects", func(t *testing.T) {
		row(t,
			line{`echo 'git push origin main' | sh <&0`, "stdin-shell"},
			line{`sh <>script`, "stdin-shell"},
			line{`sh <script`, "stdin-shell"},
		)
	})

	// item 2: an array's elements are re-read as any other segment, and a
	// keyed element's key and value are read for what they run
	t.Run("2 array elements", func(t *testing.T) {
		row(t,
			line{`a=(sh -c 'git push origin main')`, "push"},
			line{`a=(eval 'git push origin main')`, "push"},
			line{`declare -a a=(env -S 'git push origin main')`, "push"},
			line{`declare -A m=([$(git push origin main)]=1)`, "push"},
			line{`declare -A m=([a]=$(git push origin main))`, "push"},
		)
	})

	// item 3: a line the reader could not read whole, for whatever reason,
	// is read loosely too, so the guards every session meets still answer
	t.Run("3 an unread line is read loosely", func(t *testing.T) {
		lines := []line{
			{`echo "${x:-'}" ; git rebase main ; echo "'}"`, "rebase"},
			{`echo ${a[(r)$(git commit --amend)]}`, "amend"},
			{`echo @(a|$(git commit --amend))`, "amend"},
			// an accepted over-denial of an unreadable line: the loose
			// reading is blind to quotes, so the amend this line only
			// names, inside a quoted note, is denied outside a row
			{`echo $(( )) "note; git commit --amend"`, "amend"},
		}
		outside(t, lines...)
		for _, c := range lines {
			if got := verdictSays(c.cmd, true); got == "" {
				t.Errorf("in a row %q is not denied", c.cmd)
			}
		}
		// in a row the mark answers before the loose reading
		row(t, line{lines[0].cmd, "unreadable"})
	})

	// item 4: outside a row a helper's call is denied as unreadable only
	// for the depth bound and a recovered panic; any other unread line is
	// judged by its trees, its loose reading and the helper's guards
	t.Run("4 the helper's door", func(t *testing.T) {
		door(t,
			line{quoted, ""},
			line{`echo ${(U)x}`, ""},
			line{`echo "${x:-'}" ; relay run ; echo "'}"`, "helper-relay"},
			line{deep, "unreadable"},
			line{evalDeep, "unreadable"},
			line{shDeep, "unreadable"},
		)
		// in a row nothing changes: every unread line is unreadable
		row(t, line{quoted, "unreadable"}, line{deep, "unreadable"})
		if got := helperSays(t, quoted, "a1", "", true); got != "unreadable" {
			t.Errorf("at the helper's door, in a row, %q: guard %q, want unreadable", quoted, got)
		}
		// Segments' contract: the line carries unreadMark, and no other
		// word of the reader's own
		for _, cmd := range []string{deep, evalDeep, quoted} {
			marked := false
			for _, seg := range Segments(cmd) {
				marked = marked || len(seg) == 1 && seg[0] == unreadMark
				for _, w := range seg {
					if strings.HasPrefix(w, "\x00") && w != unreadMark {
						t.Errorf("Segments(%q) holds the reader's word %q", cmd, w)
					}
				}
			}
			if !marked {
				t.Errorf("Segments(%q) carries no mark", cmd)
			}
		}
		// the panic seam
		defer func() { walkTree = nil }()
		walkTree = func(*walker, *syntax.File) { panic("boom") }
		door(t, line{"echo ok", "unreadable"})
		row(t, line{"echo ok", "unreadable"})
		outside(t, line{"echo ok", ""})
	})

	// item 5: main over-denied these in a row, where nothing hidden runs:
	// the hand reader gave the first, the second and the fourth to the
	// `literal` guard. The tree reads each whole, and none is denied
	t.Run("5 over-denials lifted", func(t *testing.T) {
		row(t,
			line{`declare -A m=([a]=1 [b]=2); echo ${m[a]}`, ""},
			line{`go vet ./internal/hooks &>/dev/null`, ""},
			line{`case "$x" in a|b) echo y;; esac`, ""},
			line{`echo "$(( (a + b) * c ))"`, ""},
		)
	})

	// item 6: the tests the review found missing
	t.Run("6 rule 1: one dialect's command is judged", func(t *testing.T) {
		// bash reads a coprocess and the command it runs; zsh's grammar
		// here holds `coproc` as a plain command word
		const cmd = "coproc git push origin main"
		kinds := map[syntax.LangVariant]bool{}
		for _, lang := range dialects {
			f, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader(cmd), "")
			if err != nil || len(f.Stmts) != 1 {
				t.Fatalf("%v does not read %q: %v", lang, cmd, err)
			}
			_, kinds[lang] = f.Stmts[0].Cmd.(*syntax.CoprocClause)
		}
		if !kinds[syntax.LangBash] || kinds[syntax.LangZsh] {
			t.Fatalf("%q: a coprocess under bash %v, under zsh %v; want bash alone", cmd, kinds[syntax.LangBash], kinds[syntax.LangZsh])
		}
		row(t, line{cmd, "push"})
	})
	t.Run("6 rule 2: one dialect rejects the line", func(t *testing.T) {
		const cmd = `echo ${(U)x}`
		rejects := 0
		for _, lang := range dialects {
			if _, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader(cmd), ""); err != nil {
				rejects++
			}
		}
		if rejects != 1 {
			t.Fatalf("%d dialects reject %q, want exactly one", rejects, cmd)
		}
		row(t, line{cmd, "unreadable"})
		outside(t, line{cmd, ""})
	})
	t.Run("6 a panic in the parser", func(t *testing.T) {
		parse := parseTree
		defer func() { parseTree = parse }()
		// under every dialect: no tree, so the loose reading names the line
		parseTree = func(string, syntax.LangVariant) (*syntax.File, error) { panic("boom") }
		row(t, line{"echo ok", "unreadable"}, line{"git push origin main", "push"})
		outside(t, line{"echo ok", ""}, line{"git commit --amend", "amend"})
		door(t, line{"echo ok", "unreadable"})
		// under one dialect: the other's tree is judged, then the mark
		parseTree = func(cmd string, lang syntax.LangVariant) (*syntax.File, error) {
			if lang == syntax.LangZsh {
				panic("boom")
			}
			return parse(cmd, lang)
		}
		row(t, line{"echo ok", "unreadable"}, line{"git push origin main", "push"})
		door(t, line{"echo ok", "unreadable"})
	})
	t.Run("6 the bound on a re-read string", func(t *testing.T) {
		row(t,
			line{evalDeep, "unreadable"},
			line{shDeep, "unreadable"},
			line{"eval eval eval eval echo hi", ""},
		)
		outside(t, line{evalDeep, ""}, line{shDeep, ""})
	})

	// fix round 2: a line over maxLine is never parsed, so its nesting is
	// counted loosely. Past the depth bound it is unreadable at the
	// helper's door too, as on main; a long flat line is judged there by
	// its loose reading and the helper's guard. In a row each is unreadable
	t.Run("7 a line over maxLine at the helper's door", func(t *testing.T) {
		flat := "echo" + strings.Repeat(" a", 35<<10) // 70 KB of ordinary words
		long := []struct{ name, cmd, door, row string }{
			{`"$( x200000`, strings.Repeat(`"$(`, 200000), "unreadable", "unreadable"},
			{`$( x200000`, strings.Repeat("$(", 200000), "unreadable", "unreadable"},
			{"flat 70 KB", flat, "", "unreadable"},
			// in a row the guard that names this one answers before the mark
			{"flat 70 KB ; relay run", flat + " ; relay run", "helper-relay", "nested-run"},
		}
		for _, c := range long {
			if len(c.cmd) <= maxLine {
				t.Fatalf("%s: %d bytes, not over maxLine", c.name, len(c.cmd))
			}
			if got := helperSays(t, c.cmd, "a1", "", false); got != c.door {
				t.Errorf("at the helper's door, outside a row, %s: guard %q, want %q", c.name, got, c.door)
			}
			if got := verdictSays(c.cmd, true); got != c.row {
				t.Errorf("in a row %s: guard %q, want %q", c.name, got, c.row)
			}
		}
	})
}
