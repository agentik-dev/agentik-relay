package night

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// ar54Plan builds the AR38 fixture's plan with the queue's measure at
// seconds, on a clock that steps 250 s a line.
func ar54Plan(t *testing.T, seconds int) *Plan {
	t.Helper()
	repo, q := fixture(t)
	q.Defaults.MeasureSeconds = seconds
	fx, _, sh, _ := fakes(repo, false)
	clock := &effects.StepClock{T: time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC)}
	fx.Clock, fx.Shell = clock, &advShell{sh: sh, clock: clock, step: 250 * time.Second}
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// ar54Line is the line Print writes for a plan a run would stop on.
const ar54Line = "  a run on this plan stops at once: "

// TestAR54_1_PlanNamesTheStop (relay-r1 §15; AR54 act 1): a plan whose
// measure left checks past the budget names the stop a run on it makes,
// `measure-timeout: ` and the stop's detail; Print writes it on its own
// line after the queue problems and before the first row, the last line
// keeps `, K past the budget`, and the JSON carries `run_stop`.
func TestAR54_1_PlanNamesTheStop(t *testing.T) {
	p := ar54Plan(t, 600)
	if p.Budget == 0 {
		t.Fatalf("the measure cut nothing: %d measured, %d skipped", p.Measured, p.Skipped)
	}
	want := "measure-timeout: " + MeasureTimeoutDetail(p)
	if p.RunStop != want {
		t.Fatalf("RunStop %q; want %q", p.RunStop, want)
	}
	var b bytes.Buffer
	Print(&b, p)
	out := b.String()
	line := "\n" + ar54Line + want + "\n"
	at, row := strings.Index(out, line), strings.Index(out, "\nrow ")
	if at < 0 || row < 0 || at > row || strings.Count(out, ar54Line) != 1 {
		t.Fatalf("no %q before the first row in Print:\n%s", strings.TrimSpace(line), out)
	}
	t.Log(strings.TrimSpace(line))
	if !strings.Contains(out, fmt.Sprintf(", %d past the budget", p.Budget)) {
		t.Fatalf("the last line lost its budget:\n%s", out)
	}
	js, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	q, _ := json.Marshal(want)
	if !strings.Contains(string(js), `"run_stop":`+string(q)) {
		t.Fatalf("no run_stop %s in %s", q, js)
	}
}

// TestAR54_2_FittingPlanNamesNoStop (AR54 act 1): a plan whose measure
// fits names no stop: RunStop is "", Print writes no such line, and the
// JSON still carries `"run_stop":""`.
func TestAR54_2_FittingPlanNamesNoStop(t *testing.T) {
	p := ar54Plan(t, 14400)
	if p.Budget != 0 || p.RunStop != "" {
		t.Fatalf("%d past the budget, RunStop %q; want 0 and \"\"", p.Budget, p.RunStop)
	}
	var b bytes.Buffer
	Print(&b, p)
	if strings.Contains(b.String(), strings.TrimSpace(ar54Line)) {
		t.Fatalf("a fitting plan names a stop:\n%s", b.String())
	}
	js, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), `"run_stop":""`) {
		t.Fatalf("no empty run_stop in %s", js)
	}
}

// TestAR54_3_OneWording (relay-r1 §15; AR54 act 2): the run on the AR38
// fixture stops `measure-timeout`, and its stop line's detail is the
// plan's RunStop less its `measure-timeout: ` prefix, word for word.
func TestAR54_3_OneWording(t *testing.T) {
	repo, q := fixture(t)
	q.Defaults.MeasureSeconds = 600
	fx, _, sh, l := fakes(repo, false)
	clock := &effects.StepClock{T: time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC)}
	fx.Clock, fx.Shell = clock, &advShell{sh: sh, clock: clock, step: 250 * time.Second}
	var out bytes.Buffer
	n, err := Run(fx, &effects.FakeSpawner{}, q, NightOptions{PlanOptions: Options{Repo: repo}, State: t.TempDir(), NoTmux: true, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if n.Reason != "measure-timeout" || !strings.HasPrefix(n.Plan.RunStop, "measure-timeout: ") {
		t.Fatalf("night %s (%s), RunStop %q", n.State, n.Reason, n.Plan.RunStop)
	}
	stop := map[string]any{}
	for _, w := range l.Written() {
		if w["event"] == "stop" {
			stop = w
		}
	}
	if want := strings.TrimPrefix(n.Plan.RunStop, "measure-timeout: "); stop["reason"] != "measure-timeout" || stop["detail"] != want {
		t.Fatalf("stop line %v; want detail %q", stop, want)
	}
}
