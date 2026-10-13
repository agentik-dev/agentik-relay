package night_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The wake continues the row in its tree (usage-limit-resume r1 §2, §4
// test 1; AR67): nights on TestAR65_3's world, every wait bounded.

// ar67Clean is a part's clean result.
var ar67Clean = []byte(`{"type":"system","subtype":"init","session_id":"SESSION","model":"claude-opus-5-5"}` + "\n" +
	`{"type":"result","subtype":"success","is_error":false,"num_turns":3,"duration_ms":900,"total_cost_usd":1.25,"permission_denials":[],"result":"done","session_id":"SESSION"}` + "\n")

// ar67Limited is r1's first part: the refused five_hour event, then an
// is_error result `API Error: 429` of 40 turns and $11.5.
var ar67Limited = append(ar65Event("five_hour", ar65T.Add(time.Hour)),
	`{"type":"result","subtype":"success","is_error":true,"result":"API Error: 429","num_turns":40,"total_cost_usd":11.5}`+"\n"...)

// ar67Spawner starts r1 by its Outs, its first stream held until release
// closes (nil: at once), its second until hold2 closes (nil: at once);
// another row's first child is a stall its kill
// ends (stall) or a clean result, its later ones clean. Every row's start
// after its first commits a.txt in the tree first: each part 2.
type ar67Spawner struct {
	r1      *effects.FakeSpawner
	stall   bool
	release chan struct{}
	hold2   chan struct{}
	mu      sync.Mutex
	starts  map[string]int
	n       int
}

func ar67New(stall bool, release chan struct{}) *ar67Spawner {
	return &ar67Spawner{stall: stall, release: release, starts: map[string]int{},
		r1: &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{{Out: ar67Limited, Exit: 143}, {Out: ar67Clean}}}}
}

func (a *ar67Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	row := filepath.Base(sp.Dir)
	a.mu.Lock()
	a.n++
	a.starts[row]++
	k := a.starts[row]
	a.mu.Unlock()
	if k > 1 {
		os.WriteFile(filepath.Join(sp.Dir, "a.txt"), fmt.Appendf(nil, "%s part %d\n", row, k), 0o644)
		if out, err := exec.Command("git", "-C", sp.Dir, "commit", "-qam", "part "+fmt.Sprint(k)).CombinedOutput(); err != nil {
			return nil, nil, fmt.Errorf("commit: %v: %s", err, out)
		}
	}
	if row == "r1" {
		p, out, err := a.r1.Start(sp)
		if k == 1 && a.release != nil && err == nil {
			out = &ar67Held{ReadCloser: out, release: a.release}
		}
		if k == 2 && a.hold2 != nil && err == nil {
			out = &ar67Held{ReadCloser: out, release: a.hold2}
		}
		return p, out, err
	}
	if k == 1 && a.stall {
		if a.release != nil {
			close(a.release) // r2 has its child: r1's stream goes
		}
		s := &ar65Stall{gone: make(chan struct{})}
		return s, s, nil
	}
	return (&effects.FakeSpawner{Out: ar67Clean, PID: 4343}).Start(sp)
}

func (a *ar67Spawner) spawns() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n
}

// ar67Held is a stream that says nothing until release closes.
type ar67Held struct {
	io.ReadCloser
	release chan struct{}
}

func (h *ar67Held) Read(p []byte) (int, error) {
	<-h.release
	return h.ReadCloser.Read(p)
}

// ar67Night runs the night on sp in the background.
func ar67Night(w *world, q *queue.Queue, o night.NightOptions, sp *ar67Spawner) chan *night.Night {
	res := make(chan *night.Night, 1)
	go func() {
		n, err := night.Run(w.fx, sp, q, o)
		if err != nil {
			n = &night.Night{State: relay.NightState("error: " + err.Error())}
		}
		res <- n
	}()
	return res
}

// flagIn is the value after name in argv.
func flagIn(argv []string, name string) string {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == name {
			return argv[i+1]
		}
	}
	return ""
}

// ar67Tree is row id's worktree in w.
func ar67Tree(w *world, id string) string { return filepath.Join(w.root, ".claude", "worktrees", id) }

