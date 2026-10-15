package stream

import (
	"strings"
	"testing"
)

// TestAR86_1_ResultOutput: the exit line's output is the result's, not the
// assistant events' opening count: real.jsonl's result reads 41 where its
// assistant events read 4.
func TestAR86_1_ResultOutput(t *testing.T) {
	tl := parseFile(t, "testdata/real.jsonl")
	if tl.Usage.Output != 41 {
		t.Fatalf("usage.output %d, want 41 (the result's)", tl.Usage.Output)
	}
	if tl.Result == nil || tl.Result.Usage == nil || tl.Result.Usage.Output != 41 {
		t.Fatalf("the result's usage %+v, want output 41", tl.Result)
	}
}

// TestAR86_2_TwoResultsAndBuilders: two results' output sums (AR83: per
// query) and the builders' streamed output adds to it; input and the caches
// stay the stream's. With no result, or a result with no usage, the output
// is the stream's, as on the pin.
func TestAR86_2_TwoResultsAndBuilders(t *testing.T) {
	msg := func(parent, id string, out int) string {
		p := "null"
		if parent != "" {
			p = `"` + parent + `"`
		}
		return `{"type":"assistant","parent_tool_use_id":` + p + `,"message":{"id":"` + id + `","content":[],"usage":{"input_tokens":1,"cache_read_input_tokens":2,"cache_creation_input_tokens":3,"output_tokens":` + itoa(out) + `}}}`
	}
	result := func(out int) string {
		return `{"type":"result","subtype":"success","num_turns":1,"total_cost_usd":0.5,"usage":{"input_tokens":1000,"cache_read_input_tokens":2000,"cache_creation_input_tokens":3000,"output_tokens":` + itoa(out) + `}}`
	}
	stream := []string{msg("", "m1", 5), msg("toolu_b", "b1", 10)}
	cases := []struct {
		name  string
		lines []string
		want  Usage
	}{
		{"two results", append(append([]string{}, stream...), result(100), result(30)), Usage{Input: 2, CacheRead: 4, CacheCreate: 6, Output: 140}},
		{"no result", stream, Usage{Input: 2, CacheRead: 4, CacheCreate: 6, Output: 15}},
		{"a result without usage", append(append([]string{}, stream...), `{"type":"result","subtype":"success","num_turns":1}`), Usage{Input: 2, CacheRead: 4, CacheCreate: 6, Output: 15}},
	}
	for _, c := range cases {
		tl, err := Parse(strings.NewReader(strings.Join(c.lines, "\n")+"\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if tl.Usage != c.want {
			t.Fatalf("%s: usage %+v, want %+v", c.name, tl.Usage, c.want)
		}
	}
}
