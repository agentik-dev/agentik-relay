// Package packet parses a packet (design/relay-r1.md §2): the title, the
// six-row header table and the six numbered sections. It refuses anything
// else in that frame; prose is never validated.
package packet

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/agentik-dev/agentik-relay/internal/hooks"
)

// Ceiling is the packet size limit in bytes (conventions §5): a packet is
// under it, as doclint reads it (ADV6-16).
const Ceiling = 9216

// Rows are the header table's rows, in order.
var Rows = [6]string{"from", "pin", "wall", "may touch", "acceptance", "budget"}

// VerifyRow is the optional row after Rows: one code span, the gate line.
const VerifyRow = "verify"

// SectionTitles are the sections' title prefixes, in order.
var SectionTitles = [6]string{"What the source says", "Rulings", "Interfaces",
	"Findings that name this row", "Forbidden reads", "Report"}

// Packet is one parsed packet.
type Packet struct {
	ID, Title, From, Pin, Wall string
	MayTouch                   []Glob
	Acceptance                 []Line
	// Verify is the optional seventh row's one code span, the gate line
	// (conventions §5, §9); empty when the row is absent. Nothing reads it
	// yet: the queue's `verify: auto` resolves the gate line.
	Verify              string
	Turns, Context, USD int
	Sections            [6]string
	Bytes               int
}

// Glob is one may-touch span: a path or a per-segment glob; Dir when it ends
// in `/` and so covers everything below.
type Glob struct {
	Raw  string
	Segs []string
	Dir  bool
}

// Match reports whether path lies inside the glob: `*` stays inside one
// segment (as `.hygiene-allow`), and a Dir glob matches any deeper path.
func (g Glob) Match(p string) bool {
	segs := strings.Split(strings.TrimPrefix(p, "./"), "/")
	if g.Dir {
		if len(segs) <= len(g.Segs) {
			return false
		}
	} else if len(segs) != len(g.Segs) {
		return false
	}
	for i, pat := range g.Segs {
		if ok, err := path.Match(pat, segs[i]); err != nil || !ok {
			return false
		}
	}
	return true
}

// ExpectKind is an acceptance line's expectation.
type ExpectKind int

const (
	Reads       ExpectKind = iota // the last non-empty stdout line equals Value
	ReadsOrMore                   // the last line is a number, N or more
	Under                         // the last line is a number under N
	Exits                         // the exit status is N
	AsMain                        // equal to the recorded baseline
)

func (k ExpectKind) String() string {
	return [...]string{"reads", "reads-or-more", "under", "exits", "as-main"}[k]
}

// Line is one acceptance item: the shell line and what it must show.
type Line struct {
	Shell, Expect string
	Kind          ExpectKind
	Value         string
	N             int
}

var (
	titleRE   = regexp.MustCompile(`^# Packet ([^:\s]+): (.+)$`)
	sectionRE = regexp.MustCompile(`^## (\d+)\. (.+)$`)
	budgetRE  = regexp.MustCompile(`^(\d+) turns, (\d+)k context(?:, \$(\d+))?`)
	orMoreRE  = regexp.MustCompile(`^reads (\d+) or more\b`)
	readsRE   = regexp.MustCompile("^reads (`[^`]*`|\\S+)")
	numRE     = regexp.MustCompile(`^(under|exits) (\d+)\b`)
	asMainRE  = regexp.MustCompile(`^as main\b`)
)

