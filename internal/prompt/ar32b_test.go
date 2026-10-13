package prompt

import (
	"strings"
	"testing"
)

// ar32bMerge is the packet's merge sentence (ar32b act 2), which replaces
// ar32a's "Never merge a branch into yours" one.
const ar32bMerge = "To land a green branch the CONTEXT line names, run `git merge --no-ff --no-commit <branch>` in your worktree and its packet's acceptance lines there, then `git commit --no-edit` when all pass, or `git merge --abort` when the merge conflicts or a line fails; change nothing inside a merge, and merge no other branch into yours."

// ar32aTemplateHash is the orchestrator's template hash at ar32a (main at
// 6cc3aca), before the merge sentence and the green branches.
const ar32aTemplateHash = "585703471fbcef18885e2ccc7ec412b4cd8e1e0afa7caf0df5d28717edfef346"

// TestAR32B_2_MergeSentence is ar32b act 2 (orchestrator r1 §3, §6): the
// orchestrator's RULES carry the merge sentence once, where ar32a's
// "never merge" sentence stood, and the build RULES never carry it.
func TestAR32B_2_MergeSentence(t *testing.T) {
	r := string(OrchestratorRules())
	if strings.Count(r, ar32bMerge) != 1 {
		t.Fatalf("orchestrator RULES carry the merge sentence %d times", strings.Count(r, ar32bMerge))
	}
	if strings.Contains(r, "Never merge a branch into yours") || strings.Contains(r, "the relay lands the rows you add") {
		t.Error("orchestrator RULES keep ar32a's never-merge sentence")
	}
	queueEdit := "never change, drop or unpark a row already there."
	if i, j := strings.Index(r, queueEdit), strings.Index(r, ar32bMerge); i < 0 || j != i+len(queueEdit)+1 {
		t.Errorf("the merge sentence is not where the never-merge one stood (queue edit at %d, merge at %d)", i, j)
	}
	if strings.Contains(string(Rules()), "git merge --no-ff --no-commit") {
		t.Error("build RULES carry the merge sentence")
	}
}

// TestAR32B_2_ContextNamesGreenBranches is ar32b act 2 (§3): CONTEXT names
// the green branches to merge after the red rows, in the order given,
// "none" when there are none; the template hash moves.
func TestAR32B_2_ContextNamesGreenBranches(t *testing.T) {
	c := OrchestratorContext{Row: "o", Sitting: "o.2", Red: []string{"b3 (parked)"}, Green: []string{"b2", "b1"}}
	line := OrchestratorLine(c)
	want := "red or parked rows: b3 (parked); green branches to merge: b2, b1; open questions"
	if !strings.Contains(line, want) || strings.Count(line, "\n") != 1 {
		t.Errorf("CONTEXT lacks %q: %s", want, line)
	}
	none := OrchestratorLine(OrchestratorContext{Row: "o", Sitting: "o.1"})
	if !strings.Contains(none, "red or parked rows: none; green branches to merge: none;") {
		t.Errorf("no green branch: %s", none)
	}
	if OrchestratorTemplateHash() == ar32aTemplateHash {
		t.Error("the template hash did not move")
	}
	t.Logf("template hash %s", OrchestratorTemplateHash())
}
