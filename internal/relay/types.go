// Package relay is the core of the agent-relay driver: two pure state
// machines, one for a sitting and one for the night over a queue of
// sittings. Every transition is a function of (state, event); effects (git,
// claude, tmux, notify, the clock, the filesystem) live behind interfaces
// outside this package, and Actions names the effects owed on entering a
// state (design/relay-r1.md §1).
package relay

import (
	"errors"
	"strings"
)

// State is a node of the sitting machine.
type State string

const (
	Waiting State = "waiting" // a dependency unsatisfied
	Ready   State = "ready"
	Planned State = "planned" // packet parsed, baseline recorded, spawn argv rendered
	Staged  State = "staged"  // worktree, branch, pre-push hook, stage ledger line
	Running State = "running" // the child is alive
	Exited  State = "exited"  // the child is gone
	// Paused is a part the account's limit paused (usage-limit-resume r1
	// §2, AR64): no child; its tree, stage lock and Staged kept. A
	// continuation spawns it again; a night that ends paused halts it
	// failed, usage-limit.
	Paused  State = "paused"
	Green   State = "green"   // terminal: gate green
	Red     State = "red"     // terminal: gate red, a packet failure
	Failed  State = "failed"  // terminal: a sitting failure, Reason set
	Skipped State = "skipped" // terminal: parked
)

// States lists every sitting State, terminal ones last.
var States = []State{Waiting, Ready, Planned, Staged, Running, Exited, Paused, Green, Red, Failed, Skipped}

// Terminal reports whether s refuses every event.
func Terminal(s State) bool {
	return s == Green || s == Red || s == Failed || s == Skipped
}

// Event is an input to the sitting machine.
type Event string

const (
	DepsSatisfied Event = "deps-satisfied"
	DepsFailed    Event = "deps-failed"
	Plan          Event = "plan"
	Invalid       Event = "invalid"
	Stage         Event = "stage"
	Spawn         Event = "spawn"
	Stream        Event = "stream"
	CeilingHit    Event = "ceiling-hit"
	Exit          Event = "exit"
	ExitError     Event = "exit-error"
	GateGreen     Event = "gate-green"
	GateRed       Event = "gate-red"
	GateFailed    Event = "gate-failed"
	GateUnrun     Event = "gate-unrun" // the gate could not gather its facts: red, never a pass
	Halt          Event = "halt"
	Limited       Event = "limited" // the account's limit paused an exited part (AR64)
)

// Events lists every sitting Event.
var Events = []Event{
	DepsSatisfied, DepsFailed, Plan, Invalid, Stage, Spawn, Stream,
	CeilingHit, Exit, ExitError, GateGreen, GateRed, GateFailed, GateUnrun, Halt,
	Limited,
}

// StopReason says why a night ended, or why a sitting failed.
type StopReason string

