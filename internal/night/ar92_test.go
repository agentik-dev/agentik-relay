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

// ar92Rows is a world of rows r1.. on models, in order, at --parallel
// par, --until 05:00, its spawner's first children first and their holds
// until.
func ar92Rows(t *testing.T, models []string, par int, first map[string]effects.FakeRun, until map[string]func() bool) (*world, *queue.Queue, night.NightOptions, *ar71Spawner, *effects.StepClock) {
	var ids []string
	for i := range models {
		ids = append(ids, fmt.Sprintf("r%d", i+1))
	}
	w, q, o, _, clock := ar65World(t, nil, ids...)
	for i, m := range models {
		q.Rows[i].Model = m
	}
	o.Parallel, o.Until = par, "05:00"
	return w, q, o, &ar71Spawner{w: w, n: map[string]int{}, first: first, until: until}, clock
}

// ar92Stop asserts the night stopped usage-limit on detail, its states
// and the stop line's shelved.
func ar92Stop(t *testing.T, w *world, n *night.Night, want, detail, shelved string) {
	t.Helper()
	stop := lastOf(w.ledger.Written(), "stop")
	if n.State != relay.Stopped(relay.UsageLimit) || states(n) != want || stop["detail"] != detail || fmt.Sprint(stop["shelved"]) != shelved {
		t.Fatalf("night %s, %s, the stop line %v; want stopped:usage-limit, %s, %q, shelved %s", n.State, states(n), stop, want, detail, shelved)
	}
}

const (
	ar92Wake  = "2026-10-14T01:02:00Z"
	ar92Until = "the wake " + ar92Wake + " is at or past --until 2026-10-11T05:00:00Z"
)

// TestAR92_4_PausedSiblingShelved (AR92 ruling 2; the fix round's F1):
// r1 and r2 on opus; r1's five_hour 1 h on pauses every row, r2 killed;
// r2 comes back with its own seven_day_opus 72 h on, which shelves every
// opus row, r1 already paused among them: the second pause line [r1 r2],
// scope opus, shelved. At 02:02 the wake continues neither: one spawn
// each, both failed, the stop line's shelved [r1 r2].
func TestAR92_4_PausedSiblingShelved(t *testing.T) {
	var w *world
	pausedLine := func() bool { return w != nil && len(ar65Events(w.ledger.Written(), "pause")) > 0 }
	w, q, o, sp, clock := ar92Rows(t, []string{"claude-opus-5-5", "claude-opus-5-5"}, 2,
		map[string]effects.FakeRun{"r1": {Out: ar65Event("five_hour", ar65T.Add(time.Hour)), Exit: 143},
			"r2": {Out: ar65Event("seven_day_opus", ar65T.Add(72*time.Hour)), Exit: 143}},
		map[string]func() bool{"r2": pausedLine})
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "two pause lines, r2's sitting line", func() bool {
		all := w.ledger.Written()
		return len(ar65Events(all, "pause")) == 2 && len(rowLines(all, "sitting", "r2")) == 1
	})
	ps := ar65Events(w.ledger.Written(), "pause")
	ar71Pause(t, ps[1], "[r1 r2]", "model", "opus", ar92Wake, 1)
	if ps[1]["shelved"] != true {
		t.Fatalf("the second pause line %v; want shelved true", ps[1])
	}
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	n := ar65End(t, res)
	ar92Stop(t, w, n, "r1=failed r2=failed", ar92Until, "[r1 r2]")
	if sp.spawns("r1") != 1 || sp.spawns("r2") != 1 {
		t.Fatalf("spawns r1 %d, r2 %d; want 1 each: a shelved row is never woken", sp.spawns("r1"), sp.spawns("r2"))
	}
}

// TestAR92_5_FamilyHeld (AR92 ruling 2; the fix round's F3): r1, r3 and
// r4 on opus, r2 on fable, --parallel 3. r1's seven_day_opus 72 h on
// shelves opus: r3, running, is killed and lands shelved (one kill line,
// r3's; the pause line [r1 r3]); r4, queued, never spawns, though a slot
// is free. Released, r2 goes green and the night stops usage-limit, r4
// skipped.
func TestAR92_5_FamilyHeld(t *testing.T) {
	var released atomic.Bool
	var w *world
	pausedLine := func() bool { return w != nil && len(ar65Events(w.ledger.Written(), "pause")) > 0 }
	w, q, o, sp, _ := ar92Rows(t, []string{"claude-opus-5-5", "claude-fable-5-1", "claude-opus-5-5", "claude-opus-5-5"}, 3,
		map[string]effects.FakeRun{"r1": {Out: ar65Event("seven_day_opus", ar65T.Add(72*time.Hour)), Exit: 143},
			"r2": {Out: ar67Clean}, "r3": {Out: ar67Clean}},
		map[string]func() bool{"r2": released.Load, "r3": pausedLine})
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r3's sitting line", func() bool { return len(rowLines(w.ledger.Written(), "sitting", "r3")) == 1 })
	time.Sleep(200 * time.Millisecond) // a pick that would start r4 has run by now
	all := w.ledger.Written()
	ps, ks := ar65Events(all, "pause"), ar65Events(all, "kill")
	if len(ps) != 1 || fmt.Sprint(ps[0]["rows"]) != "[r1 r3]" || ps[0]["shelved"] != true || len(ks) != 1 || ks[0]["row"] != "r3" || sp.spawns("r4") != 0 {
		t.Fatalf("pause lines %v, kill lines %v, r4's spawns %d; want one [r1 r3] shelved, one r3's, 0", ps, ks, sp.spawns("r4"))
	}
	released.Store(true)
	n := ar65End(t, res)
	ar92Stop(t, w, n, "r1=failed r2=green r3=failed r4=skipped", ar92Until, "[r1 r3]")
	if sp.spawns("r4") != 0 {
		t.Fatalf("r4 spawned %d times on the shelved model", sp.spawns("r4"))
	}
}

