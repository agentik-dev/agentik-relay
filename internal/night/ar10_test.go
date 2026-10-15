package night

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

func nestedEnv(k string) string {
	return map[string]string{effects.MeasureDepthVar: "1"}[k]
}

// TestAR10_0_PlanNestedMeasuresNothing: a plan under RELAY_MEASURE_DEPTH
// runs no shell line (not even `make -n check`); every baseline check and
// the verify line are `unrun: nested`, the ledger says so, and Print names it.
func TestAR10_0_PlanNestedMeasuresNothing(t *testing.T) {
	repo, q := fixture(t)
	fx, _, sh, l := fakes(repo, false)
	fx.Getenv = nestedEnv
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(sh.Calls()); n != 0 || !p.Nested || !p.Valid {
		t.Fatalf("nested plan: %d shell calls %v, nested %v, valid %v", n, sh.Calls(), p.Nested, p.Valid)
	}
	if v := p.Verify; v == nil || v.Status != relay.CheckUnrun || v.MainLast != effects.NestedReason {
		t.Fatalf("verify = %+v", v)
	}
	for _, r := range p.Rows {
		if len(r.Baseline) == 0 {
			t.Fatalf("row %s: no baseline", r.ID)
		}
		for _, c := range r.Baseline {
			if c.Status != relay.CheckUnrun || c.MainLast != effects.NestedReason || c.MainExit != -1 {
				t.Fatalf("row %s: check %+v", r.ID, c)
			}
		}
	}
	for _, w := range l.Written() {
		for _, c := range w["baseline"].([]any) {
			if m := c.(map[string]any); m["status"] != relay.CheckUnrun || m["last"] != effects.NestedReason {
				t.Fatalf("ledger check %v", m)
			}
		}
	}
	var b bytes.Buffer
	Print(&b, p)
	if !strings.Contains(b.String(), "  nested ("+effects.MeasureDepthVar+" set): nothing measured\n") {
		t.Fatalf("Print does not name the guard:\n%s", b.String())
	}
	fx.Getenv = func(string) string { return "" }
	if p, _ := BuildPlan(fx, q, Options{Repo: repo, State: "/state"}); p.Nested || len(sh.Calls()) == 0 {
		t.Fatalf("an unset depth measured nothing: nested %v", p.Nested)
	}
}

// TestAR10_0_BaselineOnMainNested: the gate's baseline on main under the
// guard is every line `unrun: nested`, with no shell call.
func TestAR10_0_BaselineOnMainNested(t *testing.T) {
	repo, q := fixture(t)
	fx, _, sh, _ := fakes(repo, false)
	fx.Getenv = nestedEnv
	cs := BaselineOnMain(fx, "/root", q.Rows[0].Packet.Acceptance)
	if len(cs) != len(q.Rows[0].Packet.Acceptance) || len(sh.Calls()) != 0 {
		t.Fatalf("%d checks, %d shell calls", len(cs), len(sh.Calls()))
	}
	for _, c := range cs {
		if c.Status != relay.CheckUnrun || c.MainLast != effects.NestedReason {
			t.Fatalf("check %+v", c)
		}
	}
}

// invalidFixture is the fixture with no prior ledger: a3's dependency
// a2-relay-design is unknown to the queue, the ledger and git, so a3 is
// invalid while a4 and a5 are not.
func invalidFixture(t *testing.T) (string, *queue.Queue, effects.Effects, *effects.FakeShell, *effects.FakeLedger) {
	t.Helper()
	repo, q := fixture(t)
	fx, _, sh, l := fakes(repo, false)
	l.Prior = nil
	return repo, q, fx, sh, l
}

// TestAR10_1_InvalidRowMeasuresNothing: one invalid row and no shell line
// runs, not even `make -n check`: every row's checks and the verify line are
// `unrun: invalid`, and so is every `plan` ledger line; the run path refuses
// the plan with the shell still untouched.
func TestAR10_1_InvalidRowMeasuresNothing(t *testing.T) {
	repo, q, fx, sh, l := invalidFixture(t)
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Valid || len(sh.Calls()) != 0 || p.Rows[0].State != StateInvalid {
		t.Fatalf("valid %v, shell %v, a3 %s", p.Valid, sh.Calls(), p.Rows[0].State)
	}
	if v := p.Verify; v == nil || v.Status != relay.CheckUnrun || v.MainLast != InvalidReason {
		t.Fatalf("verify = %+v", v)
	}
	for _, r := range p.Rows {
		if len(r.Baseline) == 0 {
			t.Fatalf("row %s: no checks", r.ID)
		}
		for _, c := range r.Baseline {
			if c.Status != relay.CheckUnrun || c.MainLast != InvalidReason || c.MainExit != -1 {
				t.Fatalf("row %s: check %+v", r.ID, c)
			}
		}
	}
	for _, w := range l.Written() {
		for _, c := range w["baseline"].([]any) {
			if m := c.(map[string]any); m["status"] != relay.CheckUnrun || m["last"] != InvalidReason {
				t.Fatalf("ledger check %v", m)
			}
		}
	}
	if _, err := Run(fx, &effects.FakeSpawner{}, q, NightOptions{PlanOptions: Options{Repo: repo}, State: "/state"}); err == nil || len(sh.Calls()) != 0 {
		t.Fatalf("run: err %v, shell %v", err, sh.Calls())
	}
}

