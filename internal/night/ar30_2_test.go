package night_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// TestAR30_2_SpawnRecordsBlock (AR30 act 2): with a block rendered in
// CLAUDE.md at main, the plan's template_hash is TemplateHashFor(block);
// the plan, spawn and sitting lines' rules_hash is it; the spawn line's
// block names the source; the rendered stdin opens with the block.
func TestAR30_2_SpawnRecordsBlock(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	block := prompt.BlockBegin + "\nsource\n\n1. **Entry.** Verbatim.\n" + prompt.BlockEnd + "\n"
	os.WriteFile(filepath.Join(w.root, night.LedgerFile), []byte("# Ledger\n"), 0o644)
	os.WriteFile(filepath.Join(w.root, "CLAUDE.md"), []byte("# Title\n\n"+block+"after\n"), 0o644)
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "the block")
	mainAt := gitIn(t, w.root, "rev-parse", "main")
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	want := prompt.TemplateHashFor([]byte(block))
	if n.Plan.TemplateHash != want || want == prompt.TemplateHash() || n.Plan.BlockSource != "CLAUDE.md at "+mainAt[:12] {
		t.Fatalf("plan template_hash %s block %q; want %s at %s", n.Plan.TemplateHash, n.Plan.BlockSource, want, mainAt[:12])
	}
	lines := w.ledger.Written()
	for _, ev := range []string{"plan", "spawn", "sitting"} {
		if l := lastOf(lines, ev); l == nil || l["rules_hash"] != want {
			t.Fatalf("the %s line's rules_hash is not the plan's template_hash %s: %v", ev, want, l)
		}
	}
	if l := lastOf(lines, "night-open"); l["template_hash"] != want {
		t.Fatalf("night-open template_hash %v, want %s", l["template_hash"], want)
	}
	if l := lastOf(lines, "spawn"); l["block"] != n.Plan.BlockSource {
		t.Fatalf("the spawn line's block %v, want %s", l["block"], n.Plan.BlockSource)
	}
	rp := n.Plan.Rows[0]
	stdin, rules, _, err := night.RenderPrompt(w.fx, n.Plan, q.Rows[0], &rp, "")
	if err != nil || rules != want || !strings.HasPrefix(string(stdin), block+"\n"+string(prompt.Rules())) {
		t.Fatalf("RenderPrompt: rules %s, %v; stdin:\n%s", rules, err, stdin)
	}
}
