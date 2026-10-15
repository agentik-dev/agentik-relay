package night_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// One gate verify at a time per root, its wait uncounted (AR78): nights on
// nightWorld (Poll 20 ms) whose rows' verify is a `block` line, the root's
// gate lock held by the test (h0) where a test says so.

// blockShell wraps a shell as slowShell does (AR78 ruling 6): `block <ms>`
// sleeps ms and exits 0 reading `ok`, or 124 at its timeout. It records
// each call's dir, start and end.
type blockShell struct {
	sh    effects.Shell
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
	s.mu.Unlock()
	return exit, "ok", nil
}

// runs is every block call so far, by start.
func (s *blockShell) runs() []blockCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := append([]blockCall(nil), s.calls...)
	sort.Slice(r, func(i, j int) bool { return r[i].start.Before(r[j].start) })
	return r
}

// ar78Block puts w on the block shell and every row of q on verify; it
// returns the shell and the root the rows' gates lock.
func ar78Block(t *testing.T, w *world, q *queue.Queue, verify string) (*blockShell, string) {
	t.Helper()
	sh := &blockShell{sh: w.fx.Shell}
	w.fx.Shell = sh
	for i := range q.Rows {
		q.Rows[i].Verify = verify
	}
	root, err := w.fx.Git.Root(w.root)
	if err != nil {
		t.Fatal(err)
	}
	return sh, root
}

// ar78World is a nightWorld of rows ids, each echo ok over ok.jsonl,
// verify their verify line.
func ar78World(t *testing.T, verify string, ids ...string) (*world, *queue.Queue, night.NightOptions, *blockShell, string) {
	t.Helper()
	var specs []spec
	for _, id := range ids {
		specs = append(specs, spec{id: id, echo: "ok", fixture: "ok.jsonl"})
	}
	w, q, by := nightWorld(t, specs...)
	sh, root := ar78Block(t, w, q, verify)
	return w, q, w.opts(t, by), sh, root
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

// ar78When runs act once cond holds, polled every 5 ms for 20 s at most.
func ar78When(cond func() bool, act func()) {
	go func() {
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			if cond() {
				act()
				return
			}
		}
	}()
}

// ar78Waits reads whether row has a `verify-wait` line in w's ledger.
func ar78Waits(w *world, row string) func() bool {
	return func() bool { return len(rowLines(w.ledger.Written(), "verify-wait", row)) > 0 }
}

// ar78Verify is row's verify `gate` line in lines, nil when none.
func ar78Verify(lines []map[string]any, row string) map[string]any {
	for _, l := range rowLines(lines, "gate", row) {
		if l["check"] == "verify" {
			return l
		}
	}
	return nil
}

// ar78Control writes line to o's night's control file.
func ar78Control(o night.NightOptions, line string) {
	night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), line)
}

// TestAR78_5_OneVerifyAtATime (AR78 rulings 1, 5): r1 and r2 at --parallel
// 2, each gate held until both have reached theirs, verify `block 800`:
// the later line starts after the earlier ends; one `verify-wait` line,
// for the later row, behind the earlier; both green.
func TestAR78_5_OneVerifyAtATime(t *testing.T) {
	w, q, o, sh, _ := ar78World(t, "block 800", "r1", "r2")
	o.Parallel = 2
	var mu sync.Mutex
	arrived, both := 0, make(chan struct{})
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := row(r, rp)
		g := ro.Gate
		ro.Gate = func(st *night.Staged, seen, isError bool, gw night.GateWait) (relay.Result, relay.StopReason, relay.Gate) {
			mu.Lock()
			if arrived++; arrived == 2 {
				close(both)
			}
			mu.Unlock()
			select {
			case <-both:
			case <-time.After(10 * time.Second):
			}
			return g(st, seen, isError, gw)
		}
		return b, ro, err
	}
	n := runNight(t, w, q, o)
	if states(n) != "r1=green r2=green" {
		t.Fatalf("night %s: %s; want both green", n.State, states(n))
	}
	r := sh.runs()
	if len(r) != 2 || r[1].start.Before(r[0].end) {
		t.Fatalf("the block runs %v; want two, the later started after the earlier ended", r)
	}
	all := w.ledger.Written()
	first := ""
	for _, l := range all {
		if l["event"] == "gate" && l["check"] == "verify" {
			first = fmt.Sprint(l["row"])
			break
		}
	}
	vw := ar65Events(all, "verify-wait")
	if len(vw) != 1 || vw[0]["behind"] != first || vw[0]["row"] == first || vw[0]["night"] != o.PlanOptions.Night {
		t.Fatalf("the verify-wait lines %v; want one, for the later row, behind %s, the night named", vw, first)
	}
}

// TestAR78_6_KillEndsTheWait (AR78 rulings 1, 3, 4): r1's verify behind
// h0; `kill r1` at its `verify-wait` line ends the wait within 1 s: the
// verify unrun, last the kill's, no block line run; r1 failed owner-kill.
func TestAR78_6_KillEndsTheWait(t *testing.T) {
	w, q, o, sh, root := ar78World(t, "block 100", "r1")
	ar78Hold(t, root)
	took := make(chan time.Duration, 1)
	ar78When(ar78Waits(w, "r1"), func() {
		start := time.Now()
		ar78Control(o, "kill r1")
		ar78When(func() bool { return ar78Verify(w.ledger.Written(), "r1") != nil }, func() { took <- time.Since(start) })
	})
	n := runNight(t, w, q, o)
	if s := n.Sittings["r1"]; s.State != relay.Failed || s.Reason != relay.OwnerKill {
		t.Fatalf("r1 %s %s; want failed owner-kill", s.State, s.Reason)
	}
	v := ar78Verify(w.ledger.Written(), "r1")
	if v == nil || v["status"] != relay.CheckUnrun || !strings.HasPrefix(fmt.Sprint(v["last"]), "killed waiting for the gate lock after ") {
		t.Fatalf("r1's verify gate line %v; want unrun, last the kill's", v)
	}
	select {
	case d := <-took:
		if d >= time.Second {
			t.Fatalf("the verify line landed %s after the kill, want under 1 s", d)
		}
	case <-time.After(time.Second):
		t.Fatal("no verify gate line timed after the kill")
	}
	if r := sh.runs(); len(r) != 0 {
		t.Fatalf("the block runs %v; want none: the line never runs", r)
	}
}

