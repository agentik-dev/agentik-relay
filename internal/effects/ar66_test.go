package effects

import (
	"strings"
	"testing"
)

// TestAR66_4_PauseNotice (AR66 ruling 5): the `pause` line is a priority 3
// notice, so quiet hours keep it, whose body holds rows, kind, resets_at,
// wake and count, in that order, and no other key of the line. Red on the
// pin: `pause` refused.
func TestAR66_4_PauseNotice(t *testing.T) {
	n, ok := Notices("pause", map[string]any{"night": "2026-10-11-1", "rows": []any{"r1", "r2"}, "kind": "window",
		"resets_at": "2026-10-11T02:00:00Z", "source": "event", "wake": "2026-10-11T02:02:00Z", "count": 1.0, "ts": "2026-10-11T01:00:00Z"})
	if !ok {
		t.Fatal("pause refused")
	}
	want := "rows=r1,r2 kind=window resets_at=2026-10-11T02:00:00Z wake=2026-10-11T02:02:00Z count=1"
	if n.Priority != 3 || n.Body != want || n.Title != "relay 2026-10-11-1 pause" {
		t.Fatalf("notice %+v; want priority 3, body %q", n, want)
	}
	var keys []string
	for _, f := range strings.Fields(n.Body) {
		k, _, _ := strings.Cut(f, "=")
		keys = append(keys, k)
	}
	if strings.Join(keys, " ") != "rows kind resets_at wake count" {
		t.Fatalf("the body's keys %v", keys)
	}
}
