package night_test

import (
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR10_7_InvariantStopsNight: r1's gate says green with no commit
// while the child moved HEAD (commit-moves-head broken): the sitting line
// carries `stop: invariant` and `invariant: commit-moves-head`, r1 is
// failed invariant, the night stops `stopped:invariant` and r2 never runs.
func TestAR10_7_InvariantStopsNight(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		p, ro, err := row(r, rp)
		if r.ID == "r1" {
			ro.Gate = func(*night.Staged, bool, bool, night.GateWait) (relay.Result, relay.StopReason, relay.Gate) {
				return relay.GreenResult, "", relay.Gate{Gathered: true, ResultSeen: true, ExitOK: true,
					Commit: false, Clean: true, OriginSame: true}
			}
		}
		return p, ro, err
	}
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.Invariant) || n.Sittings["r1"].State != relay.Failed ||
		n.Sittings["r1"].Reason != relay.Invariant || n.Sittings["r2"].State == relay.Green {
		t.Fatalf("night %s: %s; r1 %+v", n.State, states(n), n.Sittings["r1"])
	}
	var sit map[string]any
	for _, l := range w.ledger.Written() {
		if l["event"] == "sitting" && l["row"] == "r1" {
			sit = l
		}
	}
	if sit["stop"] != "invariant" || sit["invariant"] != relay.InvCommit || sit["state"] != "failed" {
		t.Fatalf("sitting line %v", sit)
	}
}