// Parse parses the packet at path (the path names the ID: its file stem).
func Parse(p string, src []byte) (*Packet, error) {
	pk := &Packet{ID: strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), Bytes: len(src)}
	if len(src) >= Ceiling {
		return nil, fmt.Errorf("packet %s: %d bytes, at or over the %d-byte ceiling", pk.ID, len(src), Ceiling)
	}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i == len(lines) {
		return nil, fmt.Errorf("packet %s: title: want `# Packet <ID>: <title>`", pk.ID)
	}
	m := titleRE.FindStringSubmatch(lines[i])
	if m == nil {
		return nil, fmt.Errorf("packet %s: title: want `# Packet <ID>: <title>`", pk.ID)
	}
	stemHead, _, _ := strings.Cut(pk.ID, "-")
	if !strings.EqualFold(m[1], pk.ID) && !strings.EqualFold(m[1], stemHead) {
		return nil, fmt.Errorf("packet %s: title: id %s does not name the file stem", pk.ID, m[1])
	}
	pk.Title = m[2]
	i++
	cells, next, err := table(pk.ID, lines, i)
	if err != nil {
		return nil, err
	}
	if err := pk.header(cells); err != nil {
		return nil, fmt.Errorf("packet %s: %w", pk.ID, err)
	}
	if err := pk.sections(lines[next:]); err != nil {
		return nil, fmt.Errorf("packet %s: %w", pk.ID, err)
	}
	if !legacy[pk.ID] && !spent[pk.ID] {
		for k, l := range pk.Acceptance {
			if why := Unbounded(l.Shell); why != "" {
				return nil, fmt.Errorf("packet %s: row acceptance: item %d (`%s`): unbounded: %s", pk.ID, k+1, l.Shell, why)
			}
		}
	}
	return pk, nil
}

// legacy names the seventeen packets cut before AR10 by their stems
// (design/packets/ before 08ceb7c): records, parsed as written. §15's
// bounded-line rule binds every other id, a future one sharing a prefix too
// (ADV6-12).
var legacy = map[string]bool{
	"a3-machine": true, "a4-schemas": true, "a5-effects-plan": true, "a6-stage-gate": true,
	"a7-spawn-stream": true, "a8-night": true, "a9-tmux-notify": true, "ap1-plugin": true,
	"ux1-review": true, "ar2-adversarial": true, "ar3-readme": true, "ar4-fixes": true,
	"ar5-ux": true, "ar6-sync-and-readme": true, "ar7-fixes": true, "ar8-resync": true,
	"ar9-queue-and-resync": true,
}

// spent are the records cut after AR10 whose acceptance a later rule
// refuses: parsed as written, their ids spent, the records never rewritten.
// ar11 and ar12 ran a suite target (`make tla`, `make e2e`) before AR17C
// closed the rule by shape; the rest run a verb or a spelling off AR20's
// allowlist (ADV11-10: a `for` loop, `$(...)`, `shasum`, `make doclint`,
// `awk`, `xmllint`, `go test -tags`). ar20 names several packages on a `go
// test` line, one each since AR21 act 4 (ADV12-6).
var spent = map[string]bool{
	"ar11-tla-night": true, "ar12-e2e-night": true,
	"ar10-bounded-night": true, "ar13-fixes-r6": true, "ar14-resync": true, "ar15-readme-r7": true,
	"ar17a-gate-r10": true, "ar17b-guards-r10": true, "ar17c-bounds-r10": true, "ar17d-rulings-r10": true,
	"ar18-resync": true, "ar19-resync": true, "ar20-fixes-adv11": true,
}

// allowlist names the acceptance line's verbs (AR20 act 5, ADV11-10): `go
// test` and the read-only verbs. Every other command word is refused.
const allowlist = "go test, grep, test, wc, cut, sed -n, git log|show|diff|rev-parse, cat, paste, bc, tail, head, echo"

