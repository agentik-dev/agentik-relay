package stream

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// fixtures are the fake claude's, read in place so the parser and the fake
// agree on one set.
const fixtures = "../effects/testdata/fakeclaude/fixtures"

func parseFile(t *testing.T, path string) Tally {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tl, err := Parse(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func TestParseFixtures(t *testing.T) {
	cases := []struct {
		path          string
		turns         int
		result, isErr bool
		subtype       string
		denials       int
	}{
		{filepath.Join(fixtures, "ok.jsonl"), 3, true, false, "success", 0},
		{filepath.Join(fixtures, "error.jsonl"), 1, true, true, "error_during_execution", 1},
		{filepath.Join(fixtures, "noresult.jsonl"), 2, false, false, "", 0},
		{filepath.Join(fixtures, "over.jsonl"), 10, true, false, "success", 0},
		// The one real call (§2): haiku, one turn, CLI 2.1.280; its one
		// message arrives on two lines with one id.
		{"testdata/real.jsonl", 1, true, false, "success", 0},
	}
	for _, c := range cases {
		t.Run(filepath.Base(c.path), func(t *testing.T) {
			tl := parseFile(t, c.path)
			if tl.Turns != c.turns || tl.ResultSeen != c.result || tl.NonJSON != 0 {
				t.Fatalf("turns %d result %v nonjson %d; want %d %v 0", tl.Turns, tl.ResultSeen, tl.NonJSON, c.turns, c.result)
			}
			if tl.SessionID == "" || tl.Model == "" {
				t.Fatalf("init not read: session %q model %q", tl.SessionID, tl.Model)
			}
			if !c.result {
				return
			}
			r := tl.Result
			if r.Subtype != c.subtype || r.IsError != c.isErr || r.Denials != c.denials || r.NumTurns == 0 || r.DurationMS == 0 {
				t.Fatalf("result %+v", *r)
			}
			if tl.CostUSD <= 0 || tl.Context <= 0 {
				t.Fatalf("cost %v context %d", tl.CostUSD, tl.Context)
			}
		})
	}
	if tl := parseFile(t, "testdata/real.jsonl"); tl.Model != "claude-haiku-4-5-20251001" || tl.Result.ResultTail != "ok" {
		t.Fatalf("real: model %q tail %q", tl.Model, tl.Result.ResultTail)
	}
}

func TestTurnDedup(t *testing.T) {
	in := `{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"a"}],"usage":{"input_tokens":1,"cache_creation_input_tokens":2,"cache_read_input_tokens":3,"output_tokens":4}}}
{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}],"usage":{"input_tokens":1,"cache_creation_input_tokens":2,"cache_read_input_tokens":3,"output_tokens":4}}}
{"type":"assistant","message":{"id":"m2","content":[],"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":30,"output_tokens":4}}}
`
	var seen []int
	tl, err := Parse(strings.NewReader(in), func(_ Event, tl Tally) { seen = append(seen, tl.Turns) })
	if err != nil || tl.Turns != 2 || tl.Context != 60 || tl.LastTool != "Bash" {
		t.Fatalf("%+v %v", tl, err)
	}
	if len(seen) != 3 || seen[1] != 1 {
		t.Fatalf("onEvent turns %v", seen)
	}
}

func TestBuildersCounted(t *testing.T) {
	tl := parseFile(t, filepath.Join(fixtures, "ok.jsonl"))
	if len(tl.Builders) != 1 || tl.Builders["toolu_task1"] != 1 {
		t.Fatalf("builders %v", tl.Builders)
	}
	// A builder's message counts neither as a turn nor toward the context.
	if tl.Turns != 3 || tl.Context != 71010 {
		t.Fatalf("turns %d context %d", tl.Turns, tl.Context)
	}
}

func TestTallyLine(t *testing.T) {
	tl := parseFile(t, filepath.Join(fixtures, "ok.jsonl"))
	want := `turn 3: Bash "git commit -m act"; context 71k/130k; $1.20`
	if got := tl.Line(130000); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if got := (Tally{}).Line(130000); got != "turn 0: -; context 0k/130k; unread" {
		t.Fatalf("empty: %s", got)
	}
	long := `{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","name":"Task","input":{"prompt":"secret words","description":"d"}},{"type":"tool_use","name":"Bash","input":{"command":"` + strings.Repeat("x", 200) + `"}}]}}`
	ev, err := Decode([]byte(long))
	if err != nil || ev.Tools[0].Summary != "" || len(ev.Tools[1].Summary) != SummaryCap {
		t.Fatalf("summaries %+v %v", ev.Tools, err)
	}
}

func TestNonJSONTolerated(t *testing.T) {
	in := "Warning: no stdin\n{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"s\",\"model\":\"m\"}\n{not json\n{}\n\n{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"ok\"}"
	tl, err := Parse(strings.NewReader(in), nil)
	if err != nil || tl.NonJSON != 3 || !tl.ResultSeen || tl.SessionID != "s" {
		t.Fatalf("%+v %v", tl, err)
	}
}

func TestResultTailCapped(t *testing.T) {
	body := strings.Repeat("é", 300) + "THE END"
	ev, err := Decode([]byte(`{"type":"result","subtype":"success","result":"` + body + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.ResultTail) > TailCap || !strings.HasSuffix(ev.ResultTail, "THE END") || !strings.HasPrefix(ev.ResultTail, "é") {
		t.Fatalf("tail %d bytes: %q", len(ev.ResultTail), ev.ResultTail[:10])
	}
}

// TestLiveLineUnderCap (UX1): a long summary, quotes and a multi-byte rune
// among them, is cut with `...` so the line stays under LineCap columns.
func TestLiveLineUnderCap(t *testing.T) {
	tl := Tally{Turns: 120, LastTool: "Bash", LastSummary: strings.Repeat(`é"x`, 40), Context: 129000, CostUSD: 14.5, Result: &Event{}}
	l := tl.Line(130000)
	if n := utf8.RuneCountInString(l); n >= LineCap || !strings.Contains(l, `..."; context 129k/130k; $14.50`) {
		t.Fatalf("%d columns: %s", n, l)
	}
}

// TestTallyFresh: a message streamed as two events is fresh once; a result
// is never fresh.
func TestTallyFresh(t *testing.T) {
	var tl Tally
	var fresh []bool
	for _, ev := range []Event{{Type: "assistant", MessageID: "m1"}, {Type: "assistant", MessageID: "m1"},
		{Type: "assistant", MessageID: "m2"}, {Type: "result"}} {
		tl.Add(ev)
		fresh = append(fresh, tl.Fresh)
	}
	if fmt.Sprint(fresh) != "[true false true false]" {
		t.Fatalf("fresh %v", fresh)
	}
}
