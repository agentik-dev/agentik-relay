package night

import (
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// TestAR32B_3_LandedThroughOrchestrator is ar32b act 3 (orchestrator r1
// §6), in real git: a row merged --no-ff into sitting o.1 that itself
// landed on main is landed and DepSatisfied; one merged into sitting o.2,
// not landed, is not; nor is one merged into a landed branch that is no
// sitting's (x, o.x), nor one read with no orchestrator row.
func TestAR32B_3_LandedThroughOrchestrator(t *testing.T) {
	root, run := landedRepo(t)
	work := func(branch, from string) {
		run("checkout", "-q", "-b", branch, from)
		run("commit", "-q", "--allow-empty", "-m", branch+"'s work")
	}
	into := func(branch string, rows ...string) {
		run("checkout", "-q", branch)
		for _, r := range rows {
			run("merge", "-q", "--no-ff", "-m", "merge "+r, r)
		}
	}
	for _, b := range []string{"b1", "b2", "b3", "b4"} {
		work(b, "main")
	}
	work("o.1", "main")
	into("o.1", "b1")
	work("o.2", "main")
	into("o.2", "b2")
	work("x", "main")
	into("x", "b3")
	work("o.x", "main")
	into("o.x", "b4")
	into("main", "o.1", "x", "o.x")
	q := &queue.Queue{Rows: []queue.Row{{ID: "o", Kind: queue.KindOrchestrator}, {ID: "b1"}, {ID: "b2"}, {ID: "b3"}, {ID: "b4"}}}
	g := effects.OSGit{}
	if !landed(g, root, "main", "b1", "o") || !DepSatisfied(g, root, q, "b1") {
		t.Error("b1, merged into the landed sitting o.1, is not landed")
	}
	if landed(g, root, "main", "b1", "") || DepSatisfied(g, root, nil, "b1") {
		t.Error("b1 landed with no orchestrator row")
	}
	for _, id := range []string{"b2", "b3", "b4"} {
		if landed(g, root, "main", id, "o") || DepSatisfied(g, root, q, id) {
			t.Errorf("%s landed: its merge is on no landed sitting's first-parent line", id)
		}
	}
	if !landed(g, root, "main", "o.1", "o") {
		t.Error("o.1 itself, merged on main, is not landed")
	}
}
