package stream

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAR28_3_UsageTotals: Usage totals every assistant message, a
// builder's too, each id once at its last usage: an id streamed twice
// counts its second usage alone.
func TestAR28_3_UsageTotals(t *testing.T) {
	msg := func(parent, id string, in, read, create, out int) string {
		p := "null"
		if parent != "" {
			p = `"` + parent + `"`
		}
		return `{"type":"assistant","parent_tool_use_id":` + p + `,"message":{"id":"` + id + `","content":[],"usage":{"input_tokens":` +
			itoa(in) + `,"cache_read_input_tokens":` + itoa(read) + `,"cache_creation_input_tokens":` + itoa(create) + `,"output_tokens":` + itoa(out) + `}}}`
	}
	src := strings.Join([]string{
		msg("", "m1", 10, 100, 1000, 5),
		msg("", "m1", 10, 100, 1000, 50), // the same message, its last usage
		msg("", "m2", 1, 2, 3, 4),
		msg("toolu_b", "m1", 7, 8, 9, 10), // a builder's, its own id
	}, "\n") + "\n"
	tl, err := Parse(strings.NewReader(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Usage{Input: 18, CacheRead: 110, CacheCreate: 1012, Output: 64}); tl.Usage != want {
		t.Fatalf("usage %+v, want %+v", tl.Usage, want)
	}
	if tl.Turns != 2 || tl.Builders["toolu_b"] != 1 {
		t.Fatalf("turns %d builders %v", tl.Turns, tl.Builders)
	}
	b, _ := json.Marshal(tl.Usage)
	if string(b) != `{"input":18,"cache_read":110,"cache_create":1012,"output":64}` {
		t.Fatalf("usage json %s", b)
	}
	if l := tl.Line(130000); !strings.HasSuffix(l, "; unread") {
		t.Fatalf("the live line before a result reads a cost: %s", l)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }
