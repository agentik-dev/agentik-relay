package night_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The usage limit pauses the night (usage-limit-resume r1 §2, §4; AR65):
// nights on effects.FakeSpawner and effects.StepClock, every wait bounded.

// ar65T is the nights' clock at their start.
var ar65T = time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)

// ar65Event is a refused rate_limit_event of typ, resetsAt at.
func ar65Event(typ string, at time.Time) []byte {
	return fmt.Appendf(nil, `{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","rateLimitType":%q,"resetsAt":%d}}`+"\n", typ, at.Unix())
}

// ar65Result is an is_error result naming text.
func ar65Result(text string) []byte {
	return fmt.Appendf(nil, `{"type":"result","subtype":"success","is_error":true,"result":%q,"num_turns":1,"total_cost_usd":0}`+"\n", text)
}

// ar65Spawner starts a row's FakeSpawner by its tree's name, and any other
// row's child as a stall its kill ends.
type ar65Spawner struct {
	rows map[string]*effects.FakeSpawner
	mu   sync.Mutex
	n    int
}

func (a *ar65Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	a.mu.Lock()
	a.n++
	a.mu.Unlock()
	if f := a.rows[filepath.Base(sp.Dir)]; f != nil {
		return f.Start(sp)
	}
	s := &ar65Stall{gone: make(chan struct{})}
	return s, s, nil
}

func (a *ar65Spawner) spawns() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n
}

// ar65Stall is a child that says nothing until its kill, then exits 143.
type ar65Stall struct {
	gone chan struct{}
	once sync.Once
}

func (s *ar65Stall) Signal(os.Signal) error   { s.once.Do(func() { close(s.gone) }); return nil }
func (s *ar65Stall) Wait() (int, error)       { <-s.gone; return 143, nil }
func (s *ar65Stall) Pid() int                 { return 4343 }
func (s *ar65Stall) Read([]byte) (int, error) { <-s.gone; return 0, io.EOF }
func (s *ar65Stall) Close() error             { return nil }

// ar65World is a night of rows r1.. on the step clock, r1's child out.
func ar65World(t *testing.T, out []byte, ids ...string) (*world, *queue.Queue, night.NightOptions, *ar65Spawner, *effects.StepClock) {
	t.Helper()
	var specs []spec
	for _, id := range ids {
		specs = append(specs, spec{id: id, echo: "ok", fixture: "ok.jsonl"})
	}
	w, q, by := nightWorld(t, specs...)
	clock := &effects.StepClock{T: ar65T}
	w.fx.Clock = clock
	sp := &ar65Spawner{rows: map[string]*effects.FakeSpawner{"r1": {Out: out, Exit: 143, PID: 4242}}}
	return w, q, w.opts(t, by), sp, clock
}

// ar65Start runs the night in the background; its result arrives on the
// channel.
func ar65Start(w *world, q *queue.Queue, o night.NightOptions, sp *ar65Spawner) chan *night.Night {
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

// ar65End is the night's result within 20 s.
func ar65End(t *testing.T, res chan *night.Night) *night.Night {
	t.Helper()
	select {
	case n := <-res:
		return n
	case <-time.After(20 * time.Second):
		t.Fatal("the night did not end within 20 s")
		return nil
	}
}

// ar65Until polls cond for at most 10 s.
func ar65Until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); !cond(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("waited 10 s for %s", what)
		}
	}
}

// ar65Events is every ledger line of event.
func ar65Events(lines []map[string]any, event string) []map[string]any {
	var out []map[string]any
	for _, l := range lines {
		if l["event"] == event {
			out = append(out, l)
		}
	}
	return out
}

// ar65Paused reads whether the night's status says paused, each of ids
// too.
func ar65Paused(o night.NightOptions, ids ...string) bool {
	st := night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night)
	rows := map[string]string{}
	for _, r := range st.Rows {
		rows[r.ID] = r.State
	}
	for _, id := range ids {
		if rows[id] != "paused" {
			return false
		}
	}
	return st.State == string(relay.NightPaused)
}

