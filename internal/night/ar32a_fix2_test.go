package night_test

import "testing"

// TestAR32AFix_StopLineNamesTheRoot is ar32a review round 2 (F2): the
// ledger is shared by every repo on the account, so the `stop` line, whose
// resume ids the day's refusal reads, names the night's root as the
// `night-open`, `sitting` and `merge` lines do.
func TestAR32AFix_StopLineNamesTheRoot(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	stops := 0
	for _, l := range w.ledger.Written() {
		if l["event"] != "stop" {
			continue
		}
		stops++
		if root, _ := l["root"].(string); root == "" || root != n.Plan.Root {
			t.Errorf("stop line root = %q; want the night's root %q", root, n.Plan.Root)
		}
	}
	if stops != 1 {
		t.Fatalf("%d stop lines, want 1", stops)
	}
}