// TestAR92_6_PausedPickHolds (AR92 ruling 2; the fix round's F3): r1 and
// r3 on opus, r2 on fable, r4 on sonnet, --parallel 2. r2's Fable limit
// (2am) pauses fable; then r1's seven_day_opus 72 h on shelves opus.
// While paused the pick keeps no slot for the shelved r1 and holds r3: r4
// runs to green before the wake, r3 never spawns. At 02:02 r2 continues;
// the night stops usage-limit.
func TestAR92_6_PausedPickHolds(t *testing.T) {
	var w *world
	pausedLine := func() bool { return w != nil && len(ar65Events(w.ledger.Written(), "pause")) > 0 }
	w, q, o, sp, clock := ar92Rows(t, []string{"claude-opus-5-5", "claude-fable-5-1", "claude-opus-5-5", "claude-sonnet-4-5"}, 2,
		map[string]effects.FakeRun{"r1": {Out: ar65Event("seven_day_opus", ar65T.Add(72*time.Hour)), Exit: 143}, "r2": {Out: ar71Fable, Exit: 1}},
		map[string]func() bool{"r1": pausedLine})
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r4 green before the wake", func() bool {
		sl := rowLines(w.ledger.Written(), "sitting", "r4")
		return len(sl) == 1 && sl[0]["state"] == "green"
	})
	all := w.ledger.Written()
	if ps := ar65Events(all, "pause"); len(ps) != 2 || ps[0]["scope"] != "fable" || ps[1]["shelved"] != true || sp.spawns("r3") != 0 || !clock.Now().Equal(ar65T) {
		t.Fatalf("pause lines %v, r3's spawns %d, clock %s; want fable's then opus's shelved, 0, unmoved", ps, sp.spawns("r3"), clock.Now())
	}
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	n := ar65End(t, res)
	ar92Stop(t, w, n, "r1=failed r2=green r3=skipped r4=green", ar92Until, "[r1]")
	if sp.spawns("r3") != 0 || sp.spawns("r2") != 2 {
		t.Fatalf("spawns r2 %d, r3 %d; want 2, 0", sp.spawns("r2"), sp.spawns("r3"))
	}
}

// TestAR92_7_ShelvedSiblingWeekly (AR92 ruling 4, the shelved rows
// aside; the fix round's F2, the builder's choice): r1 and r2 on opus, r3
// on fable, --parallel 3. r1's seven_day_opus shelves opus, r2 killed; r2
// comes back with a refused seven_day of its own, which is not asked: it
// lands shelved, no stop line, and r3 runs on to green. The night stops
// usage-limit on the first shelve's detail, shelved [r1 r2].
func TestAR92_7_ShelvedSiblingWeekly(t *testing.T) {
	var released atomic.Bool
	var w *world
	pausedLine := func() bool { return w != nil && len(ar65Events(w.ledger.Written(), "pause")) > 0 }
	w, q, o, sp, _ := ar92Rows(t, []string{"claude-opus-5-5", "claude-opus-5-5", "claude-fable-5-1"}, 3,
		map[string]effects.FakeRun{"r1": {Out: ar65Event("seven_day_opus", ar65T.Add(72*time.Hour)), Exit: 143},
			"r2": {Out: ar65Event("seven_day", ar65T.Add(72*time.Hour)), Exit: 143}, "r3": {Out: ar67Clean}},
		map[string]func() bool{"r2": pausedLine, "r3": released.Load})
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r2's sitting line", func() bool { return len(rowLines(w.ledger.Written(), "sitting", "r2")) == 1 })
	time.Sleep(200 * time.Millisecond)
	all := w.ledger.Written()
	if sl := rowLines(all, "sitting", "r2"); sl[0]["limit"] != "paused" || len(ar65Events(all, "stop")) != 0 || len(rowLines(all, "kill", "r3")) != 0 {
		t.Fatalf("r2's sitting line %v, stop lines %v, r3's kill lines %v; want limit paused, none, none", sl[0], ar65Events(all, "stop"), rowLines(all, "kill", "r3"))
	}
	released.Store(true)
	n := ar65End(t, res)
	ar92Stop(t, w, n, "r1=failed r2=failed r3=green", ar92Until, "[r1 r2]")
}

