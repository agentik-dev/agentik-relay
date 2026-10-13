package night_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The guards cap the pauses (usage-limit-resume r1 §2, §4 test 4; AR69):
// nights on TestAR65_3's world, every wait bounded.

// ar69Part is r1's part k: the refused five_hour event resetting at T+k h,
// then `API Error: 429`.
func ar69Part(k int) effects.FakeRun {
	return effects.FakeRun{Out: append(ar65Event("five_hour", ar65T.Add(time.Duration(k)*time.Hour)), ar65Result("API Error: 429")...), Exit: 143}
}

// ar69To advances clock to at.
func ar69To(clock *effects.StepClock, at time.Time) { clock.Advance(at.Sub(clock.Now())) }

// ar69Stopped asserts the night stopped usage-limit naming detail, r1
// failed usage-limit, unlocked, its tree kept.
func ar69Stopped(t *testing.T, w *world, n *night.Night, detail string) {
	t.Helper()
	stop := lastOf(w.ledger.Written(), "stop")
	if n.State != relay.Stopped(relay.UsageLimit) || stop["detail"] != detail {
		t.Fatalf("night %s, detail %q; want stopped:usage-limit, %q", n.State, stop["detail"], detail)
	}
	tree := ar67Tree(w, "r1")
	if s := n.Sittings["r1"]; s.State != relay.Failed || s.Reason != relay.UsageLimit || night.Locked(tree) {
		t.Fatalf("r1 %s %s, locked %v; want failed usage-limit, unlocked", s.State, s.Reason, night.Locked(tree))
	}
	if _, err := os.Stat(tree); err != nil {
		t.Fatalf("the tree is gone: %v", err)
	}
}

// TestAR69_4_ThirteenthPause (AR69 ruling 2; r1 §4 test 4): r1's thirteen
// parts each end on a refused five_hour event resetting an hour after the
// last, the clock taken to each wake: twelve pauses (counts 1 to 12),
// twelve wakes, thirteen spawns, then the night stops usage-limit naming
// the count.
func TestAR69_4_ThirteenthPause(t *testing.T) {
	w, q, o, sp, clock := ar65World(t, nil, "r1")
	var runs []effects.FakeRun
	for k := 1; k <= 13; k++ {
		runs = append(runs, ar69Part(k))
	}
	sp.rows["r1"] = &effects.FakeSpawner{PID: 4242, Outs: runs}
	res := ar65Start(w, q, o, sp)
	for k := 1; k <= 12; k++ {
		ar65Until(t, fmt.Sprintf("pause %d", k), func() bool {
			return len(ar65Events(w.ledger.Written(), "pause")) == k && ar65Paused(o, "r1")
		})
		ar69To(clock, ar65T.Add(time.Duration(k)*time.Hour+2*time.Minute))
		ar65Until(t, fmt.Sprintf("wake %d", k), func() bool { return len(ar65Events(w.ledger.Written(), "wake")) == k })
	}
	n := ar65End(t, res)
	all := w.ledger.Written()
	ps, wk := ar65Events(all, "pause"), ar65Events(all, "wake")
	if len(ps) != 12 || len(wk) != 12 || sp.spawns() != 13 {
		t.Fatalf("%d pause lines, %d wake lines, %d spawns; want 12, 12, 13", len(ps), len(wk), sp.spawns())
	}
	for i, p := range ps {
		if number(p["count"]) != float64(i+1) {
			t.Fatalf("pause line %d: count %v, want %d", i+1, p["count"], i+1)
		}
	}
	ar69Stopped(t, w, n, "12 pauses tonight, the most a night takes")
}

