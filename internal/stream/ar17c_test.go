package stream

import (
	"strings"
	"testing"
)

// TestAR17C_4_LongLineBounded (AR17C, ADV10-23): a 2 MiB line, an event
// that would parse, between two valid events is past MaxLine: counted in
// NonJSON as a line that is not JSON, never folded in; both events either
// side are parsed.
func TestAR17C_4_LongLineBounded(t *testing.T) {
	big := `{"type":"assistant","message":{"id":"big","content":[{"type":"text","text":"` + strings.Repeat("x", 2<<20) + `"}]}}`
	in := `{"type":"system","subtype":"init","session_id":"s","model":"m"}` + "\n" + big + "\n" +
		`{"type":"result","subtype":"success","result":"ok"}` + "\n"
	var seen []string
	tl, err := Parse(strings.NewReader(in), func(ev Event, _ Tally) { seen = append(seen, ev.Type) })
	if err != nil || tl.NonJSON != 1 || tl.Turns != 0 || tl.SessionID != "s" || !tl.ResultSeen {
		t.Fatalf("nonjson %d turns %d session %q result %v err %v; want 1 0 s true nil",
			tl.NonJSON, tl.Turns, tl.SessionID, tl.ResultSeen, err)
	}
	if strings.Join(seen, ",") != "system,result" {
		t.Fatalf("events %v, want system,result", seen)
	}
}
