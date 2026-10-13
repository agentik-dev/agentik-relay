package night_test

import (
	"bytes"
	"fmt"
	"io"
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

// spec is one queue row for a test night: its edges, the word its
// acceptance line must read (echo ok reads ok is green; echo no, red), the
// fake claude's fixture and extra environment.
type spec struct {
	id, base, deps, echo, fixture, merge string
	env                                  []string
}

const packetSrc = "# Packet %s: a test row\n\n| item | value |\n|---|---|\n| from | `design/relay-r1.md` §1 |\n" +
	"| pin | branch `main` |\n| wall | this repo only |\n| may touch | `a.txt`. Nothing else |\n" +
	"| acceptance | `echo %s` reads `ok` |\n| budget | 10 turns, 20k context, $5 |\n\n" +
	"## 1. What the source says (quoted)\nQuoted.\n\n## 2. Rulings\nNone.\n\n## 3. Interfaces (signatures, not prose)\nNone.\n\n" +
	"## 4. Findings that name this row\nNone.\n\n## 5. Forbidden reads\nNone.\n\n## 6. Report (capped)\nA table.\n"

// nightWorld commits a queue of specs and their packets to a fresh root.
func nightWorld(t *testing.T, specs ...spec) (*world, *queue.Queue, map[string]spec) {
	t.Helper()
	fakeClaude(t)
	w := newWorld(t)
	var y strings.Builder
	y.WriteString("version: 1\ndefaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,\n" +
		"           merge: branch-only, device: never, verify: auto}\nwords: \"\"\nrows:\n")
	by := map[string]spec{}
	os.MkdirAll(filepath.Join(w.root, "design", "packets"), 0o755)
	for _, s := range specs {
		by[s.id] = s
		os.WriteFile(filepath.Join(w.root, "design", "packets", s.id+".md"), []byte(fmt.Sprintf(packetSrc, s.id, s.echo)), 0o644)
		fmt.Fprintf(&y, "  - id: %s\n    packet: design/packets/%s.md\n", s.id, s.id)
		if s.base != "" {
			fmt.Fprintf(&y, "    base: %s\n", s.base)
		}
		if s.deps != "" {
			fmt.Fprintf(&y, "    depends_on: [%s]\n", s.deps)
		}
		if s.merge != "" {
			fmt.Fprintf(&y, "    merge: %s\n", s.merge)
		}
	}
	os.WriteFile(filepath.Join(w.root, "queue.yaml"), []byte(y.String()), 0o644)
	os.WriteFile(filepath.Join(w.root, ".gitignore"), []byte(".claude/\n"), 0o644)
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "queue")
	q, err := queue.Load(w.root, "queue.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return w, q, by
}

// opts is a night over the fake claude with A6's gate over each row's own
// packet; poll is short so the control file is read quickly.
func (w *world) opts(t *testing.T, by map[string]spec) night.NightOptions {
	state := t.TempDir()
	return night.NightOptions{State: state, Poll: 20 * time.Millisecond,
		PlanOptions: night.Options{Repo: w.root, Night: "2026-09-22-1", AllowedTools: "Bash Edit Write", UUID: night.NewUUID},
		Row: func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
			s := by[r.ID]
			return []byte("prompt"), night.RunOptions{Context: r.Context, Env: w.env(s.fixture, s.env...), Transcripts: w.transcripts,
				Gate: func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
					res, why, g, _ := gate.Run(w.fx, st, r.Packet, rp.Baseline, &gate.ExitFacts{Seen: seen, IsError: isError}, gate.Verify{Name: r.Verify})
					return res, why, g
				}}, nil
		}}
}

func runNight(t *testing.T, w *world, q *queue.Queue, o night.NightOptions) *night.Night {
	t.Helper()
	n, err := night.Run(w.fx, effects.OSSpawner{}, q, o)
	if err != nil {
		t.Fatalf("night: %v", err)
	}
	return n
}

func states(n *night.Night) string {
	var out []string
	for _, rp := range n.Plan.Rows {
		s := n.Sittings[rp.ID]
		out = append(out, rp.ID+"="+string(s.State))
	}
	return strings.Join(out, " ")
}

