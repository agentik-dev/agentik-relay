package night_test

import (
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR32AFix_DoneIsClean is ar32a review round 1 (F1'): an orchestrated
// night that stops `done`, every row green, stopped clean and exits 0, as
// queue-empty does; a red row still makes it unclean.
func TestAR32AFix_DoneIsClean(t *testing.T) {
	green := &relay.Sitting{State: relay.Green}
	n := &night.Night{Reason: relay.Done, Sittings: map[string]*relay.Sitting{"o.1": green, "b1": green}}
	if !night.Clean(n) {
		t.Errorf("a night stopped done, every row green: not clean (exit 1)")
	}
	n.Sittings["b2"] = &relay.Sitting{State: relay.Red}
	if night.Clean(n) {
		t.Errorf("a night stopped done with a red row: clean")
	}
}
