package night

import (
	"reflect"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// TestAR65_1_PauseFor (usage-limit-resume r1 §2; AR65 ruling 1): pauseFor
// stops the night at once, naming why, while draining, on a weekly limit,
// on a model's limit with no reset, when the wake is at or past --until,
// and with no --until when the wake is over 6 h away; else it pauses, the
// wake the reset plus 2 minutes (the Limit's source), or with no reset
// the hit plus 30 minutes (source fallback).
func TestAR65_1_PauseFor(t *testing.T) {
	now := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return now.Add(d) }
	window := stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWindow, Reset: at(time.Hour), Source: "event"}
	for _, c := range []struct {
		name   string
		in     pauseIn
		stop   bool
		detail string
		source string
		wake   time.Time
	}{
		{"draining", pauseIn{Limit: window, Now: now, Draining: true}, true, "the night is draining", "event", at(time.Hour + 2*time.Minute)},
		{"weekly", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWeekly, Reset: at(48 * time.Hour), Source: "text"}, Now: now},
			true, "a weekly limit", "text", at(48*time.Hour + 2*time.Minute)},
		{"a model's, no reset", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindModel, Scope: "fable"}, Now: now},
			true, "the fable limit names no reset", "", time.Time{}},
		{"unknown, no reset: the fallback", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindUnknown}, Now: now},
			false, "", "fallback", at(30 * time.Minute)},
		{"window, no reset: the fallback", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWindow}, Now: now},
			false, "", "fallback", at(30 * time.Minute)},
		{"a model's with a reset", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindModel, Scope: "fable", Reset: at(time.Hour), Source: "text"}, Now: now},
			false, "", "text", at(time.Hour + 2*time.Minute)},
		{"the wake a minute before --until", pauseIn{Limit: window, Now: now, Until: at(time.Hour + 3*time.Minute)},
			false, "", "event", at(time.Hour + 2*time.Minute)},
		{"the wake at --until", pauseIn{Limit: window, Now: now, Until: at(time.Hour + 2*time.Minute)},
			true, "the wake 2026-10-11T11:02:00Z is at or past --until 2026-10-11T11:02:00Z", "event", at(time.Hour + 2*time.Minute)},
		{"no --until, the wake 6 h on", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWindow, Reset: at(6*time.Hour - 2*time.Minute), Source: "event"}, Now: now},
			false, "", "event", at(6 * time.Hour)},
		{"no --until, the wake 6 h 1 min on", pauseIn{Limit: stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWindow, Reset: at(6*time.Hour - time.Minute), Source: "event"}, Now: now},
			true, "the wake 2026-10-11T16:01:00Z is over 6h away, and no --until", "event", at(6*time.Hour + time.Minute)},
	} {
		got := pauseFor(c.in)
		if got.Stop != c.stop || got.Detail != c.detail || got.Source != c.source || !got.Wake.Equal(c.wake) {
			t.Errorf("%s: %+v; want stop %v, detail %q, source %q, wake %s", c.name, got, c.stop, c.detail, c.source, c.wake)
		}
	}
}

// TestAR65_2_PickSkipsPaused (usage-limit-resume r1 §2; AR65 ruling 2):
// PickIn never picks a paused row, with a slot free or not; the row based
// on it waits and is never doomed, so nothing reads blocked.
func TestAR65_2_PickSkipsPaused(t *testing.T) {
	for _, par := range []int{1, 2} {
		next, blocked, err := PickIn(q3(), sittings(map[string]relay.State{"a": relay.Paused, "b": relay.Waiting, "c": relay.Ready}), nil, par, PickOptions{})
		if err != nil || !reflect.DeepEqual(next, []string{"c"}) || blocked {
			t.Errorf("parallel %d: got %v %v %v, want [c], not blocked", par, next, blocked, err)
		}
	}
	next, blocked, _ := PickIn(q3(), sittings(map[string]relay.State{"a": relay.Paused, "b": relay.Waiting}), nil, 2, PickOptions{})
	if next != nil || blocked {
		t.Errorf("a paused base: got %v %v, want nothing, not blocked (its dependent waits)", next, blocked)
	}
}
