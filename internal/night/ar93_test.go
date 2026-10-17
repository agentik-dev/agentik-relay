package night_test

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// The two load flakes are order races (AR93): each test waits for the
// event it races, and each TestAR93_ forces the order a loaded machine
// inverts.

// ar93SlowHead is the OS git with the first head read once the ledger
// holds the soft line slow late, then the path done touched (ruling 2;
// fix round 1, F2: that read alone, turn 4's): the fake's mid-act commit
// waits for done, so it lands after the read it raced. A done never
// touched exits the fake 3 with its line (F3).
type ar93SlowHead struct {
	effects.OSGit
	w     *world
	slow  time.Duration
	done  string
	first atomic.Bool
}

func (g *ar93SlowHead) Head(dir string) (string, error) {
	if len(linesOf(g.w.ledger.Written(), "soft")) == 0 || !g.first.CompareAndSwap(false, true) {
		return g.OSGit.Head(dir)
	}
	time.Sleep(g.slow)
	h, err := g.OSGit.Head(dir)
	_ = os.WriteFile(g.done, nil, 0o644)
	return h, err
}

// ar93Spawner is the OS spawner keeping each child's Wait error, its
// stderr's tail in it, for a test's failure line (fix round 1, F3): the
// exit line keeps only `exit N` (relay-r1 §5).
type ar93Spawner struct {
	effects.OSSpawner
	mu   sync.Mutex
	errs []string
}

type ar93Proc struct {
	effects.Process
	s *ar93Spawner
}

func (s *ar93Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	p, rc, err := s.OSSpawner.Start(sp)
	if err != nil {
		return p, rc, err
	}
	return &ar93Proc{Process: p, s: s}, rc, nil
}

func (p *ar93Proc) Wait() (int, error) {
	code, err := p.Process.Wait()
	if err != nil {
		p.s.mu.Lock()
		p.s.errs = append(p.s.errs, err.Error())
		p.s.mu.Unlock()
	}
	return code, err
}

// child is every child's Wait error so far, "none" when there is none.
func (s *ar93Spawner) child() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.errs) == 0 {
		return "none"
	}
	return strings.Join(s.errs, "; ")
}

// TestAR93_1_CommitAfterTheRead: TestAR28_1_TuneOnExit's table with the
// first head read after the soft line 300 ms late, as load delayed turn
// 4's: the fake's commit still follows that read, so the tune reads
// commit_turn 5.
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
