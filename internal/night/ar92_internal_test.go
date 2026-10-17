package night

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// TestAR92_1_ShelveFor (AR92 ruling 1; the relay's entry 12): a model's
// limit with a reset whose wake is at or past --until, or with no --until
// over 6 h away, no longer stops the night: it shelves the scope's rows,
// the detail the stop's text, the Limit's scope, the wake the reset plus
// 2 minutes. A reset in reach is a plain pause; the twelfth pause, a
// window past --until, a weekly limit and a model's with no reset stop.
func TestAR92_1_ShelveFor(t *testing.T) {
	now := time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return now.Add(d) }
	opus := func(reset time.Time) stream.Limit {
		return stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindModel, Scope: "opus", Reset: reset, Named: reset, Source: "event"}
	}
	far := opus(at(72 * time.Hour))
	wake := at(72*time.Hour + WakeSlack)
	until := at(4 * time.Hour)
	p := pauseFor(pauseIn{Limit: far, Now: now, Until: until})
	if want := "the wake " + wake.Format(time.RFC3339) + " is at or past --until " + until.Format(time.RFC3339); p.Stop || !p.Shelve ||
		p.Detail != want || p.Scope != "opus" || p.Source != "event" || !p.Wake.Equal(wake) {
		t.Errorf("opus 72 h on, --until 4 h on: %+v; want a shelve, %q, scope opus, event, wake %s", p, want, wake)
	}
	p = pauseFor(pauseIn{Limit: far, Now: now})
	if want := "the wake " + wake.Format(time.RFC3339) + " is over 6h away, and no --until"; p.Stop || !p.Shelve || p.Detail != want || p.Scope != "opus" || !p.Wake.Equal(wake) {
		t.Errorf("opus 72 h on, no --until: %+v; want a shelve, %q, scope opus, wake %s", p, want, wake)
	}
	p = pauseFor(pauseIn{Limit: opus(at(time.Hour)), Now: now, Until: until})
	if p.Stop || p.Shelve || p.Detail != "" || p.Scope != "opus" || !p.Wake.Equal(at(time.Hour+WakeSlack)) {
		t.Errorf("opus 1 h on: %+v; want a plain pause, scope opus, wake %s", p, at(time.Hour+WakeSlack))
	}
	for _, c := range []struct {
		name   string
		in     pauseIn
		detail string
	}{
		{"count 12, opus 72 h on", pauseIn{Limit: far, Now: now, Until: until, Count: MaxPauses}, "12 pauses tonight, the most a night takes"},
		{"a window past --until", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWindow, Reset: at(5 * time.Hour), Source: "event"}, Now: now, Until: until},
			"the wake " + at(5*time.Hour+WakeSlack).Format(time.RFC3339) + " is at or past --until " + until.Format(time.RFC3339)},
		{"a weekly", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWeekly, Reset: at(72 * time.Hour), Source: "event"}, Now: now, Until: until}, "a weekly limit"},
		{"a model's with no reset", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindModel, Scope: "opus"}, Now: now, Until: until}, "the opus limit names no reset"},
	} {
		if p := pauseFor(c.in); !p.Stop || p.Shelve || p.Detail != c.detail {
			t.Errorf("%s: %+v; want a stop, no shelve, %q", c.name, p, c.detail)
		}
	}
}