func count(lines []map[string]any, event string) int {
	c := 0
	for _, l := range lines {
		if l["event"] == event {
			c++
		}
	}
	return c
}

// TestNightEndToEnd: two rows, the second based on the first: both green,
// two sitting lines, stopped:queue-empty, and the dashboard as printed.
func TestNightEndToEnd(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", base: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" || !night.Clean(n) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	lines := w.ledger.Written()
	if c := count(lines, "sitting"); c != 2 {
		t.Fatalf("%d sitting lines", c)
	}
	for _, e := range []string{"night-open", "stop", "night-close"} {
		if count(lines, e) != 1 {
			t.Fatalf("want one %s line: %v", e, lines)
		}
	}
	if stg := lastOf(lines, "stage"); stg["base"] != "r1" {
		t.Fatalf("r2 staged off %v", stg["base"])
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	var b bytes.Buffer
	night.PrintStatus(&b, st)
	t.Logf("the dashboard:\n%s", b.String())
	if st.State != "stopped:queue-empty" || len(st.Rows) != 2 || !strings.Contains(b.String(), "r2  ") {
		t.Fatalf("status %+v", st)
	}
}

// TestRedMovesOn: a red first row is the builder's; the independent second
// row runs; the night stops queue-empty but not clean.
func TestRedMovesOn(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "no", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=red r2=green" || night.Clean(n) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
}

// TestFailedStops: error.jsonl is a sitting failure: the night stops with
// the sitting's reason and the second row is skipped.
func TestFailedStops(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "error.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	if n.State != relay.Stopped(relay.StreamError) || states(n) != "r1=failed r2=skipped" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
}

// TestBlockedStops: a red dependency blocks its dependent: gate-failed-dependency.
func TestBlockedStops(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "no", fixture: "ok.jsonl"}, spec{id: "r2", deps: "r1", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	if n.State != relay.Stopped(relay.GateFailedDependency) || states(n) != "r1=red r2=skipped" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
}

// waitSpawn waits for the first spawn line and then writes line to the
// night's control file.
func waitSpawn(t *testing.T, w *world, o night.NightOptions, line string, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			if count(w.ledger.Written(), "spawn") > 0 {
				night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, "2026-09-22-1"), line)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// TestControlStop: `stop` while row one runs: row one finishes, row two
// never starts, the night drains to stopped:owner-stop.
func TestControlStop(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_DELAY_MS=100"}},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	var wg sync.WaitGroup
	waitSpawn(t, w, o, "stop", &wg)
	n := runNight(t, w, q, o)
	wg.Wait()
	if n.State != relay.Stopped(relay.OwnerStop) || states(n) != "r1=green r2=skipped" || !night.Clean(n) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
}

// TestControlKill: `kill r1` sends SIGTERM to the running child through the
// night's Process; the row fails owner-kill, the night stops owner-kill.
func TestControlKill(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_DELAY_MS=2000"}},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	var wg sync.WaitGroup
	waitSpawn(t, w, o, "kill r1", &wg)
	start := time.Now()
	n := runNight(t, w, q, o)
	wg.Wait()
	if n.State != relay.Stopped(relay.OwnerKill) || states(n) != "r1=failed r2=skipped" || n.Sittings["r1"].Reason != relay.OwnerKill {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	if exit := lastOf(w.ledger.Written(), "exit"); number(exit["child_exit"]) != 128+15 || time.Since(start) > 5*time.Second {
		t.Fatalf("the child was not terminated: %v", exit)
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	if len(st.Resume) != 1 || st.Resume[0] != "relay resume r1" {
		t.Fatalf("resume %v", st.Resume)
	}
}

func number(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case float64:
		return x
	}
	return -1
}

// stepClock is a clock a test moves.
type stepClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *stepClock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *stepClock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// After never fires: the night's waits under this clock are the ticker's.
func (c *stepClock) After(time.Duration) <-chan time.Time { return make(chan time.Time) }

// TestUntilAndCount: --max-sittings 1 stops count-reached after one row;
// --until stops until-reached once the clock passes it (the gate moves the
// clock two hours).
func TestUntilAndCount(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	o.MaxSittings = 1
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.CountReached) || states(n) != "r1=green r2=skipped" || !night.Clean(n) {
		t.Fatalf("count: night %s: %s", n.State, states(n))
	}

	w, q, by = nightWorld(t, spec{id: "u1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "u2", echo: "ok", fixture: "ok.jsonl"})
	clock := &stepClock{t: time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)}
	o = w.opts(t, by)
	o.Clock, o.Until = clock, "02:30"
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := row(r, rp)
		g := ro.Gate
		ro.Gate = func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
			clock.add(2 * time.Hour)
			return g(st, seen, isError)
		}
		return b, ro, err
	}
	n = runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.UntilReached) || states(n) != "u1=green u2=skipped" {
		t.Fatalf("until: night %s: %s", n.State, states(n))
	}
}