// Unbounded is why an acceptance line is not targeted (§15), "" when it
// is. It is an allowlist (AR20 act 5, ADV11-10), not a list of spellings:
// no substitution (`$(`, a backtick, `<(`, `>(`) anywhere; no redirection
// but to /dev/null or an fd; no background `&`; and every simple command
// (hooks.Segments: each side of `;`, `&&`, `||`, `|` and a subshell) is,
// by its literal first word with no env prefix or wrapper, `go test` of
// one named package with a -run that does not match every test (AR21 act
// 4), or a read-only verb (grep, test, wc, cut, `sed -n` printing, `git
// log|show|diff|rev-parse`, cat, paste, bc, tail, head, echo) whose
// operands name no device but /dev/null. A `$'`, a `<<` and a `${` outside
// single quotes are refused before any reading (AR21 act 3, AR43 act 3):
// each moves the end of a quote, and the line's readers must agree with the
// shell on where every quote ends.
func Unbounded(line string) string {
	if outsideSingle(line, "$'") {
		// AR21 act 3 (ADV12-4): its escapes read a quote no other reader here
		// closes, so it is refused, not read
		return "an ANSI-C quote (`$'...'`) is refused: its escapes hide where it ends"
	}
	if outsideSingle(line, "<<") {
		// AR43 act 3: a quoted delimiter is a quote the readers here do not
		// read, and a body or a here-string is input the line never names
		return "a heredoc or a here-string (`<<`) is refused: its delimiter hides where a quote ends"
	}
	if outsideSingle(line, "${") {
		// AR43 act 3: a quote inside it nests in a double-quoted span, and
		// the shells differ on a `'` there
		return "a parameter expansion (`${`) is refused: a quote inside it hides where a quoted span ends"
	}
	flat, why := redirections(line)
	if why != "" {
		return why
	}
	for _, words := range hooks.Segments(flat) {
		if why := allowed(words); why != "" {
			return why
		}
	}
	return ""
}

// outsideSingle is whether line has mark (`$'`, `<<` or `${`) starting
// outside a single-quoted span (AR42 act 1, grown by AR43 act 3): inside
// `'...'` every byte is literal, so the `$` that ends `-run '^X$'` opens
// nothing, and `grep -c '<<' f` holds no heredoc. One scan, three states.
// Unquoted: a backslash takes the next byte with it, `'` and `"` open a
// span. Single-quoted: literal up to the next `'`. Double-quoted: a
// backslash takes the next byte with it, `"` closes, `'` opens nothing. It
// errs toward refusal: an unquoted backslash before the mark (`\$'`) is
// refused too, and so is the mark inside double quotes.
func outsideSingle(line, mark string) bool {
	const (
		unquoted = iota
		single
		double
	)
	state := unquoted
	for i := 0; i < len(line); i++ {
		c := line[i]
		if state == single {
			if c == '\'' {
				state = unquoted
			}
			continue
		}
		switch {
		case strings.HasPrefix(line[i:], mark):
			return true
		case c == '\\':
			if state == unquoted && strings.HasPrefix(line[i+1:], mark) {
				return true
			}
			i++
		case c == '"' && state == unquoted:
			state = double
		case c == '"':
			state = unquoted
		case c == '\'' && state == unquoted:
			state = single
		}
	}
	return false
}

