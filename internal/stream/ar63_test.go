package stream

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The usage limit's facts (AR63, usage-limit-resume-r1 §2): the kind, the
// model's scope and the reset a usage-limit stop names.

// TestAR63_1_ResetCells: every cell of testdata/reset-cells.tsv, read as an
// is_error result through Parse, stops usage-limit and names its kind, its
// reset (the measured forms only, the first instant after the hit, at most
// 8 days on; RFC3339 in the reset's zone), Source `text` or "" and its scope.
func TestAR63_1_ResetCells(t *testing.T) {
	b, err := os.ReadFile("testdata/reset-cells.tsv")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if lines[0] != "text\thit\tkind\treset\tscope" {
		t.Fatalf("header %q", lines[0])
	}
	for i, l := range lines[1:] {
		f := strings.Split(l, "\t")
		if len(f) != 5 {
			t.Fatalf("cell %d %q has %d fields", i+1, l, len(f))
		}
		text, kind, reset, scope := f[0], f[2], f[3], f[4]
		hit, err := time.Parse(time.RFC3339, f[1])
		if err != nil {
			t.Fatalf("cell %d hit: %v", i+1, err)
		}
		ev, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": true, "result": text})
		tl, err := Parse(strings.NewReader(string(ev)+"\n"), nil)
		if err != nil || tl.Account != relay.UsageLimit {
			t.Errorf("cell %d %q: account %q, %v; want usage-limit", i+1, text, tl.Account, err)
			continue
		}
		lm := tl.Limit(hit)
		got := ""
		if !lm.Reset.IsZero() {
			got = lm.Reset.Format(time.RFC3339)
		}
		src := ""
		if reset != "" {
			src = "text"
		}
		if lm.Reason != relay.UsageLimit || lm.Kind != kind || got != reset || lm.Source != src || lm.Scope != scope {
			t.Errorf("cell %d %q at %s: %+v (reset %q); want %s, reset %q, source %q, scope %q",
				i+1, text, f[1], lm, got, kind, reset, src, scope)
		}
	}
}

