package prompt_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// pinTemplateHash is TemplateHash at AR68's pin (its first twelve hex):
// the RESUME block lies outside the template hash.
const pinTemplateHash = "a31ec7dddb07"

// TestAR68_1_ResumeBlock (AR68 ruling 1): the RESUME block names the
// part, the last session and the part's ceilings; lists at most 20
// commits and 40 uncommitted paths, then `(+<k> more)`; reads `none` for
// an empty list; ends in the record's sentence; and leaves TemplateHash
// as it was. A continuation's child reads it after the row's own prompt,
// its RELAY_HEAD_BEFORE the cut the commits are counted from.
func TestAR68_1_ResumeBlock(t *testing.T) {
	var commits, paths []string
	for i := 1; i <= 25; i++ {
		commits = append(commits, fmt.Sprintf("c%02d subject %d", i, i))
	}
	for i := 1; i <= 45; i++ {
		paths = append(paths, fmt.Sprintf("p%02d.go", i))
	}
	b := prompt.ResumeBlock(prompt.Resume{Part: 2, Turns: 60, USD: 5, Session: "s-part-1", Commits: commits, Paths: paths})
	first := strings.SplitN(b, "\n", 2)[0]
	if first != "RESUME: part 2; the last session s-part-1 stopped at the account's usage limit; this part's ceilings: 60 turns, $5." {
		t.Fatalf("the first line %q", first)
	}
	for _, want := range []string{"  c01 subject 1\n", "  c20 subject 20\n", "  p01.go\n", "  p40.go\n",
		"continue from the first act your commits do not cover."} {
		if !strings.Contains(b, want) {
			t.Fatalf("the block lacks %q:\n%s", want, b)
		}
	}
	for _, not := range []string{"c21", "c25", "p41", "p45", "none"} {
		if strings.Contains(b, not) {
			t.Fatalf("the block holds %q:\n%s", not, b)
		}
	}
	if n := strings.Count(b, "  (+5 more)\n"); n != 2 {
		t.Fatalf("`(+5 more)` %d times, want 2 (the commits', the paths'):\n%s", n, b)
	}
	if ci, pi := strings.Index(b, "c20"), strings.Index(b, "p01"); ci < 0 || pi < 0 || strings.Index(b[ci:pi], "(+5 more)") < 0 {
		t.Fatalf("the commits' `(+5 more)` is not between them and the paths:\n%s", b)
	}

	empty := prompt.ResumeBlock(prompt.Resume{Part: 3, Turns: 10, USD: 7, Session: "s-part-2"})
	for _, want := range []string{"RESUME: part 3; the last session s-part-2 ", "this part's ceilings: 10 turns, $7.",
		"The commits since the cut, newest first: none\n", "The uncommitted paths: none\n"} {
		if !strings.Contains(empty, want) {
			t.Fatalf("the empty block lacks %q:\n%s", want, empty)
		}
	}
	if strings.Contains(empty, "more)") {
		t.Fatalf("the empty block counts more:\n%s", empty)
	}

	if h := prompt.TemplateHash(); !strings.HasPrefix(h, pinTemplateHash) || h != prompt.HashOf(prompt.Rules()) {
		t.Fatalf("TemplateHash %s, want the pin's %s…: the RESUME block is outside it", h, pinTemplateHash)
	}
}
