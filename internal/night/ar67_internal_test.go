package night

import (
	"reflect"
	"testing"
)

// TestAR67_2_Budgets (AR67 ruling 2): the continuation's turns and dollars
// are the queue row's less its parts' tonight, at least 10 turns and $5,
// the dollars rounded down; an unread part counts at its spawn line's usd;
// continueArgv changes only the three values.
func TestAR67_2_Budgets(t *testing.T) {
	sitting := func(turns int, usd float64, read bool) map[string]any {
		return map[string]any{"event": "sitting", "row": "r1", "turns": turns, "cost_usd": usd, "cost_read": read}
	}
	spawn := func(usd int) map[string]any { return map[string]any{"event": "spawn", "row": "r1", "usd": usd} }
	for _, c := range []struct {
		name       string
		parts      []map[string]any
		turns, usd int
	}{
		{"40 turns, $11.5", []map[string]any{spawn(15), sitting(40, 11.5, true)}, 60, 5},
		{"95 turns", []map[string]any{spawn(15), sitting(95, 1, true)}, 10, 14},
		{"an unread part at its usd", []map[string]any{spawn(9), sitting(20, 0, false)}, 80, 6},
		{"two parts", []map[string]any{spawn(15), sitting(30, 2.25, true), spawn(12), sitting(10, 0, false)}, 60, 5},
		{"none", nil, 100, 15},
	} {
		if turns, usd := remainder(100, 15, c.parts); turns != c.turns || usd != c.usd {
			t.Errorf("%s: remainder %d turns $%d, want %d $%d", c.name, turns, usd, c.turns, c.usd)
		}
	}
	argv := []string{"claude", "-p", "--model", "claude-opus-5-5", "--max-turns", "100", "--max-budget-usd", "15",
		"--output-format", "stream-json", "--verbose", "--session-id", "old"}
	keep := append([]string(nil), argv...)
	got := continueArgv(argv, "new", 60, 5)
	want := []string{"claude", "-p", "--model", "claude-opus-5-5", "--max-turns", "60", "--max-budget-usd", "5",
		"--output-format", "stream-json", "--verbose", "--session-id", "new"}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(argv, keep) {
		t.Fatalf("continueArgv %q (the plan's now %q), want %q and the plan's untouched", got, argv, want)
	}
}
