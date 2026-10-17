package night_test

import (
	"bytes"
	"encoding/json"
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

// TestAR81_2_ReportHolds (ruling 5): r1 held 120 s, then admitted; r2 held
// queued, then skipped by a stop 30 s on, its sitting line with no band
// key, as AR80 writes a part that never asked; r3
// `unchecked` at its first ask, no hold; r4 held (and denied) and open.
// The report's band reads {3, 150} and its markdown `band: 3 holds, 2m30s
// waited`; a night with no band line reads {0, 0} and no band line.
func TestAR81_2_ReportHolds(t *testing.T) {
	const n = "2026-10-14-1"
	l := func(event, row string, kv ...any) map[string]any {
		m := map[string]any{"event": event, "night": n, "row": row}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	band := func(asks, wait int, verdict string) map[string]any {
		return map[string]any{"asks": asks, "wait_s": wait, "verdict": verdict}
	}
	open := map[string]any{"event": "night-open", "night": n, "root": "/r"}
	lines := []map[string]any{open,
		l("band", "r1", "part", 1, "state", "held", "verdict", "not-started", "peak", 115.0, "text", "no room"),
		l("band", "r2", "part", 1, "state", "held", "verdict", "queued", "peak", nil, "text", "behind r1", "ts", "2026-10-14T01:00:00Z"),
		l("band", "r1", "part", 1, "state", "admitted", "asks", 3, "wait_s", 120),
		l("spawn", "r1", "band", map[string]any{"verdict": "admitted", "peak": nil}),
		l("band", "r3", "part", 1, "state", "unchecked", "error", "exit 3"),
		l("spawn", "r3", "band", map[string]any{"verdict": "unchecked", "peak": nil}),
		l("band", "r4", "part", 1, "state", "held", "verdict", "not-started", "peak", 90.0, "text", "no room"),
		l("band", "r4", "part", 1, "state", "held", "verdict", "denied", "peak", 130.0, "text", "denied"),
		l("sitting", "r1", "state", "green", "band", band(3, 120, "admitted")),
		l("sitting", "r2", "state", "skipped", "reason", "owner-stop", "ts", "2026-10-14T01:00:30Z"), // no ask: no band key (AR80)
		l("sitting", "r3", "state", "green", "band", band(1, 0, "unchecked")),
	}
	r := night.BuildReport(lines, n)
	if b, _ := json.Marshal(r.Band); string(b) != `{"holds":3,"wait_s":150}` {
		t.Fatalf("the report's band %s", b)
	}
	md := string(night.Markdown(r, "/r", "/h"))
	if !strings.Contains(md, "\nband: 3 holds, 2m30s waited\n") {
		t.Fatalf("markdown:\n%s", md)
	}
	if i, j := strings.Index(md, "## Burn"), strings.Index(md, "band: 3 holds"); i < 0 || j < i || j > strings.Index(md, "## Handovers") {
		t.Fatalf("the band line not after the sittings' burn:\n%s", md)
	}

	none := night.BuildReport([]map[string]any{open, l("sitting", "r1", "state", "green")}, n)
	if b, _ := json.Marshal(none.Band); string(b) != `{"holds":0,"wait_s":0}` || strings.Contains(string(night.Markdown(none, "/r", "/h")), "band:") {
		t.Fatalf("a night with no band line: %s\n%s", b, night.Markdown(none, "/r", "/h"))
	}
}

// TestAR81_4_QueuedBehindNewHead (the fix round's finding 1): three rows,
// r1 held then admitted at +60 s; r2, the new head, asks at +120 s and
// holds; r3, queued behind r1 at T, now reads `behind r2`, still queued
// with no ask, its since T.
func TestAR81_4_QueuedBehindNewHead(t *testing.T) { ar81QueuedBehindNewHead(t, 0) }

// ar81QueuedBehindNewHead is TestAR81_4_QueuedBehindNewHead's night, the
// band answering each ask lag after its record (AR93), r1's and r2's
// children silent until r3 has asked.
func ar81QueuedBehindNewHead(t *testing.T, lag time.Duration) {
	ad := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "admitted"}}
	ns := func(text string) effects.FakeAnswer {
		return effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started", Text: text}}
	}
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {ns("r1 no room"), ad}, "r2": {ns("r2 no room"), ad}}}
	w, q, o, sp, clock := ar80World(t, band, "r1", "r2", "r3")
	w.fx.Band = &ar93LateBand{FakeBand: band, lag: lag}
	// A failed sitting stops the night: r1's and r2's children hold until
	// r3 has asked (AR93 fix round 1, F1).
	r3Asked := func() bool { return strings.Contains(ar80Asks(band), "r3 ") }
	sp.wait["r1"], sp.wait["r2"] = r3Asked, r3Asked
	o.Parallel = 3
	res := ar71Run(w, q, o, sp)
	T := ar65T.Format(time.RFC3339)
	admitted := func(row string) { // its leave read the clock: the step may follow (AR93 ruling 3)
		ar65Until(t, row+"'s admitted line", func() bool {
			for _, l := range rowLines(w.ledger.Written(), "band", row) {
				if l["state"] == "admitted" {
					return true
				}
			}
			return false
		})
	}
	ar65Until(t, "r3's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r3")) == 1 })
	ar80Ask(t, band, 1, clock, 60*time.Second)
	admitted("r1")
	ar80Ask(t, band, 2, clock, 120*time.Second)
	_, by := ar81Rows(t, o, "r2's ask", func(_ night.Status, by map[string]night.RowStatus) bool {
		return by["r2"].Band != nil && by["r2"].Band.Asks == 1
	})
	if r := by["r2"]; r.State != "held" || r.Reason != "r2 no room" || by["r1"].Band != nil {
		t.Fatalf("r2 at +120 s: %s, %q, %s; r1 %s", r.State, r.Reason, ar81Band(r.Band), ar81Band(by["r1"].Band))
	}
	if r := by["r3"]; r.State != "held" || r.Reason != "behind r2" || ar81Band(r.Band) != "{queued null "+T+" 0}" {
		t.Fatalf("r3 at +120 s: %s, %q, %s", r.State, r.Reason, ar81Band(r.Band))
	}
	ar80Ask(t, band, 3, clock, 180*time.Second)
	admitted("r2")
	ar80Ask(t, band, 4, clock, 240*time.Second)
	ar65End(t, res)
	if got := ar80Asks(band); got != "r1 +0s, r1 +60s, r2 +120s, r2 +180s, r3 +240s" {
		t.Fatalf("asks %s", got)
	}
}

