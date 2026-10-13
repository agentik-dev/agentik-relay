package night

import (
	"fmt"
	"io"
	"os"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// heldSpawner starts a child whose stream no one ever closes (a holder
// outside its group keeps it): the child ignores SIGTERM and exits 137 at
// SIGKILL.
type heldSpawner struct {
	w      *io.PipeWriter
	killed chan struct{}
	at     chan time.Time // the SIGKILL's time
}

func (h *heldSpawner) Start(effects.Spawn) (effects.Process, io.ReadCloser, error) {
	r, w := io.Pipe()
	h.w, h.killed, h.at = w, make(chan struct{}), make(chan time.Time, 1)
	return heldProc{h}, r, nil
}

type heldProc struct{ h *heldSpawner }

func (p heldProc) Signal(s os.Signal) error {
	if s == syscall.SIGKILL {
		select {
		case <-p.h.killed:
		default:
			p.h.at <- time.Now()
			close(p.h.killed)
		}
	}
	return nil
}
func (p heldProc) Wait() (int, error) { <-p.h.killed; return 137, nil }
func (p heldProc) Pid() int           { return 4242 }

// TestAR13_3_HeldStreamIsOrphans (ADV6-7): the wall kills a child whose
// stdout a holder keeps open: after the kill, KillGrace, the exit, the
// sweep and PipeGrace the stream is closed from the read end; RunRow
// returns within KillGrace + PipeGrace + 1 s of the kill, the row failed
// `orphans`, the sitting line `stop: orphans`.
func TestAR13_3_HeldStreamIsOrphans(t *testing.T) {
	repo, _ := fixture(t)
	fx, g, _, l := fakes(repo, false)
	oldKill, oldPipe := KillGrace, effects.PipeGrace
	KillGrace, effects.PipeGrace = 200*time.Millisecond, 100*time.Millisecond
	defer func() { KillGrace, effects.PipeGrace = oldKill, oldPipe }()
	sp := &heldSpawner{}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main", HeadBefore: "abc"}
	g.Heads = map[string]string{st.Path: "abc"}
	type ret struct {
		s  *relay.Sitting
		at time.Time
	}
	done := make(chan ret, 1)
	go func() {
		s, _ := RunRow(fx, sp, st, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: 50 * time.Millisecond})
		done <- ret{s, time.Now()}
	}()
	var r ret
	select {
	case r = <-done:
	case <-time.After(5 * time.Second):
		sp.w.Close() // free the hung RunRow
		t.Fatal("RunRow still running 5 s after its wall: the held stream was never closed")
	}
	kill := <-sp.at
	if d := r.at.Sub(kill); d > effects.PipeGrace+time.Second {
		t.Fatalf("RunRow returned %s after the SIGKILL, want under PipeGrace + 1 s", d)
	}
	if r.s.State != relay.Failed || r.s.Reason != relay.Orphans {
		t.Fatalf("sitting %+v, want failed orphans", r.s)
	}
	if sit := sittingOf(l, "r1"); sit["stop"] != "orphans" || sit["state"] != "failed" {
		t.Fatalf("sitting line %v", sit)
	}
}

// TestAR13_3_DescendantsSwept (ADV6-8): each watchdog poll snapshots the
// child's descendants by ppid with their start times; after the Wait and
// the group's sweep, those still alive as themselves are killed by their
// own groups and counted as orphans (the row red).
func TestAR13_3_DescendantsSwept(t *testing.T) {
	repo, _ := fixture(t)
	fx, g, _, l := fakes(repo, false)
	sp := &effects.FakeSpawner{PID: 4242}
	procs := &effects.FakeProcs{Mtime: time.Now(), Mortal: true,
		Desc:   map[int][]int{4242: {4243, 4244, 4245}},
		Starts: map[int]string{4243: "t1", 4244: "t2", 4245: "t3"}}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main", HeadBefore: "abc"}
	g.Heads = map[string]string{st.Path: "abc"}
	fx.Procs = procs
	s, _ := RunRow(fx, sp, st, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: time.Hour})
	kills := procs.Kills()
	sort.Ints(kills)
	if fmt.Sprint(kills) != "[4243 4244 4245]" {
		t.Fatalf("kills %v, want the three descendants", kills)
	}
	if sit := sittingOf(l, "r1"); number(sit["orphans"]) != 3 || s.State != relay.Red || len(procs.Kills()) != 3 {
		t.Fatalf("sitting %+v, line %v: want red, orphans 3", s, sit)
	}
}
