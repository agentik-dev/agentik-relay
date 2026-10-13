package queue

import (
	"strings"
	"testing"
)

// AR2-5: §7's ceilings are armed only when positive; a row with a zero or
// negative turns, context, usd or wall budget is invalid.
func TestAR2_5_BudgetsPositive(t *testing.T) {
	q := load(t, "valid.yaml")
	if ps := q.Validate(); len(ps) != 0 {
		t.Fatalf("valid.yaml: %v", ps)
	}
	for i := range q.Rows {
		if q.Rows[i].Parked == "" {
			q.Rows[i].Turns, q.Rows[i].Context, q.Rows[i].USD, q.Rows[i].Wall = -1, 0, 0, -1
			break
		}
	}
	got := ""
	for _, p := range q.Validate() {
		got += p.Msg + ";"
	}
	for _, want := range []string{"turns", "context", "usd", "wall"} {
		if !strings.Contains(got, want) {
			t.Errorf("a %s budget of zero or less validates: %q", want, got)
		}
	}
}