// TestAR63_2_EventLimit: a refused event's rateLimitType names the kind and
// its resetsAt the reset (Source `event`) when after the hit and at most 8
// days on; a type or a usable resetsAt the event lacks comes from a later
// usage-limit result; an auth stop names no kind; no stop is the zero Limit.
func TestAR63_2_EventLimit(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	hit := time.Date(2026, 9, 21, 22, 0, 0, 0, la)
	event := func(typ string, at time.Time) string {
		info := map[string]any{"status": "rejected"}
		if typ != "" {
			info["rateLimitType"] = typ
		}
		if !at.IsZero() {
			info["resetsAt"] = at.Unix()
		}
		b, _ := json.Marshal(map[string]any{"type": "rate_limit_event", "rate_limit_info": info})
		return string(b) + "\n"
	}
	result := func(text string) string {
		b, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": true, "result": text})
		return string(b) + "\n"
	}
	session := "You've hit your session limit · resets 1:30am (America/Los_Angeles)"
	for _, c := range []struct {
		name, src string
		want      Limit
	}{
		{"five_hour", event("five_hour", hit.Add(time.Hour)), Limit{Reason: relay.UsageLimit, Kind: KindWindow, Reset: hit.Add(time.Hour), Source: "event"}},
		{"seven_day", event("seven_day", hit.Add(3*24*time.Hour)), Limit{Reason: relay.UsageLimit, Kind: KindWeekly, Reset: hit.Add(3 * 24 * time.Hour), Source: "event"}},
		{"past", event("five_hour", hit.Add(-time.Minute)), Limit{Reason: relay.UsageLimit, Kind: KindWindow, Reset: time.Time{}, Source: ""}},
		{"nine days", event("seven_day", hit.Add(9*24*time.Hour)), Limit{Reason: relay.UsageLimit, Kind: KindWeekly, Reset: time.Time{}, Source: ""}},
		{"no type", event("", time.Time{}) + result(session), Limit{Reason: relay.UsageLimit, Kind: KindWindow, Reset: time.Date(2026, 9, 22, 1, 30, 0, 0, la), Source: "text"}},
		{"unusable resetsAt", event("five_hour", hit.Add(-time.Hour)) + result(session), Limit{Reason: relay.UsageLimit, Kind: KindWindow, Reset: time.Date(2026, 9, 22, 1, 30, 0, 0, la), Source: "text"}},
		{"auth", result("Invalid API key · Please run /login") + event("five_hour", hit.Add(time.Hour)), Limit{Reason: relay.Auth}},
		{"no stop", `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1790974800}}` + "\n" + `{"type":"result","subtype":"success","is_error":false,"result":"done"}` + "\n", Limit{}},
	} {
		tl, err := Parse(strings.NewReader(c.src), nil)
		if err != nil {
			t.Fatal(err)
		}
		got := tl.Limit(hit)
		if got.Reason != c.want.Reason || got.Kind != c.want.Kind || !got.Reset.Equal(c.want.Reset) || got.Source != c.want.Source || got.Scope != c.want.Scope {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
		line := got.Line()
		at := ""
		if !c.want.Reset.IsZero() {
			at = c.want.Reset.In(hit.Location()).Format(time.RFC3339)
		}
		if want := fmt.Sprint(map[string]any{"kind": c.want.Kind, "resets_at": at, "source": c.want.Source, "scope": c.want.Scope}); fmt.Sprint(line) != want {
			t.Errorf("%s: Line() %v, want %s", c.name, line, want)
		}
	}
}

// TestAR63Fix_1_FallBack (round one's F1): a reset is the first instant
// after the hit whose wall clock in the zone reads the clause. On the
// fall-back day 1:30am comes twice, and the second pass, 45 minutes on,
// is the reset; before the first pass, the first. On the spring-forward day
// 2:30am never comes: a time reads the next day's 2:30am, the first that
// does, and a date naming that day reads none.
func TestAR63Fix_1_FallBack(t *testing.T) {
	const at130 = "You've hit your session limit · resets 1:30am (America/Los_Angeles)"
	for _, c := range []struct{ name, text, hit, want string }{
		{"fall-back, second pass", at130, "2026-11-01T01:45:00-07:00", "2026-11-01T01:30:00-08:00"},
		{"fall-back, first pass", at130, "2026-11-01T01:10:00-07:00", "2026-11-01T01:30:00-07:00"},
		{"fall-back, a date", "You've hit your weekly limit · resets Nov 1 at 1:30am (America/Los_Angeles)", "2026-11-01T01:45:00-07:00", "2026-11-01T01:30:00-08:00"},
		{"spring-forward, a time", "You've hit your session limit · resets 2:30am (America/Los_Angeles)", "2027-03-14T01:00:00-08:00", "2027-03-15T02:30:00-07:00"},
		{"spring-forward, a date", "You've hit your weekly limit · resets Mar 14 at 2:30am (America/Los_Angeles)", "2027-03-12T12:00:00-08:00", ""},
	} {
		hit, err := time.Parse(time.RFC3339, c.hit)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := ResetIn(c.text, hit)
		got := ""
		if ok {
			got = r.Format(time.RFC3339)
		}
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s: ResetIn at %s = %q, %v; want %q", c.name, c.hit, got, ok, c.want)
		}
	}
}

// TestAR63Fix_2_ModelWords (round one's notes): `reached your <word> limit`
// is a model's limit only for a model word, as `hit your <word> limit` is;
// any other word still stops usage-limit, its kind unknown. The exit
// line's scope is capped at scopeCap bytes.
func TestAR63Fix_2_ModelWords(t *testing.T) {
	hit := time.Date(2026, 10, 7, 19, 57, 0, 0, time.UTC)
	for _, c := range []struct{ text, kind, scope string }{
		{"You've reached your usage limit", KindUnknown, ""},
		{"You've reached your context limit", KindUnknown, ""},
		{"You've reached your FABLE limit.", KindModel, "fable"},
		{"You've hit your Sonnet limit", KindModel, "sonnet"},
		{"You've reached your haiku limit", KindModel, "haiku"},
		{"You have reached your " + strings.Repeat("x", 300) + " limit", KindUnknown, ""},
	} {
		ev, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": true, "result": c.text})
		tl, _ := Parse(strings.NewReader(string(ev)+"\n"), nil)
		if lm := tl.Limit(hit); lm.Reason != relay.UsageLimit || lm.Kind != c.kind || lm.Scope != c.scope {
			t.Errorf("%.40q: %+v; want usage-limit, %s, scope %q", c.text, lm, c.kind, c.scope)
		}
	}
	if s, _ := (Limit{Reason: relay.UsageLimit, Kind: KindModel, Scope: strings.Repeat("x", 300)}).Line()["scope"].(string); len(s) > 32 {
		t.Errorf("Line's scope is %d bytes, want at most 32", len(s))
	}
}
