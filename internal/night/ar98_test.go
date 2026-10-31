package night_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar98Answers is a FakeBand's answers for r1, the last repeating.
func ar98Answers(vs ...effects.BandVerdict) *effects.FakeBand {
	as := make([]effects.FakeAnswer, len(vs))
	for i, v := range vs {
		as[i] = effects.FakeAnswer{BandVerdict: v}
	}
	return &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": as}}
}

// ar98Spawner is ar71Spawner's children, a clean one committing so its end is
// green and frees its slot, with each row's spawn times as ar80Spawner reads
// them.
type ar98Spawner struct {
	ar71 *ar71Spawner
	at   *ar80Spawner
}

func ar98Green(w *world, clock *effects.StepClock) *ar98Spawner {
	return &ar98Spawner{
		ar71: &ar71Spawner{w: w, n: map[string]int{}, first: map[string]effects.FakeRun{}, until: map[string]func() bool{}},
		at:   &ar80Spawner{clock: clock, at: map[string][]time.Time{}},
	}
}

func (a *ar98Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	a.at.mu.Lock()
	row := filepath.Base(sp.Dir)
	a.at.at[row] = append(a.at.at[row], a.at.clock.Now())
	a.at.mu.Unlock()
	return a.ar71.Start(sp)
}

func (a *ar98Spawner) spawns(row string) []string { return a.at.spawns(row) }

