package prompt

import (
	"errors"
	"strings"
)

// The markers the ledger's render verb writes around the Every session
// block in a CLAUDE.md (its render.py), matched on a trimmed line.
const (
	BlockBegin = "<!-- BEGIN GENERATED: decisions -->"
	BlockEnd   = "<!-- END GENERATED: decisions -->"
)

// ErrBlockMarkers is markers not one BEGIN before one END: a half-read
// block would drop invariants, so it is refused, never guessed.
var ErrBlockMarkers = errors.New("the block's markers are not one BEGIN before one END")

// Block is the generated block of a CLAUDE.md: the marker lines
// and every line between, verbatim, each ending in a newline. No marker at
// all is nil; any other shape but one BEGIN before one END is an error.
func Block(claudeMD []byte) ([]byte, error) {
	lines := strings.SplitAfter(string(claudeMD), "\n")
	begin, end, begins, ends := -1, -1, 0, 0
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case BlockBegin:
			begin, begins = i, begins+1
		case BlockEnd:
			end, ends = i, ends+1
		}
	}
	if begins == 0 && ends == 0 {
		return nil, nil
	}
	if begins != 1 || ends != 1 || end < begin {
		return nil, ErrBlockMarkers
	}
	out := strings.Join(lines[begin:end+1], "")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}
