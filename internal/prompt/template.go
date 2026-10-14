// Package prompt renders the child's stdin (§4): the Every session block
// when one is rendered (AR30), the RULES block, one CONTEXT line, the
// packet verbatim; and the hashes that make a changed template visible
// across nights.
package prompt

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
)

// rules is the stand-in's RULES with its NEXT sentence replaced (§4).
//
//go:embed rules.txt
var rules []byte

// contextFormat is the CONTEXT line; it is hashed with RULES, so any change
// to its wording is a new template.
const contextFormat = "CONTEXT: relay row %s; your working tree is %s on branch %s (off %s); your packet: %s; " +
	"your ceilings: %dk context (the relay asks you to stop at %dk and kills past %dk, a runaway), %d turns; " +
	"unmerged dependency branches (the owner merges): %s; the row's device: %s; " +
	"%sthe owner's words for this run: %s\n"

// VerifyOnceSeconds is the longest verify line, in seconds on main, the
// CONTEXT line tells a child to run once before its last commit (AR61 act
// 1): a tenth under the Bash tool's 600,000 ms foreground ceiling (its
// default is 120,000 ms, and a command at its timeout moves to the
// background: "Tools reference", code.claude.com, as of 2026-10-14);
// past it the line is the gate's alone.
const VerifyOnceSeconds = 540

// Context is what the CONTEXT line carries.
type Context struct {
	Row, Tree, Branch, Base string
	Unmerged                []string // "id at head"
	Device, Words           string
	Packet                  string // the packet's path, which act 0 copies in unless it lies inside the repo
	// The row's ceilings in tokens and turns (AR28): its context, the soft
	// line the relay asks a stop at, the kill line past it (a runaway's).
	Ceiling, Soft, Kill, Turns int
	// Verify is the repo's verify line and VerifySeconds its run on main,
	// rounded up, 0 when not timed (AR61 act 1); no line, no clause.
	Verify        string
	VerifySeconds int
}

// Rules is the RULES block as embedded.
func Rules() []byte { return append([]byte(nil), rules...) }

// TemplateHash is sha256 hex of RULES plus the CONTEXT format string.
func TemplateHash() string { return HashOf(rules) }

// TemplateHashFor is the template hash of a prompt opened by block (AR30):
// HashOf the block, its blank line, then RULES; no block, TemplateHash. A
// row's rules_hash so records which invariants it ran under.
func TemplateHashFor(block []byte) string {
	if block == nil {
		return TemplateHash()
	}
	b := append(append(append([]byte(nil), block...), '\n'), rules...)
	return HashOf(b)
}

// HashOf is the template hash a RULES text would give with this CONTEXT
// format; TemplateHash is HashOf the embedded RULES.
func HashOf(rules []byte) string {
	h := sha256.New()
	h.Write(rules)
	h.Write([]byte(contextFormat))
	return hex.EncodeToString(h.Sum(nil))
}

// Line is the rendered CONTEXT line, newline included.
func Line(c Context) string {
	unmerged := strings.Join(c.Unmerged, ", ")
	return fmt.Sprintf(contextFormat, c.Row, c.Tree, c.Branch, orNone(c.Base), orNone(c.Packet),
		c.Ceiling/1000, c.Soft/1000, c.Kill/1000, c.Turns, orNone(unmerged), orNone(c.Device), verifyClause(c), orNone(c.Words))
}

// verifyClause is CONTEXT's verify clause (AR61 act 1, ruling 2): the line
// and its seconds on main, run once before the last commit when they fit
// VerifyOnceSeconds, else the gate's alone; not timed (0) is no fit, since
// unknown is no fit; "" with no line.
func verifyClause(c Context) string {
	line := strings.TrimSpace(c.Verify)
	switch {
	case line == "":
		return ""
	case c.VerifySeconds <= 0:
		return fmt.Sprintf("the verify line: `%s`, not timed on main, the gate's alone; ", line)
	case c.VerifySeconds <= VerifyOnceSeconds:
		return fmt.Sprintf("the verify line: `%s`, %d s on main, once before your last commit; ", line, c.VerifySeconds)
	default:
		return fmt.Sprintf("the verify line: `%s`, %d s on main, the gate's alone; ", line, c.VerifySeconds)
	}
}

// Render is the child's stdin: the Every session block and a blank line
// when there is one (AR30), then RULES, a blank line, CONTEXT, a blank
// line, the packet verbatim. RulesHash is the template hash for the block
// (the spawn line's rules_hash); promptHash is sha256 hex of the whole stdin.
func Render(c Context, block, packet []byte) (stdin []byte, rulesHash, promptHash string) {
	var b strings.Builder
	if block != nil {
		b.Write(block)
		b.WriteString("\n")
	}
	b.Write(rules)
	b.WriteString("\n")
	b.WriteString(Line(c))
	b.WriteString("\n")
	b.Write(packet)
	stdin = []byte(b.String())
	sum := sha256.Sum256(stdin)
	return stdin, TemplateHashFor(block), hex.EncodeToString(sum[:])
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return s
}