// TestAR10_1_InvalidPlanExits2: an invalid plan's exit is 2 and Print's
// verdict says invalid with every check unrun; a valid one exits 0.
func TestAR10_1_InvalidPlanExits2(t *testing.T) {
	repo, q, fx, sh, _ := invalidFixture(t)
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a4-schemas"}, Until: "25:99"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Exit() != 2 || len(sh.Calls()) != 0 {
		t.Fatalf("exit %d, shell %v", p.Exit(), sh.Calls())
	}
	var b bytes.Buffer
	Print(&b, p)
	if s := b.String(); !strings.Contains(s, "would open with 1 row; invalid\n") || !strings.Contains(s, "(0 green, 0 red, 9 unrun)") {
		t.Fatalf("Print:\n%s", s)
	}
	fx.Ledger.(*effects.FakeLedger).Prior = []map[string]any{{"event": "gate", "row": "a2-relay-design", "result": "green"}}
	if p, _ := BuildPlan(fx, q, Options{Repo: repo, State: "/state"}); p.Exit() != 0 || len(sh.Calls()) == 0 {
		t.Fatalf("a valid plan: exit %d, %d shell calls", p.Exit(), len(sh.Calls()))
	}
}

// TestAR10_2_PendingRowUnrun: only ready rows are measured; a5, pending on
// a4, reads `unrun: pending` and adds no shell line; the verify line and the
// ready rows' lines are the measured count.
func TestAR10_2_PendingRowUnrun(t *testing.T) {
	repo, q := fixture(t)
	fx, g, sh, _ := fakes(repo, false)
	// a2-relay-design landed in git: AR22 act 3 (ADV13-7) took away the
	// ledger green line that satisfied it.
	g.Existing["a2-relay-design"], g.Merged["a2-relay-design..main"] = true, true
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	a3, a4, a5 := p.Rows[0], p.Rows[1], p.Rows[2]
	if a5.State != StatePending || len(a5.Baseline) == 0 {
		t.Fatalf("a5 = %s, %d checks", a5.State, len(a5.Baseline))
	}
	for _, c := range a5.Baseline {
		if c.Status != relay.CheckUnrun || c.MainLast != PendingReason || c.MainExit != -1 {
			t.Fatalf("a5 check %+v", c)
		}
	}
	measured := len(a3.Baseline) + len(a4.Baseline) + 1                                           // the verify line
	if p.Measured != measured || p.Skipped != len(a5.Baseline) || len(sh.Calls()) != measured+1 { // + make -n check
		t.Fatalf("measured %d skipped %d, %d shell calls; want %d, %d", p.Measured, p.Skipped, len(sh.Calls()), measured, len(a5.Baseline))
	}
}

// TestAR10_2_LandedFromLastPlanLine: a row git holds landed (AR22 act 3:
// never its own green ledger line) has its last measuring `plan` line's
// checks, with no shell line; a row merged into main with no such line
// reads `unrun: landed`.
func TestAR10_2_LandedFromLastPlanLine(t *testing.T) {
	repo, q := fixture(t)
	fx, g, sh, l := fakes(repo, false)
	g.Existing["a3-machine"] = true
	g.Ancestors["a3-machine..main"] = true
	g.Merged["a3-machine..main"] = true // merged, never planned
	g.Existing["a4-schemas"], g.Merged["a4-schemas..main"] = true, true
	var old, unrun []any
	for i := range q.Rows[1].Packet.Acceptance {
		old = append(old, map[string]any{"n": float64(i + 1), "exit": float64(i), "last": "was", "status": relay.CheckRed})
		unrun = append(unrun, map[string]any{"n": float64(i + 1), "exit": float64(-1), "last": "nested", "status": relay.CheckUnrun})
	}
	l.Prior = append(l.Prior,
		map[string]any{"event": "plan", "row": "a4-schemas", "state": "ready", "baseline": old},
		map[string]any{"event": "gate", "row": "a4-schemas", "result": "green"},
		map[string]any{"event": "plan", "row": "a4-schemas", "state": "ready", "baseline": unrun}) // measured nothing
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a3-machine", "a4-schemas"}})
	if err != nil {
		t.Fatal(err)
	}
	a3, a4 := p.Rows[0], p.Rows[1]
	if a3.State != StateLanded || a4.State != StateLanded || !p.Valid {
		t.Fatalf("a3 %s, a4 %s, valid %v", a3.State, a4.State, p.Valid)
	}
	for _, c := range a3.Baseline {
		if c.Status != relay.CheckUnrun || c.MainLast != LandedReason {
			t.Fatalf("a3 check %+v", c)
		}
	}
	for i, c := range a4.Baseline {
		if c.Status != relay.CheckRed || c.MainExit != i || c.MainLast != "was" || c.Name != q.Rows[1].Packet.Acceptance[i].Shell {
			t.Fatalf("a4 check %d %+v", i, c)
		}
	}
	if len(sh.Calls()) != 0 || p.Measured != 0 || p.Verify.Status != relay.CheckUnrun {
		t.Fatalf("no ready row: shell %v, measured %d, verify %+v", sh.Calls(), p.Measured, p.Verify)
	}
}

