package prompt_test

import (
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// mainTemplateHashAR28 is main's TemplateHash before AR28 (its first twelve hex).
const mainTemplateHashAR28 = "6fe94244b9b0"

// TestAR28_2_ContextNamesCeilings: the CONTEXT line names the row's
// ceilings after the packet clause (130k, the stop asked at 117k, the kill
// past 156k, 100 turns); RULES closes on the soft note and names no 180k;
// a new template.
func TestAR28_2_ContextNamesCeilings(t *testing.T) {
	c := prompt.Context{Row: "ar28", Tree: "/r/wt", Branch: "ar28", Base: "main", Packet: "design/packets/ar28.md",
		Ceiling: 130000, Soft: 117000, Kill: 156000, Turns: 100}
	want := "your packet: design/packets/ar28.md; your ceilings: 130k context (the relay asks you to stop at 117k " +
		"and kills past 156k, a runaway), 100 turns; unmerged"
	if l := prompt.Line(c); !strings.Contains(l, want) {
		t.Fatalf("CONTEXT does not name the ceilings after the packet clause:\n%s", l)
	}
	r := string(prompt.Rules())
	last := "When a `relay: soft ceiling` note follows a tool, or past the soft line or 90% of the turns the CONTEXT line " +
		"names, land the act in flight and exit; start no new act.\n"
	if !strings.HasSuffix(r, last) || strings.Contains(r, "180k") {
		t.Fatalf("RULES' last sentence is not the soft note's, or it names 180k:\n%s", r)
	}
	if h := prompt.TemplateHash(); strings.HasPrefix(h, mainTemplateHashAR28) {
		t.Fatalf("TemplateHash %s is main's: the changed RULES and CONTEXT are not a new template", h)
	}
}
