package night

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// TestAR71_1_ModelScope (AR71 rulings 1, 2): ofModel holds a scope word
// as one piece of the model cut at every rune not a letter or digit, ""
// every model; pauseFor's pause on a model's limit with a reset carries
// the Limit's word, a window's ""; a row Hold holds is never picked and
// counts as waiting, so its dependent waits and nothing reads blocked.
func TestAR71_1_ModelScope(t *testing.T) {
	for _, c := range []struct {
		model, scope string
		want         bool
	}{
		{"claude-fable-5-1", "fable", true},
		{"Claude-FABLE-5-1", "fable", true},
		{"claude-opus-5-5", "fable", false},
		{"fablex-1", "fable", false},
		{"claude-opus-5-5[1m]", "opus", true},
		{"claude-opus-5-5", "", true},
		{"", "", true},
		{"", "fable", false},
	} {
		if got := ofModel(c.model, c.scope); got != c.want {
			t.Errorf("ofModel(%q, %q) = %v, want %v", c.model, c.scope, got, c.want)
		}
	}
	now := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	reset := now.Add(time.Hour)
	model := stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindModel, Reset: reset, Named: reset, Source: "text", Scope: "fable"}
	if p := pauseFor(pauseIn{Limit: model, Now: now}); p.Stop || p.Scope != "fable" || !p.Wake.Equal(reset.Add(WakeSlack)) {
		t.Errorf("a model's limit with a reset: %+v, want a pause, scope fable, wake %s", p, reset.Add(WakeSlack))
	}
	window := stream.Limit{Reason: relay.UsageLimit, Kind: stream.KindWindow, Reset: reset, Named: reset, Source: "event"}
	if p := pauseFor(pauseIn{Limit: window, Now: now}); p.Stop || p.Scope != "" {
		t.Errorf("a window's limit: %+v, want a pause, scope \"\"", p)
	}
	hold := func(id string) bool { return id == "a" }
	next, blocked, err := PickIn(q3(), sittings(map[string]relay.State{"a": relay.Ready, "b": relay.Waiting, "c": relay.Ready}), nil, 2, PickOptions{Hold: hold})
	if err != nil || len(next) != 1 || next[0] != "c" || blocked {
		t.Errorf("a held ready row: got %v %v %v, want [c], not blocked", next, blocked, err)
	}
	next, blocked, _ = PickIn(q3(), sittings(map[string]relay.State{"a": relay.Ready, "b": relay.Waiting}), nil, 2, PickOptions{Hold: hold})
	if next != nil || blocked {
		t.Errorf("a held base: got %v %v, want nothing, not blocked (its dependent waits)", next, blocked)
	}
}
