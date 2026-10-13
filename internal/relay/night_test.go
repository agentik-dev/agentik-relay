package relay

import (
	"errors"
	"testing"
)

// TestNightMatrix walks every (night state, night event) pair, with
// OnSittingFailed tried under every StopReason. Terminal states refuse
// everything; the row count equals the product.
func TestNightMatrix(t *testing.T) {
	type pair struct {
		s NightState
		e NightEvent
	}
	legal := map[pair]NightState{
		{Planning, OnPlanned}:          NightRunning,
		{Planning, OnBlocked}:          Stopped(GateFailedDependency),
		{Planning, OnOwnerKill}:        Stopped(OwnerKill),
		{NightRunning, OnSittingGreen}: NightRunning,
		{NightRunning, OnSittingRed}:   NightRunning,
		{NightRunning, OnQueueEmpty}:   Stopped(QueueEmpty),
		{NightRunning, OnCount}:        Stopped(CountReached),
		{NightRunning, OnUntil}:        Stopped(UntilReached),
		{NightRunning, OnBudget}:       Stopped(BudgetReached),
		{NightRunning, OnBlocked}:      Stopped(GateFailedDependency),
		{NightRunning, OnOriginMoved}:  Stopped(OriginMoved),
		{NightRunning, OnTmuxGone}:     Stopped(TmuxGone),
		{NightRunning, OnOwnerStop}:    Draining,
		{NightRunning, OnOwnerKill}:    Stopped(OwnerKill),
		{Draining, OnSittingGreen}:     Stopped(OwnerStop),
		{Draining, OnSittingRed}:       Stopped(OwnerStop),
		{Draining, OnDrained}:          Stopped(OwnerStop),
		{Draining, OnOwnerKill}:        Stopped(OwnerKill),
		// The pause (AR64): paused holds rows, so a stop reads no drain.
		{NightRunning, OnLimited}:     NightPaused,
		{NightPaused, OnLimited}:      NightPaused,
		{NightPaused, OnWake}:         NightRunning,
		{NightPaused, OnSittingGreen}: NightPaused,
		{NightPaused, OnSittingRed}:   NightPaused,
		{NightPaused, OnOwnerStop}:    Stopped(OwnerStop),
		{NightPaused, OnOwnerKill}:    Stopped(OwnerKill),
		{NightPaused, OnUntil}:        Stopped(UntilReached),
		{NightPaused, OnBudget}:       Stopped(BudgetReached),
	}
	// A sitting failure stops a running, draining or paused night with its
	// reason.
	failing := map[NightState]bool{NightRunning: true, Draining: true, NightPaused: true}
	failures := map[StopReason]bool{
		StreamError: true, NoCommit: true, DirtyTree: true,
		CeilingKill: true, WallClock: true, OriginMoved: true, LedgerError: true, LoadOpen: true, Invariant: true, Orphans: true,
		UsageLimit: true, Auth: true, HelperStorm: true,
	}

	type row struct {
		state  NightState
		event  NightEvent
		reason StopReason
		want   NightState
		err    error
	}
	var table []row
	for _, s := range NightStates {
		for _, e := range NightEvents {
			reasons := []StopReason{""}
			if e == OnSittingFailed {
				reasons = StopReasons
			}
			for _, r := range reasons {
				w := row{s, e, r, s, ErrIllegal}
				switch {
				case NightTerminal(s):
					w.err = ErrTerminal
				case e == OnSittingFailed:
					if failing[s] && failures[r] {
						w.want, w.err = Stopped(r), nil
					}
				default:
					if next, ok := legal[pair{s, e}]; ok {
						w.want, w.err = next, nil
					}
				}
				table = append(table, w)
			}
		}
	}
	want := len(NightStates) * (len(NightEvents) - 1 + len(StopReasons))
	if len(table) != want || len(NightStates) != 34 || len(NightEvents) != 16 { // 19 until AR10 act 4 added budget, 20 until act 5 added load, 21 until act 7 added invariant, 22 until act 8 added orphans, 23 until AR13 act 3 added stalled-twice, 24 until AR17D act 0 added usage-limit and auth, 26 until AR22 act 2 added lock-moved, 27 until AR25 act 4 added helper-storm, 28 until AR32A act 4 added five, 33 until AR64 added paused:usage-limit (and the events limited and wake, 14 until then)
		t.Fatalf("matrix has %d rows over %d states and %d events, want %d over 34 and 16", len(table), len(NightStates), len(NightEvents), want)
	}
	for _, r := range table {
		t.Run(string(r.state)+"/"+string(r.event)+"/"+string(r.reason), func(t *testing.T) {
			got, err := NightStep(r.state, r.event, r.reason)
			if got != r.want || !errors.Is(err, r.err) {
				t.Fatalf("NightStep(%q, %q, %q) = %q, %v; want %q, %v", r.state, r.event, r.reason, got, err, r.want, r.err)
			}
		})
	}
}

// TestEveryStopHasReason: every terminal night state carries a listed,
// non-empty reason, and no transition reaches a stop without one.
func TestEveryStopHasReason(t *testing.T) {
	stops := 0
	for _, s := range NightStates {
		if !NightTerminal(s) {
			continue
		}
		stops++
		r, ok := StopReasonOf(s)
		if !ok || r == "" || !listed(r, StopReasons) || Stopped(r) != s {
			t.Errorf("%q parses to %q, %v", s, r, ok)
		}
	}
	if stops != len(StopReasons) {
		t.Errorf("%d terminal night states, want %d", stops, len(StopReasons))
	}
	if NightTerminal(Stopped("")) || NightTerminal(Stopped("bogus")) {
		t.Errorf("an unlisted reason reads as terminal")
	}
	for _, s := range []NightState{NightRunning, Draining, NightPaused} {
		if got, err := NightStep(s, OnSittingFailed, ""); got != s || !errors.Is(err, ErrIllegal) {
			t.Errorf("NightStep(%q, sitting-failed, \"\") = %q, %v; want refused", s, got, err)
		}
	}
	for _, s := range NightStates {
		for _, e := range NightEvents {
			for _, r := range append([]StopReason{""}, StopReasons...) {
				got, err := NightStep(s, e, r)
				if err == nil && NightTerminal(got) {
					if reason, _ := StopReasonOf(got); reason == "" {
						t.Errorf("NightStep(%q, %q, %q) stops with no reason", s, e, r)
					}
				}
				if err == nil && !NightTerminal(got) && got != Planning && got != NightRunning && got != Draining && got != NightPaused {
					t.Errorf("NightStep(%q, %q, %q) = %q, not a listed state", s, e, r, got)
				}
			}
		}
	}
}
