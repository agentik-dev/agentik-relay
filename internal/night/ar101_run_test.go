package night_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// ar101Words is a spawn line as words, whether the ledger kept it as
// []string or as decoded JSON.
func ar101Words(v any) []string {
	switch a := v.(type) {
	case []string:
		return a
	case []any:
		out := make([]string, len(a))
		for i, x := range a {
			out[i] = fmt.Sprint(x)
		}
		return out
	}
	return nil
}

// TestAR101_7_LedgerSpawnLine (AR101 ruling 2): the argv the child is
// started with and the ledger's spawn line both carry the row's level,
// high when the row names none.
func TestAR101_7_LedgerSpawnLine(t *testing.T) {
	fakeClaude(t)
	for _, c := range []struct{ effort, want string }{{"", "high"}, {"medium", "medium"}} {
		w := newWorld(t)
		id := "a1"
		rp := &night.RowPlan{ID: id, Worktree: filepath.Join(w.root, ".claude", "worktrees", id), Branch: id, Base: "main",
			SessionID: night.NewUUID()}
		rp.Spawn = night.SpawnArgv(queue.Row{ID: id, Model: "claude-opus-5-5", Turns: 100, USD: 15, Effort: c.effort},
			rp.SessionID, "Bash Edit Write", false)
		st, err := night.Stage(w.fx, rp)
		if err != nil {
			t.Fatal(err)
		}
		sp := &recorder{inner: effects.OSSpawner{}}
		night.RunRow(w.fx, sp, st, rp, []byte("the prompt\n"), night.RunOptions{
			Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Env: w.env("ok.jsonl"), Transcripts: w.transcripts})
		st.Unlock()
		want := []string{"--effort", c.want}
		has := func(argv []string) bool {
			i := slices.Index(argv, "--effort")
			return i >= 0 && i+2 <= len(argv) && slices.Equal(argv[i:i+2], want)
		}
		if len(sp.spawns) != 1 || !has(sp.spawns[0].Argv) {
			t.Fatalf("effort %q: spawns %v", c.effort, sp.spawns)
		}
		if line := lastOf(w.ledger.Written(), "spawn"); !has(ar101Words(line["argv"])) {
			t.Fatalf("effort %q: spawn line %v", c.effort, line)
		}
	}
}
