package main

import (
	"encoding/json"
	"testing"
)

// TestAK10C3_RunWall (ak10c3): describe's `run` read names its wall,
// "wall_s": 2.5 (commons GUIDE §11, the reads row); the config and doctor
// reads carry no such key, so their bytes do not change.
func TestAK10C3_RunWall(t *testing.T) {
	ak8Setup(t, true)
	_, _, out, _ := ak8Run(t, "describe", "--json")
	var raw struct{ Reads []json.RawMessage }
	if err := json.Unmarshal([]byte(out), &raw); err != nil || len(raw.Reads) != 3 {
		t.Fatalf("reads: %v, %d", err, len(raw.Reads))
	}
	for i, want := range []string{
		`{"kind":"config","name":"settings","argv":["describe","config","--json"]}`,
		`{"kind":"doctor","name":"doctor","argv":["doctor","--json"]}`,
		`{"kind":"status","name":"run","argv":["status","--json"],"wall_s":2.5}`,
	} {
		if got := string(raw.Reads[i]); got != want {
			t.Errorf("read %d = %s, want %s", i, got, want)
		}
	}
}
