package night_test

import (
	"os"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// The two load flakes are order races (AR93): each test waits for the
// event it races, and each TestAR93_ forces the order a loaded machine
// inverts.

// ar93SlowHead is the OS git with each head read once the ledger holds the
// soft line slow late, then the path done touched (ruling 2): the fake's
// mid-act commit waits for done, so it lands after the read it raced.
type ar93SlowHead struct {
	effects.OSGit
	w    *world
	slow time.Duration
	done string
}

func (g *ar93SlowHead) Head(dir string) (string, error) {
	if len(linesOf(g.w.ledger.Written(), "soft")) == 0 {
		return g.OSGit.Head(dir)
	}
	time.Sleep(g.slow)
	h, err := g.OSGit.Head(dir)
	_ = os.WriteFile(g.done, nil, 0o644)
	return h, err
}

// TestAR93_1_CommitAfterTheRead: TestAR28_1_TuneOnExit's table with each
// head read after the soft line 300 ms late, as load delayed turn 4's: the
// fake's commit still follows that read, so the tune reads commit_turn 5.
func TestAR93_1_CommitAfterTheRead(t *testing.T) { ar28TuneOnExit(t, 300*time.Millisecond) }
