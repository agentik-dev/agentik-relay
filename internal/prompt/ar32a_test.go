package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// The sentences orchestrator r1 §3 quotes, and the packet's merge one.
const (
	ar32aHeadless   = "You are headless (`claude -p`): the sitting ends when your turn ends. Never start a background command or a background agent, and never end a turn waiting for a notification."
	ar32aHelpers    = "You may use helpers, in-process subagents, in the foreground only: the turn that starts one waits for its result. A helper writes files in your worktree and never commits; only you commit."
	ar32aPrecedence = "These rules win over the whole brief, its How this runs and Driving the relay sections included: never use `/loop` or a timer, never start or stop a night or run a measuring plan, never merge into main or run git in the root checkout, and never push."
	ar32aNoMerge    = ar32bMerge // ar32b act 2: the merge sentence in the never-merge one's place
	ar32aCommit     = "Commit per act; never push; never amend or rebase; never touch a device."
	ar32aSoft       = "When a `relay: soft ceiling` note follows a tool, or past the soft line or 90% of the turns the CONTEXT line names, land the act in flight and exit; start no new act."
)

// TestAR32A_2_OrchestratorRules is ar32a act 2: the orchestrator's RULES
// carry §3's headless, helper and precedence sentences, the packet's merge
// sentence, and today's commit, push, device and soft-ceiling sentences
// verbatim (each also in the build RULES); no act 0, and helpers are on,
// since the measure found Agent admitted under `claude -p`.
func TestAR32A_2_OrchestratorRules(t *testing.T) {
	r := string(OrchestratorRules())
	for _, s := range []string{ar32aHeadless, ar32aHelpers, ar32aPrecedence, ar32aNoMerge, ar32aCommit, ar32aSoft,
		"`RELAY_ROW`", "`DONE: <why>`", "`WAITING: <what>`", "relay/queue.yaml", "## Next act", "the ledger plugin's `new` and `ask` verbs"} {
		if !strings.Contains(r, s) {
			t.Errorf("orchestrator RULES lack %q", s)
		}
	}
	for _, s := range []string{ar32aCommit, ar32aSoft} {
		if !strings.Contains(string(Rules()), s) {
			t.Errorf("build RULES lack %q: the orchestrator's copy is no longer today's", s)
		}
	}
	for _, s := range []string{"act 0", "Act 0", "Helpers are off.", "Never delegate"} {
		if strings.Contains(r, s) {
			t.Errorf("orchestrator RULES carry %q", s)
		}
	}
	if string(OrchestratorRules()) == string(Rules()) {
		t.Fatal("orchestrator RULES are the build RULES")
	}
}

// TestAR32A_2_ContextAndHash is ar32a act 2: RenderOrchestrator is the
// block, a blank line, RULES, a blank line, CONTEXT, a blank line and the
// body line naming the sections; CONTEXT names the sitting, the tree,
// main, the brief, the charter and its Next act (folded), the ceilings,
// the red rows and the open questions, never a waiting branch, a push or
// live words; the template hash is its own, the block's too, and the
// prompt hash is the stdin's.
func TestAR32A_2_ContextAndHash(t *testing.T) {
	c := OrchestratorContext{Row: "orient", Sitting: "orient.3", Tree: "/r/.claude/worktrees/orient.3", Branch: "orient.3",
		Main: "0123456789abcdef", Brief: "history/sessions/PROMPT-orient.md", CharterDir: "history/sittings",
		Charter: "history/sittings/2026-09-29-10-orient.md", NextAct: "RESPAWN: act 4,\n  the loop",
		Red: []string{"b2 (acceptance)"}, OpenQuestions: 3, Words: "ship it",
		Ceiling: 400000, Soft: 360000, Kill: 380000, Turns: 400}
	block := []byte(BlockBegin + "\n1. x\n" + BlockEnd + "\n")
	sections := []string{"How this runs", "Settled; record, do not ask"}
	stdin, rh, ph := RenderOrchestrator(c, block, sections)
	line := OrchestratorLine(c)
	body := "Continue from the Next act above; read the brief by section: `How this runs`, `Settled; record, do not ask`, and the act named.\n"
	want := string(block) + "\n" + string(OrchestratorRules()) + "\n" + line + "\n" + body
	if string(stdin) != want {
		t.Fatalf("stdin:\n%s\nwant:\n%s", stdin, want)
	}
	for _, s := range []string{"orient.3", "/r/.claude/worktrees/orient.3", "0123456789abcdef", "history/sessions/PROMPT-orient.md",
		"history/sittings/2026-09-29-10-orient.md", "RESPAWN: act 4, the loop", "400k", "360k", "380k", "400 turns",
		"b2 (acceptance)", "open questions in OPEN-QUESTIONS.md: 3", "ship it"} {
		if !strings.Contains(line, s) {
			t.Errorf("CONTEXT lacks %q: %s", s, line)
		}
	}
	if strings.Count(line, "\n") != 1 || !strings.HasPrefix(line, "CONTEXT: ") {
		t.Errorf("CONTEXT is not one line: %q", line)
	}
	for _, s := range []string{"waiting", "push", "unmerged"} {
		if strings.Contains(strings.ToLower(line), s) {
			t.Errorf("CONTEXT names %q: %s", s, line)
		}
	}
	none := OrchestratorLine(OrchestratorContext{Row: "orient", Sitting: "orient.1"})
	if !strings.Contains(none, "the newest charter at main: none, its Next act: none") || !strings.Contains(none, "red or parked rows: none") {
		t.Errorf("first sitting's CONTEXT: %s", none)
	}
	if rh != OrchestratorTemplateHashFor(block) || rh == TemplateHashFor(block) || rh == OrchestratorTemplateHashFor(nil) {
		t.Errorf("rules hash %s: not its own, or not the block's", rh)
	}
	if OrchestratorTemplateHashFor(nil) != OrchestratorTemplateHash() || OrchestratorTemplateHash() == TemplateHash() {
		t.Error("the orchestrator's template hash is not its own")
	}
	sum := sha256.Sum256(stdin)
	if ph != hex.EncodeToString(sum[:]) {
		t.Errorf("prompt hash %s, want the stdin's", ph)
	}
	if s, _, _ := RenderOrchestrator(c, nil, sections); strings.HasPrefix(string(s), "\n") || !strings.HasPrefix(string(s), string(OrchestratorRules())) {
		t.Error("no block: stdin opens with RULES")
	}
}