// TestAR78_7_StopRunsTheLine (AR78 ruling 4): a stop never ends the wait.
// r1 behind h0, `stop` at its `verify-wait` line, h0 freed at the
// `owner-stop` line: the line runs, r1 green. TestAR70_2's world: r2's
// gate behind h0, freed at the `pause` line: r2 runs, green (AR70 ruling
// 3), its sitting line green, before the wake.
func TestAR78_7_StopRunsTheLine(t *testing.T) {
	t.Run("stop", func(t *testing.T) {
		w, q, o, sh, root := ar78World(t, "block 100", "r1")
		free := ar78Hold(t, root)
		ar78When(ar78Waits(w, "r1"), func() {
			ar78Control(o, "stop")
			ar78When(func() bool { return len(ar65Events(w.ledger.Written(), "owner-stop")) > 0 }, free)
		})
		n := runNight(t, w, q, o)
		if states(n) != "r1=green" {
			t.Fatalf("night %s: %s; want r1 green", n.State, states(n))
		}
		if v := ar78Verify(w.ledger.Written(), "r1"); v == nil || v["status"] != relay.CheckGreen || len(sh.runs()) != 1 {
			t.Fatalf("r1's verify gate line %v, block runs %v; want green, one run", v, sh.runs())
		}
	})
	t.Run("the pause", func(t *testing.T) {
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		sh, root := ar78Block(t, w, q, "block 100")
		free := ar78Hold(t, root)
		release := make(chan struct{})
		sp := ar70New(release, nil)
		var once sync.Once
		row := o.Row
		o.Parallel = 2
		o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
			b, ro, err := row(r, rp)
			if g := ro.Gate; r.ID == "r2" { // r2's gate lets r1's stream go
				ro.Gate = func(st *night.Staged, seen, isError bool, gw night.GateWait) (relay.Result, relay.StopReason, relay.Gate) {
					once.Do(func() { close(release) })
					return g(st, seen, isError, gw)
				}
			}
			return b, ro, err
		}
		res := ar70Night(w, q, o, sp)
		ar65Until(t, "the pause line and r2's wait", func() bool {
			return len(ar65Events(w.ledger.Written(), "pause")) > 0 && ar78Waits(w, "r2")()
		})
		free()
		ar65Until(t, "r2 green", func() bool { return ar70Row(o, "r2") == "green" })
		clock.Advance(time.Hour + 2*time.Minute)
		n := ar65End(t, res)
		all := w.ledger.Written()
		if s := n.Sittings["r2"]; s.State != relay.Green {
			t.Fatalf("r2 %s %s; want green", s.State, s.Reason)
		}
		if v := ar78Verify(all, "r2"); v == nil || v["status"] != relay.CheckGreen {
			t.Fatalf("r2's verify gate line %v; want green", v)
		}
		sl := rowLines(all, "sitting", "r2")
		if len(sl) != 1 || sl[0]["state"] != "green" || len(sh.runs()) < 1 {
			t.Fatalf("r2's sitting lines %v, block runs %v; want one, green", sl, sh.runs())
		}
	})
}

// TestAR78_8_KilledInItsSitting (AR78 rulings 1, 3): TestAR5_OwnerKillEscalates's
// kill, r1 killed in its sitting: its gate's verify runs no block line and
// writes no `verify-wait` line, last `killed waiting for the gate lock
// after 0 s`; r1 failed owner-kill; the lock is free.
func TestAR78_8_KilledInItsSitting(t *testing.T) {
	pids := t.TempDir()
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl",
		env: []string{"FAKECLAUDE_DELAY_MS=2000", "FAKECLAUDE_RESULT_DELAY_MS=60000", "FAKECLAUDE_IGNORE_TERM=1", "FAKECLAUDE_PIDFILE=" + pids}})
	sh, root := ar78Block(t, w, q, "block 100")
	night.KillGrace = 300 * time.Millisecond
	o := w.opts(t, by)
	ar78When(func() bool {
		_, err := os.Stat(filepath.Join(pids, "r1.pid"))
		return err == nil
	}, func() { ar78Control(o, "kill r1") })
	n := runNight(t, w, q, o)
	if s := n.Sittings["r1"]; s.State != relay.Failed || s.Reason != relay.OwnerKill {
		t.Fatalf("r1 %s %s; want failed owner-kill", s.State, s.Reason)
	}
	all := w.ledger.Written()
	if v := ar78Verify(all, "r1"); v == nil || v["status"] != relay.CheckUnrun || v["last"] != "killed waiting for the gate lock after 0 s" {
		t.Fatalf("r1's verify gate line %v; want unrun, `killed waiting for the gate lock after 0 s`", v)
	}
	if vw := rowLines(all, "verify-wait", "r1"); len(vw) != 0 || len(sh.runs()) != 0 {
		t.Fatalf("verify-wait lines %v, block runs %v; want none", vw, sh.runs())
	}
	unlock, waited, err := night.GateLock(root, "t", nil, nil)
	if err != nil || waited != 0 {
		t.Fatalf("the lock after the night: waited %s, %v; want it free", waited, err)
	}
	unlock()
}
