package relay

import (
	"slices"
	"testing"
)

// TestActions is the table of effects owed per state: one row per State,
// plus the failed reasons that owe a kill first.
func TestActions(t *testing.T) {
	table := []struct {
		s    Sitting
		want []Action
	}{
		{Sitting{State: Waiting}, nil},
		{Sitting{State: Ready}, nil},
		{Sitting{State: Planned}, []Action{ActRecordPlan}},
		{Sitting{State: Staged}, []Action{ActWorktree, ActHook, ActLedger}},
		{Sitting{State: Running}, []Action{ActSpawn}},
		{Sitting{State: Exited}, []Action{ActGate}},
		{Sitting{State: Paused}, []Action{ActLedger}},
		{Sitting{State: Green}, []Action{ActLedger, ActMerge, ActNotify}},
		{Sitting{State: Red}, []Action{ActLedger, ActNotify}},
		{Sitting{State: Failed, Reason: StreamError}, []Action{ActLedger, ActNotify}},
		{Sitting{State: Skipped}, []Action{ActLedger, ActNotify}},
		{Sitting{State: Failed, Reason: CeilingKill}, []Action{ActKill, ActLedger, ActNotify}},
		{Sitting{State: Failed, Reason: WallClock}, []Action{ActKill, ActLedger, ActNotify}},
	}
	covered := map[State]bool{}
	for _, r := range table {
		covered[r.s.State] = true
		if got := Actions(r.s); !slices.Equal(got, r.want) {
			t.Errorf("Actions(%s, %q) = %v; want %v", r.s.State, r.s.Reason, got, r.want)
		}
	}
	for _, s := range States {
		if !covered[s] {
			t.Errorf("state %q has no row", s)
		}
	}
}
