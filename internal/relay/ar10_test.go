package relay

import (
	"reflect"
	"testing"
)

// holding is a Facts every invariant holds for: a gathered gate whose
// commit moved the head, clean both times, its dependencies satisfied, one
// child in its tree, verify and every check green, the result green.
func holding() Facts {
	yes := true
	return Facts{HeadBefore: "a", HeadAfter: "b", Gathered: true, Commit: true, Clean: true, Fresh: &yes,
		Deps: map[string]bool{"d1": true}, Live: map[string]int{"/t": 1, "/u": 0},
		Verify: CheckGreen, Checks: []string{CheckGreen, CheckSameAsMain}, Pass: true}
}

// TestAR10_7_CommitAndClean: commit == (head_after != head_before), and
// clean equals a fresh git status; a fact not read is not judged.
func TestAR10_7_CommitAndClean(t *testing.T) {
	no := false
	table := []struct {
		name string
		flip func(*Facts)
		want []string
	}{
		{"holds", func(*Facts) {}, nil},
		{"commit with no head move", func(f *Facts) { f.HeadAfter = f.HeadBefore }, []string{InvCommit}},
		{"no commit with a head move", func(f *Facts) { f.Commit = false }, []string{InvCommit}},
		{"no commit, no move", func(f *Facts) { f.Commit, f.HeadAfter = false, f.HeadBefore }, nil},
		{"head after unread", func(f *Facts) { f.HeadAfter = "" }, nil},
		{"clean but dirty now", func(f *Facts) { f.Fresh = &no }, []string{InvClean}},
		{"dirty but clean now", func(f *Facts) { f.Clean = false }, []string{InvClean}},
		{"fresh unread", func(f *Facts) { f.Clean, f.Fresh = false, nil }, nil},
		{"not gathered", func(f *Facts) { f.Gathered, f.Commit, f.Clean = false, false, false }, nil},
	}
	for _, r := range table {
		t.Run(r.name, func(t *testing.T) {
			f := holding()
			r.flip(&f)
			if got := Invariants(f); !reflect.DeepEqual(got, r.want) {
				t.Fatalf("Invariants = %v, want %v", got, r.want)
			}
		})
	}
}

// TestAR10_7_DepsAndOneChild: every dependency satisfied per §3, and at
// most one live child per tree.
func TestAR10_7_DepsAndOneChild(t *testing.T) {
	f := holding()
	f.Deps = map[string]bool{"d1": true, "d2": false}
	if got := Invariants(f); !reflect.DeepEqual(got, []string{InvDeps}) {
		t.Fatalf("an unsatisfied dependency: %v", got)
	}
	f = holding()
	f.Live = map[string]int{"/t": 2, "/u": 1}
	if got := Invariants(f); !reflect.DeepEqual(got, []string{InvOneChild}) {
		t.Fatalf("two children in one tree: %v", got)
	}
	f.Deps, f.Live = nil, nil
	if got := Invariants(f); got != nil {
		t.Fatalf("no deps and no children read: %v", got)
	}
}

// TestAR10_7_VerifyStatusAndUnrun: the verify status is one of green, red,
// unrun, skipped (and §6's same-as-main); an unrun verify or check never
// passes; the names come in their listed order.
func TestAR10_7_VerifyStatusAndUnrun(t *testing.T) {
	for _, s := range VerifyStatuses {
		f := holding()
		f.Verify, f.Pass = s, s != CheckUnrun && s != CheckRed
		if got := Invariants(f); got != nil {
			t.Fatalf("verify %s: %v", s, got)
		}
	}
	f := holding()
	f.Verify = "lost"
	if got := Invariants(f); !reflect.DeepEqual(got, []string{InvVerifyStatus}) {
		t.Fatalf("verify lost: %v", got)
	}
	f = holding()
	f.Verify = CheckUnrun
	if got := Invariants(f); !reflect.DeepEqual(got, []string{InvUnrunPasses}) {
		t.Fatalf("an unrun verify passing: %v", got)
	}
	f = holding()
	f.Checks = []string{CheckGreen, CheckUnrun}
	if got := Invariants(f); !reflect.DeepEqual(got, []string{InvUnrunPasses}) {
		t.Fatalf("an unrun check passing: %v", got)
	}
	f.Pass = false
	if got := Invariants(f); got != nil {
		t.Fatalf("an unrun check, not passing: %v", got)
	}
	f = holding()
	f.HeadAfter, f.Deps, f.Verify = f.HeadBefore, map[string]bool{"x": false}, "bogus"
	want := []string{InvCommit, InvDeps, InvVerifyStatus}
	if got := Invariants(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("three broken: %v, want %v", got, want)
	}
	if !listed(Invariant, StopReasons) || !listed(Invariant, SittingFailures) || Invariant != "invariant" {
		t.Fatal("invariant is a sitting failure and a night stop")
	}
}
