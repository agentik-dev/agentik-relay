package night_test

import (
	"errors"
	"fmt"
	"io"
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

// A night asks the session band at a row's first spawn and holds while it
// says no room (band-pacing-r1 §2, §3; AR80): nights on a FakeBand and the
// step clock, T ar65T, the band's reset T+2h unless a test says.

// ar80Spawner records each row's spawns on the clock: a row's first child
// runs first[row] (exit 143), silent until wait[row] reads true when set;
// every other child is clean.
type ar80Spawner struct {
	clock *effects.StepClock
	first map[string][]byte
	wait  map[string]func() bool
	mu    sync.Mutex
	at    map[string][]time.Time
}

func (a *ar80Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	row := filepath.Base(sp.Dir)
	a.mu.Lock()
	a.at[row] = append(a.at[row], a.clock.Now())
	k := len(a.at[row])
	a.mu.Unlock()
	out, ok := a.first[row]
	if k > 1 || !ok {
		return (&effects.FakeSpawner{Out: ar67Clean, PID: 4343}).Start(sp)
	}
	p, rc, err := (&effects.FakeSpawner{Out: out, Exit: 143, PID: 4242}).Start(sp)
	if u := a.wait[row]; u != nil && err == nil {
		rc = &ar69Silent{ReadCloser: rc, until: u}
	}
	return p, rc, err
}

// spawns is row's spawn times as offsets from T.
func (a *ar80Spawner) spawns(row string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, at := range a.at[row] {
		out = append(out, fmt.Sprintf("%+ds", int(at.Sub(ar65T).Seconds())))
	}
	return out
}

// ar80World is ar65World's night of ids on band, the ledger on the step
// clock, the night-open's band a file.
func ar80World(t *testing.T, band *effects.FakeBand, ids ...string) (*world, *queue.Queue, night.NightOptions, *ar80Spawner, *effects.StepClock) {
	t.Helper()
	w, q, o, _, clock := ar65World(t, nil, ids...)
	w.ledger.Clock = clock
	if band != nil {
		band.Clock = clock
		if band.ResetAt.IsZero() && band.ResetErr == nil {
			band.ResetAt = ar65T.Add(2 * time.Hour)
		}
		w.fx.Band, o.Band = band, "/bin/agentik-session"
	}
	return w, q, o, &ar80Spawner{clock: clock, first: map[string][]byte{}, wait: map[string]func() bool{}, at: map[string][]time.Time{}}, clock
}

// ar80Asks is band's asks as `<row> <offset from T>`.
func ar80Asks(band *effects.FakeBand) string {
	var out []string
	for _, a := range band.Asks() {
		f := strings.Fields(a)
		at, _ := time.Parse(time.RFC3339, f[len(f)-1])
		out = append(out, fmt.Sprintf("%s %+ds", strings.Split(f[0], "/")[1], int(at.Sub(ar65T).Seconds())))
	}
	return strings.Join(out, ", ")
}

// ar80Band is row's band lines in short: state, verdict, asks and wait,
// `error` when one is named; each line's part must be 1.
func ar80Band(t *testing.T, all []map[string]any, row string) string {
	t.Helper()
	var out []string
	for _, l := range rowLines(all, "band", row) {
		if number(l["part"]) != 1 {
			t.Fatalf("a band line's part: %v", l)
		}
		s := fmt.Sprint(l["state"])
		if v, ok := l["verdict"]; ok {
			s += " " + fmt.Sprint(v)
		}
		if a, ok := l["asks"]; ok {
			s += fmt.Sprintf(" %v/%vs", a, l["wait_s"])
		}
		if e, _ := l["error"].(string); e != "" {
			s += " error"
		}
		out = append(out, s)
	}
	return strings.Join(out, "; ")
}

// ar80Ask waits for the n-th ask, then moves the clock to T+d.
func ar80Ask(t *testing.T, band *effects.FakeBand, n int, clock *effects.StepClock, d time.Duration) {
	t.Helper()
	ar65Until(t, fmt.Sprintf("ask %d", n), func() bool { return len(band.Asks()) == n })
	ar69To(clock, ar65T.Add(d))
}

// ar80One is row's only line of event.
func ar80One(t *testing.T, all []map[string]any, event, row string) map[string]any {
	t.Helper()
	ls := rowLines(all, event, row)
	if len(ls) != 1 {
		t.Fatalf("%s's %s lines %v, want one", row, event, ls)
	}
	return ls[0]
}

// TestAR80_2_AbsentOrOff (ruling 2): no band, the night-open line says
// absent or off, nothing is asked, and no line carries a band key.
func TestAR80_2_AbsentOrOff(t *testing.T) {
	for _, says := range []string{"", "off"} {
		w, q, o, sp, _ := ar80World(t, nil, "r1")
		o.Band = says
		ar65End(t, ar71Run(w, q, o, sp))
		all := w.ledger.Written()
		want := map[string]string{"": "absent", "off": "off"}[says]
		if op := ar65Events(all, "night-open"); len(op) != 1 || op[0]["band"] != want {
			t.Fatalf("Band %q: night-open %v, want band %s", says, op, want)
		}
		_, hasSpawn := ar80One(t, all, "spawn", "r1")["band"]
		_, hasSitting := ar80One(t, all, "sitting", "r1")["band"]
		if hasSpawn || hasSitting || len(ar65Events(all, "band")) != 0 {
			t.Fatalf("Band %q: a band key or line", says)
		}
	}
}

// TestAR80_3_SpawnsAtOnce (ruling 3): admitted, unread and stale spawn at
// T with no band line; the ask carries the night, the row, part 1, the
// part's session and model; the spawn's band is the verdict and peak, the
// sitting's {1, 0, verdict}.
func TestAR80_3_SpawnsAtOnce(t *testing.T) {
	peak := 90.0
	for _, v := range []effects.BandVerdict{{Verdict: "admitted", Peak: &peak}, {Verdict: "unread"}, {Verdict: "stale"}} {
		band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {{BandVerdict: v}}}}
		w, q, o, sp, _ := ar80World(t, band, "r1")
		ar65End(t, ar71Run(w, q, o, sp))
		all := w.ledger.Written()
		spawn, sit := ar80One(t, all, "spawn", "r1"), ar80One(t, all, "sitting", "r1")
		wantPeak := "<nil>"
		if v.Peak != nil {
			wantPeak = "90"
		}
		if got := fmt.Sprint(spawn["band"]); got != "map[peak:"+wantPeak+" verdict:"+v.Verdict+"]" || spawn["ts"] != ar65T.Format(time.RFC3339) {
			t.Fatalf("%s: the spawn's band %s at %v", v.Verdict, got, spawn["ts"])
		}
		if got := fmt.Sprint(sit["band"]); got != "map[asks:1 verdict:"+v.Verdict+" wait_s:0]" {
			t.Fatalf("%s: the sitting's band %s", v.Verdict, got)
		}
		asks := band.Asks()
		if len(asks) != 1 || asks[0] != fmt.Sprintf("2026-09-22-1/r1/1 %s %s %s", spawn["session_id"], spawn["model"], ar65T.Format(time.RFC3339)) {
			t.Fatalf("%s: asks %v", v.Verdict, asks)
		}
		if got := ar80Band(t, all, "r1"); got != "" {
			t.Fatalf("%s: band lines %s, want none", v.Verdict, got)
		}
	}
}

