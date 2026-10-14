package stream

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR83_1_TwoResults (AR83 rulings 1 to 3 and 6): a child that takes one
// more turn after a background command ends writes two result events. The
// tally folds both: num_turns, duration_ms and the denials summed, the cost
// the running total the last read, the end the second's.
func TestAR83_1_TwoResults(t *testing.T) {
	tl := parseFile(t, filepath.Join(fixtures, "two-results.jsonl"))
	r := tl.Result
	if r == nil || !tl.ResultSeen {
		t.Fatalf("no result read: %+v", tl)
	}
	if r.NumTurns != 6 || r.DurationMS != 48500 || r.Denials != 2 {
		t.Fatalf("the fold: num_turns %d, duration_ms %d, denials %d; want 6, 48500, 2", r.NumTurns, r.DurationMS, r.Denials)
	}
	if r.CostUSD != 1.25 || tl.CostUSD != 1.25 {
		t.Fatalf("the cost: result %v, tally %v; want 1.25 in both (the running total, never a sum)", r.CostUSD, tl.CostUSD)
	}
	if r.IsError || r.Subtype != "success" || r.ResultTail != "the wait ended" {
		t.Fatalf("the end: is_error %v, subtype %q, tail %q; want false, success, the second's", r.IsError, r.Subtype, r.ResultTail)
	}
	if tl.Turns != 4 || tl.NonJSON != 0 {
		t.Fatalf("the tally: turns %d, non-JSON %d; want 4, 0", tl.Turns, tl.NonJSON)
	}
	if l := tl.Line(130000); !strings.HasSuffix(l, "; $1.25") {
		t.Fatalf("the live line %q; want it to end `; $1.25`", l)
	}
}

// ar83Result is one result event's line.
func ar83Result(subtype string, isErr bool, turns int, usd float64, ms int64, denials int, text, session string) string {
	d := make([]string, denials)
	for i := range d {
		d[i] = `{"tool_name":"Bash"}`
	}
	return fmt.Sprintf(`{"type":"result","subtype":%q,"is_error":%v,"num_turns":%d,"total_cost_usd":%v,"duration_ms":%d,"permission_denials":[%s],"result":%q,"session_id":%q}`,
		subtype, isErr, turns, usd, ms, strings.Join(d, ","), text, session)
}

func ar83Parse(t *testing.T, lines ...string) Tally {
	t.Helper()
	tl, err := Parse(strings.NewReader(strings.Join(lines, "\n")+"\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Result == nil || tl.NonJSON != 0 {
		t.Fatalf("result %v, non-JSON %d", tl.Result, tl.NonJSON)
	}
	return tl
}

// TestAR83_2_FoldEdges (AR83 rulings 1, 2 and 4), through Parse: one result
// reads as before the fold; a lower later cost keeps the higher; an error
// before or after a success is kept, its subtype the last erring one's, the
// tail and session the last result's; three results' turns sum.
func TestAR83_2_FoldEdges(t *testing.T) {
	one := ar83Result("success", false, 3, 1.2, 45000, 1, "act landed", "s1")
	want, err := Decode([]byte(one))
	if err != nil {
		t.Fatal(err)
	}
	want.Raw = nil
	if tl := ar83Parse(t, one); fmt.Sprintf("%+v", *tl.Result) != fmt.Sprintf("%+v", want) || tl.CostUSD != 1.2 {
		t.Fatalf("one result: %+v, cost %v; want %+v, 1.2", *tl.Result, tl.CostUSD, want)
	}

	tl := ar83Parse(t, ar83Result("success", false, 3, 1.2, 1000, 0, "a", "s1"), ar83Result("success", false, 1, 0.05, 1000, 0, "b", "s1"))
	if tl.Result.CostUSD != 1.2 || tl.CostUSD != 1.2 {
		t.Fatalf("$0.05 after $1.20: result %v, tally %v; want 1.2", tl.Result.CostUSD, tl.CostUSD)
	}

	tl = ar83Parse(t, ar83Result("error_max_turns", true, 100, 9, 1000, 0, "cut", "s1"),
		ar83Result("success", false, 1, 9.1, 1000, 0, "the wait ended", "s2"))
	if r := tl.Result; !r.IsError || r.Subtype != "error_max_turns" || r.ResultTail != "the wait ended" || r.SessionID != "s2" {
		t.Fatalf("error_max_turns then success: is_error %v, subtype %q, tail %q, session %q; want true, error_max_turns, the success's, s2",
			r.IsError, r.Subtype, r.ResultTail, r.SessionID)
	}

	tl = ar83Parse(t, ar83Result("success", false, 3, 1, 1000, 0, "a", "s1"), ar83Result("error_during_execution", true, 1, 1.1, 1000, 0, "boom", "s1"))
	if r := tl.Result; !r.IsError || r.Subtype != "error_during_execution" || r.ResultTail != "boom" {
		t.Fatalf("success then error_during_execution: is_error %v, subtype %q, tail %q", r.IsError, r.Subtype, r.ResultTail)
	}

	tl = ar83Parse(t, ar83Result("success", false, 2, 1, 1000, 0, "a", "s1"), ar83Result("success", false, 1, 1.1, 1000, 0, "b", "s1"),
		ar83Result("success", false, 1, 1.2, 1000, 0, "c", "s1"))
	if r := tl.Result; r.NumTurns != 4 || r.DurationMS != 3000 || r.ResultTail != "c" {
		t.Fatalf("three results of 2, 1 and 1 turns: num_turns %d, duration_ms %d, tail %q; want 4, 3000, c", r.NumTurns, r.DurationMS, r.ResultTail)
	}
}
