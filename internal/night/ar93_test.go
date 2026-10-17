package night_test

import (
	"context"
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

// ar93LateBand is a FakeBand that answers each ask lag after it records it
// (ruling 3), as load delayed the asking row: a step of the clock on the
// record must still land after the row's leave reads it.
type ar93LateBand struct {
	*effects.FakeBand
	lag time.Duration
}

func (b *ar93LateBand) Ask(ctx context.Context, session string, req effects.BandRequest) (effects.BandVerdict, error) {
	v, err := b.FakeBand.Ask(ctx, session, req)
	time.Sleep(b.lag)
	return v, err
}

// TestAR93_2_StepAfterTheLeave: TestAR81_4_QueuedBehindNewHead with each
// answer 50 ms after its ask is recorded: the steps past r1's and r2's
// admitting asks wait for their admitted lines, so the asks still read
// r1 +0s, r1 +60s, r2 +120s, r2 +180s, r3 +240s.
func TestAR93_2_StepAfterTheLeave(t *testing.T) { ar81QueuedBehindNewHead(t, 50*time.Millisecond) }