// TestAR80_4_HoldThenAdmit (rulings 4, 7): not-started twice, then
// admitted: asks at T, +60 s and +120 s; a held line and an admitted line
// (3 asks, 120 s); the spawn at +120 s and the sitting's wall from there.
func TestAR80_4_HoldThenAdmit(t *testing.T) {
	peak := 115.0
	ns := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started", Mode: "observe", Text: "no room", Peak: &peak}}
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {ns, ns, {BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
	w, q, o, sp, clock := ar80World(t, band, "r1")
	res := ar71Run(w, q, o, sp)
	ar80Ask(t, band, 1, clock, 60*time.Second)
	ar80Ask(t, band, 2, clock, 120*time.Second)
	ar65End(t, res)
	all := w.ledger.Written()
	if got := ar80Asks(band); got != "r1 +0s, r1 +60s, r1 +120s" {
		t.Fatalf("asks %s", got)
	}
	if got := ar80Band(t, all, "r1"); got != "held not-started; admitted 3/120s" {
		t.Fatalf("band lines %s", got)
	}
	held := rowLines(all, "band", "r1")[0]
	if number(held["peak"]) != 115 || held["text"] != "no room" || held["ts"] != ar65T.Format(time.RFC3339) {
		t.Fatalf("the held line %v", held)
	}
	sit := ar80One(t, all, "sitting", "r1")
	if got := sp.spawns("r1"); fmt.Sprint(got) != "[+120s]" || number(sit["wall_s"]) != 0 || fmt.Sprint(sit["band"]) != "map[asks:3 verdict:admitted wait_s:120]" {
		t.Fatalf("spawns %v, the sitting's wall %v and band %v", got, sit["wall_s"], sit["band"])
	}
}

// TestAR80_5_QueueOneAsks (ruling 6): at --parallel 2, r1 held as in _4;
// r2 behind it writes its held line queued and asks nothing until one
// poll after r1 left, at +180 s.
func TestAR80_5_QueueOneAsks(t *testing.T) {
	ns := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {ns, ns, {BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
	w, q, o, sp, clock := ar80World(t, band, "r1", "r2")
	o.Parallel = 2
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
	ar80Ask(t, band, 1, clock, 60*time.Second)
	ar80Ask(t, band, 2, clock, 120*time.Second)
	ar65Until(t, "r1's admitted line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r1")) == 2 })
	ar69To(clock, ar65T.Add(180*time.Second))
	ar65End(t, res)
	all := w.ledger.Written()
	if got := ar80Asks(band); got != "r1 +0s, r1 +60s, r1 +120s, r2 +180s" {
		t.Fatalf("asks %s", got)
	}
	q2 := rowLines(all, "band", "r2")[0]
	if q2["peak"] != nil || q2["text"] != "behind r1" || q2["ts"] != ar65T.Format(time.RFC3339) {
		t.Fatalf("r2's queued line %v", q2)
	}
	if got := ar80Band(t, all, "r2"); got != "held queued; admitted 1/180s" {
		t.Fatalf("r2's band lines %s", got)
	}
	if fmt.Sprint(sp.spawns("r1"), sp.spawns("r2")) != "[+120s] [+180s]" {
		t.Fatalf("spawns r1 %v, r2 %v", sp.spawns("r1"), sp.spawns("r2"))
	}
}

// TestAR80_6_HoldEnds (rulings 4, 5): the owner's stop and kill end a hold
// with the clock still, --until at +90 s ends it on the clock, each with
// no spawn, no end line and the tree kept; a reset at +150 s ends it
// `reset` and the row spawns then, unasked.
func TestAR80_6_HoldEnds(t *testing.T) {
	ns := []effects.FakeAnswer{{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}}
	for _, c := range []struct{ name, line, until, stop, state string }{
		{"stop", "stop", "", "owner-stop", "skipped"},
		{"kill", "kill r1", "", "owner-kill", "failed"},
		{"until", "", "01:01", "until-reached", "skipped"},
	} {
		band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": ns}}
		w, q, o, sp, clock := ar80World(t, band, "r1")
		if c.until != "" {
			clock.T = ar65T.Add(-30 * time.Second) // T-30 s: --until 01:01 is the start's +90 s
		}
		o.Until = c.until
		res := ar71Run(w, q, o, sp)
		ar65Until(t, c.name+": the held line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r1")) == 1 })
		if c.line != "" {
			if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), c.line); err != nil {
				t.Fatal(err)
			}
		} else {
			ar80Ask(t, band, 1, clock, 30*time.Second)
			ar80Ask(t, band, 2, clock, 60*time.Second)
		}
		n := ar65End(t, res)
		all := w.ledger.Written()
		sit := ar80One(t, all, "sitting", "r1")
		if sit["stop"] != c.stop || sit["state"] != c.state || len(sp.spawns("r1")) != 0 || ar80Band(t, all, "r1") != "held not-started" {
			t.Fatalf("%s: sitting %v / %v, spawns %v, band %s", c.name, sit["state"], sit["stop"], sp.spawns("r1"), ar80Band(t, all, "r1"))
		}
		if _, err := os.Stat(filepath.Join(w.root, ".claude", "worktrees", "r1")); err != nil {
			t.Fatalf("%s: the tree: %v", c.name, err)
		}
		if c.until != "" && (n.State != relay.Stopped(relay.UntilReached) || ar80Asks(band) != "r1 -30s, r1 +30s") {
			t.Fatalf("until: night %s, asks %s", n.State, ar80Asks(band))
		}
		if c.until == "" && !clock.Now().Equal(ar65T) {
			t.Fatalf("%s: the clock moved", c.name)
		}
	}
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": ns}, ResetAt: ar65T.Add(150 * time.Second)}
	w, q, o, sp, clock := ar80World(t, band, "r1")
	res := ar71Run(w, q, o, sp)
	ar80Ask(t, band, 1, clock, 60*time.Second)
	ar80Ask(t, band, 2, clock, 120*time.Second)
	ar80Ask(t, band, 3, clock, 150*time.Second)
	ar65End(t, res)
	all := w.ledger.Written()
	spawn, sit := ar80One(t, all, "spawn", "r1"), ar80One(t, all, "sitting", "r1")
	if got := ar80Band(t, all, "r1"); got != "held not-started; reset 3/150s" || len(band.Asks()) != 3 ||
		fmt.Sprint(sp.spawns("r1")) != "[+150s]" || fmt.Sprint(spawn["band"]) != "map[peak:<nil> verdict:reset]" ||
		fmt.Sprint(sit["band"]) != "map[asks:3 verdict:reset wait_s:150]" {
		t.Fatalf("the reset: band %s, asks %d, spawns %v, spawn band %v, sitting band %v", got, len(band.Asks()), sp.spawns("r1"), spawn["band"], sit["band"])
	}
}

