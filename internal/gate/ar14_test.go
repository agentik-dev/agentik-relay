package gate

import (
	"reflect"
	"testing"
)

// TestAR14_4_GuardedPathsNeedNaming: a diff touching a GuardedPaths name is
// inside the touch set only when a may-touch span names it exactly; `*`, a
// directory span or the RULES paths never admit it (S10's handover; §6).
func TestAR14_4_GuardedPathsNeedNaming(t *testing.T) {
	both := []string{".hygiene-allow", "design/readme/rendered.sha256"}
	if !reflect.DeepEqual(GuardedPaths[:2], both) { // AR17A act 2 adds the harness after them
		t.Fatalf("GuardedPaths = %q, want %q", GuardedPaths, both)
	}
	for _, c := range []struct {
		may     []string
		outside []string
	}{
		{[]string{"design/", "*"}, both},
		{[]string{"design/readme/*", ".*"}, both},
		{both, []string{}},
	} {
		w := newWorld(t)
		st := w.stage(t, append([]string{"history/sittings/2026-09-23-1.md"}, both...)...)
		_, _, g, _ := Run(w.fx, st, pk(c.may), nil, seen, Verify{})
		if !reflect.DeepEqual(g.Outside, c.outside) {
			t.Errorf("may touch %q: outside %q, want %q", c.may, g.Outside, c.outside)
		}
	}
}
