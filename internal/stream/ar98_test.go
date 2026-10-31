package stream

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAR98_3_FableTypeIsModelFable (AR98 ruling 5): the event's type
// `seven_day_overage_included` is a model's limit scoped to `fable`, mode
// free, its reset the event's; the types beside it read as before.
func TestAR98_3_FableTypeIsModelFable(t *testing.T) {
	hit := time.Date(2026, 10, 30, 1, 0, 0, 0, time.UTC)
	reset := hit.Add(time.Hour)
	event := func(typ string) string {
		b, _ := json.Marshal(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{
			"status": "rejected", "rateLimitType": typ, "resetsAt": reset.Unix()}})
		return string(b) + "\n"
	}
	for _, c := range []struct{ typ, kind, scope string }{
		{"seven_day_overage_included", KindModel, "fable"},
		{"seven_day_opus", KindModel, "opus"},
		{"seven_day", KindWeekly, ""},
		{"five_hour", KindWindow, ""},
	} {
		tally, err := Parse(strings.NewReader(event(c.typ)), nil)
		if err != nil {
			t.Fatalf("%s: %v", c.typ, err)
		}
		l := tally.Limit(hit)
		if l.Kind != c.kind || l.Scope != c.scope || l.Source != "event" || !l.Reset.Equal(reset) {
			t.Fatalf("%s: the limit %+v, want %s %q, the event's reset", c.typ, l, c.kind, c.scope)
		}
	}
}
