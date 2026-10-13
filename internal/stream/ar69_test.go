package stream

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAR69_1_NamedReset (AR69 ruling 1): Named is the reset the stop named
// before the bound: an event's resetsAt in the hit's zone, even when not
// after the hit (Reset zero then); with no event's, Reset. The exit line
// is unchanged: four keys, no Named.
func TestAR69_1_NamedReset(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	hit := time.Date(2026, 9, 21, 22, 0, 0, 0, la)
	event := func(at time.Time) string {
		b, _ := json.Marshal(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{
			"status": "rejected", "rateLimitType": "five_hour", "resetsAt": at.Unix()}})
		return string(b) + "\n"
	}
	text := `{"type":"result","subtype":"success","is_error":true,"result":"You've hit your session limit · resets 1:30am (America/Los_Angeles)"}` + "\n"
	for _, c := range []struct {
		name         string
		src          string
		reset, named time.Time
	}{
		{"before the hit", event(hit.Add(-time.Minute)), time.Time{}, hit.Add(-time.Minute)},
		{"after the hit", event(hit.Add(time.Hour)), hit.Add(time.Hour), hit.Add(time.Hour)},
		{"a text alone", text, time.Date(2026, 9, 22, 1, 30, 0, 0, la), time.Date(2026, 9, 22, 1, 30, 0, 0, la)},
	} {
		tl, err := Parse(strings.NewReader(c.src), nil)
		if err != nil {
			t.Fatal(err)
		}
		l := tl.Limit(hit)
		if !l.Reset.Equal(c.reset) || !l.Named.Equal(c.named) {
			t.Errorf("%s: Reset %s, Named %s; want %s, %s", c.name, l.Reset, l.Named, c.reset, c.named)
		}
		if l.Named.Location().String() != hit.Location().String() {
			t.Errorf("%s: Named in %s, want the hit's zone %s", c.name, l.Named.Location(), hit.Location())
		}
		if line := l.Line(); len(line) != 4 {
			t.Errorf("%s: the exit line %v, want its four keys", c.name, line)
		}
	}
}