// TestAR10_2_CountsInPrintAndLedger: Print ends `measured N lines, skipped
// M` and every `plan` line carries both.
func TestAR10_2_CountsInPrintAndLedger(t *testing.T) {
	repo, q := fixture(t)
	fx, _, _, l := fakes(repo, false)
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	Print(&b, p)
	want := fmt.Sprintf("measured %d lines, skipped %d\n", p.Measured, p.Skipped)
	if p.Measured == 0 || p.Skipped == 0 || !strings.HasSuffix(b.String(), want) {
		t.Fatalf("Print does not end %q:\n%s", want, b.String())
	}
	for _, w := range l.Written() {
		if number(w["measured"]) != float64(p.Measured) || number(w["skipped"]) != float64(p.Skipped) {
			t.Fatalf("plan line %v", w)
		}
	}
}

// memoPlan is the fixture with the root's HEAD known, so the memo (§15)
// keys its measurements.
func memoPlan(t *testing.T) (string, *queue.Queue, effects.Effects, *effects.FakeGit, *effects.FakeShell) {
	t.Helper()
	repo, q := fixture(t)
	fx, g, sh, _ := fakes(repo, false)
	g.Heads = map[string]string{"/root": "aaa111"}
	return repo, q, fx, g, sh
}

// TestAR10_3_SecondPlanSameHeadNoShell: a second plan at the same head
// makes no shell call; its checks equal the first plan's, and the memo is
// `<state>/baselines.json`, written by rename (no temp file left).
func TestAR10_3_SecondPlanSameHeadNoShell(t *testing.T) {
	repo, q, fx, _, sh := memoPlan(t)
	p1, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	n := len(sh.Calls())
	if n == 0 || p1.Measured == 0 {
		t.Fatalf("the first plan measured nothing: %d calls, measured %d", n, p1.Measured)
	}
	fs := fx.FS.(*effects.FakeFS)
	if !fs.Exists("/state/baselines.json") || fs.Exists("/state/baselines.json.tmp") {
		t.Fatalf("memo files: %v", fs.Files)
	}
	p2, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if m := len(sh.Calls()); m != n {
		t.Fatalf("the second plan ran %d shell lines: %v", m-n, sh.Calls()[n:])
	}
	if p2.Measured != 0 || p2.Skipped != p1.Measured+p1.Skipped {
		t.Fatalf("second plan counts: measured %d, skipped %d (first %d, %d)", p2.Measured, p2.Skipped, p1.Measured, p1.Skipped)
	}
	if fmt.Sprint(p1.Verify, p1.Rows[1].Baseline) != fmt.Sprint(p2.Verify, p2.Rows[1].Baseline) {
		t.Fatalf("checks differ:\n%v %v\n%v %v", p1.Verify, p1.Rows[1].Baseline, p2.Verify, p2.Rows[1].Baseline)
	}
	fx.Getenv = func(k string) string { return map[string]string{"XDG_STATE_HOME": "/"}[k] }
	fx.FS.WriteFile("/agentik-relay/baselines.json", fs.Files["/state/baselines.json"], 0o600)
	BaselineOnMain(fx, "/root", q.Rows[1].Packet.Acceptance)
	if m := len(sh.Calls()); m != n {
		t.Fatalf("BaselineOnMain at the same head ran %d lines", m-n)
	}
}