// TestAR69_5_SameReset (AR69 ruling 2): part 2 refused with part 1's
// resetsAt stops the night at the wake naming both resets, one pause, one
// wake, two spawns. The probe: two parts that name no reset pause once on
// the fallback, and the wake's part, refused again, stops the night. The
// probe passed (the fix round's F1): r1's part 2 green, r2 refused with no
// reset pauses again on the fallback, count 2, and both end green.
func TestAR69_5_SameReset(t *testing.T) {
	t.Run("the same reset", func(t *testing.T) {
		w, q, o, sp, clock := ar65World(t, nil, "r1")
		sp.rows["r1"] = &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{ar69Part(1), ar69Part(1)}}
		res := ar65Start(w, q, o, sp)
		ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
		ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
		n := ar65End(t, res)
		all := w.ledger.Written()
		if p, wk := ar65Events(all, "pause"), ar65Events(all, "wake"); len(p) != 1 || len(wk) != 1 || sp.spawns() != 2 {
			t.Fatalf("%d pause lines, %d wake lines, %d spawns; want 1, 1, 2", len(p), len(wk), sp.spawns())
		}
		ar69Stopped(t, w, n, "the reset 2026-10-11T02:00:00Z is not after the one waited out, 2026-10-11T02:00:00Z")
	})
	t.Run("the probe", func(t *testing.T) {
		w, q, o, sp, clock := ar65World(t, nil, "r1")
		bare := effects.FakeRun{Out: ar65Result("API Error: 429"), Exit: 143}
		sp.rows["r1"] = &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{bare, bare}}
		res := ar65Start(w, q, o, sp)
		ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
		ps := ar65Events(w.ledger.Written(), "pause")
		if len(ps) != 1 || ps[0]["source"] != "fallback" || ps[0]["wake"] != "2026-10-11T01:30:00Z" {
			t.Fatalf("pause lines %v, want one: fallback, 01:30", ps)
		}
		ar69To(clock, ar65T.Add(30*time.Minute))
		n := ar65End(t, res)
		all := w.ledger.Written()
		if p, wk := ar65Events(all, "pause"), ar65Events(all, "wake"); len(p) != 1 || len(wk) != 1 || sp.spawns() != 2 {
			t.Fatalf("%d pause lines, %d wake lines, %d spawns; want 1, 1, 2", len(p), len(wk), sp.spawns())
		}
		ar69Stopped(t, w, n, "refused again after the 30-minute wait, with no reset")
	})
	t.Run("the probe passed", func(t *testing.T) { // the fix round's F1
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		bare := effects.FakeRun{Out: ar65Result("API Error: 429"), Exit: 143}
		sp := &ar69Parts{n: map[string]int{}, rows: map[string]*effects.FakeSpawner{
			"r1": {PID: 4242, Outs: []effects.FakeRun{bare, {Out: ar67Clean}}},
			"r2": {PID: 4343, Outs: []effects.FakeRun{bare, {Out: ar67Clean}}}}}
		res := ar68Run(w, q, o, sp)
		ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
		ar69To(clock, ar65T.Add(30*time.Minute))
		ar65Until(t, "r2's pause", func() bool {
			return len(ar65Events(w.ledger.Written(), "pause")) == 2 || len(ar65Events(w.ledger.Written(), "stop")) > 0
		})
		if stop := lastOf(w.ledger.Written(), "stop"); stop != nil {
			t.Fatalf("r2 refused after r1's probe went green: stopped %v %q; want a second fallback pause", stop["reason"], stop["detail"])
		}
		ps := ar65Events(w.ledger.Written(), "pause")
		if p := ps[1]; fmt.Sprint(p["rows"]) != "[r2]" || p["source"] != "fallback" || p["wake"] != "2026-10-11T02:00:00Z" || number(p["count"]) != 2 {
			t.Fatalf("the second pause line %v; want [r2], fallback, wake 02:00, count 2", p)
		}
		ar69To(clock, ar65T.Add(time.Hour))
		n := ar65End(t, res)
		if wk := ar65Events(w.ledger.Written(), "wake"); len(wk) != 2 || n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" {
			t.Fatalf("%d wake lines, night %s, %s; want 2, stopped:queue-empty, both green", len(wk), n.State, states(n))
		}
	})
}

// ar69Parts starts each row's parts from its FakeSpawner by the tree's
// name, each part after the first committing a.txt in the tree first.
type ar69Parts struct {
	rows map[string]*effects.FakeSpawner
	mu   sync.Mutex
	n    map[string]int
}

