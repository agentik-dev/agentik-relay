package gate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR59_2_GateUsesLimit is AR59 act 2: the gate's verify line runs
// under Verify.Limit: `sleep 3` under 1 s is red, cut at 1 s, and the gate
// carries the limit for the merge; with no limit (effects.Timeout) it is
// green. The cut is proved by its outcome (AR94).
func TestAR59_2_GateUsesLimit(t *testing.T) { ar59GateCut(t, 0) }

// ar59GateCut is TestAR59_2_GateUsesLimit's body over a git whose Clone
// sleeps slow (ar94SlowClone): `sleep 3 && touch <mark>` under 1 s is red,
// exit 124, Cut 1, VerifySeconds 1, and its mark never lands (ar94Absent),
// the call bounded only by ar94Bound, a hang guard (AR94); with no limit,
// `sleep 3` is green, VerifySeconds 900.
func ar59GateCut(t *testing.T, slow time.Duration) {
	t.Helper()
	w := newWorld(t)
	fx := w.fx
	fx.Git = ar94SlowClone{slow: slow}
	st := w.stage(t, "a.txt")
	p := pk([]string{"a.txt"})
	mark := filepath.Join(t.TempDir(), "mark")
	start := time.Now()
	res, _, g, err := Run(fx, st, p, nil, seen, Verify{Name: fmt.Sprintf("sleep 3 && touch '%s'", mark), Limit: time.Second})
	back := time.Now()
	if err != nil || res != relay.RedResult || g.Verify == nil || g.Verify.Status != relay.CheckRed ||
		g.Verify.Cut != 1 || g.Verify.BranchExit != 124 || g.VerifySeconds != 1 {
		t.Fatalf("under 1 s: %s, %v; verify %+v, gate's limit %d", res, err, g.Verify, g.VerifySeconds)
	}
	if took := back.Sub(start); took >= ar94Bound {
		t.Fatalf("the cut gate took %s: a hang, past the %s guard", took, ar94Bound)
	}
	ar94Forced(t, slow, back.Sub(start))
	res, why, g, err := Run(fx, st, p, nil, seen, Verify{Name: "sleep 3"})
	if err != nil || res != relay.GreenResult || why != "" || g.Verify.Cut != 0 || g.VerifySeconds != 900 {
		t.Fatalf("no limit: %s %s, %v; verify %+v, gate's limit %d", res, why, err, g.Verify, g.VerifySeconds)
	}
	ar94Absent(t, mark, back)
}

// TestAR59_2_HandTakesLimit is AR59 act 2: Hand reads v.Limit, so a hand
// gate over a row whose verify line outlives it is red, verify-cut.
func TestAR59_2_HandTakesLimit(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	st.Unlock()
	row := queue.Row{ID: "a1", Base: "main", Verify: "sleep 3", Packet: pk([]string{"a.txt"})}
	res, why, g, err := Hand(w.fx, w.root, Verify{Limit: time.Second}, row)
	if err != nil || res != relay.RedResult || why != relay.VerifyCut || g.Verify == nil || g.Verify.Cut != 1 {
		t.Fatalf("Hand = %s %q, %v; verify %+v; want red verify-cut", res, why, err, g.Verify)
	}
}

// ar59Gate is the result `gate` line and the verify `gate` line of the
// newest gate in w's ledger.
func ar59Gate(t *testing.T, w *world) (end, verify map[string]any) {
	t.Helper()
	for _, l := range w.ledger.Written() {
		if l["event"] != "gate" {
			continue
		}
		if l["check"] == "verify" {
			verify = l
		} else if _, ok := l["result"]; ok {
			end = l
		}
	}
	if end == nil || verify == nil {
		t.Fatalf("no result or verify gate line: %v", w.ledger.Written())
	}
	return end, verify
}

