package night

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// orchQueue is fixture's repo with a queue holding the orchestrator row o
// (brief history/brief.md) and the build row a4-schemas.
func orchQueue(t *testing.T) (string, *queue.Queue) {
	t.Helper()
	repo, _ := fixture(t)
	os.WriteFile(filepath.Join(repo, "orch.yaml"), []byte(`version: 1
defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,
           merge: merge-on-green, device: never, verify: auto}
rows:
  - id: o
    kind: orchestrator
    prompt: history/brief.md
    sections: [How this runs]
  - id: a4-schemas
    packet: design/packets/a4-schemas.md
    depends_on: [ar1-relay-hooks]
`), 0o644)
	q, err := queue.Load(repo, "orch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return repo, q
}

// promptKey is the Hermetic key the plan reads the brief's presence by.
const promptKey = "/root ls-tree --full-tree --name-only " + mainAt + " -- history/brief.md"

func counter() func() string {
	n := 0
	return func() string { n++; return strings.Repeat("0", 7) + string(rune('0'+n)) }
}

func toolsOf(spawn []string) string { return flagOf(spawn, "--allowedTools") }

// TestAR32A_1_NewestCharterAndNextAct (§3): the newest charter of a row at
// main is the highest date, then the highest n as a number (-10- over
// -9-); another row's charters, one in a subdirectory and one only in the
// working tree are never read. The Next act is the first non-empty
// paragraph under `## Next act`; ActKey strips `RESPAWN: ` and folds
// whitespace. A row with no charter reads none.
func TestAR32A_1_NewestCharterAndNextAct(t *testing.T) {
	d := t.TempDir()
	landedGit(t, d, "init", "-q", "-b", "main")
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(d, rel)), 0o755)
		if err := os.WriteFile(filepath.Join(d, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	act := func(s string) string {
		return "# c\n\n## Done-ness\n- x\n\n## Next act\n\n" + s + "\n\n## Handover\n1. y\n"
	}
	write("history/sittings/2026-09-28-12-o.md", act("the older date"))
	write("history/sittings/2026-09-29-9-o.md", act("the ninth"))
	write("history/sittings/2026-09-29-10-o.md", act("RESPAWN: act 3:  write\tthe gate\nits second line\n\nlater"))
	write("history/sittings/2026-09-29-11-xo.md", act("another row's"))
	write("history/sittings/2026-09-30-1-a-o.md", act("row a-o's"))
	write("history/sittings/old/2026-12-01-1-o.md", act("a subdirectory's"))
	landedGit(t, d, "add", "-A")
	landedGit(t, d, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "charters")
	write("history/sittings/2026-12-31-1-o.md", act("only in the working tree"))
	charter, got, err := NextAct(effects.OSGit{}, d, "o")
	if err != nil {
		t.Fatal(err)
	}
	if charter != "history/sittings/2026-09-29-10-o.md" || got != "RESPAWN: act 3:  write\tthe gate\nits second line" {
		t.Fatalf("NextAct = %q, %q", charter, got)
	}
	if k := ActKey(got); k != "act 3: write the gate its second line" {
		t.Fatalf("ActKey = %q", k)
	}
	if ActKey("  RESPAWN: a  b ") != ActKey("a b") || ActKey("DONE: x") != "DONE: x" {
		t.Fatalf("ActKey folds too little or too much")
	}
	if charter, got, err := NextAct(effects.OSGit{}, d, "a-o"); err != nil || charter != "history/sittings/2026-09-30-1-a-o.md" || got != "row a-o's" {
		t.Fatalf("NextAct(a-o) = %q, %q, %v", charter, got, err)
	}
	if charter, got, err := NextAct(effects.OSGit{}, d, "zz"); err != nil || charter != "" || got != "" {
		t.Fatalf("NextAct(zz) = %q, %q, %v; want none", charter, got, err)
	}
	if NextActOf("## Next act\n\n\n") != "" || NextActOf("## Next act\n- a\n- b\n## Handover\n") != "- a\n- b" {
		t.Fatalf("NextActOf: an empty section or a list")
	}
}

// TestAR32A_1_PlanOrchestratorRow (§2, §8): without Orchestrate the plan
// never selects the orchestrator row (named by --rows, a problem); with
// it, the row is ready, its own RELAY_ROW, never landed or measured, and
// its spawn allows Agent; a brief absent at main makes it invalid.
func TestAR32A_1_PlanOrchestratorRow(t *testing.T) {
	repo, q := orchQueue(t)
	fx, g, _, _ := fakes(repo, false)
	g.Hermetics = map[string]string{revKey: mainAt, promptKey: "history/brief.md"}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 1 || p.Rows[0].ID != "a4-schemas" || !p.Valid {
		t.Fatalf("without Orchestrate: rows %v, problems %v", p.Rows, p.Problems)
	}
	if strings.Contains(toolsOf(p.Rows[0].Spawn), AgentTool) {
		t.Fatalf("a build row allows Agent: %q", toolsOf(p.Rows[0].Spawn))
	}
	if p, _ := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"o"}}); p.Valid || len(p.Rows) != 0 ||
		!strings.Contains(p.Problems[0].Msg, "only under --orchestrate") {
		t.Fatalf("--rows o without Orchestrate: rows %v, problems %v", p.Rows, p.Problems)
	}
	p, err = BuildPlan(fx, q, Options{Repo: repo, State: "/state", Orchestrate: true})
	if err != nil {
		t.Fatal(err)
	}
	o := p.Rows[0]
	if len(p.Rows) != 2 || o.ID != "o" || o.State != StateReady || o.Row != "o" || o.Base != "main" || len(o.Baseline) != 0 || !p.Valid {
		t.Fatalf("with Orchestrate: %+v, problems %v", p.Rows, p.Problems)
	}
	if tools := toolsOf(o.Spawn); tools != effects.DefaultAllowedTools+" "+AgentTool {
		t.Fatalf("the orchestrator's tools %q", tools)
	}
	g.Hermetics[promptKey] = ""
	p, err = BuildPlan(fx, q, Options{Repo: repo, State: "/state", Orchestrate: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Valid || p.Rows[0].State != StateInvalid || !strings.Contains(strings.Join(p.Rows[0].Problems, ";"), "prompt history/brief.md: absent at main") {
		t.Fatalf("an absent brief: %+v, problems %v", p.Rows[0], p.Problems)
	}
}

// TestAR32A_1_SittingPlan (§4): the sitting is `<id>.<n>`, n one past the
// highest the ledger holds for the row (never a malformed or another
// row's n); a fresh session, its own branch and worktree off main, Agent
// among the allowed tools, and Row the row's id for RELAY_ROW.
func TestAR32A_1_SittingPlan(t *testing.T) {
	repo, q := orchQueue(t)
	fx, g, _, _ := fakes(repo, false)
	g.Hermetics = map[string]string{revKey: mainAt, promptKey: "history/brief.md"}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Orchestrate: true, UUID: counter()})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := q.Orchestrator()
	lines := []map[string]any{{"row": "o.3"}, {"row": "o.12"}, {"row": "o.x"}, {"row": "o.070"}, {"row": "oo.40"}, {"row": "o"}, {"row": "a4-schemas"}}
	s := SittingPlan(p, r, lines)
	if s.ID != "o.13" || s.Branch != "o.13" || s.Base != "main" || s.Row != "o" || s.State != StateReady ||
		s.Worktree != filepath.Join("/root", ".claude", "worktrees", "o.13") {
		t.Fatalf("SittingPlan = %+v", s)
	}
	if flagOf(s.Spawn, "--session-id") != s.SessionID || s.SessionID == p.Rows[0].SessionID || toolsOf(s.Spawn) != effects.DefaultAllowedTools+" "+AgentTool {
		t.Fatalf("the sitting's spawn %q, session %s", s.Spawn, s.SessionID)
	}
	if s2 := SittingPlan(p, r, append(lines, map[string]any{"row": s.ID})); s2.ID != "o.14" || s2.SessionID == s.SessionID {
		t.Fatalf("the next sitting %s %s", s2.ID, s2.SessionID)
	}
	if s0 := SittingPlan(p, r, nil); s0.ID != "o.1" {
		t.Fatalf("the first sitting %s", s0.ID)
	}
	if WithAgent("Bash Agent") != "Bash Agent" || WithAgent("") != AgentTool {
		t.Fatalf("WithAgent adds Agent twice or leaves a space")
	}
}
