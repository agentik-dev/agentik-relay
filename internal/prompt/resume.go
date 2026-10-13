package prompt

import (
	"fmt"
	"strings"
)

// The RESUME block (usage-limit-resume r1 §2, AR68 ruling 1): what a
// continuation's child reads after its row's own prompt and a blank line.
// It lies outside the template hash: RULES and CONTEXT are the row's.

// The most commits and uncommitted paths the block lists.
const (
	resumeCommits = 20
	resumePaths   = 40
)

// resumeSentence is the record's sentence that closes the block.
const resumeSentence = "These files are yours; read them, keep what is sound, continue from the first act your commits do not cover.\n"

// Resume is what the block names: the part, its ceilings (turns and
// dollars), the last part's session, the commits since the cut (newest
// first, "<sha> <subject>") and the uncommitted paths.
type Resume struct {
	Part, Turns, USD int
	Session          string
	Commits, Paths   []string
}

// ResumeBlock is the RESUME block: one line naming the part, the last
// session and the part's ceilings; the commits (at most 20, then
// `(+<k> more)`) and the uncommitted paths (at most 40, likewise), each
// `none` when empty; then the record's sentence.
func ResumeBlock(r Resume) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RESUME: part %d; the last session %s stopped at the account's usage limit; this part's ceilings: %d turns, $%d.\n",
		r.Part, r.Session, r.Turns, r.USD)
	resumeList(&b, "The commits since the cut, newest first:", r.Commits, resumeCommits)
	resumeList(&b, "The uncommitted paths:", r.Paths, resumePaths)
	b.WriteString(resumeSentence)
	return b.String()
}

// resumeList writes head and items, one an indented line, at most most
// of them, then `(+<k> more)`; head and `none` when there are none.
func resumeList(b *strings.Builder, head string, items []string, most int) {
	if len(items) == 0 {
		b.WriteString(head + " none\n")
		return
	}
	b.WriteString(head + "\n")
	for _, it := range items[:min(len(items), most)] {
		b.WriteString("  " + it + "\n")
	}
	if k := len(items) - most; k > 0 {
		fmt.Fprintf(b, "  (+%d more)\n", k)
	}
}
