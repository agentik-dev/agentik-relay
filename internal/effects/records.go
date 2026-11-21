package effects

import (
	"strings"
	"unicode"
)

// RecordsKey is the clone's own git config key naming where a repo's
// working records live when they live outside its clone; DeclaredFile and
// DeclaredLine are the tracked file and the whole line that say they are
// out with no key (agentik-commons' agentik_records DECLARED).
const (
	RecordsKey   = "agentik.records"
	DeclaredFile = ".hygiene-allow"
	DeclaredLine = "records: out"
)

// RecordsOut is the one question of packet rr6-relay-records (R1), asked
// at a repo's root through the hermetic runner: true when `git config
// --local --path --get agentik.records` prints a value (exit 0, whatever
// the value and whether or not it resolves), or when DeclaredFile at HEAD
// holds DeclaredLine (Declares). A git failure reading either reads as the
// repo's own, as a repo with no key does. The runner's env is the pin's,
// so no GIT_CONFIG_* variable and no global file reaches it, and --local
// reads the clone's common config: a worktree sees the main checkout's key.
// No resolver: where the key points is never read.
func RecordsOut(root string) bool {
	if _, err := git(root, "config", "--local", "--path", "--get", RecordsKey); err == nil {
		return true
	}
	text, err := git(root, "cat-file", "-p", "HEAD:"+DeclaredFile)
	return err == nil && Declares(text)
}

// Declares reports whether text holds DeclaredLine as a whole row, read as
// commons' `_declares` reads it in Python: rows split as str.splitlines
// splits them, each compared with its trailing white space (str.rstrip's)
// stripped.
func Declares(text string) bool {
	for _, row := range strings.FieldsFunc(text, lineBreak) {
		if strings.TrimRightFunc(row, pySpace) == DeclaredLine {
			return true
		}
	}
	return false
}

// lineBreak is a line boundary of Python's str.splitlines.
func lineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', ' ', ' ':
		return true
	}
	return false
}

// pySpace is white space as Python's str.isspace reads it: Unicode's, and
// the four information separators.
func pySpace(r rune) bool {
	return unicode.IsSpace(r) || r >= '\x1c' && r <= '\x1f'
}