// TestAR65_3_PauseHolds (AR65 rulings 3, 4): at --parallel 2, r1's refused
// five_hour event pauses the night; r2, a stall, is killed and comes back
// paused (its Limit zero). One pause line names both rows, the window, the
// event's wake and the count; each sitting line is failed usage-limit with
// `limit: paused` and sends no notice; ten still polls add no line and no
// spawn, r3 never starts; a SIGINT stops the night owner-kill.
func TestAR65_3_PauseHolds(t *testing.T) {
	w, q, o, sp, _ := ar65World(t, ar65Event("five_hour", ar65T.Add(time.Hour)), "r1", "r2", "r3")
	o.Parallel = 2
	sig := make(chan os.Signal, 1)
	notes := &effects.FakeNotify{}
	o.Signals, o.Notify = sig, notes
	res := ar65Start(w, q, o, sp)
	ar65Until(t, "the pause, both sitting lines and the paused status", func() bool {
		all := w.ledger.Written()
		return len(ar65Events(all, "pause")) > 0 && len(rowLines(all, "sitting", "r2")) == 1 && ar65Paused(o, "r1", "r2")
	})
	all := w.ledger.Written()
	ps := ar65Events(all, "pause")
	if len(ps) != 1 {
		t.Fatalf("pause lines %v, want one", ps)
	}
	p := ps[0]
	if fmt.Sprint(p["rows"]) != "[r1 r2]" || p["kind"] != "window" || p["source"] != "event" ||
		p["wake"] != "2026-10-11T02:02:00Z" || number(p["count"]) != 1 || p["resets_at"] != "2026-10-11T02:00:00Z" {
		t.Fatalf("the pause line %v", p)
	}
	for _, id := range []string{"r1", "r2"} {
		sl := rowLines(all, "sitting", id)
		if len(sl) != 1 || sl[0]["state"] != "failed" || sl[0]["reason"] != "usage-limit" || sl[0]["limit"] != "paused" {
			t.Fatalf("%s's sitting lines %v, want one failed usage-limit, limit paused", id, sl)
		}
	}
	for _, nt := range notes.Notices() {
		if nt.Event == "sitting" {
			t.Fatalf("a paused sitting line sent a notice: %+v", nt)
		}
	}
	lines, spawns := len(all), sp.spawns()
	time.Sleep(10*o.Poll + o.Poll/2) // ten polls, the clock still
	if got := len(w.ledger.Written()); got != lines || sp.spawns() != spawns || spawns != 2 || !ar65Paused(o, "r1", "r2") {
		t.Fatalf("ten still polls: %d lines (was %d), %d spawns (was %d, want 2), paused %v", got, lines, sp.spawns(), spawns, ar65Paused(o, "r1", "r2"))
	}
	sig <- syscall.SIGINT
	if n := ar65End(t, res); n.State != relay.Stopped(relay.OwnerKill) {
		t.Fatalf("after SIGINT: %s, want stopped:owner-kill", n.State)
	}
	if len(rowLines(w.ledger.Written(), "spawn", "r3")) != 0 {
		t.Fatal("r3 spawned while the night was paused")
	}
}

// TestAR65_4_StopsAtOnce (AR65 ruling 1): a weekly limit, a model's limit
// with no reset (the Fable text), a reset past --until and one 7 h on stop
// the night usage-limit at once, no pause line, the stop line's detail
// naming why; a 401 stops it auth.
func TestAR65_4_StopsAtOnce(t *testing.T) {
	for _, c := range []struct {
		name, until string
		out         []byte
		want        relay.StopReason
		detail      string
	}{
		{"weekly", "", ar65Event("seven_day", ar65T.Add(48*time.Hour)), relay.UsageLimit, "a weekly limit"},
		{"the Fable text", "", ar65Result("You've reached your Fable limit. Run /usage-credits to keep going or switch models with /model."),
			relay.UsageLimit, "the fable limit names no reset"},
		{"past --until", "02:00", ar65Event("five_hour", ar65T.Add(time.Hour)), relay.UsageLimit,
			"the wake 2026-10-11T02:02:00Z is at or past --until 2026-10-11T02:00:00Z"},
		{"7 h on", "", ar65Event("five_hour", ar65T.Add(7*time.Hour)), relay.UsageLimit,
			"the wake 2026-10-11T08:02:00Z is over 6h away, and no --until"},
		{"401", "", ar65Result("API Error: 401 authentication_error · Please run /login"), relay.Auth, ""},
	} {
		w, q, o, sp, _ := ar65World(t, c.out, "r1", "r2")
		o.Until = c.until
		n := ar65End(t, ar65Start(w, q, o, sp))
		all := w.ledger.Written()
		stop := lastOf(all, "stop")
		if n.State != relay.Stopped(c.want) || states(n) != "r1=failed r2=skipped" || len(ar65Events(all, "pause")) != 0 {
			t.Errorf("%s: night %s, %s, %d pause lines; want stopped:%s at once", c.name, n.State, states(n), len(ar65Events(all, "pause")), c.want)
		}
		if c.detail != "" && stop["detail"] != c.detail {
			t.Errorf("%s: the stop line's detail %q, want %q", c.name, stop["detail"], c.detail)
		}
		if sl := rowLines(all, "sitting", "r1"); len(sl) != 1 || sl[0]["limit"] != nil {
			t.Errorf("%s: r1's sitting lines %v, want one, no limit", c.name, sl)
		}
	}
	// The fix round's F1: a part pauseFor would pause whose sitting line
	// breaks an invariant (a dependency unsatisfied at its start) fails as
	// before AR65: the night stops invariant, no pause line, the line has
	// no `limit` and its notice is sent.
	t.Run("a broken invariant", func(t *testing.T) {
		w, q, o, sp, _ := ar65World(t, ar65Event("five_hour", ar65T.Add(time.Hour)), "r1")
		row := o.Row
		o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
			b, ro, err := row(r, rp)
			ro.Deps = map[string]bool{"zz": false}
			return b, ro, err
		}
		notes := &effects.FakeNotify{}
		o.Notify = notes
		n := ar65End(t, ar65Start(w, q, o, sp))
		all := w.ledger.Written()
		sent := 0
		for _, nt := range notes.Notices() {
			if nt.Event == "sitting" && nt.Row == "r1" {
				sent++
			}
		}
		sl := rowLines(all, "sitting", "r1")
		if n.State != relay.Stopped(relay.Invariant) || len(ar65Events(all, "pause")) != 0 || len(sl) != 1 || sl[0]["limit"] != nil || sl[0]["reason"] != "invariant" || sent != 1 {
			t.Fatalf("night %s, %d pause lines, sitting lines %v, %d sitting notices; want stopped:invariant, none, one invariant line with no limit, one notice",
				n.State, len(ar65Events(all, "pause")), sl, sent)
		}
	})
}

