package gate

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// ar94Bound is the hang guard over a cut call (AR78's 30 s, ar94Guard):
// the cut is proved by its outcome (ar94Absent), never by the call's wall,
// which holds the fresh tree's git beside the line (AR35's ruling 2).
const ar94Bound = 30 * time.Second

// ar94Guard runs call in a goroutine and waits at most ar94Bound for it
// (AR78's device): a call that blocks fails the test naming the guard, not
// go's package timeout. took and back are the call's own wall and return.
func ar94Guard(t *testing.T, what string, call func()) (took time.Duration, back time.Time) {
	t.Helper()
	done := make(chan struct{})
	start := time.Now()
	go func() { call(); back = time.Now(); close(done) }()
	select {
	case <-done:
	case <-time.After(ar94Bound):
		t.Fatalf("%s did not return in ar94Bound (%s): a hang", what, ar94Bound)
	}
	return back.Sub(start), back
}

// ar94SlowClone is the gate's own git (effects.OSGit) with each Clone
// slow late: the fresh tree's git, inside the call, outruns the 1 s cut
// by construction, as it did under load (AR94).
type ar94SlowClone struct {
	effects.OSGit
	slow time.Duration
}

func (g ar94SlowClone) Clone(dir, path string, refs ...effects.Ref) error {
	time.Sleep(g.slow)
	return g.OSGit.Clone(dir, path, refs...)
}

// ar94Absent fails when path exists 4 s after back, the cut call's return.
// The cut line `sleep 3 && touch <path>` began before back, so a shell the
// cut left alive has touched path by back plus 3 s; 1 s is the margin.
// The mark proves the kill came before the sleep's end, so a cut up to 2 s
// late passes: the cut's timeliness is the shell's claim (effects.Cut).
func ar94Absent(t *testing.T, path string, back time.Time) {
	t.Helper()
	time.Sleep(time.Until(back.Add(4 * time.Second)))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s was touched: the cut left the line's shell alive", path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

// ar94Forced fails a forced call (slow over 0) that took under 3 s: each
// Clone sleeps slow (2 s) before the 1 s cut, so a shorter call means the
// forcing lapsed and the test proves nothing beyond its unforced twin.
func ar94Forced(t *testing.T, slow, took time.Duration) {
	t.Helper()
	if slow > 0 && took < 3*time.Second {
		t.Fatalf("the forced call took %s, under 3 s: the forcing lapsed", took)
	}
}

// TestAR94_1_GateCutUnderSlowGit is AR94 act 1: TestAR59_2_GateUsesLimit's
// body with each Clone 2 s late, so the cut gate takes 3 s or more, past
// the old bound: the cut still reads red, exit 124, Cut 1, and its mark
// never lands.
func TestAR94_1_GateCutUnderSlowGit(t *testing.T) { ar59GateCut(t, 2*time.Second) }

// TestAR94_2_MergeCutUnderSlowGit is AR94 act 2:
// TestAR59_3_MergeNamesCut's body with each Clone 2 s late, so the merge's
// cut verify takes 3 s or more, past the old bound: the refusal still
// names the cut, the root unmoved, and the mark never lands.
func TestAR94_2_MergeCutUnderSlowGit(t *testing.T) { ar59MergeCut(t, 2*time.Second) }