// TestPlanEqualsRun: over the whole night, every row's spawn line is the
// argv the plan printed, run in the plan's worktree.
func TestPlanEqualsRun(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", base: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	minted := 0 // session ids by count: the printed plan and the night's own mint the same
	o.PlanOptions.UUID = func() string { minted++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", minted) }
	p, err := night.BuildPlan(w.fx, q, o.PlanOptions)
	if err != nil {
		t.Fatal(err)
	}
	var printed bytes.Buffer
	night.Print(&printed, p)
	minted = 0
	sp := &recorder{inner: effects.OSSpawner{}}
	n, err := night.Run(w.fx, sp, q, o) // no plan handed in: the night builds its own
	if err != nil {
		t.Fatal(err)
	}
	p = n.Plan
	if len(sp.spawns) != 2 {
		t.Fatalf("%d spawns", len(sp.spawns))
	}
	joined := joinSpawns(printed.String(), os.Getenv("HOME"))
	for i, s := range sp.spawns {
		rp := p.Rows[i]
		want := "cd " + night.Quote([]string{rp.Worktree}) + " && " + night.Quote(s.Argv) + " < prompt"
		if i >= len(joined) || joined[i] != want || s.Dir != rp.Worktree || night.Quote(s.Argv) != night.Quote(rp.Spawn) {
			t.Fatalf("plan printed\n%s\nthe night ran %s in %s", printed.String(), night.Quote(s.Argv), s.Dir)
		}
	}
}

// joinSpawns is every printed spawn line of out, its ` \` continuations
// joined and `~/` written home, as a shell would read it.
func joinSpawns(out, home string) []string {
	var got []string
	cont := false
	for _, l := range strings.Split(out, "\n") {
		piece, ok := strings.CutPrefix(l, "  spawn: ")
		if !ok && !cont {
			continue
		}
		if !ok {
			piece = strings.TrimLeft(l, " ")
		}
		piece, more := strings.CutSuffix(piece, " \\")
		piece = strings.ReplaceAll(piece, "~/", home+"/")
		if cont {
			got[len(got)-1] += " " + piece
		} else {
			got = append(got, piece)
		}
		cont = more
	}
	return got
}

type recorder struct {
	inner  effects.Spawner
	mu     sync.Mutex
	spawns []effects.Spawn
}

func (r *recorder) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	r.mu.Lock()
	r.spawns = append(r.spawns, sp)
	r.mu.Unlock()
	return r.inner.Start(sp)
}

// TestParallelTwo: with --parallel 2 two independent rows run at once (both
// spawned before either sitting ends), the row based on the first waits for
// it, and the night ends queue-empty with all three green.
func TestParallelTwo(t *testing.T) {
	delay := []string{"FAKECLAUDE_DELAY_MS=100"}
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: delay},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl", env: delay}, spec{id: "r3", base: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	o.Parallel = 2
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green r3=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	var order []string
	for _, l := range w.ledger.Written() {
		if e := l["event"]; e == "spawn" || e == "sitting" {
			order = append(order, e.(string)+":"+l["row"].(string))
		}
	}
	got := strings.Join(order, " ")
	if !strings.HasPrefix(got, "spawn:r1 spawn:r2 ") && !strings.HasPrefix(got, "spawn:r2 spawn:r1 ") {
		t.Fatalf("r1 and r2 did not overlap: %s", got)
	}
	if i, j := strings.Index(got, "sitting:r1"), strings.Index(got, "spawn:r3"); i < 0 || j < i {
		t.Fatalf("r3 started before its base was green: %s", got)
	}
}