// redirections is line with its redirections to /dev/null and fd copies
// (`2>&1`) blanked, or why a redirection, a substitution or a background
// `&` outside quotes is refused.
func redirections(line string) (string, string) {
	if strings.Contains(line, "`") || strings.Contains(line, "$(") ||
		strings.Contains(line, "<(") || strings.Contains(line, ">(") {
		return "", "a substitution (`$(...)`, a backtick, `<(...)`) runs a command the line never names"
	}
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch c {
		case '\\':
			b.WriteByte(c)
			if i+1 < len(line) {
				i++
				b.WriteByte(line[i])
			}
		case '\'', '"':
			j := i + 1
			for j < len(line) && line[j] != c {
				if c == '"' && line[j] == '\\' {
					j++
				}
				j++
			}
			b.WriteString(line[i:min(j+1, len(line))])
			i = j
		case '&':
			if i+1 < len(line) && line[i+1] == '&' {
				b.WriteString("&&")
				i++
				continue
			}
			return "", "a background `&` runs a command past the line's end"
		case '>':
			// the fd before it is part of the redirection
			s := b.String()
			k := len(s)
			for k > 0 && s[k-1] >= '0' && s[k-1] <= '9' {
				k--
			}
			if k == 0 || s[k-1] == ' ' || s[k-1] == '\t' {
				b.Reset()
				b.WriteString(s[:k])
			}
			j := i + 1
			if j < len(line) && line[j] == '>' {
				j++
				// AR43's fix round: zsh reads `>>&1` as an append to a file
				// named `1`, so `>>&` is never an fd copy
				if j < len(line) && line[j] == '&' {
					return "", "`>>&` writes a file (zsh appends to a file named by the digits after it)"
				}
			}
			if j < len(line) && line[j] == '&' {
				d := j + 1
				for d < len(line) && line[d] >= '0' && line[d] <= '9' {
					d++
				}
				// AR43 act 3: digits that run on into a word (`>&2x`) name a
				// file, so the fd ends at the line's end, a blank or an operator
				if d == j+1 || d < len(line) && !strings.ContainsRune(" \t;|&)", rune(line[d])) {
					return "", "`>&` without an fd writes a file"
				}
				j = d
			} else {
				for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
					j++
				}
				rest := line[j:]
				if !strings.HasPrefix(rest, "/dev/null") ||
					len(rest) > len("/dev/null") && !strings.ContainsRune(" \t;|)", rune(rest[len("/dev/null")])) {
					return "", "a redirection to a file writes it (only /dev/null and fd copies are read-only)"
				}
				j += len("/dev/null")
			}
			b.WriteByte(' ')
			i = j - 1
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), ""
}

// allowed is why one simple command is off the allowlist, "" when it is on.
func allowed(words []string) string {
	w, args := words[0], words[1:]
	if eq := strings.IndexByte(w, '='); eq > 0 && !strings.HasPrefix(w, "-") {
		return "an env prefix (`" + w + "`) reshapes the command it prefixes"
	}
	if readVerb[w] {
		for _, a := range args {
			if devOperand(a) {
				return "`" + w + " " + a + "` reads a device (only /dev/null): it may never end"
			}
		}
	}
	switch w {
	case "grep", "test", "[", "wc", "cut", "cat", "paste", "bc", "head", "echo":
		return ""
	case "tail":
		for _, a := range args {
			if strings.HasPrefix(a, "--f") || strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "fF") {
				return "`tail -f` never ends"
			}
		}
		return ""
	case "sed":
		return sedPrint(args)
	case "git":
		return gitRead(args)
	case "go":
		if len(args) > 0 && args[0] == "test" {
			return goTest(args[1:])
		}
	}
	return "`" + w + "` is off the acceptance allowlist (" + allowlist + ")"
}

// readVerb are the allowlist's verbs that open their operands (AR21 act 4,
// ADV12-10): `test`, `[` and `echo` never do, and `go test` names packages.
var readVerb = map[string]bool{
	"grep": true, "wc": true, "cut": true, "cat": true, "paste": true, "bc": true,
	"head": true, "tail": true, "sed": true, "git": true,
}

// devOperand reports whether a read verb's word may name a path under
// /dev/ other than /dev/null (AR21 act 4, ADV12-10: `cat /dev/zero` never
// ends). It reads the word as a path, an input redirection's target
// (`<f`), a long option's value (`--file=f`) or a short option's attached
// value (`-ff`), cleaned: a first component that may spell `dev` (a glob
// too) after the root, `..` or a variable (which may be empty) is a device.
// A relative `dev/` inside the tree passes. It is a reader's rule, not a
// shell: a symlink in the tree to a device is bounded by the line timeout.
func devOperand(word string) bool {
	w := strings.TrimLeft(word, "0123456789")
	if strings.HasPrefix(w, "<") {
		word = strings.TrimLeft(w, "<")
	}
	var cands []string
	switch {
	case strings.HasPrefix(word, "--"):
		if _, v, ok := strings.Cut(word, "="); ok {
			cands = append(cands, v)
		}
	case strings.HasPrefix(word, "-"):
		if k := strings.IndexAny(word, "/.~$"); k > 0 {
			cands = append(cands, word[k:])
		}
	default:
		cands = append(cands, word)
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		clean := path.Clean(c)
		if clean == "/dev/null" {
			continue
		}
		comps := strings.Split(clean, "/")
		k, rooted := 0, false
		for k < len(comps) && (comps[k] == "" || comps[k] == ".." || strings.HasPrefix(comps[k], "$")) {
			rooted = true
			k++
		}
		if !rooted || k == len(comps) {
			continue
		}
		if ok, err := path.Match(comps[k], "dev"); ok || err != nil {
			return true
		}
	}
	return false
}

