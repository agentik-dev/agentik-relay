package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestAR17A_0_HandGateMainMoved: `relay gate` reads the stage line's
// base_sha; a base not there now is red, main-moved (ADV10-1).
func TestAR17A_0_HandGateMainMoved(t *testing.T) {
	var out, errb bytes.Buffer
	_, l := gateFakes(t, true)
	l.Append("stage", map[string]any{"row": "a3-machine", "worktree": "/wt", "branch": "a3-machine",
		"base": "ar1-relay-hooks", "head_before": "h0", "base_sha": "before"})
	code := run([]string{"gate", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb)
	var rep struct{ Result, Reason string }
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if code != exitRed || rep.Result != "red" || rep.Reason != "main-moved" {
		t.Fatalf("gate a3-machine = %d, %s", code, out.String())
	}
}
