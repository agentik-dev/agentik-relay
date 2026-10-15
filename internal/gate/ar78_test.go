package gate

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// One gate verify at a time per root, its wait uncounted (AR78): gates on
// newWorld under a 1 s limit, the root's gate lock held by the test (h0).

// blockShell wraps a shell as night's slowShell does (AR78 ruling 6):
// `block <ms>` sleeps ms and exits 0 reading `ok`, or 124 at its timeout.
// It records each call's dir, start and end, and calls after, when set,
// as each returns.
type blockShell struct {
	sh    effects.Shell
	after func(blockCall)
	mu    sync.Mutex
	calls []blockCall
}

type blockCall struct {
	dir        string
	start, end time.Time
}

func (s *blockShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	ms, ok := strings.CutPrefix(line, "block ")
	if !ok {
		return s.sh.Run(dir, line, timeout)
	}
	n, err := strconv.Atoi(ms)
	if err != nil {
		return 2, "block: " + ms, nil
	}
	c, exit := blockCall{dir: dir, start: time.Now()}, 0
	if d := time.Duration(n) * time.Millisecond; d < timeout {
		time.Sleep(d)
	} else {
		time.Sleep(timeout)
		exit = effects.CutExit
	}
	c.end = time.Now()
	s.mu.Lock()
	s.calls = append(s.calls, c)
	after := s.after
	s.mu.Unlock()
	if after != nil {
		after(c)
	}
	return exit, "ok", nil
}

// runs is every block call so far.
func (s *blockShell) runs() []blockCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]blockCall(nil), s.calls...)
}

// ar78Hold takes root's gate lock as h0; the free it returns (the test's
// cleanup too) lets it go, once.
func ar78Hold(t *testing.T, root string) func() {
	t.Helper()
	unlock, waited, err := night.GateLock(root, "h0", nil, nil)
	if err != nil || waited != 0 {
		t.Fatalf("h0's lock: waited %s, %v; want it at once", waited, err)
	}
	var once sync.Once
	free := func() { once.Do(unlock) }
	t.Cleanup(free)
	return free
}

// ar78World is newWorld on the block shell with a1 staged, and the root
// its gate's verify locks.
func ar78World(t *testing.T) (*world, *blockShell, *night.Staged, string) {
	t.Helper()
	w := newWorld(t)
	sh := &blockShell{sh: w.fx.Shell}
	w.fx.Shell = sh
	st := w.stage(t, "a.txt")
	root, err := w.fx.Git.Root(st.Path)
	if err != nil {
		t.Fatal(err)
	}
	return w, sh, st, root
}