var (
	sedAddr    = `(?:\d+|\$|/(?:[^/\\]|\\.)*/)`
	sedPrintRE = regexp.MustCompile(`^\s*(?:` + sedAddr + `(?:\s*,\s*` + sedAddr + `)?)?\s*!?\s*p\s*$`)
)

// sedPrint is why `sed <args>` is not `sed -n` printing lines, "" when it
// is: -n given, no option but -n, -E, -r and -e, and every script an
// address (a line, `$`, a /regex/, or a range of them) and `p`.
func sedPrint(args []string) string {
	quiet := false
	var scripts, rest []string
	for k := 0; k < len(args); k++ {
		a := args[k]
		switch {
		case a == "--quiet" || a == "--silent":
			quiet = true
		case a == "-e" && k+1 < len(args):
			scripts = append(scripts, args[k+1])
			k++
		case strings.HasPrefix(a, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(a, "--expression="))
		case strings.HasPrefix(a, "-") && len(a) > 1:
			if strings.Trim(a[1:], "nEr") != "" {
				return "`sed " + a + "` is off the allowlist (only `sed -n` printing)"
			}
			quiet = quiet || strings.Contains(a, "n")
		default:
			rest = append(rest, a)
		}
	}
	if len(scripts) == 0 && len(rest) > 0 {
		scripts = rest[:1]
	}
	if !quiet || len(scripts) == 0 {
		return "`sed` without -n is off the allowlist (only `sed -n` printing)"
	}
	for _, s := range scripts {
		if !sedPrintRE.MatchString(s) {
			return "`sed -n '" + s + "'` does more than print lines"
		}
	}
	return ""
}

// gitRead is why `git <args>` is not a read-only verb, "" when it is:
// log, show, diff or rev-parse with no global option before it, and never
// an output file or an external diff.
func gitRead(args []string) string {
	if len(args) == 0 {
		return "`git` wants a verb: log, show, diff or rev-parse"
	}
	switch args[0] {
	case "log", "show", "diff", "rev-parse":
	default:
		return "`git " + args[0] + "` is off the allowlist (git log, show, diff, rev-parse)"
	}
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "--output") || a == "--ext-diff" || a == "--textconv" {
			return "`git " + args[0] + " " + a + "` writes a file or runs a program"
		}
	}
	return ""
}

// goTestValued and goTestBool are the `go test` flags an acceptance line
// may give; every other flag (-exec, -tags, -o, -args, -bench ...) is
// refused.
var (
	goTestValued = map[string]bool{"run": true, "count": true, "timeout": true}
	goTestBool   = map[string]bool{"v": true, "race": true, "short": true, "failfast": true, "cover": true, "json": true}
	varRE        = regexp.MustCompile(`\$[A-Za-z_{0-9?@*#!-]`)
)