const (
	QueueEmpty           StopReason = "queue-empty"
	CountReached         StopReason = "count-reached"
	UntilReached         StopReason = "until-reached"
	BudgetReached        StopReason = "budget-reached"
	StreamError          StopReason = "stream-error"
	NoProgress           StopReason = "no-progress"
	DirtyTree            StopReason = "dirty-tree"
	NoCommit             StopReason = "no-commit"
	GateFailedDependency StopReason = "gate-failed-dependency"
	CeilingKill          StopReason = "ceiling-kill"
	WallClock            StopReason = "wall-clock"
	OriginMoved          StopReason = "origin-moved"
	TmuxGone             StopReason = "tmux-gone"
	OwnerStop            StopReason = "owner-stop"
	OwnerKill            StopReason = "owner-kill"
	LedgerError          StopReason = "ledger-error"
	MeasureTimeout       StopReason = "measure-timeout" // the measure ran out of time (§15; AR38 act 2, once `budget`): a baseline left unrun
	LoadOpen             StopReason = "load"            // the breaker still open at its Max before a spawn (§15): a pre-check
	Invariant            StopReason = "invariant"       // a runtime invariant broken at a sitting line or a gate (§15)
	Orphans              StopReason = "orphans"         // a child's process group survived three sweeps (§15, D6)
	StalledTwice         StopReason = "stalled-twice"   // a second stalled row in one night (R6 §2): the driver's, not the packet's
	UsageLimit           StopReason = "usage-limit"     // the account's usage or rate limit (run 10 ruling 1): every running row with it
	Auth                 StopReason = "auth"            // the account's login expired or refused (run 10 ruling 1): every running row with it
	// LockMoved stops the night when the account's pid file (relay.pid,
	// which only names the lock's holder) is gone or a new inode at the
	// holder's poll (AR22 act 2, ADV13-4): no new sitting starts.
	LockMoved StopReason = "lock-moved"
	// HelperStorm stops the night when a watchdog tick counts more than
	// night.MaxHelpers `claude` processes among a row's seen descendants
	// (AR25 act 4, ADV15-1): the row's group and those descendants are
	// killed at once, whatever env a script gave its helpers; a sitting
	// failure, and no gate runs.
	HelperStorm StopReason = "helper-storm"
	// The orchestrated night's stops (AR32A act 4, orchestrator r1 §4):
	// Done, the row's Next act `DONE:` with no row it added ready, running
	// or green unmerged; StillWaiting, `WAITING:` (or DONE held) with no build
	// row ready; OrchestratorRed, a red sitting once the ready rows ran;
	// MergeHeld, a green sitting whose merge failed with a plain error.
	// NoProgress and QueueInvalid are also theirs.
	Done            StopReason = "done"
	StillWaiting    StopReason = "waiting"
	OrchestratorRed StopReason = "orchestrator-red"
	MergeHeld       StopReason = "merge-held"

	// GateUnrunReason is a red row's reason when its gate was unrun; a
	// packet failure, so never a night stop and absent from StopReasons.
	GateUnrunReason StopReason = "gate-unrun"
	// Stalled is a red row's reason when the watchdog killed it (§15): no
	// commit, file or CPU for --stall-seconds; a packet failure, absent from
	// StopReasons: the first in a night moves on, a second stops it
	// StalledTwice.
	Stalled StopReason = "stalled"
	// MainMoved is a red row's reason when its base moved between stage
	// and gate by anything but the relay's own merge (ADV10-1); a packet
	// failure, absent from StopReasons: nothing merges.
	MainMoved StopReason = "main-moved"
	// VerifyCut is a red row's reason when its gate's verify line was cut
	// at its limit (effects.Cut; the repo's verify_seconds, else 900 s;
	// AR59 act 3): the end line's detail reads `verify cut at <n> s`; a
	// merge verify cut there is a MergeRefused with it (RefusedMerges). A
	// packet failure, absent from StopReasons.
	VerifyCut StopReason = "verify-cut"
	// NoCharter, QueueInvalid and RowUnproven are an orchestrator
	// sitting's red reasons (AR32A act 3, orchestrator r1 §5): no charter
	// added with a Next act; a queue at its head that fails to load or
	// validate, or edits what it may only append to; a merge commit on its
	// branch. QueueInvalid is also the night's stop when the queue re-read
	// after an orchestrator merge fails.
	NoCharter    StopReason = "no-charter"
	QueueInvalid StopReason = "queue-invalid"
	RowUnproven  StopReason = "row-unproven"
	// GitStateMoved is an unrun gate's reason, and a refused merge's, when
	// the git state the stage bound (config, hooks, info; AR20 act 0) has
	// changed since: nothing merges.
	GitStateMoved StopReason = "git-state-moved"
	// MergeTreeDiffers is a red row's reason when the root's merge commit
	// is not the tree the merge verified; the merge is undone (ADV11-5).
	MergeTreeDiffers StopReason = "merge-tree-differs"
	// NoIdentity is an unrun merge's reason when no committer identity was
	// read at start (AR21 act 0, ADV12-1): nothing merges, the row keeps
	// its gate's verdict.
	NoIdentity StopReason = "no-identity"
	// GitConfigProgram is an unrun stage's, gate's or merge's reason when
	// the root's local config or a worktree's config holds a
	// program-valued key (effects.ProgramKeys; AR21 act 1, ADV12-2):
	// nothing is checked out, judged or merged under it.
	GitConfigProgram StopReason = "git-config-program"
	// RefMoved is an unrun gate's reason, and a refused merge's, when a
	// ref the fresh clone fetched by its full refname is not at the sha
	// the gate or the merge bound (AR22 act 0, ADV13-1): nothing is
	// judged or merged there.
	RefMoved StopReason = "ref-moved"
	// ToolMoved is an unrun gate's reason, and a refused merge's, when an
	// account tool the pinned PATH reaches (go, gofmt, pyenv, python3,
	// claude) is not the file whose sha256 Pin bound at start, or its link
	// in the relay home moved (AR22 act 1, ADV13-2): no line runs.
	ToolMoved StopReason = "tool-moved"
	// UnpinnedTool is the start's refusal (exit 2) when git, make, sh or
	// ps is in no dir the account cannot write (AR22 act 1, ADV13-2).
	UnpinnedTool StopReason = "unpinned-tool"
	// LedgerBaseDiffers is an unrun hand gate's reason when the row's last
	// `stage` line names a base other than the queue row's (AR22 act 3,
	// ADV13-5): the base is the queue's, and a ledger value only refuses.
	LedgerBaseDiffers StopReason = "ledger-base-differs"
)

// MergeRefused is a merge's error when the refusal turns the row red with
// Reason (GitStateMoved, MergeTreeDiffers: AR20 act 0).
type MergeRefused struct {
	Reason StopReason
	Err    error
}

func (e *MergeRefused) Error() string { return e.Err.Error() }
func (e *MergeRefused) Unwrap() error { return e.Err }

// RefusedMerges are the reasons a MergeRefused carries, each also its
// `merge` ledger line's result (gate.MergeIfGreen): a green row whose
// merge line reads one of them is red (ar32a review round 1).
var RefusedMerges = []StopReason{GitStateMoved, GitConfigProgram, OriginMoved, MergeTreeDiffers, VerifyCut}

