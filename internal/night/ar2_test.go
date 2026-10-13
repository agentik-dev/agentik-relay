package night_test

import (
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// AR2-9: §5, no message content: the exit line never carries the child's
// stderr, which can echo the prompt and a home path.
func TestAR2_9_ExitLineHoldsNoStderr(t *testing.T) {
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	rp.Spawn = []string{"/bin/sh", "-c", "cat >&2; exit 1"}
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, []byte("SECRET PROMPT BODY from /Users/someone/private\n"),
		night.RunOptions{Context: 130000, Transcripts: w.transcripts})
	e := lastOf(w.ledger.Written(), "exit")
	if s, _ := e["error"].(string); strings.Contains(s, "SECRET") || strings.Contains(s, "/Users/") {
		t.Fatalf("the exit line holds the child's stderr: %q", s)
	}
}
