package effects

import (
	"strings"
	"testing"
)

// TestAR32B_4_StopNoticeNamesUnmerged is ar32b act 4 (orchestrator r1
// §6): the stop's notice names each build row green on its branch and
// unmerged, from the stop line as the ledger reads it back, and names none
// when the list is empty.
func TestAR32B_4_StopNoticeNamesUnmerged(t *testing.T) {
	for _, u := range []any{[]string{"b1", "b2"}, []any{"b1", "b2"}} {
		n, ok := Notices("stop", map[string]any{"night": "2026-09-29-1", "reason": "count-reached", "state": "stopped",
			"sittings": 3, "red": 0, "unmerged": u, "root": "/r", "detail": "SECRET"})
		if !ok || !strings.Contains(n.Body, "unmerged=b1,b2") || strings.Contains(n.Body, "SECRET") || strings.Contains(n.Body, "/r") {
			t.Errorf("stop notice over %T = %+v", u, n)
		}
	}
	if n, _ := Notices("stop", map[string]any{"reason": "done", "unmerged": []string{}}); strings.Contains(n.Body, "unmerged") {
		t.Errorf("an empty list is named: %q", n.Body)
	}
}