// TestAR4_UnrunGateIsRed: a gate that returns unrun steps the sitting to red
// with reason gate-unrun; the night moves on to the independent row, stops
// queue-empty, and prints the resume line; status shows `red: gate-unrun`.
func TestAR4_UnrunGateIsRed(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		p, ro, err := row(r, rp)
		if r.ID == "r1" {
			ro.Gate = func(*night.Staged, bool, bool) (relay.Result, relay.StopReason, relay.Gate) {
				return relay.Unrun, "", relay.Gate{}
			}
		}
		return p, ro, err
	}
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=red r2=green" || n.Sittings["r1"].Reason != relay.GateUnrunReason {
		t.Fatalf("night %s: %s; r1 %+v", n.State, states(n), n.Sittings["r1"])
	}
	if !strings.Contains(out.String(), "relay r1: red gate-unrun\nrelay resume r1\n") {
		t.Fatalf("out:\n%s", out.String())
	}
	var b bytes.Buffer
	night.PrintStatus(&b, night.ReadStatus(effects.OSFS{}, o.State, n.ID))
	if !strings.Contains(b.String(), "red: gate-unrun") || !strings.Contains(b.String(), "relay resume r1") {
		t.Fatalf("status:\n%s", b.String())
	}
}

// TestAR4_StateDirIsTemp: TestMain set XDG_STATE_HOME, so the state
// directory and the locks lie under the temp directory, never the machine's.
func TestAR4_StateDirIsTemp(t *testing.T) {
	tmp := filepath.Clean(os.TempDir()) + string(filepath.Separator)
	for _, d := range []string{effects.StateDir(os.Getenv), night.LockDir()} {
		if !strings.HasPrefix(d, tmp) {
			t.Errorf("%s is not under %s", d, tmp)
		}
	}
}

// TestAR5_NightLinesResumeAndTilde (UX1): the night's row lines write home
// as `~`, and a red row's resume door is a line of its own (after its
// kept logs' lines, AR31 act 2).
func TestAR5_NightLinesResumeAndTilde(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "no", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	t.Setenv("HOME", filepath.Dir(w.root))
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, w, q, o)
	if states(n) != "r1=red r2=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	got := out.String()
	// Amended at AR31 act 2: r1's red line kept its log, named under its
	// red line; the lines around it are read without it.
	if !strings.Contains(got, "relay r1: red\nrelay r1: log: ") {
		t.Errorf("no log line under r1's red line in:\n%s", got)
	}
	var rest []string
	for _, l := range strings.SplitAfter(got, "\n") {
		if !strings.HasPrefix(l, "relay r1: log: ") {
			rest = append(rest, l)
		}
	}
	got = strings.Join(rest, "")
	tilde := "~/" + filepath.Base(w.root) + "/.claude/worktrees/"
	for _, want := range []string{"relay r1: running in " + tilde + "r1\n", "relay r2: running in " + tilde + "r2\n",
		"relay r1: red\nrelay resume r1\n", "relay r2: green\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "; relay resume") || strings.Contains(got, "relay resume r2") {
		t.Fatalf("out:\n%s", got)
	}
}

