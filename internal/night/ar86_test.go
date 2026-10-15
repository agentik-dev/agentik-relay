package night_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// ar86Lines is AR81's report fixture shape: a night-open line, then lines
// of night n, each its event, row and keys.
func ar86Lines(kv ...[]any) []map[string]any {
	const n = "2026-10-15-1"
	lines := []map[string]any{{"event": "night-open", "night": n, "root": "/r", "ts": "2026-10-15T08:00:00Z"}}
	for _, l := range kv {
		m := map[string]any{"event": l[0], "night": n, "row": l[1]}
		for i := 2; i+1 < len(l); i += 2 {
			m[l[i].(string)] = l[i+1]
		}
		lines = append(lines, m)
	}
	return lines
}

// TestAR86_3_ReportOpenHold: a held line with no end line is an open hold,
// counted in Open, OpenSince its held line's ts as the ledger wrote it; the
// band line says how many are open and since when.
func TestAR86_3_ReportOpenHold(t *testing.T) {
	const ts = "2026-10-15T08:30:41Z"
	held := func(row, at string) []any {
		return []any{"band", row, "part", 1, "state", "held", "verdict", "not-started", "peak", 115.0, "text", "no room", "ts", at}
	}
	r := night.BuildReport(ar86Lines(held("r1", ts)), "2026-10-15-1")
	if b, _ := json.Marshal(r.Band); string(b) != `{"holds":1,"wait_s":0,"open":1,"open_since":"`+ts+`"}` {
		t.Fatalf("the report's band %s", b)
	}
	if md := string(night.Markdown(r, "/r", "/h")); !strings.Contains(md, "\nband: 1 hold, 0s waited, 1 open since "+ts+"\n") {
		t.Fatalf("markdown:\n%s", md)
	}
	two := night.BuildReport(ar86Lines(held("r1", ts), held("r1", "2026-10-15T08:31:00Z"), held("r2", "2026-10-15T08:40:00Z")), "2026-10-15-1")
	if b, _ := json.Marshal(two.Band); string(b) != `{"holds":2,"wait_s":0,"open":2,"open_since":"`+ts+`"}` {
		t.Fatalf("two open holds: the report's band %s", b)
	}
}

// TestAR86_4_ClosedHoldUnchanged: a hold its admitted line closed reads as
// on the pin: no open count, no open_since, no `open` on the band line.
func TestAR86_4_ClosedHoldUnchanged(t *testing.T) {
	r := night.BuildReport(ar86Lines(
		[]any{"band", "r1", "part", 1, "state", "held", "verdict", "not-started", "peak", 115.0, "text", "no room", "ts", "2026-10-15T08:30:41Z"},
		[]any{"band", "r1", "part", 1, "state", "admitted", "asks", 3, "wait_s", 120, "ts", "2026-10-15T08:32:41Z"},
	), "2026-10-15-1")
	if b, _ := json.Marshal(r.Band); string(b) != `{"holds":1,"wait_s":120}` {
		t.Fatalf("the report's band %s", b)
	}
	md := string(night.Markdown(r, "/r", "/h"))
	if !strings.Contains(md, "\nband: 1 hold, 2m0s waited\n") || strings.Contains(md, "open") {
		t.Fatalf("markdown:\n%s", md)
	}
}