func (a *ar69Parts) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	row := filepath.Base(sp.Dir)
	a.mu.Lock()
	a.n[row]++
	k := a.n[row]
	a.mu.Unlock()
	if err := ar69Commit(sp.Dir, row, k); err != nil {
		return nil, nil, err
	}
	return a.rows[row].Start(sp)
}

// ar69Commit commits a.txt in dir for part k after the first, as
// TestAR67_3's spawner does: the gate wants a commit.
func ar69Commit(dir, row string, k int) error {
	if k < 2 {
		return nil
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), fmt.Appendf(nil, "%s part %d\n", row, k), 0o644)
	if out, err := exec.Command("git", "-C", dir, "commit", "-qam", "part "+fmt.Sprint(k)).CombinedOutput(); err != nil {
		return fmt.Errorf("commit: %v: %s", err, out)
	}
	return nil
}

// ar69Spawner starts r1's parts from r1, its first stream held until r2
// has its child, and r2's first child as own, held until the ledger has
// a pause line; r2's later children are clean. Each part 2 commits a.txt
// in the tree first.
type ar69Spawner struct {
	r1      *effects.FakeSpawner
	own     effects.FakeRun
	paused  func() bool
	started chan struct{}
	mu      sync.Mutex
	n       map[string]int
}

func (a *ar69Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	row := filepath.Base(sp.Dir)
	a.mu.Lock()
	a.n[row]++
	k := a.n[row]
	a.mu.Unlock()
	if err := ar69Commit(sp.Dir, row, k); err != nil {
		return nil, nil, err
	}
	if row == "r1" {
		p, out, err := a.r1.Start(sp)
		if k == 1 && err == nil {
			out = &ar67Held{ReadCloser: out, release: a.started}
		}
		return p, out, err
	}
	if k > 1 {
		return (&effects.FakeSpawner{Out: ar67Clean, PID: 4343}).Start(sp)
	}
	close(a.started)
	p, out, err := (&effects.FakeSpawner{Outs: []effects.FakeRun{a.own}, PID: 4343}).Start(sp)
	return p, &ar69Silent{ReadCloser: out, until: a.paused}, err
}

func (a *ar69Spawner) spawns() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n["r1"] + a.n["r2"]
}

// ar69Silent is a stream that says nothing until until reads true (at
// most 10 s).
type ar69Silent struct {
	io.ReadCloser
	until func() bool
}

func (s *ar69Silent) Read(p []byte) (int, error) {
	for end := time.Now().Add(10 * time.Second); !s.until() && time.Now().Before(end); {
		time.Sleep(5 * time.Millisecond)
	}
	return s.ReadCloser.Read(p)
}

