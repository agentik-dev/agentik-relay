package relay

// nightStops maps the running night's stop events to their reasons.
var nightStops = map[NightEvent]StopReason{
	OnQueueEmpty:  QueueEmpty,
	OnCount:       CountReached,
	OnUntil:       UntilReached,
	OnBudget:      BudgetReached,
	OnBlocked:     GateFailedDependency,
	OnOriginMoved: OriginMoved,
	OnTmuxGone:    TmuxGone,
}

// NightStep is the night transition function. r is read only with
// OnSittingFailed, and must then be one of SittingFailures: a sitting failure
// stops the night with its reason, a gate red does not (D19). On error the
// state returned is s: ErrTerminal on a stopped night, ErrIllegal otherwise.
// A paused night (AR64) holds rows: it takes a later hit, a sitting outside
// the paused scope, the wake, the owner, the deadline, the budget and every
// sitting failure, and refuses what would read the held rows as gone.
func NightStep(s NightState, e NightEvent, r StopReason) (NightState, error) {
	if NightTerminal(s) {
		return s, ErrTerminal
	}
	if e == OnOwnerKill && (s == Planning || s == NightRunning || s == Draining || s == NightPaused) {
		return Stopped(OwnerKill), nil
	}
	if e == OnSittingFailed && (s == NightRunning || s == Draining || s == NightPaused) {
		if !listed(r, SittingFailures) {
			return s, ErrIllegal
		}
		return Stopped(r), nil
	}
	switch s {
	case Planning:
		switch e {
		case OnPlanned:
			return NightRunning, nil
		case OnBlocked:
			return Stopped(GateFailedDependency), nil
		}
	case NightRunning:
		if reason, ok := nightStops[e]; ok {
			return Stopped(reason), nil
		}
		switch e {
		case OnSittingGreen, OnSittingRed:
			return NightRunning, nil
		case OnOwnerStop:
			return Draining, nil
		case OnLimited:
			return NightPaused, nil
		}
	case NightPaused:
		switch e {
		case OnLimited, OnSittingGreen, OnSittingRed:
			return NightPaused, nil
		case OnWake:
			return NightRunning, nil
		case OnOwnerStop:
			return Stopped(OwnerStop), nil // no draining while paused
		case OnUntil:
			return Stopped(UntilReached), nil
		case OnBudget:
			return Stopped(BudgetReached), nil
		}
	case Draining:
		switch e {
		case OnSittingGreen, OnSittingRed, OnDrained:
			return Stopped(OwnerStop), nil
		}
	}
	return s, ErrIllegal
}
