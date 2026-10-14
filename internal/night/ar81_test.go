package night_test

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// status, report and doctor read a night's band hold (band-pacing-r1 §3;
// AR81): the status half on AR80's worlds, the report half on ledger lines.

// ar81Rows is the night's status.json once ok holds of it, and its rows by
// id; 10 s at most.
func ar81Rows(t *testing.T, o night.NightOptions, what string, ok func(st night.Status, by map[string]night.RowStatus) bool) (night.Status, map[string]night.RowStatus) {
	t.Helper()
	var st night.Status
	var by map[string]night.RowStatus
	ar65Until(t, what, func() bool {
		st = night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night)
		by = map[string]night.RowStatus{}
		for _, r := range st.Rows {
			by[r.ID] = r
		}
		return ok(st, by)
	})
	return st, by
}

// ar81Band is a row's band in short: verdict, peak, since and asks.
func ar81Band(b *night.RowBand) string {
	if b == nil {
		return "null"
	}
	peak := "null"
	if b.Peak != nil {
		peak = fmt.Sprint(*b.Peak)
	}
	return fmt.Sprintf("{%s %s %s %d}", b.Verdict, peak, b.Since, b.Asks)
}

// TestAR81_1_StatusHeld (rulings 1 to 4): TestAR80_5's world, r1's text
// long. At T r1 reads `held`, its text, {not-started, 115, T, 1}, r2
// `held`, `behind r1`, {queued, null, T, 0}, the night still running and
// `next` leading `the band holds r1 r2 since 01:00`; PrintStatus's lines
// under 100 columns, r1's `held`. At +60 s r1's asks read 2 (the re-ask
// writes no line); at +120 s r1 runs, its band null and its reason "",
// and `next` leads `the band holds r2 since 01:00`.
func TestAR81_1_StatusHeld(t *testing.T) {
	peak := 115.0
	text := "no room: " + strings.Repeat("the five-hour band projects past its ceiling; ", 3)
	ns := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started", Mode: "observe", Text: text, Peak: &peak}}
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {ns, ns, {BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
	w, q, o, sp, clock := ar80World(t, band, "r1", "r2")
	o.Parallel = 2
	var free atomic.Bool // r1's child says nothing until its read is done
	sp.wait["r1"] = free.Load
	res := ar71Run(w, q, o, sp)
	T := ar65T.Format(time.RFC3339)

	st, by := ar81Rows(t, o, "both rows held", func(_ night.Status, by map[string]night.RowStatus) bool {
		return by["r1"].Band != nil && by["r2"].Band != nil
	})
	if r := by["r1"]; r.State != "held" || r.Reason != text || ar81Band(r.Band) != "{not-started 115 "+T+" 1}" {
		t.Fatalf("r1 at T: %s, %q, %s", r.State, r.Reason, ar81Band(r.Band))
	}
	if r := by["r2"]; r.State != "held" || r.Reason != "behind r1" || ar81Band(r.Band) != "{queued null "+T+" 0}" {
		t.Fatalf("r2 at T: %s, %q, %s", r.State, r.Reason, ar81Band(r.Band))
	}
	if st.State != "running" || !strings.HasPrefix(st.Next, "the band holds r1 r2 since 01:00; ") {
		t.Fatalf("the night %s, next %q", st.State, st.Next)
	}
	var b bytes.Buffer
	pr := st
	pr.Root = "" // the test's temp root is outside HOME: the first line's width is AR37's
	night.PrintStatus(&b, pr)
	r1Held := false
	for _, l := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if n := len([]rune(l)); n >= 100 {
			t.Fatalf("%d columns: %s", n, l)
		}
		if f := strings.Fields(l); len(f) > 1 && f[0] == "r1" && strings.HasPrefix(f[1], "held") {
			r1Held = true
		}
	}
	if !r1Held || !strings.Contains(b.String(), "next stop: the band holds r1 r2 since 01:00") {
		t.Fatalf("PrintStatus:\n%s", b.String())
	}

	ar80Ask(t, band, 1, clock, 60*time.Second)
	ar81Rows(t, o, "r1's second ask", func(_ night.Status, by map[string]night.RowStatus) bool {
		return by["r1"].Band != nil && by["r1"].Band.Asks == 2
	})
	if got := ar80Band(t, w.ledger.Written(), "r1"); got != "held not-started" {
		t.Fatalf("r1's band lines at +60 s: %s", got)
	}

	ar80Ask(t, band, 2, clock, 120*time.Second)
	st, by = ar81Rows(t, o, "r1 admitted", func(_ night.Status, by map[string]night.RowStatus) bool {
		return by["r1"].Band == nil && by["r1"].State == "running"
	})
	if r := by["r1"]; r.Reason != "" || ar81Band(by["r2"].Band) != "{queued null "+T+" 0}" || !strings.HasPrefix(st.Next, "the band holds r2 since 01:00; ") {
		t.Fatalf("at +120 s: r1 %q, r2 %s, next %q", r.Reason, ar81Band(by["r2"].Band), st.Next)
	}

	free.Store(true)
	ar69To(clock, ar65T.Add(180*time.Second))
	ar65End(t, res)
	st, by = ar81Rows(t, o, "the night's end", func(st night.Status, _ map[string]night.RowStatus) bool { return st.State != "running" })
	if by["r1"].Band != nil || by["r2"].Band != nil || strings.Contains(st.Next, "the band holds") {
		t.Fatalf("at the end: r1 %s, r2 %s, next %q", ar81Band(by["r1"].Band), ar81Band(by["r2"].Band), st.Next)
	}
}
