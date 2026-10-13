package relay

import (
	"errors"
	"testing"
)

// TestStepMatrix walks every (state, event) pair. The legal column is §1's
// sitting table; every other pair of a live state is ErrIllegal, every pair
// of a terminal state ErrTerminal, and the state is kept on error.
func TestStepMatrix(t *testing.T) {
	type pair struct {
		s State
		e Event
	}
	legal := map[pair]State{
		{Waiting, DepsSatisfied}: Ready,
		{Waiting, DepsFailed}:    Skipped,
		{Waiting, Halt}:          Skipped,
		{Ready, Plan}:            Planned,
		{Ready, Invalid}:         Skipped,
		{Ready, Halt}:            Skipped,
		{Planned, Stage}:         Staged,
		{Planned, Halt}:          Skipped,
		{Staged, Spawn}:          Running,
		{Staged, ExitError}:      Failed,
		{Running, Stream}:        Running,
		{Running, Exit}:          Exited,
		{Running, ExitError}:     Failed,
		{Running, CeilingHit}:    Failed,
		{Exited, GateGreen}:      Green,
		{Exited, GateRed}:        Red,
		{Exited, GateFailed}:     Failed,
		{Exited, GateUnrun}:      Red,
		{Exited, Limited}:        Paused,
		{Paused, Spawn}:          Running,
		{Paused, Halt}:           Failed,
		{Paused, GateGreen}:      Green,
		{Paused, GateRed}:        Red,
		{Paused, GateFailed}:     Failed,
		{Paused, GateUnrun}:      Red,
	}
	type row struct {
		state State
		event Event
		want  State
		err   error
	}
	var table []row
	for _, s := range States {
		for _, e := range Events {
			r := row{s, e, s, ErrIllegal}
			if Terminal(s) {
				r.err = ErrTerminal
			} else if next, ok := legal[pair{s, e}]; ok {
				r.want, r.err = next, nil
			}
			table = append(table, r)
		}
	}
	if len(table) != len(States)*len(Events) || len(table) != 11*16 { // 10*15 until AR64 added paused and limited
		t.Fatalf("matrix has %d rows, want %d", len(table), len(States)*len(Events))
	}
	seen := 0
	for _, r := range table {
		if r.err == nil {
			seen++
		}
		t.Run(string(r.state)+"/"+string(r.event), func(t *testing.T) {
			got, err := Step(r.state, r.event)
			if got != r.want || !errors.Is(err, r.err) {
				t.Fatalf("Step(%q, %q) = %q, %v; want %q, %v", r.state, r.event, got, err, r.want, r.err)
			}
		})
	}
	if seen != len(legal) {
		t.Fatalf("%d legal pairs walked, the table lists %d", seen, len(legal))
	}
}