// TestAR81_5_QueuedCutWait (the fix round's finding 2): r1 held and r2
// queued at T; the owner's stop at +120 s writes r1's sitting line with
// its band (1 ask, 120 s) and r2's with none, as AR80 writes a part that
// never asked: the report reads {2, 240}. A kill at +60 s reads {2, 120}.
func TestAR81_5_QueuedCutWait(t *testing.T) {
	const n = "2026-10-14-1"
	at := func(s int) string { return ar65T.Add(time.Duration(s) * time.Second).Format(time.RFC3339) }
	for _, c := range []struct {
		state, reason string
		cut           int
		want          string
	}{{"skipped", "owner-stop", 120, `{"holds":2,"wait_s":240}`}, {"failed", "owner-kill", 60, `{"holds":2,"wait_s":120}`}} {
		lines := []map[string]any{{"event": "night-open", "night": n, "root": "/r", "ts": at(0)},
			{"event": "band", "night": n, "row": "r1", "part": 1, "state": "held", "verdict": "not-started", "peak": nil, "text": "no room", "ts": at(0)},
			{"event": "band", "night": n, "row": "r2", "part": 1, "state": "held", "verdict": "queued", "peak": nil, "text": "behind r1", "ts": at(0)},
			{"event": "sitting", "night": n, "row": "r1", "state": c.state, "reason": c.reason, "ts": at(c.cut),
				"band": map[string]any{"asks": 1, "wait_s": c.cut, "verdict": "not-started"}},
			{"event": "sitting", "night": n, "row": "r2", "state": c.state, "reason": c.reason, "ts": at(c.cut)},
			{"event": "stop", "night": n, "state": "stopped", "reason": c.reason, "ts": at(c.cut)},
		}
		if b, _ := json.Marshal(night.BuildReport(lines, n).Band); string(b) != c.want {
			t.Fatalf("%s at +%d s: the report's band %s, want %s", c.reason, c.cut, b, c.want)
		}
	}
}

// TestAR81_6_NextClipped (the fix round's finding 3): six held rows with
// long ids, the first held and five queued behind it: `next`'s band clause
// names the first rows that fit and `+N`, its printed line under 100
// columns.
func TestAR81_6_NextClipped(t *testing.T) {
	var ids []string
	for i := 1; i <= 6; i++ {
		ids = append(ids, fmt.Sprintf("ar81-held-row-number-%d", i))
	}
	ns := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started", Text: "no room"}}
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{ids[0]: {ns, {BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
	w, q, o, sp, clock := ar80World(t, band, ids...)
	o.Parallel = 6
	res := ar71Run(w, q, o, sp)
	st, _ := ar81Rows(t, o, "six rows held", func(_ night.Status, by map[string]night.RowStatus) bool {
		for _, id := range ids {
			if by[id].Band == nil {
				return false
			}
		}
		return true
	})
	st.Root = ""
	var b bytes.Buffer
	night.PrintStatus(&b, st)
	want := "next stop: the band holds " + ids[0] + " " + ids[1] + " +4 since 01:00"
	found := false
	for _, l := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if n := len([]rune(l)); n >= 100 {
			t.Fatalf("%d columns: %s", n, l)
		}
		found = found || l == want
	}
	if !found {
		t.Fatalf("no line %q:\n%s", want, b.String())
	}
	for i := 1; i <= 6; i++ { // the head admitted at +60 s, each queued row one poll after the last
		ar80Ask(t, band, i, clock, time.Duration(60*i)*time.Second)
	}
	ar65End(t, res)
}