// TestAR92_8_OrchestratorShelved (AR92 ruling 3; the fix round's F4):
// TestAR68_4's orchestrated world, the standing row on claude-opus-5-5;
// o.1's child a refused seven_day_opus 72 h on, no --until: o.1 is
// shelved, and the standing row's next sitting would wait on the shelve,
// so the night stops usage-limit on the 6 h detail, o.2 never cut, the
// stop line's shelved [o.1].
func TestAR92_8_OrchestratorShelved(t *testing.T) {
	w, q, files := orchNight(t, "- act 1: the row", nil)
	sit(t, files, "o.2", 2, "DONE: the row is built", nil)
	w.fx.Clock = &effects.StepClock{T: ar65T}
	o1 := &effects.FakeSpawner{Out: ar65Event("seven_day_opus", ar65T.Add(72*time.Hour)), Exit: 143, PID: 4242}
	o := w.orchOpts(t, q, files)
	o.Out = &ar68Out{}
	var n *night.Night
	select {
	case n = <-ar68Run(w, q, o, ar68Split{o1: o1}):
	case <-time.After(120 * time.Second):
		t.Fatal("the night did not end within 120 s")
	}
	stop, detail := lastOf(w.ledger.Written(), "stop"), "the wake "+ar92Wake+" is over 6h away, and no --until"
	if s := n.Sittings["o.1"]; n.State != relay.Stopped(relay.UsageLimit) || stop["detail"] != detail || fmt.Sprint(stop["shelved"]) != "[o.1]" ||
		s == nil || s.State != relay.Failed || s.Reason != relay.UsageLimit {
		t.Fatalf("night %s, o.1 %+v, the stop line %v; want stopped:usage-limit, o.1 failed usage-limit, %q, shelved [o.1]", n.State, s, stop, detail)
	}
	if len(o1.Spawns()) != 1 || len(rowLines(w.ledger.Written(), "spawn", "o.2")) != 0 {
		t.Fatalf("o.1 spawned %d times, o.2's spawn lines %v; want once, none", len(o1.Spawns()), rowLines(w.ledger.Written(), "spawn", "o.2"))
	}
}

// TestAR92_9_ShelvedSiblingFiveHour (the relay's entry 12; AR92's hardening
// round, F1): r1 and r2 on opus, r3 on fable, --parallel 3. r1's
// seven_day_opus 72 h on shelves opus, r2 killed; r2 comes back with its
// own five_hour 1 h on, which is asked as any hit's: the five-hour window
// pauses the whole night, the shelve standing. r3 is killed and paused (a
// second pause line [r3], kind window, scope "", wake 02:02, count 2); at
// 02:02 only r3 continues, green, and the night stops usage-limit on the
// shelve's detail, shelved [r1 r2].
func TestAR92_9_ShelvedSiblingFiveHour(t *testing.T) {
	var released atomic.Bool
	var w *world
	pausedLine := func() bool { return w != nil && len(ar65Events(w.ledger.Written(), "pause")) > 0 }
	w, q, o, sp, clock := ar92Rows(t, []string{"claude-opus-5-5", "claude-opus-5-5", "claude-fable-5-1"}, 3,
		map[string]effects.FakeRun{"r1": {Out: ar65Event("seven_day_opus", ar65T.Add(72*time.Hour)), Exit: 143},
			"r2": {Out: ar65Event("five_hour", ar65T.Add(time.Hour)), Exit: 143}, "r3": {Out: ar67Clean}},
		map[string]func() bool{"r2": pausedLine, "r3": released.Load})
	res := ar71Run(w, q, o, sp)
	ar65Until(t, "r2's sitting line", func() bool { return len(rowLines(w.ledger.Written(), "sitting", "r2")) == 1 })
	time.Sleep(200 * time.Millisecond)
	all := w.ledger.Written()
	ps := ar65Events(all, "pause")
	if len(ps) != 2 || len(rowLines(all, "kill", "r3")) != 1 {
		t.Fatalf("pause lines %v, r3's kill lines %v; want two, one: the five-hour window pauses the whole night", ps, rowLines(all, "kill", "r3"))
	}
	ar71Pause(t, ps[1], "[r3]", "window", "", "2026-10-11T02:02:00Z", 2)
	if _, ok := ps[1]["shelved"]; ok || ar65Paused(o) == false {
		t.Fatalf("the second pause line %v, the night not paused; want no shelved, paused", ps[1])
	}
	released.Store(true)
	ar65Until(t, "r3's sitting line, paused", func() bool {
		sl := rowLines(w.ledger.Written(), "sitting", "r3")
		return len(sl) == 1 && sl[0]["limit"] == "paused"
	})
	ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
	n := ar65End(t, res)
	ar92Stop(t, w, n, "r1=failed r2=failed r3=green", ar92Until, "[r1 r2]")
	if sp.spawns("r1") != 1 || sp.spawns("r2") != 1 || sp.spawns("r3") != 2 {
		t.Fatalf("spawns r1 %d, r2 %d, r3 %d; want 1, 1, 2", sp.spawns("r1"), sp.spawns("r2"), sp.spawns("r3"))
	}
}
