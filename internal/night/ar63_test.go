package night_test

import (
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR63_3_ExitLineLimit (AR63, usage-limit-resume-r1 §2): a usage-limit
// stop's one exit line carries `limit`, the kind, the reset in RFC3339 in its
// zone, its source and scope, at the hit (the world's FakeClock,
// 2026-09-22T01:00Z); the night still stops usage-limit. An auth stop and
// the row it never spawned carry none.
func TestAR63_3_ExitLineLimit(t *testing.T) {
	const session = "You've hit your session limit · resets 1:30am (America/Los_Angeles)"
	accountStops(t, session, relay.UsageLimit)
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_RESULT=" + session}},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	n := runNight(t, w, q, w.opts(t, by))
	if n.State != relay.Stopped(relay.UsageLimit) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	ex := rowLines(w.ledger.Written(), "exit", "r1")
	if len(ex) != 1 {
		t.Fatalf("exit lines %v, want one", ex)
	}
	lm, ok := ex[0]["limit"].(map[string]any)
	if !ok {
		t.Fatalf("exit line %v holds no limit", ex[0])
	}
	want := map[string]string{"kind": "window", "resets_at": "2026-09-22T01:30:00-07:00", "source": "text", "scope": ""}
	for k, v := range want {
		if lm[k] != v {
			t.Errorf("limit[%s] = %v, want %q (limit %v)", k, lm[k], v, lm)
		}
	}
	if len(lm) != len(want) {
		t.Errorf("limit %v, want exactly %v", lm, want)
	}
	if ex[0]["reason"] != string(relay.UsageLimit) {
		t.Errorf("exit reason %v, want usage-limit", ex[0]["reason"])
	}

	w, q, by = nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl", env: []string{"FAKECLAUDE_RESULT=" + authText}},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	if n = runNight(t, w, q, w.opts(t, by)); n.State != relay.Stopped(relay.Auth) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	for _, l := range w.ledger.Written() {
		if _, ok := l["limit"]; ok {
			t.Errorf("an auth stop's %s line holds limit: %v", l["event"], l)
		}
	}
}