// TestAR78_1_WaitIsNotTheLine (AR78 rulings 1, 2): behind h0, freed 1.2 s
// after the wait's mark, `block 600` under a 1 s limit is green: Wait 1,
// its `gate` line `wait_s` 1 and no `cut`; the marks are h0, then the
// end's "". With the lock free, `block 1500` is red verify-cut, cut 1, no
// `wait_s`, and no mark.
func TestAR78_1_WaitIsNotTheLine(t *testing.T) {
	w, sh, st, root := ar78World(t)
	p := pk([]string{"a.txt"})
	free := ar78Hold(t, root)
	var mu sync.Mutex
	var marks []string
	waiting := func(behind string, since time.Time) {
		mu.Lock()
		marks = append(marks, behind)
		mu.Unlock()
		if behind != "" {
			time.AfterFunc(1200*time.Millisecond, free)
		}
	}
	res, why, g, err := Run(w.fx, st, p, nil, seen, Verify{Name: "block 600", Limit: time.Second, Waiting: waiting})
	if err != nil || res != relay.GreenResult || why != "" || g.Verify == nil || g.Verify.Status != relay.CheckGreen || g.Verify.Wait != 1 || g.Verify.Cut != 0 {
		t.Fatalf("behind h0: Run = %s %q, %v; verify %+v; want green, Wait 1, no cut", res, why, err, g.Verify)
	}
	_, verify := ar59Gate(t, w)
	if _, cut := verify["cut"]; cut || fmt.Sprint(verify["wait_s"]) != "1" {
		t.Fatalf("behind h0: the verify gate line %v, want wait_s 1 and no cut", verify)
	}
	mu.Lock()
	got := fmt.Sprintf("%q", marks)
	mu.Unlock()
	if got != `["h0" ""]` {
		t.Fatalf("the marks %s, want h0 then the end's \"\"", got)
	}
	if r := sh.runs(); len(r) != 1 || r[0].end.Sub(r[0].start) >= time.Second {
		t.Fatalf("the block runs %v, want one, under its 1 s", r)
	}
	res, why, g, err = Run(w.fx, st, p, nil, seen, Verify{Name: "block 1500", Limit: time.Second, Waiting: waiting})
	if err != nil || res != relay.RedResult || why != relay.VerifyCut || g.Verify == nil || g.Verify.Cut != 1 || g.Verify.Wait != 0 {
		t.Fatalf("free: Run = %s %q, %v; verify %+v; want red verify-cut, cut 1, Wait 0", res, why, err, g.Verify)
	}
	_, verify = ar59Gate(t, w)
	if _, wait := verify["wait_s"]; wait || fmt.Sprint(verify["cut"]) != "1" {
		t.Fatalf("free: the verify gate line %v, want cut 1 and no wait_s", verify)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(marks) != 2 {
		t.Fatalf("the marks %q after a free lock, want no new one", marks)
	}
}

// TestAR78_2_TwoRootsNeverWait (AR78 ruling 1): one slot per root. With
// another root's lock held, this root's gate is green at once: Wait 0, no
// `wait_s`, no mark.
func TestAR78_2_TwoRootsNeverWait(t *testing.T) {
	w, _, st, root := ar78World(t)
	ar78Hold(t, filepath.Join(filepath.Dir(root), "root-a"))
	marks := 0
	res, why, g, err := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen,
		Verify{Name: "block 100", Limit: time.Second, Waiting: func(string, time.Time) { marks++ }})
	if err != nil || res != relay.GreenResult || why != "" || g.Verify == nil || g.Verify.Wait != 0 || marks != 0 {
		t.Fatalf("Run = %s %q, %v; verify %+v, %d marks; want green, Wait 0, no mark", res, why, err, g.Verify, marks)
	}
	if _, verify := ar59Gate(t, w); verify["wait_s"] != nil {
		t.Fatalf("the verify gate line %v, want no wait_s", verify)
	}
}

// TestAR78_3_MergeTakesNoGateLock (AR78 ruling 2): TestAR59_3_MergeNamesCut's
// green gate, merged over `block 100` while h0 holds the root's gate
// lock, lands in under 1 s: the merge keeps the root's lock alone.
func TestAR78_3_MergeTakesNoGateLock(t *testing.T) {
	w, sh, st, root := ar78World(t)
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	if res, _ := relay.Decide(green); res != relay.GreenResult {
		t.Fatalf("the gate is %s, want green", res)
	}
	ar78Hold(t, root)
	start := time.Now()
	sha, err := MergeIfGreen(w.fx, w.root, green, "a1", "block 100", nil)
	if err != nil || sha == "" {
		t.Fatalf("MergeIfGreen = %q, %v; want merged", sha, err)
	}
	if took := time.Since(start); took >= time.Second {
		t.Fatalf("the merge took %s behind the gate lock, want under 1 s", took)
	}
	if r := sh.runs(); len(r) != 1 {
		t.Fatalf("the block runs %v, want the merge's one", r)
	}
}

// TestAR78_4_HandGateWaits (AR78 ruling 2): Hand's verify (nil Killed)
// waits behind h0, freed 1.2 s after the row's acceptance line returns
// (the last line before the verify), then runs: green, Wait 1.
func TestAR78_4_HandGateWaits(t *testing.T) {
	w, sh, st, root := ar78World(t)
	st.Unlock()
	free := ar78Hold(t, root)
	var once sync.Once
	sh.after = func(blockCall) { once.Do(func() { time.AfterFunc(1200*time.Millisecond, free) }) }
	line := packet.Line{Shell: "block 1", Expect: "reads `ok`", Kind: packet.Reads, Value: "ok", N: 1}
	row := queue.Row{ID: "a1", Base: "main", Verify: "block 100", Packet: pk([]string{"a.txt"}, line)}
	res, why, g, err := Hand(w.fx, w.root, Verify{Limit: time.Second}, row)
	if err != nil || res != relay.GreenResult || why != "" || g.Verify == nil || g.Verify.Status != relay.CheckGreen || g.Verify.Wait != 1 {
		t.Fatalf("Hand = %s %q, %v; verify %+v; want green, Wait 1", res, why, err, g.Verify)
	}
	if r := sh.runs(); len(r) != 2 {
		t.Fatalf("the block runs %v, want the acceptance line's and the verify's", r)
	}
}
