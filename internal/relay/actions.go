package relay

// Action names an effect owed on entering a sitting state (Moore-style). The
// loop performs it and feeds the outcome back as the next event; nothing in
// this package runs one.
type Action string

const (
	ActRecordPlan Action = "record-plan"
	ActWorktree   Action = "worktree"
	ActHook       Action = "hook"
	ActLedger     Action = "ledger"
	ActSpawn      Action = "spawn"
	ActGate       Action = "gate"
	ActMerge      Action = "merge" // when the policy says so
	ActNotify     Action = "notify"
	ActKill       Action = "kill"
)

// Actions returns the effects owed on entering s.State, in order; nil for a
// state that owes none (waiting, ready).
func Actions(s Sitting) []Action {
	switch s.State {
	case Planned:
		return []Action{ActRecordPlan}
	case Staged:
		return []Action{ActWorktree, ActHook, ActLedger}
	case Running:
		return []Action{ActSpawn}
	case Exited:
		return []Action{ActGate}
	case Paused:
		// The sitting line (`limit: paused`); the notice is the night's
		// `pause` line's (AR64).
		return []Action{ActLedger}
	case Green:
		return []Action{ActLedger, ActMerge, ActNotify}
	case Failed:
		if s.Reason == CeilingKill || s.Reason == WallClock {
			return []Action{ActKill, ActLedger, ActNotify}
		}
		return []Action{ActLedger, ActNotify}
	case Red, Skipped:
		return []Action{ActLedger, ActNotify}
	}
	return nil
}
