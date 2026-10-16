package relay

// Check statuses: one acceptance line's verdict, or the verify line's.
const (
	CheckGreen      = "green"
	CheckRed        = "red"
	CheckSameAsMain = "same-as-main" // failed as it fails on main, and the packet wrote `as main`
	CheckUnrun      = "unrun"
	CheckSkipped    = "skipped"
	CheckLost       = "lost" // the verify line's: its result could not be read
)

// Check is one acceptance line run on the branch against its baseline on
// main. Head is the sha of main its Main fields were measured at: a
// baseline is compared only at that head (R6 §4).
type Check struct {
	Name       string `json:"name"`
	Expect     string `json:"expect"`
	MainExit   int    `json:"main_exit"`
	BranchExit int    `json:"branch_exit"`
	MainLast   string `json:"main_last"`
	BranchLast string `json:"branch_last"`
	Status     string `json:"status"`
	Head       string `json:"head,omitempty"`
	// BranchLog is the file holding the branch run's whole output, kept
	// when the check is not green (AR31 act 2); "" when none was kept.
	BranchLog string `json:"branch_log,omitempty"`
	// Cut is a verify line cut at its limit (effects.Cut, AR59 act 3): the
	// limit in whole seconds, its status red; 0 when it was not. The `gate`
	// ledger line carries it as `cut`. Not in --json until a row may
	// re-freeze json-keys.golden (§15), as PacketDiffers.
	Cut int `json:"-"`
	// Wait is the seconds a verify line waited for its root's gate lock
	// before its clock started (AR78); 0 when it took it at once. The
	// `gate` ledger line carries it as `wait_s`. Not in --json, as Cut.
	Wait int `json:"-"`
}

// Gate is the record of facts the gate's effects gathered for one sitting
// (§6); Decide judges it. Gathered is false until every fact is in.
type Gate struct {
	ExitOK     bool     `json:"exit_ok"`     // is_error was false
	ResultSeen bool     `json:"result_seen"` // a result event was seen
	Commit     bool     `json:"commit"`      // HEAD moved from head_before
	Clean      bool     `json:"clean"`       // git status --short is empty in the worktree
	OriginSame bool     `json:"origin_same"` // remote refs equal the snapshot taken at stage
	Outside    []string `json:"outside"`     // changed paths no may-touch entry matches
	Lines      []Check  `json:"lines"`       // the packet's acceptance lines
	Verify     *Check   `json:"verify"`      // the repo's own verify line, when it has one
	Skipped    []string `json:"skipped"`     // checks with no fact to read (exit, origin): their field reads true
	Orphans    int      `json:"orphans"`     // the child's group members its sweep killed after the Wait (§15): any is red
	Gathered   bool     `json:"gathered"`
	Head       string   `json:"head"`       // the row branch's sha every fact read, and the one a merge lands (§6)
	OffBranch  bool     `json:"off_branch"` // the tree's HEAD is not the row's branch at Head: red (ADV6-3)
	Main       string   `json:"main"`       // refs/heads/main's sha as the gate read it; a merge lands only onto it (ADV10-1)
	MainMoved  bool     `json:"main_moved"` // the base is not at its stage sha nor the relay's own last merge: red, main-moved
	// Dirty is the count of paths git status --short read when the gate
	// gathered (AR62 act 1): gate.Cause's `clean: <n> paths`. Not in
	// --json, as Cut.
	Dirty int `json:"-"`
	// VerifySeconds is the verify limit the gate ran its verify line under
	// (gate.Verify.Limit; AR59 act 2), which MergeIfGreen's verify takes
	// too; 0 (a gate built by hand) is effects.Timeout. Not in --json, as
	// Cut.
	VerifySeconds int `json:"-"`
	// PacketDiffers is a gate with a packets source whose
	// design/packets/<id>.md at Head is not the source's bytes, or is
	// absent there: red, beside OffBranch (AR29 act 3, ADV17-2). The
	// ledger's packet-source check carries it; `packet_differs` joins
	// --json when a row may re-freeze json-keys.golden (§15).
	PacketDiffers bool `json:"-"`
	// The orchestrator sitting's checks (AR32A act 3, orchestrator r1 §5):
	// NoCharter is no charter of the row added on the branch, under 4 KB,
	// with a Next act; QueueInvalid a queue at Head that does not load or
	// validate, or edits a row, a default or a trust field; Unproven a
	// merge commit on the branch's first-parent line (ar32b proves them).
	NoCharter    bool `json:"-"`
	QueueInvalid bool `json:"-"`
	Unproven     bool `json:"-"`

	// The git state bound at stage (AR20 act 0): Worktree is the row's
	// tree and StateDigest effects.GitStateDigest over it and the root,
	// which the merge recomputes; StateMoved is a gate that found it
	// changed: unrun, git-state-moved.
	Worktree    string `json:"-"`
	StateDigest string `json:"-"`
	StateMoved  bool   `json:"-"`
	// ConfigProgram is a gate that found a program-valued key in the
	// root's or a worktree's config (AR21 act 1): unrun,
	// git-config-program.
	ConfigProgram bool `json:"-"`
	// RefMoved is a gate whose fresh clone fetched the branch or its base
	// at another sha than the one it read (AR22 act 0): unrun, ref-moved.
	RefMoved bool `json:"-"`
	// ToolMoved is a gate that found an account tool of the pinned PATH
	// changed since start (effects.Pinned.Moved; AR22 act 1): unrun,
	// tool-moved.
	ToolMoved bool `json:"-"`
	// Remote is a gate whose ls-remote read of a remote failed (AR88):
	// unrun, RemoteTimeout or RemoteError; "" none.
	Remote StopReason `json:"-"`

	// Harness is the verify line's harness the gate judged with (AR20 act
	// 2: the make spellings and the row's queue `harness`); the merge lays
	// the same paths over the merged tree.
	Harness []string `json:"-"`

	// The remotes bound at stage (AR20 act 3): their names, whose pushurl
	// every line has denied, their refs as stage read them ("<remote>
	// <ref>"), and the literal URLs those were read from. The merge reads
	// the remotes again after its verify line (ADV11-6, -7).
	Remotes    []string            `json:"-"`
	RemoteRefs map[string]string   `json:"-"`
	RemoteURLs map[string][]string `json:"-"`
}

