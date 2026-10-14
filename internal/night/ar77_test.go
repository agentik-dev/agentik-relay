package night_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// After a usage-limit wake, status and report read the row's new part
// (AR77; usage-limit-resume r1 §5 packet 6, its status and report half).

// ar77Row is r1's row in the night's status.json, and the status.
func ar77Row(o night.NightOptions) (night.Status, night.RowStatus) {
	st := night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night)
	for _, r := range st.Rows {
		if r.ID == "r1" {
			return st, r
		}
	}
	return st, night.RowStatus{}
}

// TestAR77_1_StatusReadsThePart (AR77 rulings 1 and 2): on TestAR67_3's
// "elapsed while part 2 runs" world under --max-sittings 5, paused r1
// reads its part 1 (paused, usage-limit, 40 turns, $11.50); at the wake,
// part 2 held, it reads part 2's: running, no reason, every count zero,
// no resume, its elapsed from part 1's start, and `next` counts the rows
// staged (1) while the burn counts the parts (2); released, part 2's
// green, 3 turns, $1.25.
func TestAR77_1_StatusReadsThePart(t *testing.T) {
	w, q, o, _, clock := ar65World(t, nil, "r1")
	sp := ar67New(true, nil)
	sp.hold2 = make(chan struct{})
	o.MaxSittings = 5
	res := ar67Night(w, q, o, sp)
	var bad []string
	ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
	if _, r := ar77Row(o); r.Reason != "usage-limit" || r.Turns != 40 || r.CostUSD != 11.5 {
		bad = append(bad, fmt.Sprintf("paused: r1 %s %q, %d turns, $%.2f; want paused usage-limit, 40, $11.50", r.State, r.Reason, r.Turns, r.CostUSD))
	}
	clock.Advance(time.Hour + 2*time.Minute)
	ar65Until(t, "part 2's spawn", func() bool { return len(sp.r1.Spawns()) == 2 })
	clock.Advance(10 * time.Minute)
	var st night.Status
	var r night.RowStatus
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if st, r = ar77Row(o); r.State == "running" && r.Elapsed == "1h12m0s" {
			break
		}
	}
	close(sp.hold2)
	n := ar65End(t, res)
	got := fmt.Sprintf("%s %q turns %d context %d $%.2f unread %v tool %q builders %d cause %q resume %q elapsed %s",
		r.State, r.Reason, r.Turns, r.Context, r.CostUSD, r.Unread, r.LastTool, r.Builders, r.Cause, r.Resume, r.Elapsed)
	if want := `running "" turns 0 context 0 $0.00 unread false tool "" builders 0 cause "" resume "" elapsed 1h12m0s`; got != want {
		bad = append(bad, "during part 2: r1 "+got+"; want "+want)
	}
	if !strings.Contains(st.Next, "count-reached after 5 rows staged (1 so far)") || st.Burn.Sittings != 2 {
		bad = append(bad, fmt.Sprintf("during part 2: next %q, burn %d sittings; want count-reached after 5 rows staged (1 so far), 2", st.Next, st.Burn.Sittings))
	}
	if _, r := ar77Row(o); n.State != relay.Stopped(relay.QueueEmpty) || r.State != "green" || r.Reason != "" || r.Turns != 3 || r.CostUSD != 1.25 {
		bad = append(bad, fmt.Sprintf("the end: night %s, r1 %s %q, %d turns, $%.2f; want stopped:queue-empty, green, 3, $1.25", n.State, r.State, r.Reason, r.Turns, r.CostUSD))
	}
	if len(bad) > 0 {
		t.Fatal(strings.Join(bad, "\n"))
	}
}

