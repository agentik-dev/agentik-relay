package night_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar58Plan is the plan of a two-row world, r2 depending on r1, over fx
// (nil: the world's own effects).
func ar58Plan(t *testing.T, w *world, q *queue.Queue, by map[string]spec, fx *effects.Effects) *night.Plan {
	t.Helper()
	use := w.fx
	if fx != nil {
		use = *fx
	}
	p, err := night.BuildPlan(use, q, w.opts(t, by).PlanOptions)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// ar58Row is the plan's row id.
func ar58Row(t *testing.T, p *night.Plan, id string) night.RowPlan {
	t.Helper()
	for _, rp := range p.Rows {
		if rp.ID == id {
			return rp
		}
	}
	t.Fatalf("no row %s in the plan", id)
	return night.RowPlan{}
}

// ar58RefusedState is the plan state of a row Stage would refuse (AR58
// fix round, ruling 1).
const ar58RefusedState = "refused"

// ar58Refused checks r1 refused at plan with want, a row problem and never
// a plan one, so the plan stays valid; its plan line carries the state and
// the reason; and r2, which depends on r1, pending.
func ar58Refused(t *testing.T, w *world, p *night.Plan, want string) {
	t.Helper()
	r1, r2 := ar58Row(t, p, "r1"), ar58Row(t, p, "r2")
	if r1.State != ar58RefusedState || !slices.Equal(r1.Problems, []string{want}) {
		t.Fatalf("r1 %s %q; want %s [%q]", r1.State, r1.Problems, ar58RefusedState, want)
	}
	if !p.Valid || len(p.Problems) != 0 {
		t.Fatalf("plan valid %v, problems %v; want valid, none", p.Valid, p.Problems)
	}
	if r2.State != night.StatePending || !slices.Equal(r2.Pending, []string{"r1"}) {
		t.Fatalf("r2 %s pending %v; want pending on r1", r2.State, r2.Pending)
	}
	var line map[string]any
	for _, l := range w.ledger.Written() {
		if l["event"] == "plan" && l["row"] == "r1" {
			line = l
		}
	}
	if line == nil || line["state"] != ar58RefusedState || line["reason"] != want {
		t.Fatalf("r1's plan line %v; want state %s, reason %q", line, ar58RefusedState, want)
	}
}

// TestAR58_1_PlanRefusesWorktree (relay-r1 §8; AR58 fix round, ruling 1):
// a fake FS that holds the row's worktree path refuses the row at plan, in
// stage's words, and leaves the plan valid; the row depending on it reads
// pending, not ready.
func TestAR58_1_PlanRefusesWorktree(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", deps: "r1", echo: "ok", fixture: "ok.jsonl"})
	wt := filepath.Join(w.root, ".claude", "worktrees", "r1")
	fs := &effects.FakeFS{}
	fs.MkdirAll(wt, 0o755)
	fx := w.fx
	fx.FS = fs
	ar58Refused(t, w, ar58Plan(t, w, q, by, &fx), "the worktree "+wt+" exists")
}

// TestAR58_2_PlanRefusesBranch (relay-r1 §8; AR58 fix round, ruling 1):
// the same with the row's branch left in git; then the night runs, never
// stages r1, shows it refused in status from the open, and dooms r2 as a
// stage-skipped row's dependent is doomed; status and the skip line read
// as main's stage skip.
func TestAR58_2_PlanRefusesBranch(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", deps: "r1", echo: "ok", fixture: "ok.jsonl"})
	gitIn(t, w.root, "branch", "r1")
	want := "the branch r1 exists"
	ar58Refused(t, w, ar58Plan(t, w, q, by, nil), want)
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	if states(n) != "r1=skipped r2=skipped" || n.Reason != relay.GateFailedDependency {
		t.Fatalf("night %s: %s; want r1 and r2 skipped, stopped gate-failed-dependency", n.State, states(n))
	}
	for _, l := range w.ledger.Written() {
		if l["event"] == "stage" || l["event"] == "spawn" {
			t.Fatalf("a refused or doomed row was started: %v", l)
		}
	}
	ar58Skipped(t, w, o, n, "r1", "stage r1: "+want, filepath.Join(w.root, ".claude", "worktrees", "r1"))
}

// TestAR58_3_LandedUntouched (AR58 ruling 2): a landed row, its branch
// merged on main, reads landed with no problem, and its dependent ready.
func TestAR58_3_LandedUntouched(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", deps: "r1", echo: "ok", fixture: "ok.jsonl"})
	gitIn(t, w.root, "checkout", "-q", "-b", "r1")
	os.WriteFile(filepath.Join(w.root, "a.txt"), []byte("a\nr1\n"), 0o644)
	gitIn(t, w.root, "commit", "-qam", "r1")
	gitIn(t, w.root, "checkout", "-q", "main")
	gitIn(t, w.root, "merge", "-q", "--no-ff", "-m", "merge r1", "r1")
	p := ar58Plan(t, w, q, by, nil)
	r1, r2 := ar58Row(t, p, "r1"), ar58Row(t, p, "r2")
	if r1.State != night.StateLanded || len(r1.Problems) != 0 || !p.Valid {
		t.Fatalf("r1 %s %q, plan valid %v %v; want landed, no problem", r1.State, r1.Problems, p.Valid, p.Problems)
	}
	if r2.State != night.StateReady || !slices.Equal(r2.Satisfied, []string{"r1"}) {
		t.Fatalf("r2 %s satisfied %v; want ready on r1", r2.State, r2.Satisfied)
	}
}

// TestAR58_4_StageFailNotCounted (AR58 ruling 3): under --max-sittings 2
// over three independent rows, the first fails at stage after the plan
// (its worktree appears before Stage); it is no sitting, so the other two
// both run, and the night's stop line counts 2 sittings.
func TestAR58_4_StageFailNotCounted(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"},
		spec{id: "r2", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r3", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	o.MaxSittings = 2
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		if r.ID == "r1" {
			os.MkdirAll(rp.Worktree, 0o755)
		}
		return row(r, rp)
	}
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.CountReached) || states(n) != "r1=skipped r2=green r3=green" {
		t.Fatalf("night %s: %s; want stopped:count-reached, r1 skipped, r2 and r3 green", n.State, states(n))
	}
	var skip, stop map[string]any
	for _, l := range w.ledger.Written() {
		switch {
		case l["event"] == "skip" && l["row"] == "r1":
			skip = l
		case l["event"] == "stop":
			stop = l
		}
	}
	if skip == nil || !strings.Contains(skip["reason"].(string), "the worktree ") {
		t.Fatalf("r1's skip line %v; want the worktree's refusal", skip)
	}
	if stop == nil || number(stop["sittings"]) != 2 {
		t.Fatalf("stop line %v; want 2 sittings", stop)
	}
}

