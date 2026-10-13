package night_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR32BFix_StopDuringMergingSitting is ar32b review round 1 (F1): the
// owner's `relay stop` arrives while o.2, which merges b, runs. The night
// drains, o.2 lands, and b, on main through o.2's merge, takes that merge
// as its MergeSHA before the stop's early return, so the stop line names
// no row unmerged.
func TestAR32BFix_StopDuringMergingSitting(t *testing.T) {
	w, q, files := ar32bNight(t, "- act 1")
	sit(t, files, "o.1", 1, "- act 2: merge b", addsB())
	fakeFile(t, files, "b/a.txt", "b's work\n")
	fakeFile(t, files, "o.2.merge", "b\n")
	sit(t, files, "o.2", 2, "- act 3: more", nil)
	o := w.orchOpts(t, q, files)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := row(r, rp)
		if rp.ID == "o.2" { // the stop is read while o.2 runs, slowed so polls see it
			ro.Env = append(ro.Env, "FAKECLAUDE_DELAY_MS=300")
			if werr := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, "2026-09-29-1"), "stop"); werr != nil {
				t.Error(werr)
			}
		}
		return b, ro, err
	}
	n := runNight(t, w, q, o)
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "b=green", "o.2=green"}) || n.Reason != relay.OwnerStop {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	o2, b := n.Sittings["o.2"].MergeSHA, n.Sittings["b"].MergeSHA
	if o2 == "" || o2 != gitIn(t, w.root, "rev-parse", "main") {
		t.Fatalf("o.2's MergeSHA %q; want main", o2)
	}
	if b != o2 {
		t.Errorf("b's MergeSHA %q; want o.2's merge %q, which put it on main", b, o2)
	}
	if stop := stopOf(t, w); fmt.Sprint(stop["unmerged"]) != "[]" {
		t.Errorf("stop names unmerged %v; b is on main through o.2", stop["unmerged"])
	}
}