// TestAR98_1_NoneDeniedStopsAsWeekly (ruling 2): `denied` on window `none`,
// on the first ask and on an ask inside a hold, spawns nothing and stops the
// night `usage-limit` with detail `a weekly limit`, as TestAR65_4's weekly
// case reads it: the end line, no denied notice, no pause or wake, the
// sitting line with no `limit`, its `stop` what a weekly refusal in the stream
// writes, the row's log naming the end. Any other window holds as today.
func TestAR98_1_NoneDeniedStopsAsWeekly(t *testing.T) {
	week := effects.BandVerdict{Verdict: "denied", Text: "the week", Window: "none", ResumeAt: ar65T.Add(72 * time.Hour).Format(time.RFC3339)}
	ad := effects.BandVerdict{Verdict: "admitted"}
	// The stop the existing weekly path writes on the sitting line.
	cw, cq, co, csp, _ := ar80World(t, &effects.FakeBand{}, "r1")
	csp.first["r1"] = ar65Event("seven_day", ar65T.Add(48*time.Hour))
	cn := ar65End(t, ar71Run(cw, cq, co, csp))
	if cn.State != relay.Stopped(relay.UsageLimit) {
		t.Fatalf("the weekly control: night %s", cn.State)
	}
	weeklyStop := ar80One(t, cw.ledger.Written(), "sitting", "r1")["stop"]
	if weeklyStop != string(relay.UsageLimit) {
		t.Fatalf("the weekly control's stop %v", weeklyStop)
	}
	weekly := func(t *testing.T, w *world, o night.NightOptions, n *night.Night, sp *ar80Spawner, notes *effects.FakeNotify, wantBand string) {
		t.Helper()
		all := w.ledger.Written()
		stop := lastOf(all, "stop")
		if n.State != relay.Stopped(relay.UsageLimit) || stop["detail"] != "a weekly limit" {
			t.Fatalf("night %s, stop line %v; want stopped:usage-limit, a weekly limit", n.State, stop)
		}
		if p, wk := len(ar65Events(all, "pause")), len(ar65Events(all, "wake")); p != 0 || wk != 0 {
			t.Fatalf("%d pause lines, %d wake lines; want none", p, wk)
		}
		if got := fmt.Sprint(sp.spawns("r1")); got != "[]" {
			t.Fatalf("r1 spawned at %s", got)
		}
		if got := ar80Band(t, all, "r1"); got != wantBand {
			t.Fatalf("band lines %q, want %q", got, wantBand)
		}
		sit := ar80One(t, all, "sitting", "r1")
		if sit["state"] != "failed" || sit["reason"] != "usage-limit" || sit["limit"] != nil {
			t.Fatalf("the sitting line %v; want failed usage-limit with no limit", sit)
		}
		if sit["stop"] != weeklyStop {
			t.Fatalf("the sitting's stop %v, want the weekly path's %v", sit["stop"], weeklyStop)
		}
		logged, err := os.ReadFile(night.RowLog(o.State, n.ID, "r1"))
		for _, want := range []string{
			"relay r1: the band denies it on a login with no 5h window: a weekly limit\n",
			"relay r1: usage-limit before the spawn\n",
		} {
			if err != nil || !strings.Contains(string(logged), want) {
				t.Fatalf("the row's log %q, %v; want %q", logged, err, want)
			}
		}
		for _, nt := range notes.Notices() {
			if nt.Event == "band" {
				t.Fatalf("a band notice: %+v", nt)
			}
		}
	}

	t.Run("the first ask", func(t *testing.T) {
		w, q, o, sp, _ := ar80World(t, ar98Answers(week), "r1", "r2")
		notes := &effects.FakeNotify{}
		o.Notify = notes
		n := ar65End(t, ar71Run(w, q, o, sp))
		weekly(t, w, o, n, sp, notes, "weekly")
		if states(n) != "r1=failed r2=skipped" || fmt.Sprint(sp.spawns("r2")) != "[]" {
			t.Fatalf("states %s, r2's spawns %v", states(n), sp.spawns("r2"))
		}
		line := ar80One(t, w.ledger.Written(), "band", "r1")
		if _, asks := line["asks"]; asks {
			t.Fatalf("an unheld end line carries asks: %v", line)
		}
		sit := ar80One(t, w.ledger.Written(), "sitting", "r1")
		if fmt.Sprint(sit["band"]) != "map[asks:1 verdict:denied wait_s:0]" {
			t.Fatalf("the sitting's band %v", sit["band"])
		}
	})
	t.Run("inside a hold", func(t *testing.T) {
		band := ar98Answers(effects.BandVerdict{Verdict: "not-started"}, week)
		w, q, o, sp, clock := ar80World(t, band, "r1")
		notes := &effects.FakeNotify{}
		o.Notify = notes
		res := ar71Run(w, q, o, sp)
		ar80Ask(t, band, 1, clock, 60*time.Second)
		n := ar65End(t, res)
		weekly(t, w, o, n, sp, notes, "held not-started; weekly 2/60s")
	})
	t.Run("a held row behind it stops with it", func(t *testing.T) {
		band := ar98Answers(effects.BandVerdict{Verdict: "not-started"}, week)
		w, q, o, sp, clock := ar80World(t, band, "r1", "r2")
		o.Parallel = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
		ar80Ask(t, band, 1, clock, 60*time.Second)
		n := ar65End(t, res)
		if n.State != relay.Stopped(relay.UsageLimit) || len(sp.spawns("r1"))+len(sp.spawns("r2")) != 0 || ar80Asks(band) != "r1 +0s, r1 +60s" {
			t.Fatalf("night %s, spawns %v %v, asks %s", n.State, sp.spawns("r1"), sp.spawns("r2"), ar80Asks(band))
		}
		if logged, _ := os.ReadFile(night.RowLog(o.State, n.ID, "r2")); strings.Contains(string(logged), "waits for r1") {
			t.Fatalf("r2 waits on a row that never spawned: %q", logged)
		}
	})
	for _, c := range []struct{ name, window string }{{"window five_hour", "five_hour"}, {"no window", ""}} {
		t.Run("denied holds on "+c.name, func(t *testing.T) {
			band := ar98Answers(effects.BandVerdict{Verdict: "denied", Text: "the five hours", Window: c.window}, ad)
			w, q, o, sp, clock := ar80World(t, band, "r1")
			res := ar71Run(w, q, o, sp)
			ar80Ask(t, band, 1, clock, 60*time.Second)
			n := ar65End(t, res)
			if got := ar80Band(t, w.ledger.Written(), "r1"); got != "held denied; admitted 2/60s" || n.State == relay.Stopped(relay.UsageLimit) || fmt.Sprint(sp.spawns("r1")) != "[+60s]" {
				t.Fatalf("night %s, band %q, spawns %v", n.State, got, sp.spawns("r1"))
			}
		})
	}
	t.Run("a wait on none holds", func(t *testing.T) {
		band := ar98Answers(effects.BandVerdict{Verdict: "wait", Window: "none"}, ad)
		w, q, o, sp, clock := ar80World(t, band, "r1")
		res := ar71Run(w, q, o, sp)
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar65End(t, res)
		if got := ar80Band(t, w.ledger.Written(), "r1"); got != "held wait; admitted 2/60s" {
			t.Fatalf("band %q", got)
		}
	})
}