// TestAR58_5_OrchRowExempt (AR58 fix round, ruling 7, F0): a worktree or a
// branch named for the standing orchestrator row is no refusal; its
// sittings stage `<orc>.<n>`, and the night runs o.1 and stops done.
func TestAR58_5_OrchRowExempt(t *testing.T) {
	for _, left := range []string{"branch", "worktree"} {
		w, q, files := orchNight(t, "- act 1", nil)
		if left == "branch" {
			gitIn(t, w.root, "branch", "o")
		} else {
			os.MkdirAll(filepath.Join(w.root, ".claude", "worktrees", "o"), 0o755)
		}
		sit(t, files, "o.1", 1, "DONE: done", nil)
		n, err := night.Run(w.fx, effects.OSSpawner{}, q, w.orchOpts(t, q, files))
		if err != nil {
			t.Fatalf("%s left: the night refused at plan: %v", left, err)
		}
		if got := sittings(w); !slices.Equal(got, []string{"o.1=green"}) || n.Reason != relay.Done {
			t.Fatalf("%s left: sittings %v stop %s; want o.1=green, done", left, got, n.Reason)
		}
	}
}

// TestAR58_6_RereadRefusesRowOnly (AR58 fix round, ruling 8, F1): o.1 adds
// row b, whose worktree exists; the mid-night re-read refuses b alone, and
// the live night goes on to o.2 rather than stopping queue-invalid.
func TestAR58_6_RereadRefusesRowOnly(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil)
	wt := filepath.Join(w.root, ".claude", "worktrees", "b")
	os.MkdirAll(wt, 0o755)
	y := orchHead + orchRow + buildRow("b")
	sit(t, files, "o.1", 1, "- act 2", map[string]string{"queue.yaml": y,
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")})
	sit(t, files, "o.2", 2, "DONE: b is in", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	stop := stopOf(t, w)
	if n.Reason == relay.QueueInvalid || !slices.Equal(sittings(w), []string{"o.1=green", "o.2=green"}) {
		t.Fatalf("sittings %v stop %s (%v); want o.1 and o.2 green, never queue-invalid", sittings(w), n.Reason, stop["detail"])
	}
	var line map[string]any
	for _, l := range w.ledger.Written() {
		if l["event"] == "plan" && l["row"] == "b" {
			line = l
		}
		if (l["event"] == "stage" || l["event"] == "spawn") && l["row"] == "b" {
			t.Fatalf("refused row b was started: %v", l)
		}
	}
	if line == nil || line["state"] != ar58RefusedState || line["reason"] != "the worktree "+wt+" exists" {
		t.Fatalf("b's plan line %v; want refused on its worktree", line)
	}
	if skip := ar58SkipOf(w, "b"); skip == nil || skip["reason"] != "stage b: the worktree "+wt+" exists" {
		t.Fatalf("b's skip line %v; want stage's words", skip)
	}
}

// TestAR58_7_GreenUnmergedRefusedAlone (AR58 fix round, ruling 9, F2): a
// build row left green and unmerged by an earlier night, its branch in
// git, is refused alone at the next orchestrated night's plan; that night
// opens, skips b in stage's words without staging it, runs o.1 and stops
// done.
func TestAR58_7_GreenUnmergedRefusedAlone(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil, "b")
	gitIn(t, w.root, "checkout", "-q", "-b", "b")
	os.WriteFile(filepath.Join(w.root, "b.txt"), []byte("b\n"), 0o644)
	gitIn(t, w.root, "add", "b.txt")
	gitIn(t, w.root, "commit", "-qm", "b")
	gitIn(t, w.root, "checkout", "-q", "main")
	sit(t, files, "o.1", 1, "DONE: done", nil)
	n, err := night.Run(w.fx, effects.OSSpawner{}, q, w.orchOpts(t, q, files))
	if err != nil {
		t.Fatalf("the night refused at plan: %v", err)
	}
	b := ar58Row(t, n.Plan, "b")
	if b.State != ar58RefusedState || !slices.Equal(b.Problems, []string{"the branch b exists"}) || !n.Plan.Valid {
		t.Fatalf("b %s %q, plan valid %v; want refused on its branch, valid", b.State, b.Problems, n.Plan.Valid)
	}
	for _, l := range w.ledger.Written() {
		if l["event"] == "stage" && l["row"] == "b" {
			t.Fatalf("refused row b reached stage: %v", l)
		}
	}
	if skip := ar58SkipOf(w, "b"); skip == nil || skip["reason"] != "stage b: the branch b exists" {
		t.Fatalf("b's skip line %v; want stage's words", skip)
	}
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green"}) || n.Reason != relay.Done {
		t.Fatalf("sittings %v stop %s; want o.1=green, done", got, n.Reason)
	}
}

