package night

import (
	"context"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR47Fix_StopDuringWaitIdleRead (AR47 ruling 6; R6 §4): the same
// stop landing in an idle read of the breaker's wait before a spawn. A
// wait whose context ended during an idle read says no, whatever the
// share read, so the row is never spawned: at the first read (a load the
// share would not have confirmed) and at the poll whose read would have
// closed the breaker.
func TestAR47Fix_StopDuringWaitIdleRead(t *testing.T) {
	for _, c := range []struct {
		name   string
		series []float64
		at     int
	}{
		{"the first read, unconfirmed", []float64{0.70}, 1},
		{"the second quiet poll", []float64{0.10, 0.60, 0.60}, 3},
	} {
		repo, _ := fixture(t)
		fx, _, _, l := fakes(repo, false)
		ctx, cancel := context.WithCancelCause(context.Background())
		idle := &stopIdle{series: c.series, at: c.at, stop: func() { cancel(ErrOwnerStop) }}
		fx.Clock, fx.Load, fx.Idle = newJump(), &effects.FakeLoad{Series: []float64{90}}, idle
		sp := &effects.FakeSpawner{}
		rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1",
			Spawn: []string{"claude", "-p", "--model", "claude-opus-5-5"}}
		s, _ := RunRow(fx, sp, &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: time.Hour, Ctx: ctx})
		if len(sp.Spawns()) != 0 || s.State != relay.Skipped || sittingOf(l, "r1")["stop"] != string(relay.OwnerStop) || idle.n != c.at {
			t.Errorf("%s: a stop during the wait's idle read: sitting %+v, %d spawns, %d idle reads; want skipped, never spawned", c.name, s, len(sp.Spawns()), idle.n)
		}
		cancel(nil)

		// The wait itself: no once its context ended during the read.
		ctx, cancel = context.WithCancelCause(context.Background())
		brk := DefaultBreaker
		brk.Idle = &stopIdle{series: c.series, at: c.at, stop: func() { cancel(ErrOwnerStop) }}
		if st, ok := brk.Wait(ctx, &effects.FakeLoad{Series: []float64{90}}, newJump()); ok || st.Unconfirmed != 0 {
			t.Errorf("%s: Wait after its context ended during the idle read: %+v %v, want no", c.name, st, ok)
		}
		cancel(nil)
	}
}

// TestAR47Fix_StopDuringReadAfterLine pins what the third idle read does
// on a stop (green as the branch stood): the read after a line finds the
// line ended before the stop, so the line is judged and stored as it was
// measured, and the next line is refused, never started.
func TestAR47Fix_StopDuringReadAfterLine(t *testing.T) {
	_, _, fx, _, sh := memoPlan(t)
	sh.Results["make check"] = effects.ShellResult{Exit: 2, Last: "FAIL"}
	idle := &stopIdle{series: []float64{0.70}, at: 1}
	fx.Load, fx.Idle = &effects.FakeLoad{Series: []float64{3, 40}}, idle
	b := newBound(fx, "/state", "/root")
	idle.stop = func() { b.stop("owner-stop", "stop") }
	b.bound(time.Hour)
	if exit, last, err := b.Run("/root", "make check", time.Minute); exit != 2 || last != "FAIL" || err != nil || b.lastLoaded() || idle.n != 1 {
		t.Fatalf("the line that ended before the stop: %d %q %v, loaded %v, %d idle reads", exit, last, err, b.lastLoaded(), idle.n)
	}
	if e, stored := LoadMemo(fx.FS, "/state").Get("/root", "aaa111", "make check"); !stored || e.Exit != 2 {
		t.Fatalf("the line that ended before the stop: memo %+v %v, want stored", e, stored)
	}
	if _, _, err := b.Run("/root", "make check; echo $?", time.Minute); err == nil || err.Error() != "owner-stop" || len(sh.Calls()) != 1 {
		t.Fatalf("the next line after the stop: %v, %d shell calls; want owner-stop and one", err, len(sh.Calls()))
	}
}
