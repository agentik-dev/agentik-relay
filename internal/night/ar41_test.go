package night

import (
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// TestAR41_1_RunPassesRole (relay-r1 §7; AR41 act 1): a build row's child
// is metered as the session plugin's `builder` and an orchestrator sitting
// (SittingPlan's, Row set) as `orchestrator`; a parent's
// CLAUDE_SESSION_PROFILE never passes.
func TestAR41_1_RunPassesRole(t *testing.T) {
	repo, _ := fixture(t)
	fx, _, _, _ := fakes(repo, false)
	build := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1",
		Spawn: []string{"claude", "-p", "--max-turns", "80", "--model", "claude-opus-5-5"}}
	orch := SittingPlan(&Plan{Root: "/root"}, queue.Row{ID: "o", Model: "claude-opus-5-5", Turns: 80, USD: 15}, nil)
	if orch.Row != "o" {
		t.Fatalf("the sitting's Row %q", orch.Row)
	}
	var got []string
	for _, rp := range []*RowPlan{build, &orch} {
		sp := &effects.FakeSpawner{PID: 77}
		RunRow(fx, sp, &Staged{Path: rp.Worktree, Branch: rp.Branch, Base: "main"}, rp, nil,
			RunOptions{Context: 1000, Wall: time.Hour, Gate: greenGate,
				Env: []string{"HOME=/h", "CLAUDE_SESSION_PROFILE=frontier"}})
		if len(sp.Spawns()) != 1 {
			t.Fatalf("%s: %d spawns", rp.ID, len(sp.Spawns()))
		}
		var profiles []string
		for _, kv := range sp.Spawns()[0].Env {
			if v, ok := strings.CutPrefix(kv, "CLAUDE_SESSION_PROFILE="); ok {
				profiles = append(profiles, v)
			}
		}
		got = append(got, strings.Join(profiles, ","))
	}
	if strings.Join(got, " ") != "builder orchestrator" {
		t.Fatalf("the children's CLAUDE_SESSION_PROFILE = %q; want builder, then orchestrator", got)
	}
}
