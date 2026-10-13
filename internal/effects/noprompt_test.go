package effects_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// TestLedgerHoldsNoPrompt: a real ledger written by a plan over every packet
// of the corpus, plus the spawn line each would get, holds no 40-byte window
// of the rendered prompt's packet body (§5: no message content, ever).
func TestLedgerHoldsNoPrompt(t *testing.T) {
	const window = 40
	repo := t.TempDir()
	ids := []string{"a3-machine", "a4-schemas", "a5-effects-plan", "a6-stage-gate", "a7-spawn-stream", "a8-night"}
	yaml := "version: 1\ndefaults: {model: m, turns: 1, context: 1, usd: 1, wall: 1h, merge: branch-only, device: never, verify: auto}\nwords: \"\"\nrows:\n"
	bodies := map[string][]byte{}
	for _, id := range ids {
		src, err := os.ReadFile(filepath.Join("..", "packet", "testdata", id+".md"))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(repo, id+".md"), src, 0o644)
		bodies[id] = src
		yaml += fmt.Sprintf("  - id: %s\n    packet: %s.md\n", id, id)
	}
	os.WriteFile(filepath.Join(repo, "q.yaml"), []byte(yaml), 0o644)
	q, err := queue.Load(repo, "q.yaml")
	if err != nil {
		t.Fatal(err)
	}
	clock := effects.FakeClock{T: time.Date(2026, 9, 22, 22, 0, 0, 0, time.UTC)}
	state := t.TempDir()
	l := effects.NewLedger(state, clock, "2026-09-22-1")
	// The shell echoes a stretch of each command back, as a real line might.
	sh := &effects.FakeShell{Results: map[string]effects.ShellResult{}}
	for _, r := range q.Rows {
		for _, a := range r.Packet.Acceptance {
			sh.Results[a.Shell] = effects.ShellResult{Exit: 1, Last: "error"}
		}
	}
	fx := effects.Effects{Git: &effects.FakeGit{Roots: map[string]string{repo: "/root"},
		Branches: map[string]string{"/root": "main"}}, Shell: sh, Clock: clock, FS: effects.OSFS{}, Ledger: l}
	p, err := night.BuildPlan(fx, q, night.Options{Repo: repo, State: state})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range p.Rows {
		_, rh, ph := prompt.Render(prompt.Context{Row: r.ID, Tree: r.Worktree, Branch: r.Branch, Base: r.Base}, nil, bodies[r.ID])
		l.Append("spawn", map[string]any{"row": r.ID, "argv": r.Spawn, "prompt_hash": ph, "rules_hash": rh})
	}
	raw, err := os.ReadFile(filepath.Join(effects.LedgerDir(state), "2026-09-22.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines, _ := l.Lines()
	if len(lines) != 2*len(ids) {
		t.Fatalf("%d ledger lines, want %d", len(lines), 2*len(ids))
	}
	// Each file line, and each decoded string (JSON escapes undone).
	texts := strings.Split(string(raw), "\n")
	for _, line := range lines {
		texts = strs(line, texts)
	}
	for id, body := range bodies {
		for i := 0; i+window <= len(body); i++ {
			w := string(body[i : i+window])
			for _, s := range texts {
				if strings.Contains(s, w) {
					t.Fatalf("the ledger holds %q from packet %s", w, id)
				}
			}
		}
	}
}

// strs appends every string inside v.
func strs(v any, out []string) []string {
	switch x := v.(type) {
	case string:
		out = append(out, x)
	case []any:
		for _, e := range x {
			out = strs(e, out)
		}
	case map[string]any:
		for _, e := range x {
			out = strs(e, out)
		}
	}
	return out
}
