package night_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/gate"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// timeoutShell records the timeout each line ran under, by line.
type timeoutShell struct {
	sh  effects.Shell
	mu  sync.Mutex
	got map[string]time.Duration
}

func (s *timeoutShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	s.mu.Lock()
	s.got[line] = timeout
	s.mu.Unlock()
	return s.sh.Run(dir, line, timeout)
}

// TestAR59_2_PlanPassesLimit is AR59 act 2: the plan's verify baseline
// runs under Options.VerifyLimit (1200 s) when the measure's budget (3600
// s, a still clock) leaves more; an acceptance line keeps 900 s.
func TestAR59_2_PlanPassesLimit(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	q.Defaults.Verify = "echo v"
	rec := &timeoutShell{sh: w.fx.Shell, got: map[string]time.Duration{}}
	w.fx.Shell = rec
	o := w.opts(t, by)
	po := o.PlanOptions
	po.State, po.MeasureSeconds, po.VerifyLimit = o.State, 3600, 1200*time.Second
	p, err := night.BuildPlan(w.fx, q, po)
	if err != nil {
		t.Fatal(err)
	}
	if p.Verify == nil || p.Verify.Name != "echo v" {
		t.Fatalf("verify baseline %+v", p.Verify)
	}
	if got := rec.got["echo v"]; got != 1200*time.Second {
		t.Errorf("the verify line ran under %s, want 20m0s; all %v", got, rec.got)
	}
	if got := rec.got["echo ok"]; got != effects.Timeout {
		t.Errorf("the acceptance line ran under %s, want %s; all %v", got, effects.Timeout, rec.got)
	}
}

// TestAR59_3_RowReason is AR59 act 3: a row whose gate returns red,
// verify-cut takes verify-cut as its reason, printed on its red line; the
// night moves on (a packet failure).
func TestAR59_3_RowReason(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		p, ro, err := row(r, rp)
		if gate := ro.Gate; r.ID == "r1" {
			ro.Gate = func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
				_, _, g := gate(st, seen, isError)
				g.Verify = &relay.Check{Name: "make check", Expect: "exit 0", MainExit: -1, BranchExit: 124, Status: relay.CheckRed, Cut: 900}
				return relay.RedResult, relay.VerifyCut, g
			}
		}
		return p, ro, err
	}
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, w, q, o)
	if states(n) != "r1=red r2=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	if s := n.Sittings["r1"]; s.Reason != relay.VerifyCut {
		t.Fatalf("r1's reason %q, want verify-cut", s.Reason)
	}
	if !strings.Contains(out.String(), "relay r1: red verify-cut\n") {
		t.Errorf("no red verify-cut line for r1 in:\n%s", out.String())
	}
}

// TestAR59_5_MergeCutRow is AR59's fix round, block 1 (review adv1 F1): a
// green row whose merge verify is cut at the gate's limit reads red,
// verify-cut, never green with an empty reason; its `merge` line names
// the cut, and the night prints its resume line.
func TestAR59_5_MergeCutRow(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", merge: "merge-on-green"})
	o := w.opts(t, by)
	o.Merge = func(r queue.Row, g relay.Gate) (string, error) {
		g.VerifySeconds = 1 // the gate's limit; the merged tree's line runs past it
		return gate.MergeIfGreen(w.fx, w.root, g, r.ID, "sleep 3")
	}
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, w, q, o)
	if states(n) != "r1=red" || n.Sittings["r1"].Reason != relay.VerifyCut || n.Sittings["r1"].MergeSHA != "" {
		t.Fatalf("states %s, reason %q, merge %q; want r1 red verify-cut, unmerged", states(n), n.Sittings["r1"].Reason, n.Sittings["r1"].MergeSHA)
	}
	if m := lastOf(w.ledger.Written(), "merge"); m["result"] != string(relay.VerifyCut) || !strings.Contains(m["detail"].(string), "verify cut at 1 s") {
		t.Fatalf("the merge line %v, want result verify-cut naming the cut", m)
	}
	if got := out.String(); !strings.Contains(got, "relay r1: red verify-cut\n") || !strings.Contains(got, "agentik relay resume r1") {
		t.Errorf("no red verify-cut line and resume line in:\n%s", got)
	}
}

// slowShell runs every line but slow, which needs more than 900 s: under
// a smaller limit it is cut (124), under more it exits 0. It records each
// timeout slow ran under.
type slowShell struct {
	sh  effects.Shell
	mu  sync.Mutex
	got []time.Duration
}

func (s *slowShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	if line != "slow" {
		return s.sh.Run(dir, line, timeout)
	}
	s.mu.Lock()
	s.got = append(s.got, timeout)
	s.mu.Unlock()
	if timeout < 1000*time.Second {
		return 124, "ok  pkg/a 300s", nil
	}
	return 0, "ok", nil
}

// TestAR59_6_MemoUnderLimit is AR59's fix round, block 2 (review adv2
// F1): the memo keeps the limit a line ran under. At one head, a verify
// baseline cut at 900 s, from an older relay's file with no limit (read
// as 900 s), answers a plan under 900 s and is no answer under 1200 s (the
// line runs again, green); that pass then answers 1200 s and 900 s alike.
func TestAR59_6_MemoUnderLimit(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	q.Defaults.Verify = "slow"
	rec := &slowShell{sh: w.fx.Shell}
	w.fx.Shell = rec
	o := w.opts(t, by)
	po := o.PlanOptions
	po.State, po.MeasureSeconds = o.State, 3600
	plan := func(limit time.Duration, want string, runs int) {
		t.Helper()
		po.VerifyLimit = limit
		p, err := night.BuildPlan(w.fx, q, po)
		if err != nil {
			t.Fatal(err)
		}
		if p.Verify == nil || p.Verify.Status != want || len(rec.got) != runs {
			t.Fatalf("under %s: verify %+v, runs %v; want %s after %d runs", limit, p.Verify, rec.got, want, runs)
		}
	}
	plan(0, relay.CheckRed, 1)                     // 900 s: cut
	path := filepath.Join(o.State, night.MemoFile) // an older relay's file: no limit
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var drop func(v any)
	drop = func(v any) {
		if m, ok := v.(map[string]any); ok {
			if _, entry := m["status"]; entry {
				delete(m, "limit")
			}
			for _, e := range m {
				drop(e)
			}
		}
	}
	drop(doc)
	b, _ = json.Marshal(doc)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	plan(900*time.Second, relay.CheckRed, 1)    // read as measured at 900 s: answers
	plan(1200*time.Second, relay.CheckGreen, 2) // a 900 s cut is no answer under 1200 s
	plan(1200*time.Second, relay.CheckGreen, 2) // its own measure answers
	plan(900*time.Second, relay.CheckGreen, 2)  // a pass answers any limit
}
