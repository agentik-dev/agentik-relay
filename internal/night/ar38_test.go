package night

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR38_1_MeasureSecondsOrder (relay-r1 §15, §3; AR38 act 1): the
// measure's budget is the flag, then the queue's `measure_seconds`, then
// DefaultMeasureSeconds; the plan names the seconds and the source, Print
// says `measure: <s> s (<source>)`, every `plan` line carries both, and
// the budget bounds the lines (each here takes 250 s on the clock).
func TestAR38_1_MeasureSecondsOrder(t *testing.T) {
	for _, c := range []struct {
		name        string
		flag, queue int
		want        int
		from        string
		cut         bool
	}{
		{"the flag beats the queue", 600, 14400, 600, MeasureFromFlag, true},
		{"the queue beats 600", 0, 14400, 14400, MeasureFromQueue, false},
		{"neither: 600", 0, 0, DefaultMeasureSeconds, MeasureFromDefault, true},
	} {
		repo, q := fixture(t)
		q.Defaults.MeasureSeconds = c.queue
		fx, _, sh, l := fakes(repo, false)
		clock := &effects.StepClock{T: time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC)}
		fx.Clock, fx.Shell = clock, &advShell{sh: sh, clock: clock, step: 250 * time.Second}
		p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", MeasureSeconds: c.flag})
		if err != nil {
			t.Fatal(err)
		}
		if p.MeasureSeconds != c.want || p.MeasureFrom != c.from || (p.Budget > 0) != c.cut {
			t.Fatalf("%s: measure %d s from %q, %d cut; want %d s from %q, cut %v",
				c.name, p.MeasureSeconds, p.MeasureFrom, p.Budget, c.want, c.from, c.cut)
		}
		var b bytes.Buffer
		Print(&b, p)
		if line := MeasureLine(p); !strings.Contains(b.String(), "\n"+line+"\n") || !strings.HasPrefix(line, "measure: ") {
			t.Fatalf("%s: no %q in Print:\n%s", c.name, line, b.String())
		}
		lines := l.Written()
		if len(lines) == 0 {
			t.Fatalf("%s: no plan line", c.name)
		}
		for _, w := range lines {
			if number(w["measure_seconds"]) != float64(c.want) || w["measure_from"] != c.from {
				t.Fatalf("%s: plan line %v", c.name, w)
			}
		}
	}
	if got := MeasureLine(&Plan{MeasureSeconds: 3000, MeasureFrom: MeasureFromQueue}); got != "measure: 3000 s (queue)" {
		t.Fatalf("MeasureLine %q", got)
	}
}

// TestAR38_2_MeasureTimeoutNamed (relay-r1 §15, §1; AR38 act 2): a night
// whose measure outlasts its budget on the clock stops
// `stopped:measure-timeout`, its stop line's detail naming the time, its
// source and both ways to raise it; nothing spawns, and the night is not
// clean (`agentik-relay run` exits 1, as it did under `budget`). An old status
// that says `budget` still prints as it is.
func TestAR38_2_MeasureTimeoutNamed(t *testing.T) {
	repo, q := fixture(t)
	q.Defaults.MeasureSeconds = 600
	fx, _, sh, l := fakes(repo, false)
	clock := &effects.StepClock{T: time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC)}
	fx.Clock, fx.Shell = clock, &advShell{sh: sh, clock: clock, step: 250 * time.Second}
	sp := &effects.FakeSpawner{}
	var out bytes.Buffer
	n, err := Run(fx, sp, q, NightOptions{PlanOptions: Options{Repo: repo}, State: t.TempDir(), NoTmux: true, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if n.State != relay.Stopped(relay.MeasureTimeout) || n.Reason != "measure-timeout" || n.Plan.Budget == 0 {
		t.Fatalf("night %s (%s), %d unrun", n.State, n.Reason, n.Plan.Budget)
	}
	want := fmt.Sprintf("%d checks unrun: the measure ran out of time (600 s, from queue); "+
		"raise measure_seconds in the queue or pass --measure-seconds", n.Plan.Budget)
	stop := map[string]any{}
	for _, w := range l.Written() {
		if w["event"] == "stop" {
			stop = w
		}
	}
	if stop["reason"] != "measure-timeout" || stop["detail"] != want {
		t.Fatalf("stop line %v; want detail %q", stop, want)
	}
	if len(sp.Spawns()) != 0 || Clean(n) || n.Reason == relay.LoadOpen {
		t.Fatalf("%d spawns, clean %v, reason %s: want none, not clean, exit 1", len(sp.Spawns()), Clean(n), n.Reason)
	}
	if !strings.Contains(out.String(), "measure: 600 s (queue)\n") {
		t.Fatalf("the night's lines name no measure:\n%s", out.String())
	}
	if !slices.Contains(relay.StopReasons, relay.MeasureTimeout) || slices.Contains(relay.StopReasons, "budget") {
		t.Fatalf("stop reasons %v", relay.StopReasons)
	}
}

// TestAR38_2_OldBudgetStillPrints (AR38 act 2): a `status.json` or a
// `stop` ledger line written before the rename, saying `budget`, prints as
// it is in the dashboard and the report; a check's own last line for the
// measure's cut stays `budget` (BudgetReason).
func TestAR38_2_OldBudgetStillPrints(t *testing.T) {
	const id = "2026-09-29-1"
	state := t.TempDir()
	fs := effects.OSFS{}
	fs.MkdirAll(NightDir(state, id), 0o700)
	old := `{"night":"` + id + `","state":"stopped:budget","reason":"budget","rows":[],"resume":[]}` + "\n"
	if err := fs.WriteFile(filepath.Join(NightDir(state, id), "status.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	PrintStatus(&b, ReadStatus(fs, state, id))
	if !strings.Contains(b.String(), "stopped:budget") {
		t.Fatalf("an old status:\n%s", b.String())
	}
	lines := []map[string]any{{"night": id, "event": "stop", "reason": "budget", "state": "stopped:budget",
		"detail": "8 checks unrun: budget (--measure-seconds)"}}
	if md := string(Markdown(BuildReport(lines, id), "", "")); !strings.Contains(md, "The night stopped: budget.") {
		t.Fatalf("an old stop line's report:\n%s", md)
	}
	if BudgetReason != "budget" {
		t.Fatalf("a check's own cut reads %q", BudgetReason)
	}
}