// TestAR80_7_Unchecked (ruling 3): three errors, an unknown verdict, a
// holding verdict whose reset fails and one whose reset is not after now:
// an `unchecked` line naming the error each, no asks or wait, the spawn at
// T.
func TestAR80_7_Unchecked(t *testing.T) {
	ns := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}
	for _, c := range []struct {
		name  string
		a     effects.FakeAnswer
		reset time.Time
		err   error
	}{
		{"exit 3", effects.FakeAnswer{Err: errors.New("agentik-session admission admit: exit 3: the lock")}, time.Time{}, nil},
		{"timeout", effects.FakeAnswer{Err: errors.New("agentik-session admission admit: context deadline exceeded after 10s")}, time.Time{}, nil},
		{"not json", effects.FakeAnswer{Err: errors.New("the admission's output: not one JSON object")}, time.Time{}, nil},
		{"maybe", effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "maybe"}}, time.Time{}, nil},
		{"no reset", ns, time.Time{}, errors.New("band.json: no resetsAt")},
		{"past reset", ns, ar65T, nil},
	} {
		band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {c.a}}, ResetAt: c.reset, ResetErr: c.err}
		w, q, o, sp, _ := ar80World(t, band, "r1")
		ar65End(t, ar71Run(w, q, o, sp))
		all := w.ledger.Written()
		line := ar80One(t, all, "band", "r1")
		_, asks := line["asks"]
		e, _ := line["error"].(string)
		if line["state"] != "unchecked" || e == "" || asks || fmt.Sprint(sp.spawns("r1")) != "[+0s]" ||
			fmt.Sprint(ar80One(t, all, "spawn", "r1")["band"]) != "map[peak:<nil> verdict:unchecked]" {
			t.Fatalf("%s: the line %v, spawns %v", c.name, line, sp.spawns("r1"))
		}
		if want := map[string]string{"maybe": `"maybe"`, "no reset": "no resetsAt", "past reset": "not after"}[c.name]; want != "" && !strings.Contains(e, want) {
			t.Fatalf("%s: the error %q, want it naming %s", c.name, e, want)
		}
	}
}

