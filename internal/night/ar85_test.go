package night_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// An orchestrator sitting asks the band as `relay-orchestrator`, every
// other part as `relay-row` (fresh-session-r1 §2.5; band-pacing-r1 §2.2;
// AR85): nothing else in the request changes.

// ar85One is band's one request, checked whole: the type want, the
// spawn's model, background, not a fork, and the tool use id
// `<night>/<row>/1`.
func ar85One(t *testing.T, band *effects.FakeBand, night, row, model, want string) {
	t.Helper()
	reqs := band.Requests()
	if len(reqs) != 1 || len(band.Asks()) != 1 {
		t.Fatalf("requests %+v, asks %v: want one", reqs, band.Asks())
	}
	exp := effects.BandRequest{Model: model, SubagentType: want, Background: true, Fork: false, ToolUseID: night + "/" + row + "/1"}
	if reqs[0] != exp {
		t.Fatalf("the request %+v, want %+v", reqs[0], exp)
	}
}

// TestAR85_1_BuildRowAsksAsRelayRow (ruling 1): one build row, admitted;
// its one request is `relay-row`.
func TestAR85_1_BuildRowAsksAsRelayRow(t *testing.T) {
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {{BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
	w, q, o, sp, _ := ar80World(t, band, "r1")
	ar65End(t, ar71Run(w, q, o, sp))
	spawn := ar80One(t, w.ledger.Written(), "spawn", "r1")
	ar85One(t, band, "2026-09-22-1", "r1", fmt.Sprint(spawn["model"]), "relay-row")
}

// TestAR85_2_OrchestratorSittingAsksAsRelayOrchestrator (ruling 1):
// TestAR32A_4's orchestrated night, the standing row's first sitting o.1
// DONE, admitted; its one request is `relay-orchestrator`.
func TestAR85_2_OrchestratorSittingAsksAsRelayOrchestrator(t *testing.T) {
	w, q, files := orchNight(t, "- act 1: the row", nil)
	sit(t, files, "o.1", 1, "DONE: the row is built", nil)
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"o.1": {{BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
	w.fx.Band = band
	o := w.orchOpts(t, q, files)
	o.Band = "/bin/agentik-session"
	n := runNight(t, w, q, o)
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green"}) || n.Reason != relay.Done {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	spawn := ar80One(t, w.ledger.Written(), "spawn", "o.1")
	ar85One(t, band, "2026-09-29-1", "o.1", fmt.Sprint(spawn["model"]), "relay-orchestrator")
}