// goTest is why `go test <toks>` is not targeted, "" when it is: exactly
// one named package (never `...`, all, std, cmd or a variable), a -run
// that matches no test named at random, and flags off goTestValued and
// goTestBool.
func goTest(toks []string) string {
	pkgs, runs := 0, 0
	for k := 0; k < len(toks); k++ {
		tok := toks[k]
		if !strings.HasPrefix(tok, "-") {
			switch {
			case strings.Contains(tok, "..."):
				return "`go test` with `...` is the whole tree below it"
			case tok == "all" || tok == "std" || tok == "cmd":
				return "`go test " + tok + "` is every package"
			case strings.ContainsAny(tok, "$*?[{<>"):
				return "`go test " + tok + "` names no package"
			}
			pkgs++
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(tok, "-"), "=")
		switch {
		case goTestBool[name]:
		case goTestValued[name]:
			if !hasVal {
				if k+1 == len(toks) {
					return "`go test -" + name + "` wants a value"
				}
				k++
				val = toks[k]
			}
			if name == "run" {
				if why := runPattern(val); why != "" {
					return why
				}
				runs++
			}
		default:
			return "`go test " + tok + "` is off the allowlist (-run, -count, -timeout, -v, -race, -short, -failfast, -cover, -json)"
		}
	}
	if pkgs == 0 || runs == 0 {
		return "`go test` wants a named package and -run"
	}
	if pkgs > 1 {
		// AR21 act 4 (ADV12-6): a -run narrow in one package is the suite
		// across several
		return fmt.Sprintf("`go test` names %d packages; a line names one", pkgs)
	}
	return ""
}

// runProbes are test names no targeted pattern matches: a -run that
// matches one matches every test.
// AR21 act 4 (ADV12-6): the probes spell each first letter a test name may
// take after its prefix (`Test[A-Z]`, `^(Test|Example)[A-Z0-9_]`).
var runProbes = []string{"Testqzvq", "Exampleqzvq", "Fuzzqzvq",
	"TestZqzvq", "Test_qzvq", "Test9qzvq", "ExampleZqzvq", "FuzzZqzvq"}

// runPattern is why a -run pattern matches every test, "" when it does
// not. Only its top level (before the first `/`) picks the tests.
func runPattern(pat string) string {
	if varRE.MatchString(pat) {
		return "`-run " + pat + "` is a variable, any pattern"
	}
	top, _, _ := strings.Cut(pat, "/")
	re, err := regexp.Compile(top)
	if err != nil {
		return "`-run " + pat + "` is not a pattern: " + err.Error()
	}
	for _, p := range runProbes {
		if re.MatchString(p) {
			return "`-run " + pat + "` matches every test"
		}
	}
	return ""
}

// table reads the header table after the title: `| item | value |`, the rule
// row, then the rows. It returns each row's value by name, in order.
func table(id string, lines []string, i int) ([][2]string, int, error) {
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i+1 >= len(lines) || !strings.HasPrefix(lines[i], "|") || !strings.HasPrefix(lines[i+1], "|") {
		return nil, 0, fmt.Errorf("packet %s: header table missing after the title", id)
	}
	var rows [][2]string
	for i += 2; i < len(lines) && strings.HasPrefix(lines[i], "|"); i++ {
		c := splitRow(lines[i])
		name := strings.ToLower(strings.TrimSpace(c[0]))
		if len(c) != 2 {
			return nil, 0, fmt.Errorf("packet %s: row %s: %d cells, want 2: an unescaped | inside a span splits the row", id, name, len(c))
		}
		rows = append(rows, [2]string{name, strings.TrimSpace(c[1])})
	}
	return rows, i, nil
}

// splitRow splits a table row on the pipes a markdown table splits on: every
// `|` not escaped as `\|`.
func splitRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, `\|`) {
		s = s[:len(s)-1]
	}
	var out []string
	start := 0
	for j := 0; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == '|' {
			out = append(out, s[start:j])
			start = j + 1
		}
	}
	return append(out, s[start:])
}

func unescape(s string) string { return strings.ReplaceAll(s, `\|`, "|") }

