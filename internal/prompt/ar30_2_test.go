package prompt_test

import (
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// mainTemplateHashAR30 is main's TemplateHash before AR30 (its first twelve hex).
const mainTemplateHashAR30 = "f37c321572af"

// TestAR30_2_BlockOpensPrompt (AR30 act 2): the stdin opens with the block
// and a blank line, then today's bytes; two blocks give two hashes; no
// block gives today's bytes and TemplateHash; RULES names no ledger, keeps
// AR26's opening and AR28's close; a new template.
func TestAR30_2_BlockOpensPrompt(t *testing.T) {
	c := prompt.Context{Row: "ar30", Tree: "/r/wt", Branch: "ar30", Base: "main", Packet: "design/packets/ar30.md"}
	packet := []byte("# Packet AR30: x\n")
	today, rulesToday, _ := prompt.Render(c, nil, packet)
	if rulesToday != prompt.TemplateHash() || prompt.TemplateHashFor(nil) != prompt.TemplateHash() {
		t.Fatalf("no block: rules hash %s, TemplateHashFor(nil) %s; want TemplateHash %s", rulesToday, prompt.TemplateHashFor(nil), prompt.TemplateHash())
	}
	if !strings.HasPrefix(string(today), string(prompt.Rules())+"\n"+prompt.Line(c)) {
		t.Fatalf("no block: stdin is not today's bytes:\n%s", today)
	}
	one := []byte(prompt.BlockBegin + "\nsource\n\n1. **One.** Verbatim.\n" + prompt.BlockEnd + "\n")
	two := []byte(prompt.BlockBegin + "\nsource\n\n1. **Two.** Verbatim.\n" + prompt.BlockEnd + "\n")
	stdin, rulesHash, promptHash := prompt.Render(c, one, packet)
	if string(stdin) != string(one)+"\n"+string(today) {
		t.Fatalf("stdin does not open with the block, a blank line, then today's bytes:\n%s", stdin)
	}
	if rulesHash != prompt.TemplateHashFor(one) || rulesHash == rulesToday {
		t.Fatalf("rules hash %s is not TemplateHashFor(block)", rulesHash)
	}
	if want := prompt.HashOf(append(append(append([]byte(nil), one...), '\n'), prompt.Rules()...)); rulesHash != want {
		t.Fatalf("TemplateHashFor(block) %s is not HashOf the block, its blank line and RULES %s", rulesHash, want)
	}
	if _, h2, p2 := prompt.Render(c, two, packet); h2 == rulesHash || p2 == promptHash {
		t.Fatal("two blocks give one template hash or one prompt hash")
	}
	r := string(prompt.Rules())
	if strings.Contains(strings.ToLower(r), "ledger") || strings.Contains(r, "snapshot") {
		t.Fatalf("RULES still names the ledger read (entry 12 carries it):\n%s", r)
	}
	if !strings.HasPrefix(r, "You are relay-run, unattended. You are headless (`claude -p`)") ||
		!strings.Contains(r, "Commit per act; never push; never amend or rebase; never touch a device. Write your charter at open") ||
		!strings.HasSuffix(r, "land the act in flight and exit; start no new act.\n") {
		t.Fatalf("RULES lost AR26's opening, the mechanics or AR28's close:\n%s", r)
	}
	if h := prompt.TemplateHash(); strings.HasPrefix(h, mainTemplateHashAR30) {
		t.Fatalf("TemplateHash %s is main's: the changed RULES is not a new template", h)
	}
}
