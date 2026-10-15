package night_test

import (
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// ar80Green is ar71's spawner over w: every child clean, its tree
// committed first, so each row goes green and the night runs on.
func ar80Green(w *world) *ar71Spawner {
	return &ar71Spawner{w: w, n: map[string]int{}, first: map[string]effects.FakeRun{}, until: map[string]func() bool{}}
}

// TestAR80_13_QueuedHeadStaysHeld (AR80 fix round, finding 3): at
// --parallel 2, r1 held twice then admitted and green, r2 queued; r3,
// launched at +120 s behind r2 (now the head, its own hold open), writes
// its queued line then, asks at +240 s and spawns after a 120 s hold.
func TestAR80_13_QueuedHeadStaysHeld(t *testing.T) {
	ns := effects.FakeAnswer{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {ns, ns, {BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
	w, q, o, _, clock := ar80World(t, band, "r1", "r2", "r3")
	o.Parallel = 2
	res := ar71Run(w, q, o, ar80Green(w))
	ar65Until(t, "r2's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 1 })
	ar80Ask(t, band, 1, clock, 60*time.Second)
	ar80Ask(t, band, 2, clock, 120*time.Second)
	ar65Until(t, "r3's queued line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r3")) == 1 })
	ar69To(clock, ar65T.Add(180*time.Second))
	ar65Until(t, "r2's admitted line", func() bool { return len(rowLines(w.ledger.Written(), "band", "r2")) == 2 })
	ar69To(clock, ar65T.Add(240*time.Second))
	ar65End(t, res)
	all := w.ledger.Written()
	if got := ar80Asks(band); got != "r1 +0s, r1 +60s, r1 +120s, r2 +180s, r3 +240s" {
		t.Fatalf("asks %s", got)
	}
	r3 := rowLines(all, "band", "r3")[0]
	if got := ar80Band(t, all, "r3"); got != "held queued; admitted 1/120s" || r3["text"] != "behind r2" || r3["ts"] != ar65T.Add(120*time.Second).Format(time.RFC3339) {
		t.Fatalf("r3's band lines %s, the first %v", got, r3)
	}
}

// ar80Slow is the ledger, r1's held band line appended 200 ms late.
type ar80Slow struct{ *effects.FakeLedger }

func (l *ar80Slow) Append(event string, m map[string]any) error {
	if event == "band" && m["row"] == "r1" && m["state"] == "held" {
		time.Sleep(200 * time.Millisecond)
	}
	return l.FakeLedger.Append(event, m)
}

// TestAR80_14_HeadLineFirst (AR80 fix round, finding 4): r1's held line
// lands before r2's queued line, even when r1's append is slow.
func TestAR80_14_HeadLineFirst(t *testing.T) {
	band := &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{"r1": {{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}}}}
	w, q, o, sp, _ := ar80World(t, band, "r1", "r2")
	o.Parallel = 2
	w.fx.Ledger = &ar80Slow{w.ledger}
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "both held lines", func() bool { return len(ar65Events(w.ledger.Written(), "band")) == 2 })
	if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), "stop"); err != nil {
		t.Fatal(err)
	}
	ar65End(t, res)
	var rows []any
	for _, l := range ar65Events(w.ledger.Written(), "band") {
		rows = append(rows, l["row"])
	}
	if len(rows) != 2 || rows[0] != "r1" || rows[1] != "r2" {
		t.Fatalf("the band lines' rows %v, want r1 then r2", rows)
	}
}
