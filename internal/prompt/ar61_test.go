package prompt_test

import (
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// mainTemplateHashAR61 is main's TemplateHash before AR61 (its first twelve hex).
const mainTemplateHashAR61 = "33b03bb2e87a"

// ar61Once is the RULES sentence AR61 act 1 adds, byte for byte (ruling 2).
const ar61Once = "Each act runs the tests of the packages it touched (with `-run` where the packet names tests) " +
	"and the repo's lint; the verify line the CONTEXT line names runs at most once, in the foreground, " +
	"before your last commit, never where CONTEXT calls it the gate's alone, even if the packet orders it " +
	"per act; never run it in the background or again after a time limit cuts it: the relay runs it after you exit."

// TestAR61_1_RulesVerifyOnce (AR61 act 1, ruling 2): RULES carries the
// verify-once sentence once, after "re-run by the relay after you exit."
// and before the soft-ceiling sentence; still one paragraph; a new
// template, whose hash the test logs.
func TestAR61_1_RulesVerifyOnce(t *testing.T) {
	r := string(prompt.Rules())
	if n := strings.Count(r, ar61Once); n != 1 {
		t.Fatalf("RULES carry the verify-once sentence %d times:\n%s", n, r)
	}
	if want := "re-run by the relay after you exit. " + ar61Once + " When a `relay: soft ceiling`"; !strings.Contains(r, want) {
		t.Fatalf("the verify-once sentence is not between its neighbours:\n%s", r)
	}
	if strings.Count(r, "\n") != 1 || !strings.HasSuffix(r, "\n") || strings.Contains(r, "\n\n") {
		t.Fatalf("RULES is not one paragraph ending in one newline: %q", r)
	}
	h := prompt.TemplateHash()
	if strings.HasPrefix(h, mainTemplateHashAR61) {
		t.Fatalf("TemplateHash %s is main's: the changed RULES and CONTEXT are not a new template", h)
	}
	t.Logf("template hash %s (main %s…)", h, mainTemplateHashAR61)
}

// TestAR61_2_ContextVerifyClause (AR61 act 1, ruling 2): CONTEXT names the
// verify line before the owner's words: run once at 300 s and at 540 s
// (VerifyOnceSeconds), the gate's alone past it and when not timed; no
// line, no clause, so an empty Context keeps five `none`.
func TestAR61_2_ContextVerifyClause(t *testing.T) {
	if prompt.VerifyOnceSeconds != 540 {
		t.Fatalf("VerifyOnceSeconds %d, want 540", prompt.VerifyOnceSeconds)
	}
	base := prompt.Context{Row: "r", Tree: "t", Branch: "b", Device: "never", Words: "w", Verify: "make check"}
	for _, c := range []struct {
		secs int
		want string
	}{
		{300, "the verify line: `make check`, 300 s on main, once before your last commit; "},
		{540, "the verify line: `make check`, 540 s on main, once before your last commit; "},
		{541, "the verify line: `make check`, 541 s on main, the gate's alone; "},
		{0, "the verify line: `make check`, not timed on main, the gate's alone; "},
	} {
		c2 := base
		c2.VerifySeconds = c.secs
		l := prompt.Line(c2)
		if want := "the row's device: never; " + c.want + "the owner's words for this run: w\n"; !strings.HasSuffix(l, want) {
			t.Fatalf("%d s: CONTEXT does not end %q:\n%s", c.secs, want, l)
		}
		if strings.Count(l, "the verify line: ") != 1 || strings.Count(l, "\n") != 1 {
			t.Fatalf("%d s: the clause is not once on one line:\n%s", c.secs, l)
		}
	}
	none := base
	none.Verify, none.VerifySeconds = "", 300
	if l := prompt.Line(none); strings.Contains(l, "verify line") || !strings.Contains(l, "the row's device: never; the owner's words") {
		t.Fatalf("no verify line carries a clause:\n%s", l)
	}
	if l := prompt.Line(prompt.Context{Row: "r", Tree: "t", Branch: "b"}); strings.Count(l, "none") != 5 || strings.Contains(l, "verify") {
		t.Fatalf("an empty Context does not keep five none: %q", l)
	}
}