// Logs are the logs g's checks kept: each line's, then the verify's.
func (g Gate) Logs() []string {
	var logs []string
	for _, c := range g.Lines {
		if c.BranchLog != "" {
			logs = append(logs, c.BranchLog)
		}
	}
	if g.Verify != nil && g.Verify.BranchLog != "" {
		logs = append(logs, g.Verify.BranchLog)
	}
	return logs
}

// Result is the gate's verdict.
type Result string

const (
	GreenResult  Result = "green"
	RedResult    Result = "red"    // a packet failure: the night moves on
	FailedResult Result = "failed" // a sitting failure: the night stops
	Unrun        Result = "unrun"  // never a pass
)

// Decide judges g in §6's order: the first check that fails decides. The
// reason is set with FailedResult, with RedResult only as MainMoved,
// NoCharter, QueueInvalid or RowUnproven (AR32A act 3), or VerifyCut after
// them, ahead of every red with no reason (AR59 act 3), and
// with Unrun only as GitStateMoved, GitConfigProgram, RefMoved,
// ToolMoved, RemoteTimeout or RemoteError (AR88).
func Decide(g Gate) (Result, StopReason) {
	switch {
	case g.ConfigProgram:
		return Unrun, GitConfigProgram
	case g.StateMoved:
		return Unrun, GitStateMoved
	case g.RefMoved:
		return Unrun, RefMoved
	case g.ToolMoved:
		return Unrun, ToolMoved
	case g.Remote != "":
		return Unrun, g.Remote
	case !g.Gathered:
		return Unrun, ""
	case !g.ResultSeen || !g.ExitOK:
		return FailedResult, StreamError
	case !g.Commit:
		return FailedResult, NoCommit
	case !g.Clean:
		return FailedResult, DirtyTree
	case !g.OriginSame:
		return FailedResult, OriginMoved
	case g.MainMoved:
		return RedResult, MainMoved
	case g.NoCharter:
		return RedResult, NoCharter
	case g.QueueInvalid:
		return RedResult, QueueInvalid
	case g.Unproven:
		return RedResult, RowUnproven
	case g.Verify != nil && g.Verify.Cut > 0:
		return RedResult, VerifyCut
	case g.OffBranch, g.PacketDiffers, g.Orphans > 0, len(g.Outside) > 0:
		return RedResult, ""
	}
	for _, c := range g.Lines {
		if !passes(c.Status) {
			return RedResult, ""
		}
	}
	if g.Verify != nil && !passes(g.Verify.Status) {
		return RedResult, ""
	}
	return GreenResult, ""
}

// passes is the pass statuses, listed, so a status no one set never passes.
func passes(status string) bool {
	return status == CheckGreen || status == CheckSameAsMain || status == CheckSkipped
}
