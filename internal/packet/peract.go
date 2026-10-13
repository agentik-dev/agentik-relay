package packet

import (
	"regexp"
	"strings"
)

// The per-act reading (AR61 act 2, ruling 3): a clause's text outside code
// spans names an act it repeats in (perActRE), and names none of the words
// that make it a limit instead (perActGuardRE).
var (
	perActRE      = regexp.MustCompile(`(?i)\b(each\s+act|every\s+act|per\s+act|after\s+an\s+act|act\s+gate)\b|\bacts\s+\d`)
	perActGuardRE = regexp.MustCompile(`(?i)(^|[^\p{L}'])(never|no|not|don't|once|last|final)([^\p{L}']|$)`)
)

// perActPlaces are the places OrdersPerAct reads, in order: the wall, then
// §2, §3, §5 and §6 (§1 and §4 quote), by index into Sections.
var perActPlaces = []struct {
	name    string
	section int // -1: the wall
}{{"wall", -1}, {"§2", 1}, {"§3", 2}, {"§5", 4}, {"§6", 5}}

// OrdersPerAct is the place, `wall` or `§N`, of the packet's first clause
// that orders line run per act (AR61 act 2); "" for none, or an empty
// line. It reads the wall, then §2, §3, §5 and §6, with text in double
// quotes outside code spans, closed on its line, dropped first. A clause
// ends at a newline, or outside a code span at `;` or at a `.` before a
// space or the end, except the `.` of `e.g.` or `i.e.`. A clause orders
// line when a code span in it holds line's words as a contiguous run
// (words starting with `-` dropped from both), its text outside the spans
// names each act, every act, per act, after an act, act gate, or acts
// before a number, case aside, and that text holds none of never, no,
// not, don't, once, last or final.
func (pk *Packet) OrdersPerAct(line string) string {
	want := runWords(line)
	if len(want) == 0 {
		return ""
	}
	for _, p := range perActPlaces {
		text := pk.Wall
		if p.section >= 0 {
			text = pk.Sections[p.section]
		}
		for _, l := range strings.Split(text, "\n") {
			for _, c := range clauses(dropQuotes(l)) {
				if c.orders(want) {
					return p.name
				}
			}
		}
	}
	return ""
}

// clause is one clause: its text outside code spans (each span a unit
// separator, so no phrase runs across one) and its spans.
type clause struct {
	text  string
	spans []string
}

func (c clause) orders(want []string) bool {
	if !perActRE.MatchString(c.text) || perActGuardRE.MatchString(strings.ReplaceAll(c.text, "’", "'")) {
		return false
	}
	for _, s := range c.spans {
		if holdsRun(runWords(s), want) {
			return true
		}
	}
	return false
}

// runWords is s's words with those starting with `-` dropped.
func runWords(s string) []string {
	var out []string
	for _, w := range strings.Fields(s) {
		if !strings.HasPrefix(w, "-") {
			out = append(out, w)
		}
	}
	return out
}

// holdsRun reports whether want is a contiguous run of have.
func holdsRun(have, want []string) bool {
	for i := 0; i+len(want) <= len(have); i++ {
		match := true
		for j, w := range want {
			if have[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// spanEnd is the index of the backtick closing a span opened at l[j], -1
// when none closes it on the line (the backtick is then text).
func spanEnd(l string, j int) int {
	if end := strings.IndexByte(l[j+1:], '`'); end >= 0 {
		return j + 1 + end
	}
	return -1
}

// quoteClose is the closing mark for an opening double quote at l[j], and
// the opening mark's width; "" when l[j] opens none.
func quoteClose(l string, j int) (string, int) {
	switch {
	case l[j] == '"':
		return `"`, 1
	case strings.HasPrefix(l[j:], "“"):
		return "”", len("“")
	}
	return "", 0
}

// dropQuotes drops from one line each text in double quotes outside code
// spans that closes on the line, the marks and any span inside included.
func dropQuotes(l string) string {
	var b strings.Builder
	for j := 0; j < len(l); j++ {
		if l[j] == '`' {
			if end := spanEnd(l, j); end >= 0 {
				b.WriteString(l[j : end+1])
				j = end
				continue
			}
		}
		if mark, w := quoteClose(l, j); mark != "" {
			if end := closeOutsideSpans(l, j+w, mark); end >= 0 {
				j = end + len(mark) - 1
				continue
			}
		}
		b.WriteByte(l[j])
	}
	return b.String()
}

// closeOutsideSpans is the index of mark in l at or after from, outside
// code spans; -1 when there is none.
func closeOutsideSpans(l string, from int, mark string) int {
	for j := from; j < len(l); j++ {
		if l[j] == '`' {
			if end := spanEnd(l, j); end >= 0 {
				j = end
				continue
			}
		}
		if strings.HasPrefix(l[j:], mark) {
			return j
		}
	}
	return -1
}

// clauses splits one line into clauses: outside code spans at `;`, and at
// a `.` before a space or the line's end, unless it closes `e.g.` or `i.e.`.
func clauses(l string) []clause {
	var out []clause
	var cur clause
	var text strings.Builder
	end := func() {
		cur.text = text.String()
		out = append(out, cur)
		cur, text = clause{}, strings.Builder{}
	}
	for j := 0; j < len(l); j++ {
		switch c := l[j]; {
		case c == '`' && spanEnd(l, j) >= 0:
			e := spanEnd(l, j)
			cur.spans = append(cur.spans, l[j+1:e])
			text.WriteByte('\x1f')
			j = e
		case c == ';':
			end()
		case c == '.' && (j+1 == len(l) || l[j+1] == ' ' || l[j+1] == '\t') && !abbrev(text.String()):
			end()
		default:
			text.WriteByte(c)
		}
	}
	end()
	return out
}

// abbrev reports whether text before a `.` ends `e.g` or `i.e` as a word.
func abbrev(text string) bool {
	low := strings.ToLower(text)
	for _, a := range []string{"e.g", "i.e"} {
		if strings.HasSuffix(low, a) {
			rest := low[:len(low)-len(a)]
			if rest == "" || !isLetter(rest[len(rest)-1]) {
				return true
			}
		}
	}
	return false
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