// TestAR10_3_MovedHeadMeasuresAgain: a moved head measures again, and the
// memo drops the old head's keys for that root.
func TestAR10_3_MovedHeadMeasuresAgain(t *testing.T) {
	repo, q, fx, g, sh := memoPlan(t)
	if _, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"}); err != nil {
		t.Fatal(err)
	}
	n := len(sh.Calls())
	g.Heads["/root"] = "bbb222"
	if _, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"}); err != nil {
		t.Fatal(err)
	}
	if m := len(sh.Calls()); m != 2*n {
		t.Fatalf("a moved head ran %d lines, want %d", m-n, n)
	}
	b, _ := fx.FS.ReadFile("/state/baselines.json")
	if strings.Contains(string(b), "aaa111") || !strings.Contains(string(b), "bbb222") {
		t.Fatalf("memo keeps the old head:\n%s", b)
	}
}

// TestAR10_3_UnrunNeverStored: a line that could not start is unrun and
// never memoized, so the next plan at the same head runs it again.
func TestAR10_3_UnrunNeverStored(t *testing.T) {
	repo, q, fx, _, sh := memoPlan(t)
	line := q.Rows[1].Packet.Acceptance[0].Shell
	sh.Results[line] = effects.ShellResult{Exit: -1, Err: fmt.Errorf("cannot start")}
	runs := func() int {
		n := 0
		for _, c := range sh.Calls() {
			if c == "/root: "+line {
				n++
			}
		}
		return n
	}
	var per [2]int
	for i := range per {
		if _, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"}); err != nil {
			t.Fatal(err)
		}
		per[i] = runs()
	}
	b, _ := fx.FS.ReadFile("/state/baselines.json")
	if per[0] == 0 || per[1] != 2*per[0] || strings.Contains(string(b), `"status": "unrun"`) || strings.Contains(string(b), line) {
		t.Fatalf("unrun line ran %v times; memo:\n%s", per, b)
	}
}

// advShell is a shell each line of which takes step on a StepClock; it
// records every line's timeout.
type advShell struct {
	sh       effects.Shell
	clock    *effects.StepClock
	step     time.Duration
	mu       sync.Mutex
	timeouts []time.Duration
}

func (a *advShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	a.mu.Lock()
	a.timeouts = append(a.timeouts, timeout)
	a.mu.Unlock()
	defer a.clock.Advance(a.step)
	return a.sh.Run(dir, line, timeout)
}

// TestAR10_4_BudgetStopsLines: --measure-seconds is the whole plan's
// budget: each line runs under the lesser of 900 s and the remainder, none
// starts past it, the rest are `unrun: budget`, counted on Plan.Budget,
// the plan lines and Print's summary.
func TestAR10_4_BudgetStopsLines(t *testing.T) {
	repo, q := fixture(t)
	fx, _, sh, l := fakes(repo, false)
	clock := &effects.StepClock{T: time.Date(2026, 9, 23, 22, 0, 0, 0, time.UTC)}
	adv := &advShell{sh: sh, clock: clock, step: 250 * time.Second}
	fx.Clock, fx.Shell = clock, adv
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", MeasureSeconds: 600})
	if err != nil {
		t.Fatal(err)
	}
	// The probe at 0 s (its own minute), verify at 250 s (350 s left), one
	// acceptance line at 500 s (100 s left); at 750 s the budget is spent.
	if got := fmt.Sprint(adv.timeouts); got != "[1m0s 5m50s 1m40s]" {
		t.Fatalf("timeouts %s", got)
	}
	if p.Verify == nil || p.Verify.Status == relay.CheckUnrun {
		t.Fatalf("verify %+v", p.Verify)
	}
	cut := 0
	for _, r := range p.Rows {
		for _, c := range r.Baseline {
			if c.MainLast == BudgetReason {
				if c.Status != relay.CheckUnrun || c.MainExit != -1 {
					t.Fatalf("check %+v", c)
				}
				cut++
			}
		}
	}
	if cut == 0 || p.Budget != cut || p.Measured != 2 {
		t.Fatalf("cut %d, Plan.Budget %d, measured %d", cut, p.Budget, p.Measured)
	}
	for _, w := range l.Written() {
		if number(w["budget"]) != float64(cut) {
			t.Fatalf("plan line budget %v, want %d", w["budget"], cut)
		}
	}
	var b bytes.Buffer
	Print(&b, p)
	if want := fmt.Sprintf(", %d past the budget\n", cut); !strings.HasSuffix(b.String(), want) {
		t.Fatalf("Print does not count the budget (%q):\n%s", want, b.String())
	}
	if d, _ := (&Options{}).measureBudget(nil); d != 600*time.Second || effects.Timeout != 900*time.Second {
		t.Fatal("the default budget is not 600 s or the line cap not 900 s")
	}
}

// inflight is a shell that sleeps a little per line and records the most
// lines it ever ran at once in one directory.
type inflight struct {
	sh  effects.Shell
	mu  sync.Mutex
	now map[string]int
	max int
}

