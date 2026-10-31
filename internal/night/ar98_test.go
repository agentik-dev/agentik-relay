package night_test

import (
	"encoding/json"
	"errors"
	"fmt"
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

// TestAR98_1_NoneDeniedStopsAsWeekly (ruling 2): `denied` on window `none`,
// on the first ask and on an ask inside a hold, spawns nothing and stops the
// night `usage-limit` with detail `a weekly limit`, as TestAR65_4's weekly
// case reads it: the end line, no denied notice, no pause or wake, the
// sitting line with no `limit`. Any other window holds as today.
func TestAR98_1_NoneDeniedStopsAsWeekly(t *testing.T) {
	week := effects.BandVerdict{Verdict: "denied", Text: "the week", Window: "none", ResumeAt: ar65T.Add(72 * time.Hour).Format(time.RFC3339)}
	ad := effects.BandVerdict{Verdict: "admitted"}
	weekly := func(t *testing.T, w *world, n *night.Night, sp *ar80Spawner, notes *effects.FakeNotify, wantBand string) {
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
		weekly(t, w, n, sp, notes, "weekly denied")
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
		weekly(t, w, n, sp, notes, "held not-started; weekly denied 2/60s")
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
// BandPoll to its verdict; a row behind it holds; a row behind any other
// head and every other verdict stay `unchecked` on Reset's error. The
// answer's `resume_at` is the hold's reset; one past or not RFC 3339 falls
// to Reset.
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
	t.Run("a row behind any other head stays unchecked", func(t *testing.T) {
		held := effects.BandVerdict{Verdict: "not-started", ResumeAt: ar65T.Add(time.Hour).Format(time.RFC3339)}
		band := ar98Answers(held)
		band.ResetErr = noReset
		w, q, o, sp, _ := ar80World(t, band, "r1", "r2")
		o.Parallel = 2
		res := ar71Run(w, q, o, sp)
		ar65Until(t, "r2's line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
		line := rowLines(w.ledger.Written(), "band", "r2")[0]
		if e, _ := line["error"].(string); line["state"] != "unchecked" || !strings.Contains(e, "no resetsAt") {
			t.Fatalf("r2's line %v", line)
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
