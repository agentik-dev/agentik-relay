package stream

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// bgWait is the 2026-10-16 probe's stream on CLI 2.1.302 (AR90): a child
// that backgrounded `sleep 75`, wrote its first result, waited, took one
// more turn on the task notification and wrote a second.
var bgWait = filepath.Join(fixtures, "bg-wait.jsonl")

// TestAR90_2_RealBackgroundWait (AR90 ruling 2), a proof on the pin: the
// real wait reads through the fold as AR83 ruled it. On 2.1.302 a result's
// turns, time and usage are its own query's and its cost the running
// total, so the fold counts no query twice: 2+1 turns, 4208+1324 ms, the
// usage the two results' summed, which is the last result's `modelUsage`,
// and the cost the second's.
func TestAR90_2_RealBackgroundWait(t *testing.T) {
	tl := parseFile(t, bgWait)
	if tl.Turns != 3 || tl.NonJSON != 0 || tl.SessionID != "fb524734-4f4a-4082-bd17-ee43185daaa5" ||
		tl.Model != "claude-haiku-5-5" || tl.Account != "" {
		t.Fatalf("the tally: turns %d, non-JSON %d, session %q, model %q, account %q", tl.Turns, tl.NonJSON, tl.SessionID, tl.Model, tl.Account)
	}
	r := tl.Result
	if r == nil || !tl.ResultSeen {
		t.Fatalf("no result read: %+v", tl)
	}
	if r.NumTurns != 3 || r.DurationMS != 5532 || r.Denials != 0 {
		t.Fatalf("the fold: num_turns %d, duration_ms %d, denials %d; want 3, 5532, 0", r.NumTurns, r.DurationMS, r.Denials)
	}
	if r.CostUSD != 0.00499084 || tl.CostUSD != 0.00499084 {
		t.Fatalf("the cost: result %v, tally %v; want 0.00499084, the second result's running total", r.CostUSD, tl.CostUSD)
	}
	if r.IsError || r.Subtype != "success" || r.ResultTail != lastResultText(t) {
		t.Fatalf("the end: is_error %v, subtype %q, tail %q; want false, success, the second's text", r.IsError, r.Subtype, r.ResultTail)
	}
	want := Usage{Input: 6, CacheRead: 81224, CacheCreate: 20565, Output: 130}
	if mu := lastModelUsage(t); mu != want {
		t.Fatalf("the last result's modelUsage reads %+v; want %+v", mu, want)
	}
	if r.Usage == nil || *r.Usage != want || tl.Usage != want {
		t.Fatalf("the usage: result %+v, tally %+v; want %+v (the last modelUsage)", r.Usage, tl.Usage, want)
	}
	if tl.Context != 34417 {
		t.Fatalf("the context %d; want 34417 (2 + 578 + 33837, the last message's)", tl.Context)
	}
}

// lastResult is the fixture's last result line, decoded as a map.
func lastResult(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(bgWait)
	if err != nil {
		t.Fatal(err)
	}
	var last map[string]any
	for _, l := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(l, &m) == nil && m["type"] == "result" {
			last = m
		}
	}
	if last == nil {
		t.Fatal("no result line in the fixture")
	}
	return last
}

func lastResultText(t *testing.T) string {
	s, _ := lastResult(t)["result"].(string)
	return tail(s, TailCap)
}

// lastModelUsage is the last result's modelUsage summed over its models,
// in the tally's terms.
func lastModelUsage(t *testing.T) Usage {
	var u Usage
	mu, _ := lastResult(t)["modelUsage"].(map[string]any)
	for _, v := range mu {
		m, _ := v.(map[string]any)
		n := func(k string) int { f, _ := m[k].(float64); return int(f) }
		u = u.plus(Usage{Input: n("inputTokens"), CacheRead: n("cacheReadInputTokens"), CacheCreate: n("cacheCreationInputTokens"), Output: n("outputTokens")}, 1)
	}
	return u
}

// TestAR90_3_ResultsCount (AR90 ruling 3): the tally counts the result
// events it read, so a night's record tells a background wait's second
// query from a sitting of one.
func TestAR90_3_ResultsCount(t *testing.T) {
	for _, c := range []struct {
		file string
		want int
	}{{"noresult.jsonl", 0}, {"ok.jsonl", 1}, {"two-results.jsonl", 2}, {"bg-wait.jsonl", 2}} {
		if tl := parseFile(t, filepath.Join(fixtures, c.file)); tl.Results != c.want {
			t.Fatalf("%s: %d results; want %d", c.file, tl.Results, c.want)
		}
	}
}