func (pk *Packet) header(rows [][2]string) error {
	if n := len(rows); n > 0 && rows[n-1][0] == VerifyRow {
		if err := pk.verify(rows[n-1][1]); err != nil {
			return err
		}
		rows = rows[:n-1]
	}
	got := map[string]string{}
	for _, r := range rows {
		if r[0] == VerifyRow {
			return fmt.Errorf("row %s: out of order, want it last, after row budget", VerifyRow)
		}
		if !contains(Rows[:], r[0]) {
			return fmt.Errorf("row %s: extra header row", r[0])
		}
		if _, dup := got[r[0]]; dup {
			return fmt.Errorf("row %s: duplicate header row", r[0])
		}
		got[r[0]] = r[1]
	}
	for _, name := range Rows {
		if _, ok := got[name]; !ok {
			return fmt.Errorf("row %s: missing header row", name)
		}
		if got[name] == "" {
			return fmt.Errorf("row %s: empty", name)
		}
	}
	for k, r := range rows {
		if r[0] != Rows[k] {
			return fmt.Errorf("row %s: out of order, want row %s here", r[0], Rows[k])
		}
	}
	pk.From, pk.Pin, pk.Wall = unescape(got["from"]), unescape(got["pin"]), unescape(got["wall"])
	var err error
	if pk.MayTouch, err = mayTouch(got["may touch"]); err != nil {
		return err
	}
	if pk.Acceptance, err = acceptance(got["acceptance"]); err != nil {
		return err
	}
	m := budgetRE.FindStringSubmatch(got["budget"])
	// RE2 has no lookahead: a dollar the pattern left unread (`$15.50`) is
	// refused here, never read as its whole part.
	if m != nil {
		rest := got["budget"][len(m[0]):]
		if strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, ", $") {
			m = nil
		}
	}
	if m == nil {
		return fmt.Errorf("row budget: want `<n> turns, <n>k context` with an optional `, $<n>`")
	}
	pk.Turns, _ = strconv.Atoi(m[1])
	pk.Context, _ = strconv.Atoi(m[2])
	pk.Context *= 1000
	if m[3] != "" {
		pk.USD, _ = strconv.Atoi(m[3])
	}
	return nil
}

// verify keeps the verify row's cell when it is exactly one code span.
func (pk *Packet) verify(cell string) error {
	var n int
	var span string
	err := spans(cell, "", func(s string, _ int) { n++; span = s })
	if err != nil || n != 1 || strings.TrimSpace(cell) != "`"+span+"`" || strings.TrimSpace(span) == "" {
		return fmt.Errorf("row %s: want one code span, the gate line", VerifyRow)
	}
	pk.Verify = unescape(span)
	return nil
}

func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// spans walks s's backtick spans, reporting each with the parenthesis depth
// of the prose around it; the walk stops at stop, when stop is non-empty and
// found outside a span.
func spans(s, stop string, f func(span string, depth int)) error {
	depth := 0
	for j := 0; j < len(s); j++ {
		switch {
		case s[j] == '`':
			end := strings.IndexByte(s[j+1:], '`')
			if end < 0 {
				return fmt.Errorf("unterminated backtick span")
			}
			f(s[j+1:j+1+end], depth)
			j += end + 1
		case s[j] == '(':
			depth++
		case s[j] == ')' && depth > 0:
			depth--
		case stop != "" && strings.HasPrefix(s[j:], stop):
			return nil
		}
	}
	return nil
}

// mayTouch takes the spans outside parentheses, up to "Nothing else"; a span
// inside parentheses is commentary on the path before it.
func mayTouch(cell string) ([]Glob, error) {
	var globs []Glob
	var bad error
	err := spans(cell, "Nothing else", func(span string, depth int) {
		if depth > 0 || bad != nil {
			return
		}
		raw := unescape(span)
		if strings.Contains(raw, "**") {
			bad = fmt.Errorf("row may touch: span %q holds **, which never matches", raw)
			return
		}
		g := Glob{Raw: raw, Dir: strings.HasSuffix(raw, "/")}
		g.Segs = strings.Split(strings.TrimSuffix(raw, "/"), "/")
		globs = append(globs, g)
	})
	if err == nil {
		err = bad
	}
	if err != nil {
		return nil, fmt.Errorf("row may touch: %w", err)
	}
	if len(globs) == 0 {
		return nil, fmt.Errorf("row may touch: no path span")
	}
	return globs, nil
}