// TestAR69_6_SiblingsOwnLimit (AR69 ruling 5): at --parallel 2 r1's window
// pauses the night; r2, killed at the pause, comes back with its own
// refused event. A weekly one stops the night at once, the clock unmoved,
// both rows failed usage-limit, unlocked. A window resetting after r1's
// wake moves the wake there with a second pause line, the count the same;
// one resetting before it leaves the pause as it was.
func TestAR69_6_SiblingsOwnLimit(t *testing.T) {
	run := func(t *testing.T, own []byte) (*world, *ar69Spawner, *effects.StepClock, chan *night.Night, night.NightOptions) {
		w, q, o, _, clock := ar65World(t, nil, "r1", "r2")
		o.Parallel = 2
		sp := &ar69Spawner{own: effects.FakeRun{Out: own, Exit: 143}, started: make(chan struct{}), n: map[string]int{},
			paused: func() bool { return len(ar65Events(w.ledger.Written(), "pause")) > 0 },
			r1:     &effects.FakeSpawner{PID: 4242, Outs: []effects.FakeRun{{Out: ar67Limited, Exit: 143}, {Out: ar67Clean}}}}
		res := make(chan *night.Night, 1)
		go func() {
			n, err := night.Run(w.fx, sp, q, o)
			if err != nil {
				n = &night.Night{State: relay.NightState("error: " + err.Error())}
			}
			res <- n
		}()
		return w, sp, clock, res, o
	}
	t.Run("seven_day", func(t *testing.T) {
		w, sp, clock, res, _ := run(t, ar65Event("seven_day", ar65T.Add(48*time.Hour)))
		n := ar65End(t, res)
		all := w.ledger.Written()
		stop := lastOf(all, "stop")
		if n.State != relay.Stopped(relay.UsageLimit) || stop["detail"] != "a weekly limit" || !clock.Now().Equal(ar65T) || len(ar65Events(all, "wake")) != 0 {
			t.Fatalf("night %s, detail %q, clock %s, %d wake lines; want stopped:usage-limit, a weekly limit, unmoved, none",
				n.State, stop["detail"], clock.Now(), len(ar65Events(all, "wake")))
		}
		for _, id := range []string{"r1", "r2"} {
			tree := ar67Tree(w, id)
			if s := n.Sittings[id]; s.State != relay.Failed || s.Reason != relay.UsageLimit || night.Locked(tree) {
				t.Fatalf("%s %s %s, locked %v; want failed usage-limit, unlocked", id, s.State, s.Reason, night.Locked(tree))
			}
			if _, err := os.Stat(tree); err != nil {
				t.Fatalf("%s's tree is gone: %v", id, err)
			}
		}
		if sl := rowLines(all, "sitting", "r2"); len(sl) != 1 || sl[0]["limit"] == "paused" {
			t.Fatalf("r2's sitting lines %v, want one, not limit paused", sl)
		}
		if sp.spawns() != 2 {
			t.Fatalf("%d spawns, want 2", sp.spawns())
		}
	})
	t.Run("five_hour at T+2h", func(t *testing.T) {
		w, _, clock, res, o := run(t, ar65Event("five_hour", ar65T.Add(2*time.Hour)))
		ar65Until(t, "the second pause line", func() bool {
			return len(ar65Events(w.ledger.Written(), "pause")) == 2 && ar65Paused(o, "r1", "r2")
		})
		ps := ar65Events(w.ledger.Written(), "pause")
		if p := ps[1]; fmt.Sprint(p["rows"]) != "[r2]" || p["kind"] != "window" || p["resets_at"] != "2026-10-11T03:00:00Z" ||
			p["source"] != "event" || p["wake"] != "2026-10-11T03:02:00Z" || number(p["count"]) != 1 {
			t.Fatalf("the second pause line %v; want [r2], window, 03:00, event, wake 03:02, count 1", p)
		}
		ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
		time.Sleep(10 * o.Poll)
		if wk := ar65Events(w.ledger.Written(), "wake"); len(wk) != 0 {
			t.Fatalf("a wake line at T+1h2m: %v", wk)
		}
		ar69To(clock, ar65T.Add(2*time.Hour+2*time.Minute))
		n := ar65End(t, res)
		wk := ar65Events(w.ledger.Written(), "wake")
		if len(wk) != 1 || wk[0]["wake"] != "2026-10-11T03:02:00Z" || n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green r2=green" {
			t.Fatalf("wake lines %v, night %s, %s; want one at 03:02, stopped:queue-empty, both green", wk, n.State, states(n))
		}
	})
	t.Run("five_hour at T+30m", func(t *testing.T) {
		w, _, clock, res, o := run(t, ar65Event("five_hour", ar65T.Add(30*time.Minute)))
		ar65Until(t, "the pause, r2's sitting line", func() bool {
			all := w.ledger.Written()
			return len(ar65Events(all, "pause")) > 0 && len(rowLines(all, "sitting", "r2")) == 1 && ar65Paused(o, "r1", "r2")
		})
		ar69To(clock, ar65T.Add(time.Hour+2*time.Minute))
		n := ar65End(t, res)
		all := w.ledger.Written()
		ps, wk := ar65Events(all, "pause"), ar65Events(all, "wake")
		if len(ps) != 1 || ps[0]["wake"] != "2026-10-11T02:02:00Z" || len(wk) != 1 || wk[0]["wake"] != "2026-10-11T02:02:00Z" || states(n) != "r1=green r2=green" {
			t.Fatalf("pause lines %v, wake lines %v, %s; want one each at 02:02, both green", ps, wk, states(n))
		}
	})
}