// TestAR59_3_CutReason is AR59 act 3: a verify line cut at its limit is
// red with its `gate` line carrying `cut`, and the result line reads
// reason verify-cut, detail `verify cut at 1 s`; a line that exits 124
// well under its limit is a plain red: no reason, no cut.
func TestAR59_3_CutReason(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	p := pk([]string{"a.txt"})
	res, why, _, err := Run(w.fx, st, p, nil, seen, Verify{Name: "sleep 3", Limit: time.Second})
	if err != nil || res != relay.RedResult || why != relay.VerifyCut {
		t.Fatalf("cut: Run = %s %q, %v; want red verify-cut", res, why, err)
	}
	end, verify := ar59Gate(t, w)
	if fmt.Sprint(verify["cut"]) != "1" || verify["status"] != relay.CheckRed {
		t.Fatalf("the verify gate line %v, want cut 1, red", verify)
	}
	if end["result"] != "red" || end["reason"] != "verify-cut" || end["detail"] != "verify cut at 1 s" {
		t.Fatalf("the result line %v, want red, verify-cut, `verify cut at 1 s`", end)
	}
	res, why, _, err = Run(w.fx, st, p, nil, seen, Verify{Name: "exit 124", Limit: 10 * time.Second})
	if err != nil || res != relay.RedResult || why != "" {
		t.Fatalf("exit 124 under 10 s: Run = %s %q, %v; want red with no reason", res, why, err)
	}
	end, verify = ar59Gate(t, w)
	if _, ok := verify["cut"]; ok || fmt.Sprint(verify["exit"]) != "124" {
		t.Fatalf("exit 124 under 10 s: the verify gate line %v, want exit 124 and no cut", verify)
	}
	if end["reason"] != "" || end["detail"] != nil {
		t.Fatalf("exit 124 under 10 s: the result line %v, want no reason, no detail", end)
	}
}

// TestAR59_3_MergeNamesCut is AR59 act 3: a green gate built by hand with
// VerifySeconds 1, merged over `sleep 3`: the error and the `merge` line
// (verify-cut, a MergeRefused) name the cut, and the root is where it was.
// The cut is proved by its outcome (AR94).
func TestAR59_3_MergeNamesCut(t *testing.T) { ar59MergeCut(t, 0) }

// ar59MergeCut is TestAR59_3_MergeNamesCut's body over a git whose Clone
// sleeps slow (ar94SlowClone), Gather's too: the merge's verify line
// `sleep 3 && touch <mark>` under the gate's 1 s is refused verify-cut, and
// its mark never lands (ar94Absent), the call bounded only by ar94Bound, a
// hang guard (AR94).
func ar59MergeCut(t *testing.T, slow time.Duration) {
	t.Helper()
	w := newWorld(t)
	fx := w.fx
	fx.Git = ar94SlowClone{slow: slow}
	st := w.stage(t, "a.txt")
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	green := Gather(fx, st, pk([]string{"a.txt"}), nil, seen)
	if res, _ := relay.Decide(green); res != relay.GreenResult {
		t.Fatalf("the gate is %s, want green", res)
	}
	green.VerifySeconds = 1
	mark := filepath.Join(t.TempDir(), "mark")
	start := time.Now()
	_, err := MergeIfGreen(fx, w.root, green, "a1", fmt.Sprintf("sleep 3 && touch '%s'", mark), nil)
	back := time.Now()
	if err == nil || !strings.Contains(err.Error(), `merge a1: verify cut at 1 s, last ""`) {
		t.Fatalf("MergeIfGreen = %v, want the cut named", err)
	}
	if took := back.Sub(start); took >= ar94Bound {
		t.Fatalf("the cut merge took %s: a hang, past the %s guard", took, ar94Bound)
	}
	ar94Forced(t, slow, back.Sub(start))
	l := w.ledger.Written()
	if m := l[len(l)-1]; m["event"] != "merge" || m["result"] != string(relay.VerifyCut) || !strings.Contains(fmt.Sprint(m["detail"]), "verify cut at 1 s") {
		t.Fatalf("the merge line %v, want verify-cut naming the cut", m)
	}
	var refused *relay.MergeRefused
	if !errors.As(err, &refused) || refused.Reason != relay.VerifyCut {
		t.Fatalf("MergeIfGreen = %v, want a MergeRefused, verify-cut", err)
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("the root moved to %s, want %s", head, before)
	}
	if s := gitIn(t, w.root, "status", "--porcelain", "--untracked-files=no"); s != "" {
		t.Fatalf("the root is dirty: %q", s)
	}
	ar94Absent(t, mark, back)
}
