package prompt_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// mainTemplateHash is main's TemplateHash before AR26 (its first twelve hex).
const mainTemplateHash = "7af0aa7a5c1b"

// TestAR26_RulesCopyInAndHeadless (AR26 act 1): RULES carries the stand-in's
// headless sentences and its act-0 copy-in sentence, byte for byte, right
// after the opening sentence; still one paragraph, no NEXT; a new template.
func TestAR26_RulesCopyInAndHeadless(t *testing.T) {
	r := string(prompt.Rules())
	headless := "You are headless (`claude -p`): the sitting ends the moment your turn ends. " +
		"Never delegate an act to an agent, never start a background command or a background agent, " +
		"never end a turn waiting for a notification. Do every act yourself, in the foreground, " +
		"and commit it before the next act starts."
	copyIn := "Act 0 comes before any other act: unless the packet the CONTEXT line names already " +
		"lies inside this repository, copy it verbatim into `design/packets/<row>.md`, `<row>` " +
		"being your row id, and commit that file alone."
	if want := "You are relay-run, unattended. " + headless + " " + copyIn + " Commit per act;"; !strings.HasPrefix(r, want) {
		t.Fatalf("RULES does not open with the headless and copy-in sentences:\n%s", r)
	}
	if strings.Count(r, "\n") != 1 || !strings.HasSuffix(r, "\n") || strings.Contains(r, "\n\n") {
		t.Fatalf("RULES is not one paragraph ending in one newline: %q", r)
	}
	if bytes.Contains(prompt.Rules(), []byte("NEXT")) {
		t.Fatal("RULES names NEXT (the queue replaced it)")
	}
	if h := prompt.TemplateHash(); strings.HasPrefix(h, mainTemplateHash) {
		t.Fatalf("TemplateHash %s is main's: the changed RULES and CONTEXT are not a new template", h)
	}
}

// TestAR26_ContextNamesPacket (AR26 act 1): the CONTEXT line names the
// packet act 0 reads, after the branch clause; an empty one reads none.
func TestAR26_ContextNamesPacket(t *testing.T) {
	c := prompt.Context{Row: "ar26", Tree: "/r/wt", Branch: "ar26", Base: "main", Packet: "design/packets/ar26.md"}
	l := prompt.Line(c)
	if want := "your working tree is /r/wt on branch ar26 (off main); your packet: design/packets/ar26.md; "; !strings.Contains(l, want) {
		t.Fatalf("CONTEXT does not name the packet after the branch clause: %q", l)
	}
	stdin, _, _ := prompt.Render(c, nil, []byte("# Packet AR26: x\n"))
	if parts := strings.SplitN(string(stdin), "\n\n", 3); len(parts) != 3 || !strings.Contains(parts[1], "your packet: design/packets/ar26.md;") {
		t.Fatalf("the rendered CONTEXT does not carry the packet:\n%s", stdin)
	}
	c.Packet = "  "
	if l := prompt.Line(c); !strings.Contains(l, "; your packet: none; ") {
		t.Fatalf("an empty packet does not read none: %q", l)
	}
}
