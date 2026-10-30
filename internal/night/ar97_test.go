package night_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR97_1_HeldLineNamesLever (AR97 ruling 1): a hold's log line names
// its verdict, its peak when the band gives one, and the owner's lever;
// a `denied` hold's notice ends with the lever too (band-pacing-r1 §6).
// The held line is the row's log's (`launch` sets RunOptions.Log), so it is
// read there, as status_test reads it.
func TestAR97_1_HeldLineNamesLever(t *testing.T) {
	const lever = "band: off in the relay config stops the hold"
	p180, p131 := 180.0, 131.0
	ad := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "admitted"}}
	for _, c := range []struct {
		held       effects.FakeAnswer
		line, note string
	}{
		{effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started", Text: "no room", Peak: &p180}},
			"relay r1: the band holds it: not-started (peak 180%); " + lever, ""},
		{effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "wait", Text: "the window"}},
			"relay r1: the band holds it: wait; " + lever, ""},
		{effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "denied", Text: "the week", Peak: &p131}},
			"relay r1: the band holds it: denied (peak 131%); " + lever,
			"verdict=denied peak=131; move accounts, or " + lever},
	} {
		band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {c.held, ad}}}
		w, q, o, sp, clock := ar80World(t, band, "r1")
		notes := &effects.FakeNotify{}
		o.Notify = notes
		res := ar71Run(w, q, o, sp)
		ar80Ask(t, band, 1, clock, 60*time.Second)
		n := ar65End(t, res)
		log, err := os.ReadFile(night.RowLog(o.State, n.ID, "r1"))
		if err != nil || !strings.Contains(string(log), c.line+"\n") {
			t.Errorf("%s: the held line is not %q in the row's log (%v)\n%s", c.held.Verdict, c.line, err, log)
		}
		var bodies []string
		for _, n := range notes.Notices() {
			if n.Event == "band" {
				bodies = append(bodies, n.Body)
			}
		}
		if want := []string{c.note}; c.note == "" && len(bodies) != 0 || c.note != "" && strings.Join(bodies, "|") != strings.Join(want, "|") {
			t.Errorf("%s: band notices %q, want %q", c.held.Verdict, bodies, c.note)
		}
	}
}
