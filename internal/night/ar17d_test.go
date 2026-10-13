package night_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The account's stop (AR17D act 0, run 10 ruling 1): a usage limit or an
// expired login stops the whole night with its own reason, never the gate.

const (
	usageText = "You've hit your weekly limit · resets Sep 28 at 4pm (America/Los_Angeles)"
	authText  = "Invalid API key · Please run /login"
)

// rowLines is every ledger line of event for row.
func rowLines(lines []map[string]any, event, row string) []map[string]any {
	var out []map[string]any
	for _, l := range lines {
		if l["event"] == event && l["row"] == row {
			out = append(out, l)
		}
	}
	return out
}

// accountStops runs r1 whose result text is text beside an independent r2:
// the night stops want, r1 failed with it before any gate, r2 never spawned.
func accountStops(t *testing.T, text string, want relay.StopReason) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_RESULT=" + text}},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	if n.State != relay.Stopped(want) || states(n) != "r1=failed r2=skipped" || n.Sittings["r1"].Reason != want {
		t.Fatalf("night %s: %s (r1 %s)", n.State, states(n), n.Sittings["r1"].Reason)
	}
	all := w.ledger.Written()
	if g := rowLines(all, "gate", "r1"); len(g) != 0 {
		t.Errorf("the gate ran on an account stop: %v", g)
	}
	if ex := rowLines(all, "exit", "r1"); len(ex) != 1 || ex[0]["reason"] != string(want) {
		t.Errorf("exit lines %v, want one with reason %s", ex, want)
	}
	if sp := rowLines(all, "spawn", "r2"); len(sp) != 0 {
		t.Errorf("r2 spawned after the account stop")
	}
}

func TestAR17D_0_UsageLimitStopsNight(t *testing.T) { accountStops(t, usageText, relay.UsageLimit) }

func TestAR17D_0_AuthStopsNight(t *testing.T) { accountStops(t, authText, relay.Auth) }

// TestAR17D_0_ParallelRowsKilled: under --parallel 2, r1's usage limit
// kills r2 mid-sitting (TERM within KillGrace), r2 reads failed usage-limit
// with a kill line naming the reason, and r3 is never spawned.
func TestAR17D_0_ParallelRowsKilled(t *testing.T) {
	w, q, by := nightWorld(t,
		spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_RESULT=" + usageText, "FAKECLAUDE_DELAY_MS=50"}},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_DELAY_MS=4000"}},
		spec{id: "r3", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	o.Parallel = 2
	start := time.Now()
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.UsageLimit) || states(n) != "r1=failed r2=failed r3=skipped" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	if r := n.Sittings["r2"].Reason; r != relay.UsageLimit {
		t.Fatalf("r2 reason %s, want usage-limit", r)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("the night took %s: r2 was not killed", d.Round(time.Second))
	}
	all := w.ledger.Written()
	if k := rowLines(all, "kill", "r2"); len(k) != 1 || k[0]["reason"] != "usage-limit" {
		t.Errorf("kill lines %v", k)
	}
	if ex := rowLines(all, "exit", "r2"); len(ex) != 1 || ex[0]["reason"] != "usage-limit" {
		t.Errorf("r2 exit lines %v", ex)
	}
	if g := rowLines(all, "gate", "r2"); len(g) != 0 {
		t.Errorf("the gate ran on the killed row: %v", g)
	}
	if sp := rowLines(all, "spawn", "r3"); len(sp) != 0 {
		t.Errorf("r3 spawned after the account stop")
	}
}

// TestAR17D_0_UncommittedKept: the failing child's uncommitted file stays in
// its worktree, untracked, for the next attempt.
func TestAR17D_0_UncommittedKept(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_RESULT=" + authText}})
	n := runNight(t, w, q, w.opts(t, by))
	if n.State != relay.Stopped(relay.Auth) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	wt := n.Plan.Rows[0].Worktree
	if b, err := os.ReadFile(filepath.Join(wt, "acts", "r1-wip.txt")); err != nil || string(b) != "wip\n" {
		t.Fatalf("the wip file: %q, %v", b, err)
	}
	if st := gitIn(t, wt, "status", "--porcelain"); !strings.Contains(st, "?? acts/") {
		t.Fatalf("status %q, want acts/ untracked", st)
	}
	if night.Clean(n) {
		t.Fatalf("an account stop reads clean")
	}
}

// TestAR17D_0_StaleSkipsAccountStop: `relay clean --stale` keeps a worktree
// whose row's last exit line carries usage-limit or auth, and only then.
func TestAR17D_0_StaleSkipsAccountStop(t *testing.T) {
	lines := []map[string]any{
		{"event": "exit", "row": "a", "reason": "usage-limit"},
		{"event": "exit", "row": "b", "reason": "auth"},
		{"event": "exit", "row": "c", "reason": "auth"},
		{"event": "exit", "row": "c", "reason": "wall-clock"},
		{"event": "exit", "row": "d"},
		{"event": "sitting", "row": "e", "stop": "auth"},
	}
	for row, want := range map[string]relay.StopReason{"a": relay.UsageLimit, "b": relay.Auth, "c": "", "d": "", "e": ""} {
		if got := night.AccountKept(lines, row); got != want {
			t.Errorf("row %s: %q, want %q", row, got, want)
		}
	}
}