// acceptance splits the cell on `;` outside spans; each item is a shell span
// and an expectation.
func acceptance(cell string) ([]Line, error) {
	var items []string
	start, in := 0, false
	for j := 0; j < len(cell); j++ {
		switch {
		case cell[j] == '`':
			in = !in
		case cell[j] == ';' && !in:
			items = append(items, cell[start:j])
			start = j + 1
		}
	}
	if in {
		return nil, fmt.Errorf("row acceptance: unterminated backtick span")
	}
	items = append(items, cell[start:])
	var out []Line
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		n := len(out) + 1
		if !strings.HasPrefix(it, "`") {
			return nil, fmt.Errorf("row acceptance: item %d: no shell span", n)
		}
		end := strings.IndexByte(it[1:], '`') + 1
		l := Line{Shell: unescape(it[1:end]), Expect: strings.TrimSpace(it[end+1:])}
		if err := l.expect(); err != nil {
			return nil, fmt.Errorf("row acceptance: item %d (`%s`): %w", n, l.Shell, err)
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("row acceptance: no item")
	}
	return out, nil
}

func (l *Line) expect() error {
	e := l.Expect
	if m := orMoreRE.FindStringSubmatch(e); m != nil {
		l.Kind, l.Value = ReadsOrMore, m[1]
		l.N, _ = strconv.Atoi(m[1])
		return nil
	}
	if m := readsRE.FindStringSubmatch(e); m != nil {
		l.Kind, l.Value = Reads, unescape(strings.Trim(m[1], "`"))
		if n, err := strconv.Atoi(l.Value); err == nil {
			l.N = n
		}
		return nil
	}
	if m := numRE.FindStringSubmatch(e); m != nil {
		l.Kind = map[string]ExpectKind{"under": Under, "exits": Exits}[m[1]]
		l.Value = m[2]
		l.N, _ = strconv.Atoi(m[2])
		return nil
	}
	if asMainRE.MatchString(e) {
		l.Kind = AsMain
		return nil
	}
	return fmt.Errorf("no expectation (want reads, reads <n> or more, under, exits or as main)")
}

// sections checks the six `## N. <title>` headings, in order, outside code
// fences, and keeps each one's body.
func (pk *Packet) sections(lines []string) error {
	type head struct {
		num   int
		title string
		at    int
	}
	var heads []head
	fence := false
	for j, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if fence || !strings.HasPrefix(l, "## ") {
			continue
		}
		m := sectionRE.FindStringSubmatch(l)
		if m == nil {
			return fmt.Errorf("section %q: extra section (want `## N. <title>`)", strings.TrimPrefix(l, "## "))
		}
		n, _ := strconv.Atoi(m[1])
		heads = append(heads, head{n, m[2], j})
	}
	titled := func(t string) bool {
		for _, h := range heads {
			if strings.HasPrefix(h.title, t) {
				return true
			}
		}
		return false
	}
	for k, h := range heads {
		if k < len(SectionTitles) && h.num == k+1 && strings.HasPrefix(h.title, SectionTitles[k]) {
			continue
		}
		known := false
		for _, t := range SectionTitles {
			known = known || strings.HasPrefix(h.title, t)
		}
		switch {
		case !known || k >= len(SectionTitles):
			return fmt.Errorf("section %d. %s: extra section", h.num, h.title)
		case titled(SectionTitles[k]):
			return fmt.Errorf("section %d. %s: out of order, want %d. %s here", h.num, h.title, k+1, SectionTitles[k])
		default:
			return fmt.Errorf("section %d. %s: missing", k+1, SectionTitles[k])
		}
	}
	if len(heads) < len(SectionTitles) {
		return fmt.Errorf("section %d. %s: missing", len(heads)+1, SectionTitles[len(heads)])
	}
	for k, h := range heads {
		end := len(lines)
		if k+1 < len(heads) {
			end = heads[k+1].at
		}
		pk.Sections[k] = strings.TrimSpace(strings.Join(lines[h.at+1:end], "\n"))
	}
	return nil
}
