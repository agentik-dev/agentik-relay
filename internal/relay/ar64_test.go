package relay

import (
	"errors"
	"slices"
	"testing"
)

// TestAR64_1_SittingPause: the account's limit pauses an exited part
// (usage-limit-resume r1 §2, §3): exited takes `limited` to paused, live and
// after exited in States; paused takes `spawn` (a continuation) to running and
// `halt` to failed, and refuses every other event; `limited` is refused on
// every other live state and terminal on the terminal ones. Entering paused
// owes the sitting line only.
func TestAR64_1_SittingPause(t *testing.T) {
	if i := slices.Index(States, Paused); i < 0 || States[i-1] != Exited {
		t.Fatalf("States = %v; want paused after exited", States)
	}
	if Events[len(Events)-1] != Limited {
		t.Fatalf("Events = %v; want limited last", Events)
	}
	if string(Paused) != "paused" || string(Limited) != "limited" {
		t.Fatalf("the names read %q, %q", Paused, Limited)
	}
	if Terminal(Paused) {
		t.Fatalf("paused reads as terminal")
	}
	pairs := map[[2]string]State{
		{string(Exited), string(Limited)}: Paused,
		{string(Paused), string(Spawn)}:   Running,
		{string(Paused), string(Halt)}:    Failed,
	}
	for k, want := range pairs {
		if got, err := Step(State(k[0]), Event(k[1])); got != want || err != nil {
			t.Errorf("Step(%s, %s) = %q, %v; want %q", k[0], k[1], got, err, want)
		}
	}
	for _, e := range Events {
		switch e {
		case Spawn, Halt, GateGreen, GateRed, GateFailed, GateUnrun: // the four gate events since AR70
			continue
		}
		if got, err := Step(Paused, e); got != Paused || !errors.Is(err, ErrIllegal) {
			t.Errorf("Step(paused, %s) = %q, %v; want paused, refused", e, got, err)
		}
	}
	for _, s := range States {
		if s == Exited {
			continue
		}
		want := ErrIllegal
		if Terminal(s) {
			want = ErrTerminal
		}
		if got, err := Step(s, Limited); got != s || !errors.Is(err, want) {
			t.Errorf("Step(%s, limited) = %q, %v; want %q, %v", s, got, err, s, want)
		}
	}
	if got := Actions(Sitting{State: Paused}); !slices.Equal(got, []Action{ActLedger}) {
		t.Errorf("Actions(paused) = %v; want [ledger]", got)
	}
}

// TestAR64_2_NightPause: the night's pause (usage-limit-resume r1 §2): a
// running night takes `limited` to paused:usage-limit, live and fourth in
// NightStates; paused holds rows, so it takes a later hit, a sitting outside
// the paused scope, the wake, the owner's stop and kill, the deadline and the
// budget, and every sitting failure, and refuses the rest. Only paused takes
// `wake`; planning and draining refuse `limited`.
func TestAR64_2_NightPause(t *testing.T) {
	if string(NightPaused) != "paused:usage-limit" || NightTerminal(NightPaused) {
		t.Fatalf("%q: terminal %v", NightPaused, NightTerminal(NightPaused))
	}
	if r, ok := StopReasonOf(NightPaused); ok || r != "" {
		t.Fatalf("StopReasonOf(paused) = %q, %v; want none", r, ok)
	}
	if len(NightStates) < 4 || NightStates[3] != NightPaused {
		t.Fatalf("NightStates[:4] = %v; want paused fourth", NightStates[:4])
	}
	n := len(NightEvents)
	if string(OnLimited) != "limited" || string(OnWake) != "wake" || NightEvents[n-2] != OnLimited || NightEvents[n-1] != OnWake {
		t.Fatalf("NightEvents = %v; want limited, wake last", NightEvents)
	}
	if len(StopReasons) != 30 {
		t.Fatalf("%d stop reasons, want 30", len(StopReasons))
	}
	type pair struct {
		s NightState
		e NightEvent
	}
	legal := map[pair]NightState{
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
	for p, want := range legal {
		if got, err := NightStep(p.s, p.e, ""); got != want || err != nil {
			t.Errorf("NightStep(%s, %s) = %q, %v; want %q", p.s, p.e, got, err, want)
		}
	}
	for _, r := range SittingFailures {
		if got, err := NightStep(NightPaused, OnSittingFailed, r); got != Stopped(r) || err != nil {
			t.Errorf("NightStep(paused, sitting-failed, %s) = %q, %v; want %q", r, got, err, Stopped(r))
		}
	}
	for _, r := range []StopReason{"", QueueEmpty, OwnerStop, Done} {
		if got, err := NightStep(NightPaused, OnSittingFailed, r); got != NightPaused || !errors.Is(err, ErrIllegal) {
			t.Errorf("NightStep(paused, sitting-failed, %q) = %q, %v; want refused", r, got, err)
		}
	}
	refused := []pair{
		{NightPaused, OnPlanned}, {NightPaused, OnQueueEmpty}, {NightPaused, OnCount},
		{NightPaused, OnBlocked}, {NightPaused, OnOriginMoved}, {NightPaused, OnTmuxGone},
		{NightPaused, OnDrained},
		{Planning, OnLimited}, {Draining, OnLimited},
		{Planning, OnWake}, {NightRunning, OnWake}, {Draining, OnWake},
	}
	for _, p := range refused {
		if got, err := NightStep(p.s, p.e, ""); got != p.s || !errors.Is(err, ErrIllegal) {
			t.Errorf("NightStep(%s, %s) = %q, %v; want refused", p.s, p.e, got, err)
		}
	}
	for _, e := range []NightEvent{OnLimited, OnWake} {
		if got, err := NightStep(Stopped(UsageLimit), e, ""); got != Stopped(UsageLimit) || !errors.Is(err, ErrTerminal) {
			t.Errorf("NightStep(stopped:usage-limit, %s) = %q, %v; want terminal", e, got, err)
		}
	}
	s := NightRunning
	for _, e := range []NightEvent{OnLimited, OnWake, OnUntil} {
		next, err := NightStep(s, e, "")
		if err != nil {
			t.Fatalf("the walk: NightStep(%s, %s): %v", s, e, err)
		}
		s = next
	}
	if s != Stopped(UntilReached) {
		t.Errorf("running, limited, wake, until ends %q; want stopped:until-reached", s)
	}
}
