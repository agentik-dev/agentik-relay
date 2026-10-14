package stream

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The kinds a refused event names (AR82): the Agent SDK references
// document five rateLimitType values; each maps to a kind and a scope.

// TestAR82_1_EventKinds (AR82 rulings 1, 4): a refused event's
// rateLimitType, read through Parse at the hit: seven_day_opus and
// seven_day_sonnet are a model's limit scoped to their family, overage its
// own kind, five_hour and seven_day as before; resetsAt gives the reset
// whatever the kind; an undocumented type falls to the text's kind. The
// kinds are spelled as the exit line spells them.
func TestAR82_1_EventKinds(t *testing.T) {
	hit := time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)
	reset := hit.Add(time.Hour)
	event := func(typ string, at time.Time) string {
		info := map[string]any{"status": "rejected", "rateLimitType": typ}
		if !at.IsZero() {
			info["resetsAt"] = at.Unix()
		}
		b, _ := json.Marshal(map[string]any{"type": "rate_limit_event", "rate_limit_info": info})
		return string(b) + "\n"
	}
	session, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": true,
		"result": "You've hit your session limit · resets 3am (UTC)"})
	for _, c := range []struct {
		name, src           string
		kind, scope, source string
		reset               time.Time
	}{
		{"seven_day_opus", event("seven_day_opus", reset), "model", "opus", "event", reset},
		{"seven_day_sonnet", event("seven_day_sonnet", reset), "model", "sonnet", "event", reset},
		{"overage", event("overage", reset), "overage", "", "event", reset},
		{"five_hour", event("five_hour", reset), "window", "", "event", reset},
		{"seven_day", event("seven_day", reset), "weekly", "", "event", reset},
		{"seven_day_opus, no resetsAt", event("seven_day_opus", time.Time{}), "model", "opus", "", time.Time{}},
		{"undocumented", event("seven_day_haiku", time.Time{}) + string(session) + "\n", "window", "", "text", hit.Add(2 * time.Hour)},
	} {
		tl, err := Parse(strings.NewReader(c.src), nil)
		if err != nil {
			t.Fatal(err)
		}
		got := tl.Limit(hit)
		if got.Reason != relay.UsageLimit || got.Kind != c.kind || got.Scope != c.scope || got.Source != c.source || !got.Reset.Equal(c.reset) {
			t.Errorf("%s: %+v; want usage-limit, kind %q, scope %q, source %q, reset %s", c.name, got, c.kind, c.scope, c.source, c.reset)
		}
		line := got.Line()
		if fmt.Sprint(line["kind"]) != c.kind || fmt.Sprint(line["scope"]) != c.scope {
			t.Errorf("%s: Line() %v; want kind %q, scope %q", c.name, line, c.kind, c.scope)
		}
	}
}
