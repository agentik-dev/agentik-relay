package night

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar90Fixture is the 2026-10-16 probe's background-wait stream (CLI
// 2.1.302): two queries, a result each, the wait between them.
func ar90Fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "effects", "testdata", "fakeclaude", "fixtures", "bg-wait.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ar90Row is a row over fakes with a group that leaves no survivor.
func ar90Row(t *testing.T) (effects.Effects, *effects.FakeLedger, *Staged, *RowPlan) {
	repo, _ := fixture(t)
	fx, _, _, l := fakes(repo, false)
	fx.Procs = &effects.FakeProcs{}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
	return fx, l, &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}, rp
}

func ar90Exit(l *effects.FakeLedger) map[string]any {
	var exit map[string]any
	for _, w := range l.Written() {
		if w["event"] == "exit" {
			exit = w
		}
	}
	return exit
}

// TestAR90_3_WaitedChildGreen (AR90 ruling 3): a child that waited for
// its background work, its exit after the second result. No rule reads
// the first result as the end: the gate follows the Wait, the row is
// green, nothing is swept, and the `exit` line reads both results' fold
// with `results: 2`. The live log prints a line at each result.
func TestAR90_3_WaitedChildGreen(t *testing.T) {
	fx, l, st, rp := ar90Row(t)
	var log bytes.Buffer
	s, g := RunRow(fx, &effects.FakeSpawner{Out: ar90Fixture(t), PID: 77}, st, rp, nil,
		RunOptions{Context: 130000, Wall: time.Hour, Gate: greenGate, Transcripts: t.TempDir(), Log: &log})
	if s.State != relay.Green || s.Reason != "" || g.Orphans != 0 {
		t.Fatalf("sitting %+v, orphans %d; want green, no reason, 0", s, g.Orphans)
	}
	exit := ar90Exit(l)
	if number(exit["results"]) != 2 || number(exit["turns"]) != 3 || number(exit["duration_ms"]) != 5532 || exit["cost_usd"] != 0.00499084 {
		t.Fatalf("the exit line: results %v, turns %v, duration_ms %v, cost_usd %v; want 2, 3, 5532, 0.00499084",
			exit["results"], exit["turns"], exit["duration_ms"], exit["cost_usd"])
	}
	if _, ok := exit["reason"]; ok {
		t.Fatalf("the exit line's reason %v; want none", exit["reason"])
	}
	if sit := sittingOf(l, "r1"); number(sit["orphans"]) != 0 || sit["state"] != "green" {
		t.Fatalf("the sitting line %v", sit)
	}
	// Each message's held line, then a line at each result: the first
	// result's at turn 2, the first priced; the second's after turn 3's.
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(log.String()), "\n") {
		if strings.HasPrefix(line, "turn ") {
			got = append(got, line[:strings.Index(line, ":")]+" "+line[strings.LastIndex(line, " ")+1:])
		}
	}
	if want := "turn 1 unread|turn 2 unread|turn 2 $0.00|turn 3 $0.00|turn 3 $0.00"; strings.Join(got, "|") != want {
		t.Fatalf("the live log's turn lines %q; want %s\n%s", got, want, log.String())
	}
}

// ar90Hold writes its stream to the first result, then holds the stream
// and its Wait until signalled: a child waiting on a background command.
type ar90Hold struct {
	head    []byte
	w       *io.PipeWriter
	written chan struct{}
	gone    chan struct{}
	once    sync.Once
}

func (h *ar90Hold) Start(effects.Spawn) (effects.Process, io.ReadCloser, error) {
	r, w := io.Pipe()
	h.w, h.written, h.gone = w, make(chan struct{}), make(chan struct{})
	go func() { w.Write(h.head); close(h.written) }()
	return ar90Proc{h}, r, nil
}

type ar90Proc struct{ h *ar90Hold }

func (p ar90Proc) Signal(os.Signal) error {
	p.h.once.Do(func() { <-p.h.written; p.h.w.Close(); close(p.h.gone) })
	return nil
}
func (p ar90Proc) Wait() (int, error) { <-p.h.gone; return 143, nil }
func (p ar90Proc) Pid() int           { return 77 }

// TestAR90_3_WallBoundsWait (AR90 ruling 3): the wall, armed at the spawn,
// bounds a wait that outlives it: the child past its first result, held,
// is killed at a 300 ms wall, failed `wall-clock`, the `exit` line reading
// the one result: `results: 1`, 2 turns.
func TestAR90_3_WallBoundsWait(t *testing.T) {
	fx, l, st, rp := ar90Row(t)
	b := ar90Fixture(t)
	i := bytes.Index(b, []byte(`"type":"result"`))
	if i < 0 {
		t.Fatal("no result in the fixture")
	}
	head := b[:i+bytes.IndexByte(b[i:], '\n')+1] // to the first result's line end
	s, _ := RunRow(fx, &ar90Hold{head: head}, st, rp, nil,
		RunOptions{Context: 130000, Wall: 300 * time.Millisecond, Gate: greenGate, Transcripts: t.TempDir(), Log: io.Discard})
	if s.State != relay.Failed || s.Reason != relay.WallClock {
		t.Fatalf("sitting %+v; want failed %s", s, relay.WallClock)
	}
	if exit := ar90Exit(l); number(exit["results"]) != 1 || number(exit["turns"]) != 2 || exit["reason"] != string(relay.WallClock) {
		t.Fatalf("the exit line: results %v, turns %v, reason %v; want 1, 2, %s", exit["results"], exit["turns"], exit["reason"], relay.WallClock)
	}
}
