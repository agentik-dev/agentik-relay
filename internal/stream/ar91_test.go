package stream

import (
	"path/filepath"
	"testing"
)

// TestAR91_1_LastResult (AR91 rulings 1, 2 and 4): the child's end is its
// last result's, IsError and Subtype with the tail, so an erring result
// followed by a success reads the success; an erring cap (`error_max…`)
// holds through every later result, its subtype with it. Errors counts the
// result events read with is_error true, beside Results.
func TestAR91_1_LastResult(t *testing.T) {
	tl := ar83Parse(t, ar83Result("error_during_execution", true, 3, 1, 1000, 0, "API Error: 529 overloaded_error", "s1"),
		ar83Result("success", false, 1, 1.1, 1000, 0, "the wait ended", "s1"))
	if r := tl.Result; r.IsError || r.Subtype != "success" || r.ResultTail != "the wait ended" || tl.Errors != 1 || tl.Results != 2 {
		t.Fatalf("error_during_execution then success: is_error %v, subtype %q, tail %q, errors %d, results %d; want false, success, the success's, 1, 2",
			r.IsError, r.Subtype, r.ResultTail, tl.Errors, tl.Results)
	}

	tl = ar83Parse(t, ar83Result("success", false, 3, 1, 1000, 0, "a", "s1"), ar83Result("error_during_execution", true, 1, 1.1, 1000, 0, "boom", "s1"))
	if r := tl.Result; !r.IsError || r.Subtype != "error_during_execution" || tl.Errors != 1 {
		t.Fatalf("success then error_during_execution: is_error %v, subtype %q, errors %d; want true, error_during_execution, 1", r.IsError, r.Subtype, tl.Errors)
	}

	tl = ar83Parse(t, ar83Result("error_max_turns", true, 100, 9, 1000, 0, "cut", "s1"), ar83Result("success", false, 1, 9.1, 1000, 0, "the wait ended", "s1"))
	if r := tl.Result; !r.IsError || r.Subtype != "error_max_turns" || tl.Errors != 1 {
		t.Fatalf("error_max_turns then success: is_error %v, subtype %q, errors %d; want true, error_max_turns (the cap kept), 1", r.IsError, r.Subtype, tl.Errors)
	}

	tl = ar83Parse(t, ar83Result("error_during_execution", true, 2, 1, 1000, 0, "a", "s1"), ar83Result("error_during_execution", true, 1, 1.1, 1000, 0, "b", "s1"),
		ar83Result("success", false, 1, 1.2, 1000, 0, "c", "s1"))
	if r := tl.Result; r.IsError || r.Subtype != "success" || tl.Errors != 2 || tl.Results != 3 {
		t.Fatalf("two errors then a success: is_error %v, subtype %q, errors %d, results %d; want false, success, 2, 3", r.IsError, r.Subtype, tl.Errors, tl.Results)
	}

	for f, want := range map[string]int{"ok.jsonl": 0, "two-results.jsonl": 0, "error.jsonl": 1} {
		if tl := parseFile(t, filepath.Join(fixtures, f)); tl.Errors != want {
			t.Fatalf("%s: errors %d; want %d", f, tl.Errors, want)
		}
	}

	// recovered.jsonl: two-results.jsonl with its first result an API error.
	tl = parseFile(t, filepath.Join(fixtures, "recovered.jsonl"))
	r := tl.Result
	if r == nil || r.IsError || r.Subtype != "success" || r.ResultTail != "the wait ended" {
		t.Fatalf("recovered.jsonl's end: %+v; want not is_error, success, the second's tail", r)
	}
	if r.NumTurns != 6 || r.DurationMS != 48500 || r.Denials != 2 || r.CostUSD != 1.25 {
		t.Fatalf("recovered.jsonl's fold: num_turns %d, duration_ms %d, denials %d, cost %v; want 6, 48500, 2, 1.25", r.NumTurns, r.DurationMS, r.Denials, r.CostUSD)
	}
	if tl.Errors != 1 || tl.Results != 2 || tl.Account != "" {
		t.Fatalf("recovered.jsonl's tally: errors %d, results %d, account %q; want 1, 2, none", tl.Errors, tl.Results, tl.Account)
	}
}
