package night_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR31_2_NightNamesLog is AR31 act 2: a red row whose gate kept a log
// prints `relay <row>: log: <path>` between its red line and its resume
// line, where the owner reads the night; a green row that kept none
// prints none. r1's gate is the real one, gathered, turned red with its
// line's log.
func TestAR31_2_NightNamesLog(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		p, ro, err := row(r, rp)
		if gate := ro.Gate; r.ID == "r1" {
			ro.Gate = func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
				_, _, g := gate(st, seen, isError)
				if len(g.Lines) > 0 {
					g.Lines[0].Status, g.Lines[0].BranchLog = relay.CheckRed, "/x/r1-line1.log"
				}
				return relay.RedResult, "", g
			}
		}
		return p, ro, err
	}
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, w, q, o)
	if states(n) != "r1=red r2=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	got := out.String()
	if !strings.Contains(got, "relay r1: red\nrelay r1: log: /x/r1-line1.log\nagentik relay resume r1\n") {
		t.Errorf("no r1 log line between its red line and its resume line in:\n%s", got)
	}
	if !strings.Contains(got, "relay r2: green\n") || strings.Contains(got, "relay r2: log:") {
		t.Errorf("r2, green with no log:\n%s", got)
	}
}