// TestAR80_8_DeniedNotice (rulings 4, 7): denied twice holds with one
// held line and one notice; not-started then denied writes a second held
// line, denied, and one notice; each body ends "; move accounts".
func TestAR80_8_DeniedNotice(t *testing.T) {
	peak := 131.0
	dn := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "denied", Text: "the week", Peak: &peak}}
	ns := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}
	ad := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "admitted"}}
	for _, c := range []struct {
		as   []effects.FakeAnswer
		want string
	}{
		{[]effects.FakeAnswer{dn, dn, ad}, "held denied; admitted 3/120s"},
		{[]effects.FakeAnswer{ns, dn, ad}, "held not-started; held denied; admitted 3/120s"},
	} {
		band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": c.as}}
		w, q, o, sp, clock := ar80World(t, band, "r1")
		notes := &effects.FakeNotify{}
		o.Notify = notes
		res := ar71Run(w, q, o, sp)
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar80Ask(t, band, 2, clock, 120*time.Second)
		ar65End(t, res)
		var bands []effects.Notice
		for _, nt := range notes.Notices() {
			if nt.Event == "band" {
				bands = append(bands, nt)
			}
		}
		if got := ar80Band(t, w.ledger.Written(), "r1"); got != c.want || len(bands) != 1 || bands[0].Body != "verdict=denied peak=131; move accounts" || bands[0].Priority != 3 {
			t.Fatalf("band lines %s (want %s), band notices %+v", got, c.want, bands)
		}
	}
}

