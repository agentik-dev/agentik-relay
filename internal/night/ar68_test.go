package night_test

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The continuation's prompt and environment, and the orchestrator sitting
// at the wake (usage-limit-resume r1 §2; AR68).

// ar68Rec records every spawn, then starts it on sp.
type ar68Rec struct {
	sp     effects.Spawner
	mu     sync.Mutex
	spawns []effects.Spawn
}

func (r *ar68Rec) Start(s effects.Spawn) (effects.Process, io.ReadCloser, error) {
	r.mu.Lock()
	r.spawns = append(r.spawns, s)
	r.mu.Unlock()
	return r.sp.Start(s)
}

func (r *ar68Rec) all() []effects.Spawn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.spawns)
}

// ar68Out is the night's own lines, written under a lock.
type ar68Out struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (o *ar68Out) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

func (o *ar68Out) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.String()
}

// ar68Run runs the night on sp in the background.
func ar68Run(w *world, q *queue.Queue, o night.NightOptions, sp effects.Spawner) chan *night.Night {
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

// ar68Env is env's RELAY_HEAD_BEFORE entries.
func ar68Env(env []string) []string {
	var out []string
	for _, kv := range env {
		if strings.HasPrefix(kv, "RELAY_HEAD_BEFORE=") {
			out = append(out, kv)
		}
	}
	return out
}

// TestAR68_3_ContinuationPrompt (AR68 rulings 1, 3): TestAR67_3's world,
// r1's a.txt committed while paused and then edited again. r1's part 2
// reads its row's prompt, a blank line, then a RESUME block naming part
// 2, part 1's session, the commit's subject, a.txt and the part's
// ceilings, 60 turns and $5. Every part's environment holds
// RELAY_HEAD_BEFORE at its stage line's head_before, once. The blank
// line is one whether or not the row's prompt ends in a newline (the fix
// round's F2).
func TestAR68_3_ContinuationPrompt(t *testing.T) {
	for _, nl := range []bool{false, true} {
		t.Run(map[bool]string{false: "a prompt with no final newline", true: "a prompt ending in a newline"}[nl], func(t *testing.T) {
			ar68Continuation(t, nl)
		})
	}
}

// ar68Continuation is TestAR68_3 on a row prompt that ends in a newline
// (nl) or not.
func ar68Continuation(t *testing.T, nl bool) {
	w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
	if nl { // the row's prompt ends in a newline
		row := o.Row
		o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
			b, ro, err := row(r, rp)
			return append(b, '\n'), ro, err
		}
	}
	inner := ar67New(true, make(chan struct{}))
	sp := &ar68Rec{sp: inner}
	o.Parallel = 2
	res := ar68Run(w, q, o, sp)
	ar65Until(t, "the pause and both sitting lines", func() bool {
		all := w.ledger.Written()
		return len(ar65Events(all, "pause")) > 0 && len(rowLines(all, "sitting", "r2")) == 1 && ar65Paused(o, "r1", "r2")
	})
	tree := ar67Tree(w, "r1")
	os.WriteFile(filepath.Join(tree, "a.txt"), []byte("while paused\n"), 0o644)
	gitIn(t, tree, "commit", "-qam", "while paused")
	os.WriteFile(filepath.Join(tree, "a.txt"), []byte("edited again\n"), 0o644)
	clock.Advance(time.Hour + 2*time.Minute)
	n := ar65End(t, res)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" {
		t.Fatalf("night %s, %s; want stopped:queue-empty, both green", n.State, states(n))
	}
	all := w.ledger.Written()

	var r1 []effects.Spawn
	for _, s := range sp.all() {
		if filepath.Base(s.Dir) == "r1" {
			r1 = append(r1, s)
		}
	}
	if len(r1) != 2 {
		t.Fatalf("r1's spawns %d, want 2", len(r1))
	}
	one, two := string(r1[0].Stdin), string(r1[1].Stdin)
	s1 := flagIn(r1[0].Argv, "--session-id")
	sep := strings.TrimSuffix(one, "\n") + "\n\n" // the row's prompt, then a blank line, whether or not it ends in a newline (the fix round's F2)
	if !strings.HasPrefix(two, sep) {
		t.Fatalf("part 2's stdin %q does not open with part 1's %q and a blank line", two, one)
	}
	block := strings.TrimPrefix(two, sep)
	head := "RESUME: part 2; the last session " + s1 + " stopped at the account's usage limit; this part's ceilings: 60 turns, $5.\n"
	if !strings.HasPrefix(block, head) {
		t.Fatalf("part 2's RESUME block opens %q, want %q", strings.SplitN(block, "\n", 2)[0], head)
	}
	for _, want := range []string{" while paused\n", "  a.txt\n"} {
		if !strings.Contains(block, want) {
			t.Fatalf("part 2's RESUME block lacks %q:\n%s", want, block)
		}
	}
	if !strings.HasSuffix(block, "continue from the first act your commits do not cover.\n") {
		t.Fatalf("part 2's RESUME block does not end in the record's sentence:\n%s", block)
	}

	parts := map[string]int{}
	for _, s := range sp.all() {
		id := filepath.Base(s.Dir)
		parts[id]++
		stage := rowLines(all, "stage", id)
		if len(stage) != 1 {
			t.Fatalf("%s: %d stage lines, want 1", id, len(stage))
		}
		want := fmt.Sprintf("RELAY_HEAD_BEFORE=%v", stage[0]["head_before"])
		if got := ar68Env(s.Env); len(got) != 1 || got[0] != want || stage[0]["head_before"] == "" {
			t.Fatalf("%s part %d: RELAY_HEAD_BEFORE entries %q, want one, %s", id, parts[id], got, want)
		}
	}
	if parts["r1"] != 2 || parts["r2"] != 2 {
		t.Fatalf("parts %v, want two a row", parts)
	}
}