// StopReasons lists every StopReason in the order §1 gives.
var StopReasons = []StopReason{
	QueueEmpty, CountReached, UntilReached, BudgetReached, StreamError,
	NoProgress, DirtyTree, NoCommit, GateFailedDependency, CeilingKill,
	WallClock, OriginMoved, TmuxGone, OwnerStop, OwnerKill, LedgerError, MeasureTimeout, LoadOpen, Invariant, Orphans,
	StalledTwice, UsageLimit, Auth, LockMoved, HelperStorm,
	Done, StillWaiting, OrchestratorRed, MergeHeld, QueueInvalid,
}

// SittingFailures are the reasons a sitting can fail with; each stops the
// night (D19).
var SittingFailures = []StopReason{StreamError, NoCommit, DirtyTree, CeilingKill, WallClock, OriginMoved, LedgerError, LoadOpen, Invariant, Orphans, UsageLimit, Auth, HelperStorm}

// AccountStop reports whether r is the account's (run 10 ruling 1): a
// usage limit or a login, which stops the whole night, kills every running
// row with the same reason, and keeps each row's worktree as it stands.
func AccountStop(r StopReason) bool { return r == UsageLimit || r == Auth }

func listed(r StopReason, in []StopReason) bool {
	for _, x := range in {
		if x == r {
			return true
		}
	}
	return false
}

// NightState is a node of the night machine.
type NightState string

const (
	Planning     NightState = "planning"
	NightRunning NightState = "running"
	Draining     NightState = "draining" // relay stop: no new sitting, the running ones finish
	// NightPaused is a night the account's limit paused until its wake
	// (usage-limit-resume r1 §2, AR64): live, it holds the paused rows; no
	// row of the paused scope is picked.
	NightPaused NightState = "paused:usage-limit"
)

const stoppedPrefix = "stopped:"

// Stopped is the terminal night state for reason r.
func Stopped(r StopReason) NightState {
	return NightState(stoppedPrefix + string(r))
}

// StopReasonOf returns the reason a terminal night state carries; ok is
// false for a live state or a reason not listed in StopReasons.
func StopReasonOf(s NightState) (r StopReason, ok bool) {
	rest, found := strings.CutPrefix(string(s), stoppedPrefix)
	if !found || !listed(StopReason(rest), StopReasons) {
		return "", false
	}
	return StopReason(rest), true
}

// NightStates lists the four live night states, then Stopped(r) for every
// StopReason.
var NightStates = func() []NightState {
	out := []NightState{Planning, NightRunning, Draining, NightPaused}
	for _, r := range StopReasons {
		out = append(out, Stopped(r))
	}
	return out
}()

// NightTerminal reports whether s is a stopped night.
func NightTerminal(s NightState) bool {
	_, ok := StopReasonOf(s)
	return ok
}

// NightEvent is an input to the night machine. The names carry an On prefix
// because six of them would otherwise collide with a StopReason or a State.
type NightEvent string

const (
	OnPlanned       NightEvent = "planned"
	OnSittingGreen  NightEvent = "sitting-green"
	OnSittingRed    NightEvent = "sitting-red"
	OnSittingFailed NightEvent = "sitting-failed"
	OnQueueEmpty    NightEvent = "queue-empty"
	OnCount         NightEvent = "count"
	OnUntil         NightEvent = "until"
	OnBudget        NightEvent = "budget"
	OnBlocked       NightEvent = "blocked"
	OnOriginMoved   NightEvent = "origin-moved"
	OnTmuxGone      NightEvent = "tmux-gone"
	OnOwnerStop     NightEvent = "owner-stop"
	OnOwnerKill     NightEvent = "owner-kill"
	OnDrained       NightEvent = "drained"
	OnLimited       NightEvent = "limited" // the account's limit paused the night (AR64)
	OnWake          NightEvent = "wake"    // a paused night's wake: its rows continue (AR64)
)

// NightEvents lists every NightEvent.
var NightEvents = []NightEvent{
	OnPlanned, OnSittingGreen, OnSittingRed, OnSittingFailed, OnQueueEmpty,
	OnCount, OnUntil, OnBudget, OnBlocked, OnOriginMoved, OnTmuxGone,
	OnOwnerStop, OnOwnerKill, OnDrained, OnLimited, OnWake,
}

var (
	// ErrIllegal is returned for a (state, event) pair the table does not
	// list; the state is kept.
	ErrIllegal = errors.New("illegal transition")
	// ErrTerminal is returned for any event on a terminal state.
	ErrTerminal = errors.New("terminal state")
	// ErrNotImplemented is returned by the verbs cmd/relay still stubs.
	ErrNotImplemented = errors.New("not implemented")
)

// Sitting is one Claude Code session building one packet.
type Sitting struct {
	ID       string
	State    State
	Reason   StopReason // set when State is Failed
	MergeSHA string     // set when State is Green and the policy merged
}

// Night is one run of the relay over a queue of packets.
type Night struct {
	State  NightState
	Reason StopReason
}

// Packet is one unit of work from the queue; its schema is §2's, a later
// packet's.
type Packet struct{}
