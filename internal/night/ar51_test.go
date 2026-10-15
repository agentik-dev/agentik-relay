package night_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar51Night is the test night's id: the world's ledger's and the plan's.
const ar51Night = "2026-09-22-1"

// totalOf is the report's totals as its Burn table's last row sums them
// (the context is the largest, as status's is).
func totalOf(r night.Report) night.Burn {
	var b night.Burn
	for _, s := range r.Sittings {
		b.Sittings, b.Turns, b.CostUSD, b.Unread = b.Sittings+s.Sittings, b.Turns+s.Turns, b.CostUSD+s.CostUSD, b.Unread+s.Unread
		b.Builders, b.BuilderOutput = b.Builders+s.Builders, b.BuilderOutput+s.BuilderOutput
		b.Context = max(b.Context, s.Context)
	}
	return b
}

// holdGate makes row's gate wait: reached closes once the gate is asked
// (the row's `exit` line is written by then), release lets it run. The
// test's end releases it too.
func holdGate(t *testing.T, o *night.NightOptions, row string) (reached chan struct{}, release func()) {
	reached = make(chan struct{})
	hold := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(hold) }) }
	inner := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := inner(r, rp)
		if r.ID == row && ro.Gate != nil {
			g := ro.Gate
			ro.Gate = func(st *night.Staged, seen, isError bool, gw night.GateWait) (relay.Result, relay.StopReason, relay.Gate) {
				close(reached)
				<-hold
				return g(st, seen, isError, gw)
			}
		}
		return b, ro, err
	}
	t.Cleanup(release)
	return reached, release
}

// liveNight runs the night in the background; the returned wait is its end
// (the test's end waits for it too, after the held gates are released).
func liveNight(t *testing.T, w *world, q *queue.Queue, o night.NightOptions) (wait func()) {
	done := make(chan error, 1)
	ended := make(chan struct{})
	go func() {
		_, err := night.Run(w.fx, effects.OSSpawner{}, q, o)
		done <- err
		close(ended)
	}()
	var once sync.Once
	wait = func() {
		once.Do(func() {
			select {
			case <-ended:
				if err := <-done; err != nil {
					t.Errorf("night: %v", err)
				}
			case <-time.After(60 * time.Second):
				t.Errorf("the night did not end")
			}
		})
	}
	t.Cleanup(wait) // registered after holdGate's: runs once the gates are released
	return wait
}

func reachedAR51(t *testing.T, ch chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(60 * time.Second):
		t.Fatalf("never reached: %s", what)
	}
}

// statusWhen reads status.json until ok holds.
func statusWhen(t *testing.T, state, what string, ok func(night.Status) bool) night.Status {
	t.Helper()
	var st night.Status
	for i := 0; i < 3000; i++ {
		if st = night.ReadStatus(effects.OSFS{}, state, ar51Night); ok(st) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("status never read %s: %+v", what, st)
	return st
}

// ar51Clock is a clock the test steps; After is FakeClock's (at once for a
// wait that is not positive, else never), so a step fires no timer.
type ar51Clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *ar51Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *ar51Clock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.Now()
	}
	return ch
}

func (c *ar51Clock) step(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// TestAR51_1_OneTurnCount (AR51 ruling 1; relay-r1 §5): a stream of twelve
// top-level messages whose result says num_turns 17. The `exit` line and
// the `sitting` line both read 17, the number the cost belongs to; the
// `turn` line keeps the live tally (10, the one written at every tenth). A
// stream cut before its result has only the tally: both lines read it.
func TestAR51_1_OneTurnCount(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	path := streamOf(t, true, 1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000, 11000, 12000)
	src, _ := os.ReadFile(path)
	if !strings.Contains(string(src), `"num_turns":12`) {
		t.Fatalf("the fixture has no num_turns 12:\n%s", src)
	}
	os.WriteFile(path, []byte(strings.Replace(string(src), `"num_turns":12`, `"num_turns":17`, 1)), 0o644)
	st, rp := w.stage(t, "a1")
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Transcripts: w.transcripts, Env: w.envAt(path)})
	lines := w.ledger.Written()
	exit, sitting, turn := lastOf(lines, "exit"), lastOf(lines, "sitting"), lastOf(lines, "turn")
	if exit["turns"] != float64(17) || sitting["turns"] != float64(17) || turn["turns"] != float64(10) {
		t.Fatalf("num_turns 17 over a tally of 12: exit turns %v, sitting turns %v, last turn line %v; want 17, 17, 10",
			exit["turns"], sitting["turns"], turn["turns"])
	}
	if sitting["cost_usd"] != exit["cost_usd"] || sitting["cost_read"] != true {
		t.Fatalf("the sitting's cost %v (read %v), the exit's %v", sitting["cost_usd"], sitting["cost_read"], exit["cost_usd"])
	}

	w = newWorld(t)
	st, rp = w.stage(t, "a2")
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Transcripts: w.transcripts, Env: w.envAt(streamOf(t, false, 1000, 2000, 3000))})
	lines = w.ledger.Written()
	exit, sitting = lastOf(lines, "exit"), lastOf(lines, "sitting")
	if exit["turns"] != float64(3) || sitting["turns"] != float64(3) || sitting["cost_read"] != false {
		t.Fatalf("no result, a tally of 3: exit turns %v, sitting turns %v, cost_read %v; want 3, 3, false",
			exit["turns"], sitting["turns"], sitting["cost_read"])
	}
}