// TestAR68_4_OrchestratorWake (AR68 rulings 4, 5): TestAR32A_4_RestartsAfterMerge's
// world on the step clock; o.1's child is TestAR65_3's stream (a window
// refused, exit 143), every other the fake claude; o.2's charter DONE.
// The night pauses; at the wake one `wake` line, and o.1 is halted, never
// continued: failed usage-limit, unlocked, its tree kept, one spawn, no
// `agentik relay resume o.1`. The loop cuts o.2 under a new session, its
// SittingStart.Red without o.1, and the night stops done, clean: the stop
// line's red 0, its resume empty. A night that stops after the halt and
// before the cut (the fix round's F1) names o.1 in its resume lines and
// is not clean.
func TestAR68_4_OrchestratorWake(t *testing.T) {
	w, n, o1, out, reds := ar68Orch(t, 0, nil)
	all := w.ledger.Written()
	if wk := ar65Events(all, "wake"); len(wk) != 1 || fmt.Sprint(wk[0]["rows"]) != "[o.1]" {
		t.Fatalf("the wake lines %v, want one naming o.1", wk)
	}
	if n.State != relay.Stopped(relay.Done) {
		t.Fatalf("night %s (%v), want stopped:done; out:\n%s", n.State, stopOf(t, w)["detail"], out)
	}
	ar68Halted(t, w, n, o1)
	sessions := map[string]string{}
	for _, l := range all {
		if l["event"] == "spawn" {
			sessions[fmt.Sprint(l["row"])] = fmt.Sprint(l["session_id"])
		}
	}
	if len(sessions) != 2 || sessions["o.2"] == "" || sessions["o.1"] == sessions["o.2"] {
		t.Fatalf("spawns %v: want o.1 and o.2, fresh sessions", sessions)
	}
	if red, cut := reds["o.2"]; !cut || slices.Contains(red, "o.1") {
		t.Fatalf("o.2's SittingStart.Red %v (cut %v), want it without o.1", red, cut)
	}
	stop := stopOf(t, w)
	if rs := stop["resume"]; number(stop["red"]) != 0 || (rs != nil && fmt.Sprint(rs) != "[]") { // none: the line's null, as on a clean night
		t.Fatalf("the stop line's red %v, resume %v; want 0, empty", stop["red"], stop["resume"])
	}
	if !night.Clean(n) {
		t.Fatalf("night.Clean false: %s, %s", n.Reason, states(n))
	}
	if strings.Contains(out.String(), "agentik relay resume o.1") {
		t.Fatalf("the night printed `agentik relay resume o.1`:\n%s", out)
	}

	// The fix round's F1: o.1 is recovered only once o.2 is cut. Stopped
	// before (--max-sittings 1 reached at the wake; o.2's worktree path
	// taken while paused, so it cannot stage), o.1 is red: the stop line's
	// red 1 and resume `agentik relay resume o.1`, printed once, the night not clean.
	for _, c := range []struct {
		name   string
		max    int
		paused func(w *world)
		want   relay.StopReason
	}{
		{"--max-sittings 1 at the wake", 1, nil, relay.CountReached},
		{"o.2 cannot stage", 0, func(w *world) { os.MkdirAll(ar67Tree(w, "o.2"), 0o755) }, relay.NoProgress},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, n, o1, out, _ := ar68Orch(t, c.max, c.paused)
			if n.State != relay.Stopped(c.want) {
				t.Fatalf("night %s (%v), want stopped:%s; out:\n%s", n.State, stopOf(t, w)["detail"], c.want, out)
			}
			ar68Halted(t, w, n, o1)
			if rows := rowLines(w.ledger.Written(), "spawn", "o.2"); len(rows) != 0 {
				t.Fatalf("o.2 spawned: %v", rows)
			}
			stop := stopOf(t, w)
			if number(stop["red"]) != 1 || fmt.Sprint(stop["resume"]) != "[agentik relay resume o.1]" {
				t.Fatalf("the stop line's red %v, resume %v; want 1, [agentik relay resume o.1]", stop["red"], stop["resume"])
			}
			if night.Clean(n) {
				t.Fatalf("night.Clean true: %s, %s", n.Reason, states(n))
			}
			if k := strings.Count(out.String(), "agentik relay resume o.1\n"); k != 1 {
				t.Fatalf("the night printed `agentik relay resume o.1` %d times, want once:\n%s", k, out)
			}
		})
	}
}

