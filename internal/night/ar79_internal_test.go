package night

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestAR79_2_TextLine (AR79 rulings 1, 3): statusNow at 00:02:00Z,
// PrintStatus over r1 running with `wait` `verify behind h0 since
// 23:59:00Z` (the day before: 3 m) and r2 green with none: r1's line ends
// `verify waits behind h0 3m`, one cell past its merge, under no header;
// r2's ends in its merge cell; the header keeps its ten cells; a wait that
// does not parse prints as it is. A held row (AR81) never meets the cell:
// the band holds before the spawn, the gate lock after the child.
// json.Marshal(RowStatus{}) holds `"wait":""`.
func TestAR79_2_TextLine(t *testing.T) {
	was := statusNow
	t.Cleanup(func() { statusNow = was })
	statusNow = func() time.Time { return time.Date(2026, 10, 14, 0, 2, 0, 0, time.UTC) }
	s := Status{Night: "2026-10-14-1", State: "running", Rows: []RowStatus{
		{ID: "r1", State: "running", Wait: "verify behind h0 since 23:59:00Z"},
		{ID: "r2", State: "green", MergeSHA: "0123456789abcdef"},
		{ID: "r3", State: "running", Wait: "verify behind h0 at noon"},
	}}
	var b bytes.Buffer
	PrintStatus(&b, s)
	lines := map[string]string{}
	for _, l := range strings.Split(b.String(), "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			lines[f[0]] = l
		}
	}
	if !strings.HasSuffix(lines["r1"], "  verify waits behind h0 3m") {
		t.Errorf("r1's line %q; want it to end `verify waits behind h0 3m`", lines["r1"])
	}
	if !strings.HasSuffix(lines["r2"], "  "+short("0123456789abcdef")) {
		t.Errorf("r2's line %q; want it to end in its merge cell", lines["r2"])
	}
	if !strings.HasSuffix(lines["r3"], "  verify behind h0 at noon") {
		t.Errorf("r3's line %q; want its wait as it is", lines["r3"])
	}
	if h := regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(lines["row"]), -1); len(h) != 10 || h[9] != "merge" {
		t.Errorf("the header %q; want its ten cells, merge last", h)
	}
	j, err := json.Marshal(RowStatus{})
	if err != nil || !strings.Contains(string(j), `"wait":""`) {
		t.Errorf("json.Marshal(RowStatus{}) %s, %v; want it to hold \"wait\":\"\"", j, err)
	}
}