// ar77Line is a ledger line of night n: event, row ("" none), then key
// and value pairs.
func ar77Line(n, event, row string, kv ...any) map[string]any {
	m := map[string]any{"night": n, "event": event}
	if row != "" {
		m["row"] = row
	}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

// ar77Paused is row's `sitting` line the night paused on (§2's sitting
// line): failed, usage-limit, limit paused, its turns, read or unread.
func ar77Paused(n, row string, turns int, read bool) map[string]any {
	return ar77Line(n, "sitting", row, "state", "failed", "reason", "usage-limit", "limit", "paused",
		"turns", turns, "context", 0, "cost_usd", 0.0, "cost_read", read, "gate", map[string]any{"cause": ""})
}

// ar77Report is the report's verdict in one string: each stopped row
// (row, state, reason, resume), the resume lines, the waiting rows and
// the burn's states.
func ar77Report(r night.Report) string {
	var stopped, burn []string
	for _, s := range r.Stopped.Rows {
		stopped = append(stopped, s.Row+" "+s.State+" "+s.Reason+" "+s.Resume)
	}
	for _, b := range r.Sittings {
		burn = append(burn, b.State)
	}
	return fmt.Sprintf("stopped %q resume %q waiting %q burn %v", stopped, r.Resume, r.Waiting, burn)
}

// TestAR77_2_ReportWhilePaused (AR77 rulings 3 and 4): r1 spawned and r2
// not (killed before its spawn), each a paused `sitting` line, then the
// `pause` line: no stopped row, no resume line, nothing waiting, the burn
// `paused paused`, and the Markdown neither stopped short nor failed. A
// `stop` line naming r1 alone halts both: r1 failed usage-limit with its
// resume line, r2 (a sitting the wake halted) failed with none, the burn
// `failed failed`, the Resume block r1's line once; the stop line's resume
// read back ([]any) or in memory ([]string) alike.
func TestAR77_2_ReportWhilePaused(t *testing.T) {
	const n = "2026-10-14-9"
	lines := []map[string]any{
		ar77Line(n, "night-open", ""),
		ar77Line(n, "stage", "r1", "branch", "r1", "base", "main"),
		ar77Line(n, "spawn", "r1", "turns", 70),
		ar77Line(n, "stage", "r2", "branch", "r2", "base", "main"),
		ar77Line(n, "exit", "r1", "turns", 1, "context", 0, "cost_usd", 0.0, "cost_read", true, "reason", "usage-limit"),
		ar77Paused(n, "r1", 1, true),
		ar77Line(n, "kill", "r2", "reason", "usage-limit"),
		ar77Line(n, "pause", "", "rows", []any{"r1", "r2"}, "count", 1, "kind", "window"),
		ar77Paused(n, "r2", 0, false),
	}
	r := night.BuildReport(lines, n)
	if got, want := ar77Report(r), `stopped [] resume [] waiting [] burn [paused paused]`; got != want {
		t.Fatalf("paused: %s; want %s", got, want)
	}
	if md := string(night.Markdown(r, "", "")); !strings.Contains(md, "0 stopped short") || strings.Contains(md, "failed") {
		t.Fatalf("paused, the Markdown:\n%s", md)
	}
	for _, resume := range []any{[]any{relay.ResumeLine("r1")}, []string{relay.ResumeLine("r1")}} {
		stop := ar77Line(n, "stop", "", "state", "stopped:owner-stop", "reason", "owner-stop", "resume", resume)
		r = night.BuildReport(append(append([]map[string]any{}, lines...), stop), n)
		want := fmt.Sprintf("stopped [%q %q] resume [%q] waiting [] burn [failed failed]",
			"r1 failed usage-limit "+relay.ResumeLine("r1"), "r2 failed usage-limit ", relay.ResumeLine("r1"))
		if got := ar77Report(r); got != want {
			t.Fatalf("halted (resume %T): %s; want %s", resume, got, want)
		}
		md := string(night.Markdown(r, "", ""))
		block := md[strings.Index(md, "## Resume"):strings.Index(md, "## Burn")]
		if !strings.Contains(md, "2 stopped short") || strings.Count(block, relay.ResumeLine("r1")) != 1 || strings.Contains(block, relay.ResumeLine("r2")) {
			t.Fatalf("halted, the Markdown:\n%s", md)
		}
	}
}

// TestAR77_3_ReportAfterTheWake (AR77 rulings 3 and 4): night
// 2026-10-13-7's lines (§1) to 07:33:40Z, its three rows paused, woken
// and spawned again: no stopped row, no resume line, the burn three parts
// paused then three running, `6 sittings (3 in flight)` and no `failed`.
// Then its part 2 verdicts and its stop line: po2fix and us3afix red
// verify-cut with one resume line each, po1bfix green and waiting, the
// burn `paused paused paused red red green`.
func TestAR77_3_ReportAfterTheWake(t *testing.T) {
	const n = "2026-10-13-7"
	sitting := func(row, state, reason string, turns int) map[string]any {
		return ar77Line(n, "sitting", row, "state", state, "reason", reason, "part", 2, "turns", turns,
			"context", 100000, "cost_usd", 1.0, "cost_read", true, "gate", map[string]any{"cause": ""})
	}
	lines := []map[string]any{
		ar77Line(n, "night-open", ""),
		ar77Line(n, "stage", "po1bfix", "branch", "po1bfix", "base", "main"),
		ar77Line(n, "spawn", "po1bfix", "turns", 70),
		ar77Line(n, "stage", "po2fix", "branch", "po2fix", "base", "main"),
		ar77Line(n, "stage", "us3afix", "branch", "us3afix", "base", "main"),
		ar77Line(n, "exit", "po1bfix", "turns", 1, "context", 0, "cost_usd", 0.0, "cost_read", true, "reason", "usage-limit"),
		ar77Paused(n, "po1bfix", 1, true),
		ar77Line(n, "kill", "po2fix", "reason", "usage-limit"),
		ar77Line(n, "kill", "us3afix", "reason", "usage-limit"),
		ar77Line(n, "pause", "", "rows", []any{"po1bfix", "po2fix", "us3afix"}, "count", 1, "kind", "window"),
		ar77Paused(n, "po2fix", 0, false),
		ar77Paused(n, "us3afix", 0, false),
		ar77Line(n, "wake", "", "rows", []any{"po1bfix", "po2fix", "us3afix"}, "count", 1),
		ar77Line(n, "spawn", "po1bfix", "part", 2, "turns", 69),
		ar77Line(n, "spawn", "po2fix", "part", 2, "turns", 75),
		ar77Line(n, "spawn", "us3afix", "part", 2, "turns", 70),
		ar77Line(n, "turn", "po2fix", "part", 2, "turns", 10, "context", 61225),
	}
	r := night.BuildReport(lines, n)
	if got, want := ar77Report(r), `stopped [] resume [] waiting [] burn [paused paused paused running running running]`; got != want {
		t.Fatalf("at 07:33:40Z: %s; want %s", got, want)
	}
	if md := string(night.Markdown(r, "", "")); !strings.Contains(md, "6 sittings (3 in flight)") || strings.Contains(md, "failed") {
		t.Fatalf("at 07:33:40Z, the Markdown:\n%s", md)
	}
	lines = append(lines,
		ar77Line(n, "exit", "us3afix", "part", 2, "turns", 36, "context", 101000, "cost_usd", 1.0, "cost_read", true),
		ar77Line(n, "exit", "po2fix", "part", 2, "turns", 46, "context", 112503, "cost_usd", 1.0, "cost_read", true),
		ar77Line(n, "exit", "po1bfix", "part", 2, "turns", 33, "context", 101979, "cost_usd", 1.0, "cost_read", true),
		sitting("po2fix", "red", "verify-cut", 46), sitting("us3afix", "red", "verify-cut", 36), sitting("po1bfix", "green", "", 33),
		ar77Line(n, "stop", "", "state", "stopped:gate-failed-dependency", "reason", "gate-failed-dependency",
			"resume", []any{relay.ResumeLine("po2fix"), relay.ResumeLine("us3afix")}))
	r = night.BuildReport(lines, n)
	want := fmt.Sprintf("stopped [%q %q] resume [%q %q] waiting [\"po1bfix (off main)\"] burn [paused paused paused red red green]",
		"po2fix red verify-cut "+relay.ResumeLine("po2fix"), "us3afix red verify-cut "+relay.ResumeLine("us3afix"),
		relay.ResumeLine("po2fix"), relay.ResumeLine("us3afix"))
	if got := ar77Report(r); got != want {
		t.Fatalf("the end: %s; want %s", got, want)
	}
	if !reflect.DeepEqual(r.Merged, []night.Merged{}) {
		t.Fatalf("merged %v; want none", r.Merged)
	}
}