// TestAR67_3_WakeContinues (AR67 rulings 3 to 6): at --parallel 2 r1's
// window pauses the night and r2, a stall, comes back paused; while
// paused the test commits a.txt in r1's tree. At the wake one `wake` line
// names both rows, the wake and the count; each row's part 2 runs in its
// own tree under a new session, r1's at 60 turns and $5, on the first
// stage (one `stage` line a row; part 2's `spawn` and `sitting` lines
// `part` 2, the latter's head_before the stage line's); both green,
// queue-empty, unlocked. With --max-sittings 2 both part 2s run, then
// count-reached. While part 2 runs, status reads the row running, its
// elapsed counted from part 1's start (the fix round's F1).
func TestAR67_3_WakeContinues(t *testing.T) {
	t.Run("elapsed while part 2 runs", func(t *testing.T) {
		w, q, o, _, clock := ar65World(t, nil, "r1")
		sp := ar67New(true, nil)
		sp.hold2 = make(chan struct{})
		res := ar67Night(w, q, o, sp)
		ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
		clock.Advance(time.Hour + 2*time.Minute)
		ar65Until(t, "part 2's spawn", func() bool { return len(sp.r1.Spawns()) == 2 })
		clock.Advance(10 * time.Minute)
		var state, elapsed string
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			for _, r := range night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night).Rows {
				if r.ID == "r1" {
					state, elapsed = r.State, r.Elapsed
				}
			}
			if state == "running" && elapsed == "1h12m0s" {
				break
			}
		}
		close(sp.hold2)
		ar65End(t, res)
		if state != "running" || elapsed != "1h12m0s" {
			t.Fatalf("during part 2 at +1h12m: r1 %s, elapsed %q; want running, 1h12m0s", state, elapsed)
		}
	})

	for _, c := range []struct {
		maxSittings int
		want        relay.StopReason
	}{{0, relay.QueueEmpty}, {2, relay.CountReached}} {
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		sp := ar67New(true, make(chan struct{}))
		o.Parallel, o.MaxSittings = 2, c.maxSittings
		res := ar67Night(w, q, o, sp)
		ar65Until(t, "the pause and both sitting lines", func() bool {
			all := w.ledger.Written()
			return len(ar65Events(all, "pause")) > 0 && len(rowLines(all, "sitting", "r2")) == 1 && ar65Paused(o, "r1", "r2")
		})
		tree := ar67Tree(w, "r1")
		os.WriteFile(filepath.Join(tree, "a.txt"), []byte("while paused\n"), 0o644)
		gitIn(t, tree, "commit", "-qam", "while paused")
		clock.Advance(time.Hour + 2*time.Minute)
		n := ar65End(t, res)
		all := w.ledger.Written()
		wk := ar65Events(all, "wake")
		if len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[r1 r2]" || wk[0]["wake"] != "2026-10-11T02:02:00Z" || number(wk[0]["count"]) != 1 {
			t.Fatalf("max-sittings %d: the wake lines %v, want one: [r1 r2], 02:02, 1", c.maxSittings, wk)
		}
		ss := sp.r1.Spawns()
		if len(ss) != 2 || ss[1].Dir != tree || ss[0].Dir != tree {
			t.Fatalf("max-sittings %d: r1's spawns %d, want 2 in %s", c.maxSittings, len(ss), tree)
		}
		a1, a2 := ss[0].Argv, ss[1].Argv
		if flagIn(a2, "--session-id") == flagIn(a1, "--session-id") || flagIn(a2, "--max-turns") != "60" || flagIn(a2, "--max-budget-usd") != "5" {
			t.Fatalf("max-sittings %d: part 2's argv %q (part 1's %q): want a new session, 60 turns, $5", c.maxSittings, a2, a1)
		}
		for _, id := range []string{"r1", "r2"} {
			stage, spawn, sit := rowLines(all, "stage", id), rowLines(all, "spawn", id), rowLines(all, "sitting", id)
			if len(stage) != 1 || len(spawn) < 1 || len(sit) != 2 {
				t.Fatalf("max-sittings %d: %s: %d stage, %d spawn, %d sitting lines; want 1, a part 2's, 2", c.maxSittings, id, len(stage), len(spawn), len(sit))
			}
			last, s2 := spawn[len(spawn)-1], sit[1]
			if number(last["part"]) != 2 || number(s2["part"]) != 2 || sit[0]["part"] != nil || s2["head_before"] != stage[0]["head_before"] {
				t.Fatalf("max-sittings %d: %s: part 2's spawn %v, sitting part %v head_before %v; part 1's %v; the stage's %v",
					c.maxSittings, id, last["part"], s2["part"], s2["head_before"], sit[0]["part"], stage[0]["head_before"])
			}
		}
		if n.State != relay.Stopped(c.want) || states(n) != "r1=green r2=green" {
			t.Fatalf("max-sittings %d: night %s, %s; want stopped:%s, both green", c.maxSittings, n.State, states(n), c.want)
		}
		if night.Locked(tree) || night.Locked(ar67Tree(w, "r2")) {
			t.Fatalf("max-sittings %d: a tree still locked", c.maxSittings)
		}
	}
}