// TestAR51_2_LiveReportCounts (AR51 rulings 2 and 3; relay-r1 §9): a live
// night's lines. a1 has spawned and exited and its gate is running (no
// `sitting` line); a2 has spawned and is at its 12th turn. The report counts
// both from their spawn, a1 at its `exit` line's turns and cost, a2 at its
// last `turn` line and no cost, and its first line says they are in flight.
// Then the same on a real night held at a1's gate: the report's totals are
// status's burn, field by field.
func TestAR51_2_LiveReportCounts(t *testing.T) {
	lines := []map[string]any{
		{"night": ar51Night, "event": "stage", "row": "a1", "branch": "a1", "base": "main"},
		{"night": ar51Night, "event": "spawn", "row": "a1", "usd": 15.0},
		{"night": ar51Night, "event": "turn", "row": "a1", "turns": 60.0, "context": 118000.0},
		{"night": ar51Night, "event": "exit", "row": "a1", "turns": 61.0, "context": 120000.0, "cost_usd": 6.69, "cost_read": true},
		{"night": ar51Night, "event": "stage", "row": "a2", "branch": "a2", "base": "main"},
		{"night": ar51Night, "event": "spawn", "row": "a2", "usd": 15.0},
		{"night": ar51Night, "event": "turn", "row": "a2", "turns": 11.0, "context": 30000.0},
		{"night": ar51Night, "event": "turn", "row": "a2", "turns": 12.0, "context": 40000.0},
		{"night": "2026-09-21-1", "event": "spawn", "row": "a1", "usd": 15.0}, // another night's
	}
	r := night.BuildReport(lines, ar51Night)
	md := string(night.Markdown(r, "", ""))
	first := strings.SplitN(strings.SplitN(md, "\n\n", 2)[1], "\n", 2)[0]
	want := night.Burn{Sittings: 2, Turns: 73, Context: 120000, CostUSD: 6.69}
	if got := totalOf(r); got != want {
		t.Fatalf("a live night's report: totals %+v, first line %q; want %+v", got, first, want)
	}
	if first != "2 sittings (2 in flight), $6.69: 0 merged, 0 stopped short, 0 waiting on a hand merge." {
		t.Fatalf("a live night's report, its first line: %q", first)
	}
	if len(r.Stopped.Rows) != 0 || len(r.Waiting) != 0 || len(r.Resume) != 0 {
		t.Fatalf("a sitting in flight is stopped, waiting or resumable: %+v", r)
	}
	t.Logf("the live-night fixture's report: %s\n%s", first, md[strings.Index(md, "## Burn"):strings.Index(md, "\n## Handovers")])

	w, q, by := nightWorld(t, spec{id: "a1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	reached, release := holdGate(t, &o, "a1")
	wait := liveNight(t, w, q, o)
	reachedAR51(t, reached, "a1's gate")
	st := statusWhen(t, o.State, "a1's exit", func(s night.Status) bool { return s.Burn.CostUSD > 0 })
	r = night.BuildReport(w.ledger.Written(), ar51Night)
	md = string(night.Markdown(r, "", ""))
	if got := totalOf(r); got != st.Burn || got.Sittings != 1 || got.Turns != 3 || got.CostUSD != 1.2 {
		t.Fatalf("at a1's gate: the report's totals %+v, status's burn %+v; want one sitting of 3 turns and $1.20 in both", got, st.Burn)
	}
	if !strings.Contains(md, "\n1 sitting (1 in flight), $1.20: 0 merged, 0 stopped short, 0 waiting on a hand merge.\n") {
		t.Fatalf("at a1's gate, the report:\n%s", md)
	}
	release()
	wait()
	md = string(night.Markdown(night.BuildReport(w.ledger.Written(), ar51Night), "", ""))
	if !strings.Contains(md, "\n1 sitting, $1.20: 0 merged, 0 stopped short, 1 waiting on a hand merge.\n") {
		t.Fatalf("the ended night's first line changed:\n%s", md)
	}
}

// TestAR51_3_BurnAgrees (AR51 rulings 2 and 3): after a whole night of two
// rows, one green and one red, status's burn is the report's totals, field
// by field, the builders' output tokens among them and above 0.
func TestAR51_3_BurnAgrees(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "no", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	if states(n) != "r1=green r2=red" {
		t.Fatalf("states %s", states(n))
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	got := totalOf(night.BuildReport(w.ledger.Written(), n.ID))
	if st.Burn != got {
		t.Fatalf("an ended night: status's burn %+v, the report's totals %+v", st.Burn, got)
	}
	if got.Sittings != 2 || got.Turns != 6 || got.CostUSD != 2.4 || got.Builders != 2 || got.BuilderOutput <= 0 {
		t.Fatalf("the night's burn %+v; want 2 sittings, 6 turns, $2.40, 2 builders and their output tokens", got)
	}
}

// TestAR51_4_StageFailureIsNoSitting (AR51 ruling 2): r1 is picked and
// cannot stage (its worktree's path is taken); r2 runs. Nothing spawned for
// r1, so status counts one sitting, as the report does.
func TestAR51_4_StageFailureIsNoSitting(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	inner := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		if r.ID == "r1" { // after the pick, before the stage
			os.MkdirAll(filepath.Join(rp.Worktree, "taken"), 0o755)
		}
		return inner(r, rp)
	}
	n := runNight(t, w, q, o)
	lines := w.ledger.Written()
	if states(n) != "r1=skipped r2=green" || count(lines, "spawn") != 1 || count(lines, "skip") != 1 {
		t.Fatalf("states %s, %d spawn lines, %d skip lines; want r1 skipped at its stage, r2 green", states(n), count(lines, "spawn"), count(lines, "skip"))
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	r := night.BuildReport(lines, n.ID)
	if st.Burn.Sittings != 1 || len(r.Sittings) != 1 {
		t.Fatalf("a pick that could not stage: status counts %d sittings, the report %d; want 1 and 1", st.Burn.Sittings, len(r.Sittings))
	}
}

// TestAR51_5_ElapsedEnds (AR51 ruling 4; relay-r1 §9): r1 runs five minutes
// on the clock and ends green; while r2 runs, the clock steps an hour. Every
// status written after r1's `sitting` line reads r1's elapsed as 5m0s; r2,
// still running, reads the hour.
func TestAR51_5_ElapsedEnds(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	clock := &ar51Clock{t: time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)}
	w.fx.Clock, w.ledger.Clock = clock, clock
	o := w.opts(t, by)
	reached1, release1 := holdGate(t, &o, "r1")
	reached2, release2 := holdGate(t, &o, "r2")
	wait := liveNight(t, w, q, o)

	reachedAR51(t, reached1, "r1's gate")
	clock.step(5 * time.Minute)
	release1()
	reachedAR51(t, reached2, "r2's gate")
	if c := count(w.ledger.Written(), "sitting"); c != 1 {
		t.Fatalf("%d sitting lines at r2's gate, want r1's", c)
	}
	one := statusWhen(t, o.State, "r1 green", func(s night.Status) bool { return len(s.Rows) == 2 && s.Rows[0].State == "green" })
	clock.step(time.Hour)
	two := statusWhen(t, o.State, "r2 an hour in", func(s night.Status) bool { return len(s.Rows) == 2 && s.Rows[1].Elapsed == "1h0m0s" })
	release2()
	wait()
	last := night.ReadStatus(effects.OSFS{}, o.State, ar51Night)
	if one.Rows[0].Elapsed != "5m0s" || two.Rows[0].Elapsed != "5m0s" || last.Rows[0].Elapsed != "5m0s" {
		t.Fatalf("r1's elapsed after its sitting line: %s, then %s after an hour's step, %s at the night's end; want 5m0s in all three",
			one.Rows[0].Elapsed, two.Rows[0].Elapsed, last.Rows[0].Elapsed)
	}
	if last.Rows[1].Elapsed != "1h0m0s" {
		t.Fatalf("r2's elapsed at the night's end: %s, want 1h0m0s", last.Rows[1].Elapsed)
	}
}
