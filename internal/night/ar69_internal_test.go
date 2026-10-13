package night

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// TestAR69_2_PauseGuards (AR69 ruling 2): after a pause, a limit with no
// reset while the last wake was no reset's (the probe) stops; a named
// reset not after the one waited out stops, even when the bound dropped
// it; the twelfth pause is the last. Each stop keeps its wake and source.
// With Count 0 the record's rules hold (TestAR65_1's cases).
func TestAR69_2_PauseGuards(t *testing.T) {
	now := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return now.Add(d) }
	r := at(-2 * time.Minute) // the reset waited out
	window := func(reset, named time.Time) stream.Limit {
		l := stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWindow, Reset: reset, Named: named}
		if !reset.IsZero() {
			l.Source = "event"
		}
		return l
	}
	probe := "refused again after the 30-minute wait, with no reset"
	same := "the reset 2026-10-11T09:58:00Z is not after the one waited out, 2026-10-11T09:58:00Z"
	most := "12 pauses tonight, the most a night takes"
	for _, c := range []struct {
		name   string
		in     pauseIn
		stop   bool
		detail string
		source string
		wake   time.Time
	}{
		{"count 1, no wait: no reset, the probe", pauseIn{Limit: window(time.Time{}, time.Time{}), Now: now, Count: 1},
			true, probe, "fallback", at(30 * time.Minute)},
		{"count 1, no wait, the probe passed: no reset, the fallback", pauseIn{Limit: window(time.Time{}, time.Time{}), Now: now, Count: 1, ProbePassed: true},
			false, "", "fallback", at(30 * time.Minute)},
		{"count 12, the probe passed: no reset, the cap", pauseIn{Limit: window(time.Time{}, time.Time{}), Now: now, Count: 12, ProbePassed: true},
			true, most, "fallback", at(30 * time.Minute)},
		{"count 1, no wait: a reset pauses", pauseIn{Limit: window(at(time.Hour), at(time.Hour)), Now: now, Count: 1},
			false, "", "event", at(time.Hour + 2*time.Minute)},
		{"waited R: R named, the bound dropped it", pauseIn{Limit: window(time.Time{}, r), Now: now, Count: 1, Waited: r},
			true, same, "fallback", at(30 * time.Minute)},
		{"waited R: R+1h pauses", pauseIn{Limit: window(r.Add(time.Hour), r.Add(time.Hour)), Now: now, Count: 1, Waited: r},
			false, "", "event", r.Add(time.Hour + 2*time.Minute)},
		{"waited R: no reset named, the fallback", pauseIn{Limit: window(time.Time{}, time.Time{}), Now: now, Count: 1, Waited: r},
			false, "", "fallback", at(30 * time.Minute)},
		{"count 12 stops", pauseIn{Limit: window(at(time.Hour), at(time.Hour)), Now: now, Count: 12, Waited: r},
			true, most, "event", at(time.Hour + 2*time.Minute)},
		{"count 11 pauses", pauseIn{Limit: window(at(time.Hour), at(time.Hour)), Now: now, Count: 11, Waited: r},
			false, "", "event", at(time.Hour + 2*time.Minute)},
		{"count 0, no reset: the fallback", pauseIn{Limit: window(time.Time{}, time.Time{}), Now: now},
			false, "", "fallback", at(30 * time.Minute)},
		{"count 12, weekly: the weekly stop first", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWeekly}, Now: now, Count: 12},
			true, "a weekly limit", "fallback", at(30 * time.Minute)},
		{"count 1, the wake past --until: the probe first", pauseIn{Limit: window(time.Time{}, time.Time{}), Now: now, Count: 1, Until: at(10 * time.Minute)},
			true, probe, "fallback", at(30 * time.Minute)},
	} {
		got := pauseFor(c.in)
		if got.Stop != c.stop || got.Detail != c.detail || got.Source != c.source || !got.Wake.Equal(c.wake) {
			t.Errorf("%s: %+v; want stop %v, detail %q, source %q, wake %s", c.name, got, c.stop, c.detail, c.source, c.wake)
		}
	}
	if MaxPauses != 12 {
		t.Fatalf("MaxPauses %d, want 12", MaxPauses)
	}
}

// TestAR69_3_FloorCapped (AR69 ruling 3): the continuation's floor is 10
// turns and $5, or the row's own turns and usd when less.
func TestAR69_3_FloorCapped(t *testing.T) {
	for _, c := range []struct {
		name                  string
		turns, usd            int
		parts                 []map[string]any
		wantTurns, wantDollar int
	}{
		{"8 turns, $3, nothing spent", 8, 3, nil, 8, 3},
		{"8 turns, $3, all spent", 8, 3, []map[string]any{{"event": "sitting", "row": "r1", "turns": 8, "cost_usd": 3.0, "cost_read": true}}, 8, 3},
		{"100 turns, $15, 95 and $14 spent", 100, 15, []map[string]any{{"event": "sitting", "row": "r1", "turns": 95, "cost_usd": 14.0, "cost_read": true}}, 10, 5},
	} {
		if turns, usd := remainder(c.turns, c.usd, c.parts); turns != c.wantTurns || usd != c.wantDollar {
			t.Errorf("%s: remainder %d turns $%d, want %d $%d", c.name, turns, usd, c.wantTurns, c.wantDollar)
		}
	}
}