// TestAR5_OwnerKillEscalates (AR2-18): `kill r1` on a child that ignores
// SIGTERM escalates to SIGKILL after KillGrace, as the ceiling does; the row
// fails owner-kill and the child is gone. The fake holds its result a
// minute (AR35): SIGKILL and no result read prove the kill came before the
// child's work would finish, however loaded the machine; the time bound
// only guards against a hang.
func TestAR5_OwnerKillEscalates(t *testing.T) {
	pids := t.TempDir()
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl",
		env: []string{"FAKECLAUDE_DELAY_MS=2000", "FAKECLAUDE_RESULT_DELAY_MS=60000", "FAKECLAUDE_IGNORE_TERM=1", "FAKECLAUDE_PIDFILE=" + pids}})
	night.KillGrace = 300 * time.Millisecond
	o := w.opts(t, by)
	var wg sync.WaitGroup
	// The kill waits for the fake's pid file, written after it ignores
	// SIGTERM: a kill at the spawn line raced the fake's start (AR17B). The
	// wait is the fake's own signal with a minute's deadline, or the
	// night's end (AR35), never a count of short tries.
	ended := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); {
			if _, err := os.Stat(filepath.Join(pids, "r1.pid")); err == nil {
				night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, "2026-09-22-1"), "kill r1")
				return
			}
			select {
			case <-ended:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	start := time.Now()
	n := runNight(t, w, q, o)
	close(ended)
	wg.Wait()
	if n.State != relay.Stopped(relay.OwnerKill) || states(n) != "r1=failed" || n.Sittings["r1"].Reason != relay.OwnerKill {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	// A hang guard: the kill is due in about a second, the result in a minute.
	if exit := lastOf(w.ledger.Written(), "exit"); number(exit["child_exit"]) != 128+9 || exit["cost_read"] != false || time.Since(start) > 20*time.Second {
		t.Fatalf("the child was not killed: %v after %v", exit, time.Since(start))
	}
}

// TestAR5_OriginMovedStopsNight (AR2-12): the remote refs change after row
// one is staged; its gate sees origin moved, the sitting fails and the night
// ends stopped:origin-moved with row two never run.
func TestAR5_OriginMovedStopsNight(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		p, ro, err := row(r, rp)
		inner := ro.Gate
		ro.Gate = func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
			gitIn(t, w.root, "update-ref", "refs/remotes/origin/main", "HEAD")
			return inner(st, seen, isError)
		}
		return p, ro, err
	}
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.OriginMoved) || states(n) != "r1=failed r2=skipped" || n.Sittings["r1"].Reason != relay.OriginMoved {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	if stop := lastOf(w.ledger.Written(), "stop"); stop["state"] != "stopped:origin-moved" {
		t.Fatalf("stop line %v", stop)
	}
}

// TestAR5_NightMerges (AR2-22): a merge-on-green row on a root that is on
// main merges when green; main's head is the MergeSHA in status.json and
// the sha of the `merge` ledger line.
func TestAR5_NightMerges(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", merge: "merge-on-green"})
	o := w.opts(t, by)
	o.Merge = func(r queue.Row, g relay.Gate) (string, error) {
		return gate.MergeIfGreen(w.fx, w.root, g, r.ID, q.Defaults.Verify)
	}
	n := runNight(t, w, q, o)
	if states(n) != "r1=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	head := gitIn(t, w.root, "rev-parse", "main")
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	merge := lastOf(w.ledger.Written(), "merge")
	if len(st.Rows) != 1 || st.Rows[0].MergeSHA != head || merge["sha"] != head || n.Sittings["r1"].MergeSHA != head {
		t.Fatalf("main %s; status %+v; merge line %v", head, st.Rows, merge)
	}
	if got := gitIn(t, w.root, "rev-list", "--count", "main^2..r1"); got != "0" {
		t.Fatalf("r1 is not merged into main: %s commits beyond", got)
	}
}

// TestAR7_PickInOrderError: a queue Order() refuses (a base cycle) is an
// error from PickIn, never blocked; the night stops no-progress with the
// text as the stop line's detail.
func TestAR7_PickInOrderError(t *testing.T) {
	cyc := &queue.Queue{Rows: []queue.Row{{ID: "a", Base: "b"}, {ID: "b", Base: "a"}}}
	all := map[string]*relay.Sitting{"a": {ID: "a", State: relay.Ready}, "b": {ID: "b", State: relay.Ready}}
	next, blocked, err := night.PickIn(cyc, all, nil, 1, night.PickOptions{})
	if err == nil || blocked || next != nil {
		t.Fatalf("PickIn over a cycle: %v %v %v", next, blocked, err)
	}
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	pl, perr := night.BuildPlan(w.fx, q, o.PlanOptions)
	if perr != nil {
		t.Fatal(perr)
	}
	o.Plan, q.Rows[0].Base, q.Rows[1].Base = pl, "r2", "r1"
	_, oerr := q.Order()
	n := runNight(t, w, q, o)
	stop := lastOf(w.ledger.Written(), "stop")
	if oerr == nil || n.State != relay.Stopped(relay.NoProgress) || stop["detail"] != oerr.Error() {
		t.Fatalf("night %s, stop %v, want detail %v", n.State, stop, oerr)
	}
}
