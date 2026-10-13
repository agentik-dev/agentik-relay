package relay

// The invariants' names (§15): each is one named test and a runtime guard,
// read by Invariants at the sitting line, gate.Run and gate.Hand.
const (
	InvCommit       = "commit-moves-head"  // commit == (head_after != head_before)
	InvClean        = "clean-is-fresh"     // clean equals a fresh git status
	InvDeps         = "deps-satisfied"     // every dependency satisfied per §3
	InvOneChild     = "one-child-per-tree" // at most one live child per tree
	InvVerifyStatus = "verify-status"      // the verify line's status is a known one
	InvUnrunPasses  = "unrun-never-passes" // an unrun check never passes
)

// VerifyStatuses are the verify line's statuses: §15's four and §6's
// same-as-main (the verify line failing as it fails on main, a pass).
var VerifyStatuses = []string{CheckGreen, CheckRed, CheckUnrun, CheckSkipped, CheckSameAsMain}

// Facts are what a call site read when a sitting or a gate ends. A fact
// its call site could not read is left zero and its invariant is not read:
// Gathered false skips commit and clean, an empty head skips commit, a nil
// Fresh skips clean, an empty Verify skips the verify status.
type Facts struct {
	HeadBefore, HeadAfter string
	Gathered              bool  // the gate gathered Commit and Clean
	Commit                bool  // the gate's: HEAD moved
	Clean                 bool  // the gate's: git status --short empty
	Fresh                 *bool // a fresh git status read now: true when clean
	Deps                  map[string]bool
	Live                  map[string]int // live children per tree
	Verify                string         // the verify line's status
	Checks                []string       // the acceptance lines' statuses
	Pass                  bool           // the result is green
}

// Invariants returns the name of every invariant f breaks, in the order
// the constants list them; none is nil. Pure.
func Invariants(f Facts) []string {
	var out []string
	if f.Gathered && f.HeadBefore != "" && f.HeadAfter != "" && f.Commit != (f.HeadAfter != f.HeadBefore) {
		out = append(out, InvCommit)
	}
	if f.Gathered && f.Fresh != nil && f.Clean != *f.Fresh {
		out = append(out, InvClean)
	}
	for _, ok := range f.Deps {
		if !ok {
			out = append(out, InvDeps)
			break
		}
	}
	for _, n := range f.Live {
		if n > 1 {
			out = append(out, InvOneChild)
			break
		}
	}
	if f.Verify != "" && !contains(VerifyStatuses, f.Verify) {
		out = append(out, InvVerifyStatus)
	}
	if f.Pass && (f.Verify == CheckUnrun || contains(f.Checks, CheckUnrun)) {
		out = append(out, InvUnrunPasses)
	}
	return out
}

func contains(in []string, s string) bool {
	for _, x := range in {
		if x == s {
			return true
		}
	}
	return false
}
