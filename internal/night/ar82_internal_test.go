package night

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// TestAR82_2_PauseForKinds (AR82 rulings 2, 3): an overage limit with a
// reset in reach and no --until stops the night at once, naming it, its
// wake and source kept as the weekly stop keeps them; a family's weekly
// cap, a model's limit scoped opus with that reset, pauses only its rows;
// ofModel reads the sonnet family as it reads the others. The kinds are
// spelled as the exit line spells them.
func TestAR82_2_PauseForKinds(t *testing.T) {
	now := time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)
	reset := now.Add(time.Hour)
	overage := stream.Limit{Reason: relay.UsageLimit, Kind: "overage", Reset: reset, Named: reset, Source: "event"}
	if p := pauseFor(pauseIn{Limit: overage, Now: now}); !p.Stop || p.Detail != "the overage limit" || p.Source != "event" || !p.Wake.Equal(reset.Add(WakeSlack)) || p.Scope != "" {
		t.Errorf("an overage limit: %+v; want a stop, %q, event, wake %s, no scope", p, "the overage limit", reset.Add(WakeSlack))
	}
	family := stream.Limit{Reason: relay.UsageLimit, Kind: "model", Reset: reset, Named: reset, Source: "event", Scope: "opus"}
	if p := pauseFor(pauseIn{Limit: family, Now: now}); p.Stop || p.Scope != "opus" || !p.Wake.Equal(reset.Add(WakeSlack)) {
		t.Errorf("the opus family's weekly cap: %+v; want a pause, scope opus, wake %s", p, reset.Add(WakeSlack))
	}
	for _, c := range []struct {
		model, scope string
		want         bool
	}{
		{"claude-sonnet-4-5", "sonnet", true},
		{"claude-opus-5-5", "sonnet", false},
		{"claude-opus-5-5", "opus", true},
		{"claude-fable-5-1", "opus", false},
	} {
		if got := ofModel(c.model, c.scope); got != c.want {
			t.Errorf("ofModel(%q, %q) = %v, want %v", c.model, c.scope, got, c.want)
		}
	}
}