// TestAR67_4_TreeMoved (AR67 ruling 4): r1's tree put on a new branch
// while paused stops the night usage-limit at the wake, that detail, one
// spawn; r1 failed usage-limit, unlocked, its tree there.
func TestAR67_4_TreeMoved(t *testing.T) {
	w, q, o, _, clock := ar65World(t, nil, "r1")
	sp := ar67New(true, nil)
	res := ar67Night(w, q, o, sp)
	ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
	tree := ar67Tree(w, "r1")
	gitIn(t, tree, "checkout", "-qb", "moved")
	clock.Advance(time.Hour + 2*time.Minute)
	n := ar65End(t, res)
	stop := lastOf(w.ledger.Written(), "stop")
	if n.State != relay.Stopped(relay.UsageLimit) || stop["detail"] != "r1: the tree moved while paused" || sp.spawns() != 1 {
		t.Fatalf("night %s, detail %q, %d spawns; want stopped:usage-limit, r1: the tree moved while paused, one", n.State, stop["detail"], sp.spawns())
	}
	if s := n.Sittings["r1"]; s.State != relay.Failed || s.Reason != relay.UsageLimit || night.Locked(tree) {
		t.Fatalf("r1 %s %s, locked %v; want failed usage-limit, unlocked", s.State, s.Reason, night.Locked(tree))
	}
	if _, err := os.Stat(tree); err != nil {
		t.Fatalf("the tree is gone: %v", err)
	}
}

// TestAR67_5_NoHeldStage (AR67 ruling 4, the fold's B1; AR70 ruling 6): at
// --parallel 2 r1's stream goes once r2's gate starts, and that gate is
// held until r1's pause line, so r2 is killed in its gate: its verdict
// lands while paused, green from its one sitting line, and once status
// reads it green the clock moves. The wake names r1 alone and continues
// it: both green, queue-empty, 3 spawns.
func TestAR67_5_NoHeldStage(t *testing.T) {
	w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
	release := make(chan struct{})
	sp := ar70New(release, nil) // r2's part commits: its gate goes green
	o.Parallel = 2
	var once sync.Once
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := row(r, rp)
		if g := ro.Gate; r.ID == "r2" {
			ro.Gate = func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
				once.Do(func() { close(release) })
				for end := time.Now().Add(10 * time.Second); len(ar65Events(w.ledger.Written(), "pause")) == 0 && time.Now().Before(end); {
					time.Sleep(5 * time.Millisecond)
				}
				return g(st, seen, isError)
			}
		}
		return b, ro, err
	}
	res := ar70Night(w, q, o, sp)
	ar65Until(t, "the pause, r1 paused, r2 green", func() bool { return ar65Paused(o, "r1") && ar70Row(o, "r2") == "green" })
	clock.Advance(time.Hour + 2*time.Minute)
	n := ar65End(t, res)
	all := w.ledger.Written()
	if sl := rowLines(all, "sitting", "r2"); len(sl) != 1 || sl[0]["state"] != "green" {
		t.Fatalf("r2's sitting lines %v; want one, green", sl)
	}
	if wk := ar65Events(all, "wake"); len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[r1]" {
		t.Fatalf("the wake lines %v; want one, [r1]", wk)
	}
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" {
		t.Fatalf("night %s, %s; want stopped:queue-empty, both green", n.State, states(n))
	}
	if len(sp.r1.Spawns()) != 2 || sp.spawns() != 3 {
		t.Fatalf("spawns: r1 %d, all %d; want r1's part 2 (2, 3)", len(sp.r1.Spawns()), sp.spawns())
	}
}