// ar98Seconds is a print's baseline time, blanked for the comparison.
var ar98Seconds = regexp.MustCompile(` in [0-9.]+s`)

// TestAR98_4_PlanShowsBand (ruling 6): on band.json's window `none` the plan
// prints one line, with `demand`'s band and the until's own HH:MM when it is
// there, and its JSON gains `band`; on any other window, `band: off`, a file
// not readable or none, the print and the JSON are today's. It never asks.
func TestAR98_4_PlanShowsBand(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	band := &effects.FakeBand{}
	w.fx.Band = band
	home := t.TempDir()
	state := filepath.Join(home, "state")
	env := func(k string) string {
		return map[string]string{"HOME": home, "XDG_STATE_HOME": state}[k]
	}
	build := func(getenv func(string) string) (string, string, *night.Plan) {
		t.Helper()
		minted := 0
		o.PlanOptions.UUID = func() string { minted++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", minted) }
		fx := w.fx
		fx.Getenv = getenv
		p, err := night.BuildPlan(fx, q, o.PlanOptions)
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		night.Print(&out, p)
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		return ar98Seconds.ReplaceAllString(out.String(), " in Xs"), string(b), p
	}
	put := func(folder, body string) {
		t.Helper()
		p := filepath.Join(state, folder, "admission", "band.json")
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	today, _, _ := build(nil)
	if strings.Contains(today, "band:") {
		t.Fatalf("today's plan names the band:\n%s", today)
	}

	same := func(name string) {
		t.Helper()
		out, js, _ := build(env)
		if out != today {
			t.Fatalf("%s: the print moved:\n%s\nwant:\n%s", name, out, today)
		}
		if strings.Contains(js, `"band"`) {
			t.Fatalf("%s: the JSON gained band: %s", name, js)
		}
	}
	same("no file")
	for _, c := range []struct{ name, body string }{
		{"five_hour", `{"window":"five_hour","resetsAt":"2026-10-30T10:30:00-07:00"}`},
		{"five_hour with demand", `{"window":"five_hour","demand":{"band":"avoid","until":"2026-10-30T11:00:00-07:00"}}`},
		{"no window", `{"resetsAt":"2026-10-30T10:30:00-07:00"}`},
		{"not JSON", `not json`},
	} {
		put(effects.BandTool, c.body)
		same(c.name)
	}

	for _, c := range []struct{ name, body, line, demand string }{
		{"demand", `{"window":"none","demand":{"band":"avoid","until":"2026-10-30T11:00:00-07:00","next":{"band":"free"}}}`,
			"band: no 5h window · avoid to 11:00", `"demand":{"band":"avoid","until":"2026-10-30T11:00:00-07:00"}`},
		{"the until's own digits, UTC", `{"window":"none","demand":{"band":"avoid","until":"2026-10-30T18:05:00Z"}}`,
			"band: no 5h window · avoid to 18:05", `"demand":{"band":"avoid","until":"2026-10-30T18:05:00Z"}`},
		{"no demand", `{"window":"none"}`, "band: no 5h window", ""},
		{"a null demand", `{"window":"none","demand":null}`, "band: no 5h window", ""},
		{"an until with no digits", `{"window":"none","demand":{"band":"avoid","until":"soon"}}`, "band: no 5h window", `"demand":{"band":"avoid","until":"soon"}`},
		{"a demand with no band", `{"window":"none","demand":{"until":"2026-10-30T11:00:00-07:00"}}`, "band: no 5h window", `"demand":{"band":"","until":"2026-10-30T11:00:00-07:00"}`},
		{"an until with its date not first", `{"window":"none","demand":{"band":"avoid","until":"by 2026-10-30T11:00:00-07:00"}}`, "band: no 5h window", `"demand":{"band":"avoid","until":"by 2026-10-30T11:00:00-07:00"}`},
		{"a demand that is a string", `{"window":"none","demand":"weird"}`, "band: no 5h window", ""},
		{"a demand that is a list", `{"window":"none","demand":[]}`, "band: no 5h window", ""},
		{"an until that is a number", `{"window":"none","demand":{"until":5}}`, "band: no 5h window", ""},
	} {
		put(effects.BandTool, c.body)
		out, js, p := build(env)
		var lines []string
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "band:") {
				lines = append(lines, l)
			}
		}
		if fmt.Sprint(lines) != fmt.Sprint([]string{c.line}) {
			t.Fatalf("%s: band lines %q, want %q", c.name, lines, c.line)
		}
		if without := strings.Replace(out, c.line+"\n", "", 1); without != today {
			t.Fatalf("%s: the rest of the print moved:\n%s\nwant:\n%s", c.name, without, today)
		}
		want := `"band":{"window":"none"`
		if c.demand != "" {
			want += "," + c.demand
		}
		if !strings.Contains(js, want+"}") {
			t.Fatalf("%s: the JSON %s; want it holding %s}", c.name, js, want)
		}
		if p.Band == nil || p.Band.Window != "none" {
			t.Fatalf("%s: the plan's band %+v", c.name, p.Band)
		}
	}

	// A read error other than no file reads as no line.
	unread := filepath.Join(state, effects.BandTool, "admission", "band.json")
	if err := os.Remove(unread); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(unread, 0o700); err != nil {
		t.Fatal(err)
	}
	same("a read error")
	os.Remove(unread)

	// `band: off`, on the same file: today's.
	put(effects.BandTool, `{"window":"none"}`)
	cfg := filepath.Join(w.root, effects.RepoConfigFile)
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("relay:\n  band: off\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	same("band: off")
	os.Remove(cfg)

	if asks := band.Asks(); len(asks) != 0 {
		t.Fatalf("the plan asked: %v", asks)
	}
}

// TestAR98_5_WaitWithoutResetHolds (rulings 3, 4): a `wait` with no
// `resume_at` and a Reset that errs holds with no reset, re-asking each
// BandPoll to its verdict; a row behind it holds, one arriving after it came
// to the head too; a `wait` with no `window` and every other verdict stay
// `unchecked` on Reset's error (Amendment 1, ruling 1). The answer's
// `resume_at` is the hold's reset; one past or not RFC 3339 falls to Reset.
func TestAR98_5_WaitWithoutResetHolds(t *testing.T) {
	noReset := errors.New("band.json: no resetsAt")
	wait := effects.BandVerdict{Verdict: "wait", Text: "one at a time", Window: "five_hour"}
	ad := effects.BandVerdict{Verdict: "admitted"}

	t.Run("it holds and re-asks", func(t *testing.T) {
		band := ar98Answers(wait, wait, wait, ad)
		band.ResetErr = noReset
		w, q, o, sp, clock := ar80World(t, band, "r1")
		res := ar71Run(w, q, o, sp)
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar80Ask(t, band, 2, clock, 120*time.Second)
		ar80Ask(t, band, 3, clock, 3*time.Hour) // past any reset a hold might have read
		n := ar65End(t, res)
		all := w.ledger.Written()
		if got := ar80Band(t, all, "r1"); got != "held wait; admitted 4/10800s" {
			t.Fatalf("band %q", got)
		}
		if got := ar80Asks(band); got != "r1 +0s, r1 +60s, r1 +120s, r1 +10800s" {
			t.Fatalf("asks %s", got)
		}
		if fmt.Sprint(sp.spawns("r1")) != "[+10800s]" {
			t.Fatalf("spawns %v", sp.spawns("r1"))
		}
		held := rowLines(all, "band", "r1")[0]
		if held["verdict"] != "wait" || held["text"] != "one at a time" || held["peak"] != nil {
			t.Fatalf("the held line %v", held)
		}
		if _, ok := held["reset"]; ok {
			t.Fatalf("the held line names a reset: %v", held)
		}
		logged, err := os.ReadFile(night.RowLog(o.State, n.ID, "r1"))
		if want := "relay r1: the band holds it: wait; " + effects.BandLever + "\n"; err != nil || !strings.Contains(string(logged), want) {
			t.Fatalf("the row's log %q, %v; want %q", logged, err, want)
		}
	})
	t.Run("--until ends it", func(t *testing.T) {
		band := ar98Answers(wait)
		band.ResetErr = noReset
		w, q, o, sp, clock := ar80World(t, band, "r1")
		clock.T = ar65T.Add(-30 * time.Second) // T-30 s: --until 01:01 is the start's +90 s
		o.Until = "01:01"
		res := ar71Run(w, q, o, sp)
		ar80Ask(t, band, 1, clock, 30*time.Second)
		ar80Ask(t, band, 2, clock, 60*time.Second)
		n := ar65End(t, res)
		if n.State != relay.Stopped(relay.UntilReached) || len(sp.spawns("r1")) != 0 || ar80Band(t, w.ledger.Written(), "r1") != "held wait" {
			t.Fatalf("night %s, spawns %v, band %s", n.State, sp.spawns("r1"), ar80Band(t, w.ledger.Written(), "r1"))
		}
	})
	t.Run("the owner's stop ends it", func(t *testing.T) {
		band := ar98Answers(wait)
		band.ResetErr = noReset
		w, q, o, sp, _ := ar80World(t, band, "r1")
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "the held line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r1")) == 1 })
		if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), "stop"); err != nil {
			t.Fatal(err)
		}
		n := ar65End(t, res)
		if n.State != relay.Stopped(relay.OwnerStop) || len(sp.spawns("r1")) != 0 {
			t.Fatalf("night %s, spawns %v", n.State, sp.spawns("r1"))
		}
	})
	t.Run("a row behind it holds", func(t *testing.T) {
		band := ar98Answers(wait, ad)
		band.ResetErr = noReset
		w, q, o, sp, clock := ar80World(t, band, "r1", "r2")
		o.Parallel = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
		if got := ar80Band(t, w.ledger.Written(), "r2"); got != "held queued" {
			t.Fatalf("r2's band %q", got)
		}
		if len(sp.spawns("r2")) != 0 {
			t.Fatalf("r2 spawned behind a hold: %v", sp.spawns("r2"))
		}
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar65Until(t, "r1's admitted line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r1")) == 2 })
		ar69To(clock, ar65T.Add(120*time.Second))
		ar65End(t, res)
		all := w.ledger.Written()
		if got := ar80Band(t, all, "r2"); got != "held queued; admitted 1/120s" {
			t.Fatalf("r2's band %q", got)
		}
		if got := ar80Asks(band); got != "r1 +0s, r1 +60s, r2 +120s" {
			t.Fatalf("asks %s", got)
		}
	})
	t.Run("a third row behind a queued one holds", func(t *testing.T) {
		band := ar98Answers(wait, ad)
		band.Answers["r2"] = []effects.FakeAnswer{{BandVerdict: wait}, {BandVerdict: ad}}
		band.ResetErr = noReset
		w, q, o, sp, clock := ar80World(t, band, "r1", "r2", "r3")
		o.Parallel = 3
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "the queued lines", func() bool {
			all := w.ledger.Written()
			return len(rowLines(all, "band", "r2")) == 1 && len(rowLines(all, "band", "r3")) == 1
		})
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar65Until(t, "r1's admitted line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r1")) == 2 })
		ar69To(clock, ar65T.Add(120*time.Second))
		ar65Until(t, "r2's first ask", func() bool { return len(band.Asks()) == 3 })
		ar69To(clock, ar65T.Add(180*time.Second))
		ar65Until(t, "r2's admitted line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 2 })
		ar69To(clock, ar65T.Add(240*time.Second))
		ar65End(t, res)
		all := w.ledger.Written()
		for _, l := range rowLines(all, "band", "r3") {
			if l["state"] == "unchecked" {
				t.Fatalf("r3 unchecked: %v", l)
			}
		}
		if got := ar80Asks(band); got != "r1 +0s, r1 +60s, r2 +120s, r2 +180s, r3 +240s" {
			t.Fatalf("asks %s", got)
		}
	})
	t.Run("a row arriving after a bare queued row is promoted holds", func(t *testing.T) {
		band := ar98Answers(wait, ad)
		band.Answers["r2"] = []effects.FakeAnswer{{BandVerdict: wait}}
		band.ResetErr = noReset
		w, q, o, _, clock := ar80World(t, band, "r1", "r2", "r3")
		sp := ar98Green(w, clock)
		o.Parallel = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar65Until(t, "r3's line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r3")) == 1 })
		all := w.ledger.Written()
		if got := ar80Band(t, all, "r3"); got != "held queued" || len(sp.spawns("r3")) != 0 {
			t.Fatalf("r3's band %q, spawns %v", got, sp.spawns("r3"))
		}
		if line := rowLines(all, "band", "r3")[0]; line["text"] != "behind r2" {
			t.Fatalf("r3's line %v", line)
		}
		if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), "stop"); err != nil {
			t.Fatal(err)
		}
		ar65End(t, res)
	})
	for _, c := range []struct {
		name string
		v    effects.BandVerdict
		want string
	}{
		{"a wait with no window", effects.BandVerdict{Verdict: "wait"}, "no resetsAt"},
		{"not-started", effects.BandVerdict{Verdict: "not-started"}, "no resetsAt"},
		{"deferred", effects.BandVerdict{Verdict: "deferred"}, "no resetsAt"},
		{"denied", effects.BandVerdict{Verdict: "denied", Window: "five_hour"}, "no resetsAt"},
		{"a wait with a resume_at not RFC 3339", effects.BandVerdict{Verdict: "wait", ResumeAt: "11:00"}, "no resetsAt"},
		{"a wait with a resume_at past", effects.BandVerdict{Verdict: "wait", ResumeAt: ar65T.Add(-time.Hour).Format(time.RFC3339)}, "no resetsAt"},
	} {
		t.Run(c.name+" with Reset's error is unchecked", func(t *testing.T) {
			band := ar98Answers(c.v)
			band.ResetErr = noReset
			w, q, o, sp, _ := ar80World(t, band, "r1")
			ar65End(t, ar71Run(w, q, o, sp))
			line := ar80One(t, w.ledger.Written(), "band", "r1")
			if e, _ := line["error"].(string); line["state"] != "unchecked" || !strings.Contains(e, c.want) || fmt.Sprint(sp.spawns("r1")) != "[+0s]" {
				t.Fatalf("the line %v, spawns %v", line, sp.spawns("r1"))
			}
		})
	}
	t.Run("a wait whose reset is past stays unchecked", func(t *testing.T) {
		band := ar98Answers(wait)
		band.ResetAt = ar65T
		w, q, o, sp, _ := ar80World(t, band, "r1")
		ar65End(t, ar71Run(w, q, o, sp))
		line := ar80One(t, w.ledger.Written(), "band", "r1")
		if e, _ := line["error"].(string); line["state"] != "unchecked" || !strings.Contains(e, "not after") {
			t.Fatalf("the line %v", line)
		}
	})
	for _, c := range []struct {
		name, resume string
		resetAt      time.Time
		resetErr     error
		end          time.Duration
	}{
		{"resume_at beats Reset", ar65T.Add(150 * time.Second).Format(time.RFC3339), ar65T.Add(2 * time.Hour), nil, 150 * time.Second},
		{"resume_at with Reset's error", ar65T.Add(150 * time.Second).Format(time.RFC3339), time.Time{}, noReset, 150 * time.Second},
		{"a past resume_at reads Reset", ar65T.Add(-time.Hour).Format(time.RFC3339), ar65T.Add(150 * time.Second), nil, 150 * time.Second},
		{"a bad resume_at reads Reset", "11:00", ar65T.Add(150 * time.Second), nil, 150 * time.Second},
		{"no resume_at reads Reset", "", ar65T.Add(150 * time.Second), nil, 150 * time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			band := ar98Answers(effects.BandVerdict{Verdict: "not-started", ResumeAt: c.resume})
			band.ResetAt, band.ResetErr = c.resetAt, c.resetErr
			w, q, o, sp, clock := ar80World(t, band, "r1")
			res := ar71Run(w, q, o, sp)
			ar80Ask(t, band, 1, clock, 60*time.Second)
			ar80Ask(t, band, 2, clock, 120*time.Second)
			ar80Ask(t, band, 3, clock, c.end)
			ar65End(t, res)
			want := fmt.Sprintf("held not-started; reset 3/%ds", int(c.end.Seconds()))
			if got := ar80Band(t, w.ledger.Written(), "r1"); got != want || fmt.Sprint(sp.spawns("r1")) != fmt.Sprintf("[+%ds]", int(c.end.Seconds())) {
				t.Fatalf("band %q, want %q; spawns %v", got, want, sp.spawns("r1"))
			}
		})
	}
}

