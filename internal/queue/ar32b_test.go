package queue

import (
	"strings"
	"testing"
)

// TestAR32B_4_OrchestratorMergesOnGreen is ar32b act 4 (orchestrator r1
// §2, §6): over branch-only defaults, an orchestrator row that names no
// merge merges on green and validates, while the build rows keep the
// defaults' branch-only; one naming branch-only is still a problem.
func TestAR32B_4_OrchestratorMergesOnGreen(t *testing.T) {
	head := strings.Replace(orchHead, "merge: merge-on-green", "merge: branch-only", 1)
	build := "  - {id: a3-machine, packet: design/packets/a3-machine.md}\n"
	q, err := loadAR32A(t, head+orchRow+build)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := q.Orchestrator()
	if !ok || o.Merge != MergeOnGreen || len(q.Validate()) != 0 {
		t.Fatalf("the standing row merges %q, problems %v; want merge-on-green, none", o.Merge, q.Validate())
	}
	if b := q.Rows[1]; b.Merge != BranchOnly {
		t.Errorf("the build row merges %q; want the defaults' branch-only", b.Merge)
	}
	q, err = loadAR32A(t, head+orchRow+"    merge: branch-only\n"+build)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, p := range q.Validate() {
		found = found || p.Row == "o" && strings.Contains(p.Msg, "an orchestrator row merges merge-on-green")
	}
	if !found {
		t.Errorf("a standing row naming branch-only: problems %v; want its merge named", q.Validate())
	}
}