// ar68Orch runs TestAR68_4's night at --max-sittings maxSittings, paused run
// while the night is paused (nil: nothing), the clock then moved to the
// wake: the world, the night, o.1's spawner, the night's own lines and
// each sitting's SittingStart.Red as Row read it.
func ar68Orch(t *testing.T, maxSittings int, paused func(w *world)) (*world, *night.Night, *effects.FakeSpawner, *ar68Out, map[string][]string) {
	t.Helper()
	w, q, files := orchNight(t, "- act 1: the row", nil)
	sit(t, files, "o.2", 2, "DONE: the row is built", nil)
	clock := &effects.StepClock{T: ar65T}
	w.fx.Clock = clock
	o1 := &effects.FakeSpawner{Out: ar65Event("five_hour", ar65T.Add(time.Hour)), Exit: 143, PID: 4242}
	sp := &ar68Rec{sp: ar68Split{o1: o1}}
	o := w.orchOpts(t, q, files)
	o.MaxSittings = maxSittings
	out := &ar68Out{}
	o.Out = out
	var mu sync.Mutex
	reds := map[string][]string{}
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		if rp.Sitting != nil {
			mu.Lock()
			reds[rp.ID] = slices.Clone(rp.Sitting.Red)
			mu.Unlock()
		}
		return row(r, rp)
	}
	res := ar68Run(w, q, o, sp)
	ar65Until(t, "o.1's pause", func() bool {
		return len(ar65Events(w.ledger.Written(), "pause")) == 1 && ar65Paused(o, "o.1")
	})
	if paused != nil {
		paused(w)
	}
	clock.Advance(time.Hour + 2*time.Minute)
	var n *night.Night
	select {
	case n = <-res:
	case <-time.After(120 * time.Second):
		t.Fatal("the night did not end within 120 s")
	}
	mu.Lock()
	defer mu.Unlock()
	return w, n, o1, out, maps.Clone(reds)
}

// ar68Halted checks o.1 halted at the wake: failed usage-limit, its tree
// there and unlocked, one spawn.
func ar68Halted(t *testing.T, w *world, n *night.Night, o1 *effects.FakeSpawner) {
	t.Helper()
	if s := n.Sittings["o.1"]; s == nil || s.State != relay.Failed || s.Reason != relay.UsageLimit {
		t.Fatalf("o.1 %+v, want failed usage-limit", s)
	}
	tree := ar67Tree(w, "o.1")
	if _, err := os.Stat(tree); err != nil || night.Locked(tree) {
		t.Fatalf("o.1's tree: %v, locked %v; want there, unlocked", err, night.Locked(tree))
	}
	if len(o1.Spawns()) != 1 {
		t.Fatalf("o.1 spawned %d times, want once", len(o1.Spawns()))
	}
}

// ar68Split starts o.1 on its FakeSpawner, every other child on the OS.
type ar68Split struct{ o1 *effects.FakeSpawner }

func (a ar68Split) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	if filepath.Base(sp.Dir) == "o.1" {
		return a.o1.Start(sp)
	}
	return effects.OSSpawner{}.Start(sp)
}