// TestAR98_6_QueuedTakesHeadReset (Amendment 1, ruling 2): a row queued behind
// a held head takes the head's reset and never asks Reset, so it neither goes
// `unchecked` on Reset's error nor overtakes a head held to the week's reset.
func TestAR98_6_QueuedTakesHeadReset(t *testing.T) {
	noReset := errors.New("band.json: no resetsAt")
	week := 7 * 24 * time.Hour
	weekS := int(week.Seconds())
	onWeek := effects.BandVerdict{Verdict: "denied", Text: "the week", Window: "five_hour", ResumeAt: ar65T.Add(week).Format(time.RFC3339)}
	for _, c := range []struct {
		name     string
		resetAt  time.Time
		resetErr error
	}{
		{"Reset errs", time.Time{}, noReset},
		{"Reset answers a five-hour reset", ar65T.Add(150 * time.Second), nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			band := ar98Answers(onWeek)
			band.ResetAt, band.ResetErr = c.resetAt, c.resetErr
			w, q, o, _, clock := ar80World(t, band, "r1", "r2")
			sp := ar98Green(w, clock)
			o.Parallel = 2
			res := ar71Run(w, q, o, sp)
			ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
			if got := ar80Band(t, w.ledger.Written(), "r2"); got != "held queued" {
				t.Fatalf("r2's band %q", got)
			}
			ar80Ask(t, band, 1, clock, 60*time.Second)
			ar80Ask(t, band, 2, clock, 120*time.Second)
			ar80Ask(t, band, 3, clock, 180*time.Second) // past the five-hour reset
			ar80Ask(t, band, 4, clock, week-time.Second)
			ar80Ask(t, band, 5, clock, week)
			ar65Until(t, "the spawns", func() bool { return len(sp.spawns("r1")) == 1 && len(sp.spawns("r2")) == 1 })
			ar65End(t, res)
			all := w.ledger.Written()
			if got, want := ar80Band(t, all, "r2"), fmt.Sprintf("held queued; reset 0/%ds", weekS); got != want {
				t.Fatalf("r2's band %q, want %q", got, want)
			}
			at := fmt.Sprintf("[+%ds]", weekS)
			if fmt.Sprint(sp.spawns("r1")) != at || fmt.Sprint(sp.spawns("r2")) != at {
				t.Fatalf("spawns r1 %v, r2 %v; want %s each", sp.spawns("r1"), sp.spawns("r2"), at)
			}
		})
	}
	t.Run("a head's reset read from Reset passes on as read", func(t *testing.T) {
		band := ar98Answers(effects.BandVerdict{Verdict: "not-started"})
		band.ResetAt = ar65T.Add(150 * time.Second)
		w, q, o, sp, clock := ar80World(t, band, "r1", "r2")
		o.Parallel = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar80Ask(t, band, 2, clock, 120*time.Second)
		ar80Ask(t, band, 3, clock, 150*time.Second)
		ar65End(t, res)
		if got := ar80Band(t, w.ledger.Written(), "r2"); got != "held queued; reset 0/150s" || fmt.Sprint(sp.spawns("r2")) != "[+150s]" {
			t.Fatalf("r2's band %q, spawns %v", got, sp.spawns("r2"))
		}
	})
	t.Run("a row arriving after a queued row is promoted takes the same reset", func(t *testing.T) {
		held := effects.BandVerdict{Verdict: "not-started", ResumeAt: ar65T.Add(time.Hour).Format(time.RFC3339)}
		band := ar98Answers(held, effects.BandVerdict{Verdict: "admitted"})
		band.Answers["r2"] = []effects.FakeAnswer{{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}}
		band.ResetErr = noReset
		w, q, o, _, clock := ar80World(t, band, "r1", "r2", "r3")
		sp := ar98Green(w, clock)
		o.Parallel = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar65Until(t, "r3's line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r3")) == 1 })
		ar69To(clock, ar65T.Add(time.Hour))
		ar65End(t, res)
		all := w.ledger.Written()
		if got, want := ar80Band(t, all, "r3"), "held queued; reset 0/3540s"; got != want || fmt.Sprint(sp.spawns("r3")) != "[+3600s]" {
			t.Fatalf("r3's band %q, want %q; spawns %v", got, want, sp.spawns("r3"))
		}
	})
	t.Run("a head's later hold moves it for a row that opens after", func(t *testing.T) {
		first := effects.BandVerdict{Verdict: "not-started", ResumeAt: ar65T.Add(time.Hour).Format(time.RFC3339)}
		later := effects.BandVerdict{Verdict: "not-started", ResumeAt: ar65T.Add(2 * time.Hour).Format(time.RFC3339)}
		band := ar98Answers(first)
		band.Answers["r3"] = []effects.FakeAnswer{{BandVerdict: later}}
		band.ResetErr = noReset
		w, q, o, _, clock := ar80World(t, band, "r1", "r2", "r3", "r4")
		sp := ar98Green(w, clock)
		o.Parallel = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar80Ask(t, band, 2, clock, time.Hour)
		ar65Until(t, "r3's place", func() bool { return len(sp.spawns("r1")) == 1 && len(sp.spawns("r2")) == 1 })
		ar69To(clock, ar65T.Add(time.Hour+60*time.Second))
		ar65Until(t, "r4's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r4")) == 1 })
		ar69To(clock, ar65T.Add(2*time.Hour))
		ar65End(t, res)
		if got, want := ar80Band(t, w.ledger.Written(), "r4"), "held queued; reset 0/3540s"; got != want || fmt.Sprint(sp.spawns("r4")) != "[+7200s]" {
			t.Fatalf("r4's band %q, want %q; spawns %v", got, want, sp.spawns("r4"))
		}
	})
}
