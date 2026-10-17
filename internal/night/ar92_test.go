package night_test

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// A family's cap past the night's end shelves that family's rows; the
// others run on (AR92 rulings 2 to 5; the relay's entry 12): nights on
// TestAR82_3's world cut to two rows at --parallel 2, r1 on
// claude-opus-5-5, r2 on claude-fable-5-1, r2's first child clean and
// silent until release reads true.

// ar92World is the world, --until 05:00 (ar65T plus 4 h), r1's part 1
// first.
func ar92World(t *testing.T, first effects.FakeRun, release func() bool) (*world, *queue.Queue, night.NightOptions, *ar71Spawner, *effects.StepClock) {
	w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
	q.Rows[0].Model, q.Rows[1].Model = "claude-opus-5-5", "claude-fable-5-1"
	o.Parallel, o.Until = 2, "05:00"
	sp := &ar71Spawner{w: w, n: map[string]int{},
		first: map[string]effects.FakeRun{"r1": first, "r2": {Out: ar67Clean}},
		until: map[string]func() bool{"r2": release}}
	return w, q, o, sp, clock
}

// TestAR92_2_FamilyShelved (AR92 rulings 1 to 3, 5): r1's part 1 is a
// refused seven_day_opus event resetting 72 h on, exit 143: one pause
// line [r1], scope opus, shelved; no kill line; the status reads the
// night running, r1 paused usage-limit and shelved, r2 running, its next
// the usage-limit end. Released, r2 goes green and, the clock unmoved,
// the night stops usage-limit on the --until detail: r1 failed, its
// resume line, the stop line's shelved [r1], 2 spawns, the report's r1
// shelved.
func TestAR92_2_FamilyShelved(t *testing.T) {
	var released atomic.Bool
	w, q, o, sp, clock := ar92World(t, effects.FakeRun{Out: ar65Event("seven_day_opus", ar65T.Add(72*time.Hour)), Exit: 143}, released.Load)
	var out bytes.Buffer
	o.Out = &out
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r1 paused in the status", func() bool {
		for _, r := range night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night).Rows {
			if r.ID == "r1" && r.State == "paused" {
				return true
			}
		}
		return false
	})
	all := w.ledger.Written()
	ps := ar65Events(all, "pause")
	if len(ps) != 1 || len(ar65Events(all, "kill")) != 0 {
		t.Fatalf("pause lines %v, kill lines %v; want one, none", ps, ar65Events(all, "kill"))
	}
	const wake = "2026-10-14T01:02:00Z"
	ar71Pause(t, ps[0], "[r1]", "model", "opus", wake, 1)
	if ps[0]["shelved"] != true {
		t.Fatalf("the pause line %v; want shelved true", ps[0])
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night)
	rows := map[string]night.RowStatus{}
	for _, r := range st.Rows {
		rows[r.ID] = r
	}
	if st.State != string(relay.NightRunning) || rows["r1"].State != "paused" || rows["r1"].Reason != string(relay.UsageLimit) ||
		rows["r1"].Shelved != wake || rows["r2"].State != "running" || rows["r2"].Shelved != "" || !strings.Contains(st.Next, "usage-limit") {
		t.Fatalf("status %s, r1 %+v, r2 %s, next %q; want running, r1 paused usage-limit shelved %s, r2 running, next naming usage-limit",
			st.State, rows["r1"], rows["r2"].State, st.Next, wake)
	}
	released.Store(true)
	n := ar65End(t, res)
	all = w.ledger.Written()
	stop := lastOf(all, "stop")
	detail := "the wake " + wake + " is at or past --until 2026-10-11T05:00:00Z"
	if n.State != relay.Stopped(relay.UsageLimit) || states(n) != "r1=failed r2=green" || stop["detail"] != detail ||
		fmt.Sprint(stop["shelved"]) != "[r1]" || sp.spawns("r1")+sp.spawns("r2") != 2 || !clock.Now().Equal(ar65T) {
		t.Fatalf("night %s, %s, stop line %v, %d spawns, clock %s; want stopped:usage-limit, r1=failed r2=green, %q, shelved [r1], 2, unmoved",
			n.State, states(n), stop, sp.spawns("r1")+sp.spawns("r2"), clock.Now(), detail)
	}
	if s := n.Sittings["r1"]; s.Reason != relay.UsageLimit || !strings.Contains(out.String(), relay.ResumeLine("r1")) {
		t.Fatalf("r1 %s %s, the output %q; want failed usage-limit, its resume line", s.State, s.Reason, out.String())
	}
	rep := night.BuildReport(all, n.ID)
	var r1 *night.StoppedRow
	for i := range rep.Stopped.Rows {
		if rep.Stopped.Rows[i].Row == "r1" {
			r1 = &rep.Stopped.Rows[i]
		}
	}
	if r1 == nil || !r1.Shelved || r1.State != "failed" || !strings.Contains(string(night.Markdown(rep, "", "")), "- r1: failed, usage-limit, shelved") {
		t.Fatalf("the report's stopped rows %+v; want r1 failed, shelved", rep.Stopped.Rows)
	}
}

// TestAR92_3_WindowUnchanged (AR92 ruling 4): the same world, r2 released
// at the pause line, r1's event five_hour resetting 1 h on: one pause
// line [r1 r2], scope "", no shelved, r2's kill line, the night paused
// (r2's killed part collected, its sitting line paused); at 02:02 both
// continue, all green, queue-empty.
func TestAR92_3_WindowUnchanged(t *testing.T) {
	var w *world
	pausedLine := func() bool { return len(ar65Events(w.ledger.Written(), "pause")) > 0 }
	w, q, o, sp, clock := ar92World(t, effects.FakeRun{Out: ar65Event("five_hour", ar65T.Add(time.Hour)), Exit: 143}, func() bool { return w != nil && pausedLine() })
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "both rows paused, r2's killed part collected", func() bool {
		sl := rowLines(w.ledger.Written(), "sitting", "r2")
		return ar65Paused(o, "r1", "r2") && len(sl) == 1 && sl[0]["limit"] == "paused"
	})
	all := w.ledger.Written()
	ps := ar65Events(all, "pause")
	if len(ps) != 1 || fmt.Sprint(rowLines(all, "kill", "r2")) == "[]" {
		t.Fatalf("pause lines %v, r2's kill lines %v; want one, one", ps, rowLines(all, "kill", "r2"))
	}
	ar71Pause(t, ps[0], "[r1 r2]", "window", "", "2026-10-11T02:02:00Z", 1)
	if _, ok := ps[0]["shelved"]; ok {
		t.Fatalf("the pause line %v; want no shelved", ps[0])
	}
	if st := night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night); st.State != string(relay.NightPaused) {
		t.Fatalf("status %s; want paused", st.State)
	}
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	n := ar65End(t, res)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" || sp.spawns("r1")+sp.spawns("r2") != 4 {
		t.Fatalf("night %s, %s, %d spawns; want stopped:queue-empty, all green, 4", n.State, states(n), sp.spawns("r1")+sp.spawns("r2"))
	}
}