// TestAR65_5_WakeHalts (AR65 rulings 5, 6; AR67 ruling 7): a minute
// before the wake the night stays paused; with --until 02:05, the clock at
// 02:05 wakes it (T+1h+2m passed) and it stops until-reached, halting r1:
// failed usage-limit, its stage unlocked, its tree kept, one sitting line,
// its resume line in the stop line's, one spawn. With --max-usd 1 (r1's
// unread spawn counted at its usd) the wake stops budget-reached. A wake
// that passes both continues the row (TestAR67_3).
func TestAR65_5_WakeHalts(t *testing.T) {
	for _, c := range []struct {
		name, until string
		maxUSD      float64
		last        time.Duration // the clock's last step, from the minute before the wake
		want        relay.StopReason
		detail      string
	}{
		{"--until 02:05", "02:05", 0, 4 * time.Minute, relay.UntilReached, ""},
		{"--max-usd 1", "", 1, time.Minute, relay.BudgetReached, ""},
	} {
		w, q, o, sp, clock := ar65World(t, ar65Event("five_hour", ar65T.Add(time.Hour)), "r1")
		o.MaxUSD, o.Until = c.maxUSD, c.until
		res := ar65Start(w, q, o, sp)
		ar65Until(t, c.name+": the pause", func() bool { return ar65Paused(o, "r1") })
		clock.Advance(time.Hour + time.Minute)
		time.Sleep(3 * o.Poll)
		select {
		case n := <-res:
			t.Fatalf("%s: a minute before the wake the night ended %s", c.name, n.State)
		default:
		}
		if !ar65Paused(o, "r1") {
			t.Fatalf("%s: a minute before the wake the night is not paused", c.name)
		}
		clock.Advance(c.last)
		n := ar65End(t, res)
		all := w.ledger.Written()
		stop := lastOf(all, "stop")
		wt := n.Plan.Rows[0].Worktree
		if n.State != relay.Stopped(c.want) || n.Sittings["r1"].State != relay.Failed || n.Sittings["r1"].Reason != relay.UsageLimit || sp.spawns() != 1 {
			t.Fatalf("%s: night %s, r1 %s %s, %d spawns; want stopped:%s, r1 failed usage-limit, one spawn", c.name, n.State, n.Sittings["r1"].State, n.Sittings["r1"].Reason, sp.spawns(), c.want)
		}
		if night.Locked(wt) || len(rowLines(all, "sitting", "r1")) != 1 || fmt.Sprint(stop["resume"]) != "[relay resume r1]" {
			t.Fatalf("%s: locked %v, sitting lines %d, the stop line's resume %v", c.name, night.Locked(wt), len(rowLines(all, "sitting", "r1")), stop["resume"])
		}
		if _, err := os.Stat(wt); err != nil {
			t.Fatalf("%s: the tree is gone: %v", c.name, err)
		}
		if c.detail != "" && stop["detail"] != c.detail {
			t.Fatalf("%s: the stop line's detail %q, want %q", c.name, stop["detail"], c.detail)
		}
	}
}
