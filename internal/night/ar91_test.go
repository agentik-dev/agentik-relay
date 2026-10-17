package night_test

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR91_2_RecoveredGreen (AR91 rulings 1, 2 and 5): a child over
// recovered.jsonl, an API error on its first result and a success on its
// second. The child ended in the success: the `exit` line reads not
// is_error, success, both results and one erred, the fold's turns and
// cost; the gate is green.
func TestAR91_2_RecoveredGreen(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Transcripts: w.transcripts, Env: w.env("recovered.jsonl")})
	lines := w.ledger.Written()
	exit := lastOf(lines, "exit")
	if exit["is_error"] != false || exit["subtype"] != "success" || exit["results"] != float64(2) || exit["errors"] != float64(1) ||
		exit["turns"] != float64(6) || exit["cost_usd"] != 1.25 {
		t.Fatalf("the exit line: is_error %v, subtype %v, results %v, errors %v, turns %v, cost_usd %v; want false, success, 2, 1, 6, 1.25",
			exit["is_error"], exit["subtype"], exit["results"], exit["errors"], exit["turns"], exit["cost_usd"])
	}
	if g, _ := lastOf(lines, "sitting")["gate"].(map[string]any); g["result"] != "green" {
		t.Fatalf("the gate %v; want green", g)
	}
}

// TestAR91_3_NightRunsOn (AR91 rulings 3 and 5): a recovered row no longer
// stops the night; a row whose last result erred still does (D19). r1
// recovered, r2 ok, r3 error.jsonl, r4 ok: r1 and r2 green, r3 failed,
// r4 skipped, the night stopped:stream-error, r3's exit line one erred.
func TestAR91_3_NightRunsOn(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "recovered.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"},
		spec{id: "r3", echo: "ok", fixture: "error.jsonl"}, spec{id: "r4", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	if n.State != relay.Stopped(relay.StreamError) || states(n) != "r1=green r2=green r3=failed r4=skipped" {
		t.Fatalf("night %s: %s; want stopped:stream-error, r1=green r2=green r3=failed r4=skipped", n.State, states(n))
	}
	exits := rowLines(w.ledger.Written(), "exit", "r3")
	if len(exits) != 1 || exits[0]["errors"] != float64(1) {
		t.Fatalf("r3's exit lines %v; want one, its errors 1", exits)
	}
}
