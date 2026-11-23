package main

import (
	"strings"
	"testing"
)

// TestAK8W_1_RespokenSummaries (ak8 owed-1, R1, R2): the two rewritten
// summaries, byte for byte, in process through run(), and no setting's
// summary says "child" or "read by no code".
func TestAK8W_1_RespokenSummaries(t *testing.T) {
	ak8Setup(t, true)
	code, m, _, _ := ak8Run(t, "describe", "config", "--json")
	if code != exitOK {
		t.Fatalf("exit %d: %v", code, m)
	}
	set := ak8Settings(t, m)
	want := map[string]string{
		"allowed_tools": "The tools a row's sitting may use without asking, passed to Claude Code as written.",
		"merge":         "Not read by this tool: each row's own merge decides how its branch lands.",
	}
	for key, w := range want {
		if got, _ := set[key]["summary"].(string); got != w {
			t.Errorf("%s: summary %q, want %q", key, got, w)
		}
	}
	for key, s := range set {
		sum, _ := s["summary"].(string)
		for _, word := range []string{"child", "read by no code"} {
			if strings.Contains(sum, word) {
				t.Errorf("%s: summary %q holds %q", key, sum, word)
			}
		}
	}
}