func (f *inflight) Run(dir, line string, timeout time.Duration) (int, string, error) {
	f.mu.Lock()
	f.now[dir]++
	if f.now[dir] > f.max {
		f.max = f.now[dir]
	}
	f.mu.Unlock()
	time.Sleep(2 * time.Millisecond)
	defer func() { f.mu.Lock(); f.now[dir]--; f.mu.Unlock() }()
	return f.sh.Run(dir, line, timeout)
}

// TestAR10_4_OneMeasurementPerRoot: plans and baselines on one root, run at
// once, measure one line at a time (the root's mutex); run with -race.
func TestAR10_4_OneMeasurementPerRoot(t *testing.T) {
	repo, q := fixture(t)
	_, _, sh, _ := fakes(repo, false)
	shared := &inflight{sh: sh, now: map[string]int{}}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		fx, _, _, _ := fakes(repo, false)
		fx.Shell = shared
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if _, err := BuildPlan(fx, q, Options{Repo: repo, State: fmt.Sprintf("/state%d", i)}); err != nil {
				t.Error(err)
			}
		}(i)
		go func() {
			defer wg.Done()
			BaselineOnMain(fx, "/root", q.Rows[1].Packet.Acceptance)
		}()
	}
	wg.Wait()
	if n := len(sh.Calls()); shared.max != 1 || n < 8 {
		t.Fatalf("at most %d lines at once in the root over %d calls", shared.max, n)
	}
}

// jumpClock is a StepClock whose waits of at most jump pass at once (it
// moves itself to their deadline: the breaker's polls); longer waits (the
// budget) fire only when the clock reaches them.
type jumpClock struct {
	*effects.StepClock
	jump time.Duration
}

func (c jumpClock) After(d time.Duration) <-chan time.Time {
	ch := c.StepClock.After(d)
	if d > 0 && d <= c.jump {
		c.StepClock.Advance(d)
	}
	return ch
}

func newJump() jumpClock {
	return jumpClock{&effects.StepClock{T: time.Date(2026, 9, 23, 23, 0, 0, 0, time.UTC)}, DefaultBreaker.Poll}
}

func breakerLines(l *effects.FakeLedger) []string {
	var out []string
	for _, w := range l.Written() {
		if w["event"] == "breaker" {
			out = append(out, fmt.Sprint(w["state"], " ", w["load"], "/", w["cores"]))
		}
	}
	return out
}

