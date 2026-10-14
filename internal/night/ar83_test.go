package night_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR83_3_SittingLine (AR83 rulings 2 to 5): a child over
// two-results.jsonl, a second turn after a background command ended. The
// `exit` and `sitting` lines read the fold: 6 turns, 48500 ms, 2 denials,
// $1.25, no error; the end is the second result's; the gate is green. The
// live log still prints a line per result, each at the cost it read.
func TestAR83_3_SittingLine(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	var log bytes.Buffer
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Transcripts: w.transcripts, Env: w.env("two-results.jsonl"), Log: &log})
	lines := w.ledger.Written()
	for _, ev := range []string{"exit", "sitting"} {
		l := lastOf(lines, ev)
		if l["turns"] != float64(6) || l["duration_ms"] != float64(48500) || l["denials"] != float64(2) || l["cost_usd"] != 1.25 || l["is_error"] != false {
			t.Fatalf("the %s line: turns %v, duration_ms %v, denials %v, cost_usd %v, is_error %v; want 6, 48500, 2, 1.25, false",
				ev, l["turns"], l["duration_ms"], l["denials"], l["cost_usd"], l["is_error"])
		}
	}
	if exit := lastOf(lines, "exit"); exit["subtype"] != "success" || exit["result_tail"] != "the wait ended" {
		t.Fatalf("the exit line's end: subtype %v, result_tail %v; want success, the second result's", exit["subtype"], exit["result_tail"])
	}
	if g, _ := lastOf(lines, "sitting")["gate"].(map[string]any); g["result"] != "green" {
		t.Fatalf("the gate %v; want green", g)
	}
	// The priced lines: the first result's (turn 3), msg_04's held line
	// (printed as the second result arrives) and the second result's.
	var priced []string
	for _, l := range strings.Split(strings.TrimSpace(log.String()), "\n") {
		if strings.HasPrefix(l, "turn ") && !strings.HasSuffix(l, "unread") {
			priced = append(priced, l)
		}
	}
	if len(priced) != 3 || !strings.HasPrefix(priced[0], "turn 3: ") || !strings.HasSuffix(priced[0], "; $1.20") ||
		!strings.HasPrefix(priced[2], "turn 4: ") || !strings.HasSuffix(priced[2], "; $1.25") {
		t.Fatalf("the live log's priced lines %q; want the first result's at turn 3 and $1.20, the second's at turn 4 and $1.25\n%s", priced, log.String())
	}
}

// ar83Burn is the report's Burn table row for row: its turns and cost cells.
func ar83Burn(md, row string) (turns, cost string) {
	i, j := strings.Index(md, "\n## Burn"), strings.Index(md, "\n## Handovers")
	if i < 0 || j < i {
		return "", ""
	}
	for _, l := range strings.Split(md[i:j], "\n") {
		c := strings.Split(strings.Trim(strings.TrimSpace(l), "|"), "|")
		if len(c) > 4 && strings.TrimSpace(c[0]) == row {
			return strings.TrimSpace(c[2]), strings.TrimSpace(c[4])
		}
	}
	return "", ""
}

// TestAR83_4_StatusReportCap (AR83 rulings 2, 3 and 5): three rows over
// the fixture under --max-usd 2. Each sitting costs $1.25, the running
// total, so r1 leaves room and r2 reaches the cap; a summed cost ($2.45)
// would stop after r1. Status, the report and the stop line read the fold.
func TestAR83_4_StatusReportCap(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "two-results.jsonl"},
		spec{id: "r2", echo: "ok", fixture: "two-results.jsonl"}, spec{id: "r3", echo: "ok", fixture: "two-results.jsonl"})
	o := w.opts(t, by)
	o.MaxUSD = 2
	n := runNight(t, w, q, o)
	lines := w.ledger.Written()
	spawns := 0
	for _, l := range lines {
		if l["event"] == "spawn" {
			spawns++
		}
	}
	if spawns != 2 || n.Reason != relay.BudgetReached {
		t.Fatalf("spawns %d, reason %s (states %s); want 2, %s", spawns, n.Reason, states(n), relay.BudgetReached)
	}
	if stop := lastOf(lines, "stop"); stop["cost_usd"] != 2.5 || stop["spent"] != 2.5 {
		t.Fatalf("the stop line's cost_usd %v, spent %v; want 2.5, 2.5", stop["cost_usd"], stop["spent"])
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	seen := 0
	for _, r := range st.Rows {
		if r.ID == "r1" || r.ID == "r2" {
			seen++
			if r.Turns != 6 || r.CostUSD != 1.25 {
				t.Fatalf("status's %s: %d turns, $%.2f; want 6, $1.25", r.ID, r.Turns, r.CostUSD)
			}
		}
	}
	if seen != 2 || st.Burn.Sittings != 2 || st.Burn.Turns != 12 || st.Burn.CostUSD != 2.5 {
		t.Fatalf("status: rows %+v, burn %+v; want r1 and r2, 2 sittings, 12 turns, $2.50", st.Rows, st.Burn)
	}
	r := night.BuildReport(lines, n.ID)
	if got := totalOf(r); got != st.Burn {
		t.Fatalf("the report's totals %+v, status's burn %+v", got, st.Burn)
	}
	md := string(night.Markdown(r, "", ""))
	for _, c := range [][3]string{{"r1", "6", "$1.25"}, {"r2", "6", "$1.25"}, {"total", "12", "$2.50"}} {
		if turns, cost := ar83Burn(md, c[0]); turns != c[1] || cost != c[2] {
			t.Fatalf("the report's burn row %s: %q turns, %q; want %s, %s\n%s", c[0], turns, cost, c[1], c[2], md)
		}
	}
}