// ar58SkipOf is row id's skip line, nil when none; a second one fails.
func ar58SkipOf(w *world, id string) map[string]any {
	var skip map[string]any
	for _, l := range w.ledger.Written() {
		if l["event"] == "skip" && l["row"] == id {
			if skip != nil {
				return map[string]any{"reason": "two skip lines"}
			}
			skip = l
		}
	}
	return skip
}

// ar58Skipped checks row id as main's stage-skipped row reads (AR58
// hardening round): one skip line with why, written after night-open; the
// report's stopped rows list it skipped with why; and status shows it
// skipped with why and its worktree.
func ar58Skipped(t *testing.T, w *world, o night.NightOptions, n *night.Night, id, why, worktree string) {
	t.Helper()
	opened := false
	for _, l := range w.ledger.Written() {
		opened = opened || l["event"] == "night-open"
		if l["event"] == "skip" && l["row"] == id && !opened {
			t.Fatalf("%s's skip line before night-open: %v", id, l)
		}
	}
	if skip := ar58SkipOf(w, id); skip == nil || skip["reason"] != why {
		t.Fatalf("%s's skip line %v; want reason %q", id, skip, why)
	}
	rep := night.BuildReport(w.ledger.Written(), n.ID)
	if !slices.Contains(rep.Stopped.Rows, night.StoppedRow{Row: id, State: "skipped", Reason: why}) {
		t.Fatalf("report's stopped rows %+v; want %s skipped, %q", rep.Stopped.Rows, id, why)
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	var got *night.RowStatus
	for i := range st.Rows {
		if st.Rows[i].ID == id {
			got = &st.Rows[i]
		}
	}
	if got == nil || got.State != "skipped" || got.Reason != why || got.Worktree != worktree {
		t.Fatalf("status %s %+v; want skipped, %q, worktree %s", id, got, why, worktree)
	}
}

// TestAR58_8_RefusedReportsAsSkipped (AR58 hardening round): a row refused
// at plan reads from the night's open as main's stage-skipped row: one
// skip line in stage's words (main's own for a worktree, `stage r1: the
// branch r1 exists` for a branch), the report's stopped rows, and status
// with its worktree.
func TestAR58_8_RefusedReportsAsSkipped(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	wt := filepath.Join(w.root, ".claude", "worktrees", "r1")
	os.MkdirAll(wt, 0o755)
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	if n.Reason != relay.QueueEmpty || states(n) != "r1=skipped" {
		t.Fatalf("worktree: night %s: %s; want queue-empty, r1 skipped", n.State, states(n))
	}
	ar58Skipped(t, w, o, n, "r1", fmt.Sprintf("stage %s: the worktree %s exists", "r1", wt), wt) // Stage's own format

	w, q, by = nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"}, spec{id: "r2", deps: "r1", echo: "ok", fixture: "ok.jsonl"})
	gitIn(t, w.root, "branch", "r1")
	o = w.opts(t, by)
	n = runNight(t, w, q, o)
	ar58Skipped(t, w, o, n, "r1", "stage r1: the branch r1 exists", filepath.Join(w.root, ".claude", "worktrees", "r1"))
}

// TestAR58_9_RefusedStopMatchesMain (AR58 hardening round): the stop and
// the exit are main's for the same queue. A refused chain (r0 and r1 both
// left, r1 on r0) and a refused row behind a red one each stop
// gate-failed-dependency and are not clean (exit 1), never queue-empty.
func TestAR58_9_RefusedStopMatchesMain(t *testing.T) {
	for _, c := range []struct {
		name, r0echo, want string
		branches           []string
	}{
		{"chain", "ok", "r0=skipped r1=skipped", []string{"r0", "r1"}},
		{"behind a red", "no", "r0=red r1=skipped", []string{"r1"}},
	} {
		w, q, by := nightWorld(t, spec{id: "r0", echo: c.r0echo, fixture: "ok.jsonl"}, spec{id: "r1", deps: "r0", echo: "ok", fixture: "ok.jsonl"})
		for _, b := range c.branches {
			gitIn(t, w.root, "branch", b)
		}
		n := runNight(t, w, q, w.opts(t, by))
		if n.State != relay.Stopped(relay.GateFailedDependency) || night.Clean(n) || states(n) != c.want {
			t.Fatalf("%s: night %s, clean %v: %s; want stopped:gate-failed-dependency, not clean, %s",
				c.name, n.State, night.Clean(n), states(n), c.want)
		}
	}
}