// TestAR10_5_BreakerOpensAndCloses: above 4x the cores the breaker opens,
// polls every 30 s through the clock and closes under 2x; between the two
// it never opens. Through a plan: one `breaker` line at the open, one at
// the close, `breaker: {opens, wait_s, load}` on Plan, every plan line and
// the --json key.
func TestAR10_5_BreakerOpensAndCloses(t *testing.T) {
	var notes []string
	note := func(open bool, avg float64, cores int) { notes = append(notes, fmt.Sprint(open, avg, cores)) }
	st, ok := DefaultBreaker.Wait(context.Background(), &effects.FakeLoad{Series: []float64{70, 70, 20}}, newJump(), note)
	if !ok || st.Opens != 1 || st.WaitS != 60 || st.Load != 70 || fmt.Sprint(notes) != "[true 70 16 false 20 16]" {
		t.Fatalf("stats %+v ok %v notes %v", st, ok, notes)
	}
	if st, ok := DefaultBreaker.Wait(context.Background(), &effects.FakeLoad{Series: []float64{50}}, newJump()); !ok || st.Opens != 0 || st.WaitS != 0 {
		t.Fatalf("under 4x: stats %+v ok %v", st, ok)
	}
	if b := DefaultBreaker; b.Open != 4 || b.Close != 2 || b.Poll != 30*time.Second || b.Max != 10*time.Minute {
		t.Fatalf("DefaultBreaker %+v", b)
	}

	repo, q := fixture(t)
	fx, _, sh, l := fakes(repo, false)
	fx.Clock, fx.Load = newJump(), &effects.FakeLoad{Series: []float64{70, 70, 20}}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(breakerLines(l)); got != "[open 70/16 close 20/16]" {
		t.Fatalf("breaker lines %s", got)
	}
	if p.Breaker.Opens != 1 || p.Breaker.WaitS != 60 || p.Breaker.Load != 70 || len(sh.Calls()) == 0 || p.Budget != 0 {
		t.Fatalf("Plan.Breaker %+v, %d calls, budget %d", p.Breaker, len(sh.Calls()), p.Budget)
	}
	for _, w := range l.Written() {
		if w["event"] != "plan" {
			continue
		}
		if b, _ := w["breaker"].(map[string]any); b == nil || number(b["opens"]) != 1 || number(b["wait_s"]) != 60 || number(b["load"]) != 70 {
			t.Fatalf("plan line breaker %v", w["breaker"])
		}
	}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"breaker":{"opens":1,"wait_s":60,"load":70}`) {
		t.Fatalf("--json lacks the breaker: %s", b)
	}
}

// TestAR10_5_StillOpenSpawnStopsLoad: a load still above 4x at 10 minutes
// leaves every baseline `unrun: load` (one open, no shell line) and, before
// a spawn, spawns nothing: no spawn or exit line, the row failed `load`,
// `stop: load` and the breaker on the sitting line, and the night stops
// `stopped:load`.
func TestAR10_5_StillOpenSpawnStopsLoad(t *testing.T) {
	repo, q := fixture(t)
	fx, _, sh, l := fakes(repo, false)
	fx.Clock, fx.Load = newJump(), &effects.FakeLoad{Series: []float64{90}}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(sh.Calls()); n != 0 || p.Verify == nil || p.Verify.Status != relay.CheckUnrun || p.Verify.MainLast != LoadReason || p.Measured != 0 {
		t.Fatalf("%d calls %v, verify %+v, measured %d", n, sh.Calls(), p.Verify, p.Measured)
	}
	for _, c := range p.Rows[1].Baseline {
		if c.Status != relay.CheckUnrun || c.MainLast != LoadReason {
			t.Fatalf("check %+v", c)
		}
	}
	if got := fmt.Sprint(breakerLines(l)); got != "[open 90/16]" || p.Breaker.WaitS != 600 {
		t.Fatalf("breaker lines %s, stats %+v", got, p.Breaker)
	}

	sp := &effects.FakeSpawner{}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1",
		Spawn: []string{"claude", "-p", "--model", "claude-opus-5-5"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}
	s, _ := RunRow(fx, sp, st, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: time.Hour})
	if s.State != relay.Failed || s.Reason != relay.LoadOpen || relay.LoadOpen != "load" || len(sp.Spawns()) != 0 {
		t.Fatalf("sitting %+v, %d spawns", s, len(sp.Spawns()))
	}
	var sit map[string]any
	for _, w := range l.Written() {
		if w["row"] != "r1" {
			continue
		}
		switch w["event"] {
		case "spawn", "exit":
			t.Fatalf("a %s line for a row never spawned: %v", w["event"], w)
		case "sitting":
			sit = w
		}
	}
	if b, _ := sit["breaker"].(map[string]any); sit["stop"] != "load" || b == nil || number(b["opens"]) != 1 || number(b["wait_s"]) != 600 {
		t.Fatalf("sitting line %v", sit)
	}
	if n, err := relay.NightStep(relay.NightRunning, relay.OnSittingFailed, s.Reason); err != nil || n != relay.Stopped(relay.LoadOpen) {
		t.Fatalf("the night after a load stop: %s %v", n, err)
	}
}

// TestAR10_5_RedBaselineLoadedUnrun: D4. Measured above 2x the cores (the
// breaker shut: under 4x), a red check or verify line is `unrun: loaded`,
// never a same-as-main anchor; a green one stands; neither is stored.
// Unloaded, the same red measures again and stands, stored with its load.
func TestAR10_5_RedBaselineLoadedUnrun(t *testing.T) {
	repo, q, fx, _, sh := memoPlan(t)
	sh.Results["make check"] = effects.ShellResult{Exit: 2, Last: "FAIL"}
	fx.Load = &effects.FakeLoad{Series: []float64{40}}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	if v := p.Verify; v == nil || v.Status != relay.CheckUnrun || v.MainLast != LoadedReason || v.MainExit != -1 || p.Breaker.Opens != 0 {
		t.Fatalf("loaded red verify %+v, breaker %+v", v, p.Breaker)
	}
	green, loaded := 0, 0
	for _, c := range p.Rows[1].Baseline {
		switch {
		case c.Status == relay.CheckGreen: // `make check; echo $?` reads 0
			green++
		case c.Status == relay.CheckUnrun && c.MainLast == LoadedReason && c.MainExit == -1:
			loaded++ // a red (an empty last line)
		default:
			t.Fatalf("check %+v", c)
		}
	}
	if b, _ := fx.FS.ReadFile("/state/baselines.json"); green == 0 || loaded == 0 || strings.Contains(string(b), "make check") {
		t.Fatalf("%d green stood, %d reds unrun: loaded; a loaded line stored:\n%s", green, loaded, b)
	}
	fx.Load = &effects.FakeLoad{Series: []float64{3}}
	p, _ = BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if v := p.Verify; v == nil || v.Status != relay.CheckRed || v.MainExit != 2 || v.MainLast != "FAIL" {
		t.Fatalf("unloaded red verify %+v", v)
	}
	if e, ok := LoadMemo(fx.FS, "/state").Get("/root", "aaa111", "make check"); !ok || e.Load != 3 || e.Exit != 2 {
		t.Fatalf("the unloaded verify's memo entry %+v %v", e, ok)
	}
}

// hangSpawner starts a child that writes nothing and lives until killed:
// its stream closes and Wait returns 137 (SIGKILL) once kill is called.
type hangSpawner struct {
	pid  int
	w    *io.PipeWriter
	gone chan struct{}
	once sync.Once
}

func (h *hangSpawner) Start(effects.Spawn) (effects.Process, io.ReadCloser, error) {
	r, w := io.Pipe()
	h.w, h.gone = w, make(chan struct{})
	return hangProc{h}, r, nil
}

func (h *hangSpawner) kill(int) { h.once.Do(func() { h.w.Close(); close(h.gone) }) }

type hangProc struct{ h *hangSpawner }

func (p hangProc) Signal(os.Signal) error { return nil }
func (p hangProc) Wait() (int, error)     { <-p.h.gone; return 137, nil }
func (p hangProc) Pid() int               { return p.h.pid }

func sittingOf(l *effects.FakeLedger, row string) map[string]any {
	var sit map[string]any
	for _, w := range l.Written() {
		if w["row"] == row && w["event"] == "sitting" {
			sit = w
		}
	}
	return sit
}

// TestAR10_6_StallKillsGroup: a child whose HEAD, newest mtime and CPU
// seconds stay unchanged for --stall-seconds (2700 by default), sampled
// every 60 s through the clock, has its group killed; the row is red,
// `stop: stalled`, and the sitting line carries `stall: {head, mtime,
// cpu_s, at}`.
func TestAR10_6_StallKillsGroup(t *testing.T) {
	repo, _ := fixture(t)
	fx, g, _, l := fakes(repo, false)
	clock := jumpClock{&effects.StepClock{T: time.Date(2026, 9, 23, 23, 0, 0, 0, time.UTC)}, WatchdogPoll}
	t0 := clock.Now()
	sp := &hangSpawner{pid: 4242}
	procs := &effects.FakeProcs{Mtime: t0.Add(-time.Hour), CPU: []float64{5}, OnKill: sp.kill}
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main", HeadBefore: "abc"}
	g.Heads = map[string]string{st.Path: "abc"}
	fx.Clock, fx.Procs = clock, procs
	s, _ := RunRow(fx, sp, st, rp, []byte("prompt"), RunOptions{Context: 1000, Wall: time.Hour})
	if s.State != relay.Red || s.Reason != relay.Stalled || relay.Stalled != "stalled" || fmt.Sprint(procs.Kills()) != "[4242]" {
		t.Fatalf("sitting %+v, kills %v", s, procs.Kills())
	}
	sit := sittingOf(l, "r1")
	stall, _ := sit["stall"].(map[string]any)
	if sit["stop"] != "stalled" || sit["state"] != "red" || stall == nil || stall["head"] != "abc" || number(stall["cpu_s"]) != 5 ||
		stall["at"] != t0.Add(DefaultStallSeconds*time.Second).Format(time.RFC3339) || stall["mtime"] != t0.Add(-time.Hour).Format(time.RFC3339) {
		t.Fatalf("sitting line %v", sit)
	}
	if DefaultStallSeconds != 2700 || WatchdogPoll != time.Minute {
		t.Fatal("the stall default is not 2700 s or the poll not 60 s")
	}
}

// TestAR10_6_ProgressResetsWatchdog: any change of the three resets the
// stall: CPU climbing for nine minutes stalls 2700 s after its last change,
// not after the first sample; a sitting that ends first is never stalled.
func TestAR10_6_ProgressResetsWatchdog(t *testing.T) {
	clock := jumpClock{&effects.StepClock{T: time.Date(2026, 9, 23, 23, 0, 0, 0, time.UTC)}, WatchdogPoll}
	t0 := clock.Now()
	g := &effects.FakeGit{Heads: map[string]string{"/wt": "abc"}}
	procs := &effects.FakeProcs{Mtime: t0, CPU: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}
	wd := Watchdog{Git: g, Procs: procs, Clock: clock, Dir: "/wt", Pgid: 7, Stall: 2700 * time.Second}
	smp, why := wd.Run(make(chan struct{}))
	if stalled := why == relay.Stalled; !stalled || smp.CPUS != 10 || !smp.At.Equal(t0.Add(540*time.Second+2700*time.Second)) {
		t.Fatalf("stalled %v at %v (want %v), cpu %v", stalled, smp.At.Sub(t0), 3240*time.Second, smp.CPUS)
	}
	done := make(chan struct{})
	close(done)
	wd.Clock = effects.FakeClock{T: t0}
	if _, why := wd.Run(done); why != "" {
		t.Fatal("an ended sitting stalled")
	}
}

// TestAR10_6_WallNeverZero: a sitting given no wall runs under the queue's
// default, 4h (DefaultWall), never none; the spawn line records the limit.
func TestAR10_6_WallNeverZero(t *testing.T) {
	repo, q := fixture(t)
	for _, c := range []struct {
		wall time.Duration
		want float64
	}{{0, 14400}, {90 * time.Minute, 5400}} {
		fx, _, _, l := fakes(repo, false)
		rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
		RunRow(fx, &effects.FakeSpawner{}, &Staged{Path: rp.Worktree}, rp, nil, RunOptions{Context: 1000, Wall: c.wall})
		var spawn map[string]any
		for _, w := range l.Written() {
			if w["event"] == "spawn" {
				spawn = w
			}
		}
		if number(spawn["wall_limit_s"]) != c.want {
			t.Fatalf("wall %v: spawn line %v", c.wall, spawn)
		}
	}
	if DefaultWall != 4*time.Hour || q.Rows[0].Wall != DefaultWall {
		t.Fatalf("DefaultWall %v, the queue's %v", DefaultWall, q.Rows[0].Wall)
	}
}

// greenGate is a gathered green gate every invariant holds for over a
// staged tree with no HeadBefore and a clean fake status.
func greenGate(*Staged, bool, bool, GateWait) (relay.Result, relay.StopReason, relay.Gate) {
	return relay.GreenResult, "", relay.Gate{Gathered: true, ResultSeen: true, ExitOK: true, Commit: true, Clean: true, OriginSame: true}
}

// TestAR10_8_OrphansRed: after the Wait, two members of the child's group
// are still there; the sweep kills the group once, reads it empty, and a
// green gate turns red with `orphans: 2` on the sitting line.
func TestAR10_8_OrphansRed(t *testing.T) {
	repo, _ := fixture(t)
	fx, _, _, l := fakes(repo, false)
	procs := &effects.FakeProcs{Groups: map[int][]int{77: {77, 901, 902}}}
	procs.OnKill = func(int) { procs.Groups = nil } // one kill sweeps the group
	fx.Procs = procs
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}
	s, g := RunRow(fx, &effects.FakeSpawner{PID: 77}, st, rp, nil, RunOptions{Context: 1000, Wall: time.Hour, Gate: greenGate})
	if s.State != relay.Red || g.Orphans != 2 || fmt.Sprint(procs.Kills()) != "[77]" {
		t.Fatalf("sitting %+v, orphans %d, kills %v", s, g.Orphans, procs.Kills())
	}
	if sit := sittingOf(l, "r1"); number(sit["orphans"]) != 2 || sit["state"] != "red" || sit["stop"] != "" {
		t.Fatalf("sitting line %v", sit)
	}
	procs.Groups, procs.OnKill = nil, nil
	if s, g := RunRow(fx, &effects.FakeSpawner{PID: 77}, st, rp, nil, RunOptions{Context: 1000, Wall: time.Hour, Gate: greenGate}); s.State != relay.Green || g.Orphans != 0 {
		t.Fatalf("no orphans: sitting %+v, orphans %d", s, g.Orphans)
	}
}

// TestAR10_8_SurvivorsStopOrphans: a group that outlives three kills fails
// the sitting `orphans`, `stop: orphans` on its line, and stops the night
// `stopped:orphans` (a sitting failure, D6).
func TestAR10_8_SurvivorsStopOrphans(t *testing.T) {
	repo, _ := fixture(t)
	fx, _, _, l := fakes(repo, false)
	procs := &effects.FakeProcs{Groups: map[int][]int{77: {901}}}
	fx.Procs = procs
	rp := &RowPlan{ID: "r1", Worktree: "/root/.claude/worktrees/r1", SessionID: "s1", Spawn: []string{"claude", "-p"}}
	st := &Staged{Path: rp.Worktree, Branch: "r1", Base: "main"}
	s, _ := RunRow(fx, &effects.FakeSpawner{PID: 77}, st, rp, nil, RunOptions{Context: 1000, Wall: time.Hour, Gate: greenGate})
	if s.State != relay.Failed || s.Reason != relay.Orphans || relay.Orphans != "orphans" || fmt.Sprint(procs.Kills()) != "[77 77 77]" {
		t.Fatalf("sitting %+v, kills %v", s, procs.Kills())
	}
	if sit := sittingOf(l, "r1"); sit["stop"] != "orphans" || number(sit["orphans"]) != 1 {
		t.Fatalf("sitting line %v", sit)
	}
	if n, err := relay.NightStep(relay.NightRunning, relay.OnSittingFailed, s.Reason); err != nil || n != relay.Stopped(relay.Orphans) {
		t.Fatalf("the night after survivors: %s %v", n, err)
	}
}
