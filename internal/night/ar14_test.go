package night

import (
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR14_3_SpawnEnvCarriesRowTurns: the spawned child's env carries the
// spawn line's --max-turns as CLAUDE_SESSION_MAX_TURNS, depth 1, the
// builder profile (AR41) and the spawn line's --model (AR74), and nothing
// else of CLAUDE* (§7): a parent's
// CLAUDECODE and turns dropped.
func TestAR14_3_SpawnEnvCarriesRowTurns(t *testing.T) {
	repo, _ := fixture(t)
	fx, _, _, _ := fakes(repo, false)
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1",
		Spawn: []string{"claude", "-p", "--max-turns", "80", "--model", "claude-opus-5-5"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}
	sp := &effects.FakeSpawner{PID: 77}
	RunRow(fx, sp, st, rp, nil, RunOptions{Context: 1000, Wall: time.Hour, Gate: greenGate,
		Env: []string{"HOME=/h", "CLAUDECODE=1", "CLAUDE_SESSION_MAX_TURNS=999"}})
	if len(sp.Spawns()) != 1 {
		t.Fatalf("%d spawns", len(sp.Spawns()))
	}
	var claude []string
	for _, kv := range sp.Spawns()[0].Env {
		if strings.HasPrefix(kv, "CLAUDE") {
			claude = append(claude, kv)
		}
	}
	if got := strings.Join(claude, " "); got != "CLAUDE_SESSION_DEPTH=1 CLAUDE_SESSION_MAX_TURNS=80 CLAUDE_SESSION_PROFILE=builder CLAUDE_SESSION_MODEL=claude-opus-5-5" {
		t.Fatalf("the child's CLAUDE* = %q", got)
	}
}