// TestAR80_9_PartsNeverAsk (ruling 3): r1 refused by a five_hour event
// resetting at T+1h asks once, and its part 2 spawns at the wake unasked;
// RunRow at Part 2 asks nothing where Part 1 asks; ar71World's model pause
// asks once per row, its part at the reset unasked.
func TestAR80_9_PartsNeverAsk(t *testing.T) {
	band := &effects.FakeBand{}
	w, q, o, sp, clock := ar80World(t, band, "r1")
	sp.first["r1"] = ar65Event("five_hour", ar65T.Add(time.Hour))
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	ar65End(t, res)
	spawns := rowLines(w.ledger.Written(), "spawn", "r1")
	_, asked := spawns[len(spawns)-1]["band"]
	if ar80Asks(band) != "r1 +0s" || fmt.Sprint(sp.spawns("r1")) != "[+0s +3720s]" || asked {
		t.Fatalf("asks %s, spawns %v, part 2's band %v", ar80Asks(band), sp.spawns("r1"), asked)
	}

	for _, part := range []int{2, 1} {
		rw := newWorld(t)
		st, rp := rw.stage(t, fmt.Sprintf("p%d", part))
		b := &effects.FakeBand{ResetAt: ar65T.Add(time.Hour)}
		fx := rw.fx
		fx.Band = b
		night.RunRow(fx, &effects.FakeSpawner{Out: ar67Clean, PID: 4343}, st, rp, []byte("prompt"),
			night.RunOptions{Part: part, Env: rw.env("ok.jsonl"), Transcripts: rw.transcripts})
		_, asked := ar80One(t, rw.ledger.Written(), "spawn", rp.ID)["band"]
		if want := part == 1; asked != want || (len(b.Asks()) == 1) != want {
			t.Fatalf("RunRow at Part %d: asks %v, the spawn's band %v", part, b.Asks(), asked)
		}
	}

	mw, mq, mo, msp, mclock := ar71World(t, nil)
	mb := &effects.FakeBand{ResetAt: ar65T.Add(2 * time.Hour)}
	mw.fx.Band = mb
	mres := ar71Run(mw, mq, mo, msp)
	ar65Until(t, "r3's sitting line, the night paused", func() bool {
		return len(rowLines(mw.ledger.Written(), "sitting", "r3")) == 1 && ar65Paused(mo, "r1")
	})
	ar69To(mclock, ar65T.Add(time.Hour+2*time.Minute))
	ar65End(t, mres)
	var rows []string
	for _, a := range mb.Asks() {
		rows = append(rows, strings.Split(strings.Fields(a)[0], "/")[1])
	}
	ms := rowLines(mw.ledger.Written(), "spawn", "r1")
	_, asked = ms[len(ms)-1]["band"]
	if fmt.Sprint(rows) != "[r1 r2 r3]" || len(ms) != 2 || asked {
		t.Fatalf("the model pause: asks %v, r1's spawns %d, part 2's band %v", rows, len(ms), asked)
	}
}

// TestAR80_10_HeldThenPaused (rulings 5, 6): at --parallel 2, r1 asks and
// spawns at T; r2 holds not-started; r1's five_hour refusal pauses the
// night and r2's hold ends failed usage-limit with `limit: paused`; at the
// wake r2's part 2 spawns unasked.
func TestAR80_10_HeldThenPaused(t *testing.T) {
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r2": {{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}}}}
	w, q, o, sp, clock := ar80World(t, band, "r1", "r2")
	o.Parallel = 2
	sp.first["r1"] = ar65Event("five_hour", ar65T.Add(time.Hour))
	sp.wait["r1"] = func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) > 0 }
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r2's sitting line, the night paused", func() bool {
		return len(rowLines(w.ledger.Written(), "sitting", "r2")) == 1 && ar65Paused(o, "r1", "r2")
	})
	sl := rowLines(w.ledger.Written(), "sitting", "r2")[0]
	if sl["state"] != "failed" || sl["reason"] != "usage-limit" || sl["limit"] != "paused" || len(sp.spawns("r2")) != 0 {
		t.Fatalf("r2's sitting line %v, spawns %v", sl, sp.spawns("r2"))
	}
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	ar65End(t, res)
	r2 := rowLines(w.ledger.Written(), "spawn", "r2")
	if len(r2) != 1 || ar80Asks(band) != "r1 +0s, r2 +0s" || fmt.Sprint(sp.spawns("r2")) != "[+3720s]" {
		t.Fatalf("r2's spawns %v, asks %s", sp.spawns("r2"), ar80Asks(band))
	}
	if _, asked := r2[0]["band"]; asked || ar80Band(t, w.ledger.Written(), "r2") != "held not-started" {
		t.Fatalf("r2's part 2 band %v, lines %s", r2[0]["band"], ar80Band(t, w.ledger.Written(), "r2"))
	}
}
