package night_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// status names a verify waiting for its root's gate lock (verify-cut-r1
// §3, §5 packet 2; AR79): AR78's worlds, status.json read as the owner's
// `status` reads it.

// ar79Rows is the night's status.json and its rows by id.
func ar79Rows(o night.NightOptions) (night.Status, map[string]night.RowStatus) {
	st := night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night)
	by := map[string]night.RowStatus{}
	for _, r := range st.Rows {
		by[r.ID] = r
	}
	return st, by
}

// ar79Until reads status.json every 5 ms until ok holds of it, d at most;
// it returns the last read and whether ok held.
func ar79Until(o night.NightOptions, d time.Duration, ok func(st night.Status, by map[string]night.RowStatus) bool) (night.Status, map[string]night.RowStatus, bool) {
	for end := time.Now().Add(d); ; time.Sleep(5 * time.Millisecond) {
		st, by := ar79Rows(o)
		if ok(st, by) {
			return st, by, true
		}
		if time.Now().After(end) {
			return st, by, false
		}
	}
}

var ar79Wait = regexp.MustCompile(`^verify behind h0 since \d\d:\d\d:\d\dZ$`)

// TestAR79_1_StatusNamesTheWait (AR79 rulings 1, 2, 4): r1 (verify `block
// 1500`) and r2 (no verify line) at --parallel 2, h0 holding the root:
// while r1's `verify-wait` line stands, status.json's r1 reads `wait`
// `verify behind h0 since <hh:mm:ss>Z` within 2 s of the line, its state
// `running`, the night `running`, and r2's `wait` is "" throughout; `stop`
// written, the night `draining`, r1's `wait` unchanged; h0 freed, r1's
// `wait` reads "" while its line runs; at the end every `wait` is "", both
// green. A held row (AR81's band) never reads a wait: the band holds
// before the spawn, the gate lock after the child, so r1's band is null
// while it waits.
func TestAR79_1_StatusNamesTheWait(t *testing.T) {
	w, q, o, sh, root := ar78World(t, "block 1500", "r1", "r2")
	q.Rows[1].Verify = "" // r2: no verify line, so no lock
	o.Parallel = 2
	free := ar78Hold(t, root)
	var mu sync.Mutex
	var fails []string
	fail := func(f string, a ...any) {
		mu.Lock()
		fails = append(fails, fmt.Sprintf(f, a...))
		mu.Unlock()
	}
	// While r1's line runs (the lock taken, its mark ended): its wait "".
	w.fx.Shell = &ar78Held{sh: sh, hold: func(line string) {
		if !strings.HasPrefix(line, "block ") {
			return
		}
		if _, by, ok := ar79Until(o, 1200*time.Millisecond, func(_ night.Status, by map[string]night.RowStatus) bool {
			return by["r1"].Wait == ""
		}); !ok {
			fail("r1's wait %q while its line runs; want \"\"", by["r1"].Wait)
		}
	}}
	// r2's wait, and r1's before its line, read every 5 ms until the night ends.
	ended, watched := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watched)
		for {
			_, by := ar79Rows(o)
			if r2 := by["r2"]; r2.Wait != "" {
				fail("r2's wait %q; want \"\" throughout", r2.Wait)
				return
			}
			if r1 := by["r1"]; r1.Wait != "" && !ar79Waits(w, "r1") {
				fail("r1's wait %q before its verify-wait line; want \"\"", r1.Wait)
				return
			}
			select {
			case <-ended:
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}()
	scripted := make(chan struct{})
	go func() {
		defer close(scripted)
		defer free() // a failed step never leaves the night behind h0
		for end := time.Now().Add(20 * time.Second); !ar79Waits(w, "r1"); time.Sleep(5 * time.Millisecond) {
			if time.Now().After(end) {
				fail("no verify-wait line for r1 in 20 s")
				return
			}
		}
		st, by, ok := ar79Until(o, 2*time.Second, func(_ night.Status, by map[string]night.RowStatus) bool { return by["r1"].Wait != "" })
		r1 := by["r1"]
		if !ok || !ar79Wait.MatchString(r1.Wait) || r1.State != "running" || st.State != "running" || r1.Band != nil {
			fail("2 s after r1's verify-wait line: r1 wait %q state %s band %v, the night %s; want `verify behind h0 since <hh:mm:ss>Z`, running, null, running",
				r1.Wait, r1.State, r1.Band, st.State)
			return
		}
		wait := r1.Wait
		ar78Control(o, "stop")
		st, by, ok = ar79Until(o, 5*time.Second, func(st night.Status, _ map[string]night.RowStatus) bool { return st.State == "draining" })
		if !ok || by["r1"].Wait != wait {
			fail("after stop: the night %s, r1's wait %q; want draining, %q unchanged", st.State, by["r1"].Wait, wait)
		}
	}()
	n := runNight(t, w, q, o)
	close(ended)
	<-watched
	<-scripted
	for _, f := range fails {
		t.Error(f)
	}
	if states(n) != "r1=green r2=green" {
		t.Fatalf("night %s: %s; want both green", n.State, states(n))
	}
	if r := sh.runs(); len(r) != 1 {
		t.Fatalf("the block runs %v; want r1's one", r)
	}
	st, _ := ar79Rows(o)
	for _, r := range st.Rows {
		if r.Wait != "" {
			t.Errorf("at the end %s's wait %q; want \"\"", r.ID, r.Wait)
		}
	}
}

// ar79Waits reads whether row has a `verify-wait` line in w's ledger.
func ar79Waits(w *world, row string) bool { return ar78Waits(w, row)() }
