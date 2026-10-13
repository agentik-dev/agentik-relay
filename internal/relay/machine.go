package relay

// step is the sitting table of §1: the legal (state, event) pairs and the
// next state. Every pair absent here is ErrIllegal.
var step = map[State]map[Event]State{
	Waiting: {DepsSatisfied: Ready, DepsFailed: Skipped, Halt: Skipped},
	Ready:   {Plan: Planned, Invalid: Skipped, Halt: Skipped},
	Planned: {Stage: Staged, Halt: Skipped},
	Staged:  {Spawn: Running, ExitError: Failed},
	Running: {Stream: Running, Exit: Exited, ExitError: Failed, CeilingHit: Failed},
	Exited:  {GateGreen: Green, GateRed: Red, GateFailed: Failed, GateUnrun: Red, Limited: Paused},
	// A continuation spawns a paused part; a halt fails it, the loop
	// setting usage-limit (AR64). A sibling killed in its gate lands that
	// gate's verdict as exited does, never gated again (AR70).
	Paused: {Spawn: Running, Halt: Failed, GateGreen: Green, GateRed: Red, GateFailed: Failed, GateUnrun: Red},
}

// Step is the sitting transition function: the next state for (s, e). Pure:
// no effects, no clock. On error the state returned is s: ErrTerminal for
// any event on a terminal state, ErrIllegal for a pair the table omits.
func Step(s State, e Event) (State, error) {
	if Terminal(s) {
		return s, ErrTerminal
	}
	if next, ok := step[s][e]; ok {
		return next, nil
	}
	return s, ErrIllegal
}
