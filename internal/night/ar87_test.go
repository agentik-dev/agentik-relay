package night_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// A parallel night's next band ask waits until the row admitted ahead of
// it shows to the band: its second main-thread message, its child's exit,
// or BandSettle (band-pacing-r1 §2.4; AR87): nights on ar80World, a
// FakeBand and the step clock, T ar65T, --parallel 2 unless named.

// ar87Msg is a main-thread assistant message, id its message id.
func ar87Msg(id string) string {
	return `{"type":"assistant","parent_tool_use_id":null,"session_id":"SESSION","message":{"id":"` + id +
		`","model":"claude-opus-5-5","content":[{"type":"text","text":"working"}]}}` + "\n"
}

// ar87Shown is a child's first two main-thread messages: at the second the
// row has shown to the band (AR87 ruling 1).
var ar87Shown = ar87Msg("m1") + ar87Msg("m2")

// ar87Child is a child the test writes: a pipe of main-thread messages;
// end writes a clean result and exits 0, a kill exits 143.
type ar87Child struct {
	r    *io.PipeReader
	w    *io.PipeWriter
	gone chan struct{}
	once sync.Once
	exit int
}

func newAR87Child() *ar87Child {
	r, w := io.Pipe()
	return &ar87Child{r: r, w: w, gone: make(chan struct{})}
}

// say writes message id; it returns once the stream has read it.
func (c *ar87Child) say(t *testing.T, id string) {
	t.Helper()
	if _, err := io.WriteString(c.w, ar87Msg(id)); err != nil {
		t.Fatalf("message %s: %v", id, err)
	}
}

// end writes ar67Clean's result and exits 0.
func (c *ar87Child) end(t *testing.T) {
	t.Helper()
	if _, err := c.w.Write(bytes.SplitAfter(ar67Clean, []byte("\n"))[1]); err != nil {
		t.Fatalf("the result: %v", err)
	}
	c.stop(0)
}

func (c *ar87Child) stop(exit int) {
	c.once.Do(func() {
		c.exit = exit
		c.w.Close()
		close(c.gone)
	})
}

func (c *ar87Child) Signal(os.Signal) error     { c.stop(143); return nil }
func (c *ar87Child) Wait() (int, error)         { <-c.gone; return c.exit, nil }
func (c *ar87Child) Pid() int                   { return 4545 }
func (c *ar87Child) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *ar87Child) Close() error               { return c.r.Close() }

// ar87Spawner starts a row's first child as child[row] when set, or errs
// with err[row]; every other child is clean and commits a.txt first (the
// gate wants a commit, so a serial night goes on).
type ar87Spawner struct {
	child map[string]*ar87Child
	err   map[string]error
	mu    sync.Mutex
	n     map[string]int
}

func (a *ar87Spawner) Start(sp effects.Spawn) (effects.Process, io.ReadCloser, error) {
	row := filepath.Base(sp.Dir)
	a.mu.Lock()
	if a.n == nil {
		a.n = map[string]int{}
	}
	a.n[row]++
	k := a.n[row]
	a.mu.Unlock()
	if k == 1 && a.err[row] != nil {
		return nil, nil, a.err[row]
	}
	if c := a.child[row]; k == 1 && c != nil {
		return c, c, nil
	}
	os.WriteFile(filepath.Join(sp.Dir, "a.txt"), []byte(row+"\n"), 0o644)
	if out, err := exec.Command("git", "-C", sp.Dir, "commit", "-qam", row).CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("commit: %v: %s", err, out)
	}
	return (&effects.FakeSpawner{Out: ar67Clean, PID: 4343}).Start(sp)
}

// ar87Night is a night of r1 and r2 on band at parallel, r1's first
// child c; it runs in the background.
func ar87Night(t *testing.T, band *effects.FakeBand, parallel int, c *ar87Child) (*world, night.NightOptions, *effects.StepClock, chan *night.Night) {
	t.Helper()
	w, q, o, _, clock := ar80World(t, band, "r1", "r2")
	o.Parallel = parallel
	sp := &ar87Spawner{child: map[string]*ar87Child{}}
	if c != nil {
		sp.child["r1"] = c
	}
	return w, o, clock, ar71Run(w, q, o, sp)
}

// ar87Waits is how many times row's log says the band waits for ahead
// to show (AR87 ruling 5).
func ar87Waits(t *testing.T, o night.NightOptions, row, ahead string) int {
	t.Helper()
	b, _ := os.ReadFile(night.RowLog(o.State, o.PlanOptions.Night, row))
	return strings.Count(string(b), "relay "+row+": the band waits for "+ahead+" to show\n")
}

// ar87Spawned waits for row's spawn line.
func ar87Spawned(t *testing.T, w *world, row string) {
	t.Helper()
	ar65Until(t, row+"'s spawn line", func() bool { return len(rowLines(w.ledger.Written(), "spawn", row)) == 1 })
}

// ar87Admitted is FakeBand answering row not-started, then admitted.
func ar87Admitted(row string) *effects.FakeBand {
	return &effects.FakeBand{Answers: map[string][]effects.FakeAnswer{row: {
		{BandVerdict: effects.BandVerdict{Verdict: "not-started"}}, {BandVerdict: effects.BandVerdict{Verdict: "admitted"}}}}}
}

// TestAR87_1_NextWaitsForShown (rulings 1, 2, 5): r1 admitted at T; its
// first message does not show it, so r2 has not asked 300 ms after it is
// read; r2 asks at its second, the clock still, and r2's log says once
// that it waits for r1.
func TestAR87_1_NextWaitsForShown(t *testing.T) {
	c := newAR87Child()
	band := &effects.FakeBand{}
	_, o, clock, res := ar87Night(t, band, 2, c)
	c.say(t, "m1")
	time.Sleep(300 * time.Millisecond)
	if got := ar80Asks(band); got != "r1 +0s" {
		t.Fatalf("asks %s 300 ms after r1's first message, want r1 +0s", got)
	}
	c.say(t, "m2")
	ar65Until(t, "r2's ask", func() bool { return len(band.Asks()) == 2 })
	if got := ar80Asks(band); got != "r1 +0s, r2 +0s" || !clock.Now().Equal(ar65T) {
		t.Fatalf("asks %s at %s", got, clock.Now())
	}
	c.end(t)
	ar65End(t, res)
	if n := ar87Waits(t, o, "r2", "r1"); n != 1 {
		t.Fatalf("r2's log says it waits for r1 %d times, want 1", n)
	}
}

// TestAR87_2_CapEndsWait (ruling 3): r1 admitted, its child silent until
// its kill: r2 has not asked at +119 s and asks at +120 s; `kill r1` ends
// the night.
func TestAR87_2_CapEndsWait(t *testing.T) {
	c := newAR87Child()
	band := &effects.FakeBand{}
	w, o, clock, res := ar87Night(t, band, 2, c)
	ar87Spawned(t, w, "r1")
	ar69To(clock, ar65T.Add(119*time.Second))
	time.Sleep(300 * time.Millisecond)
	if got := ar80Asks(band); got != "r1 +0s" {
		t.Fatalf("asks %s at +119 s, want r1 +0s", got)
	}
	ar80Ask(t, band, 1, clock, 120*time.Second)
	ar65Until(t, "r2's ask", func() bool { return len(band.Asks()) == 2 })
	if got := ar80Asks(band); got != "r1 +0s, r2 +120s" {
		t.Fatalf("asks %s, want r1 +0s, r2 +120s", got)
	}
	if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), "kill r1"); err != nil {
		t.Fatal(err)
	}
	ar65End(t, res)
}

// TestAR87_3_ExitEndsWait (ruling 2): on the still clock r1's child that
// writes one message and ends clean, r1's clean child of no message, and
// r1's start that errs each let r2 ask at once: r1 +0s, r2 +0s, the night
// ending with the clock at T.
func TestAR87_3_ExitEndsWait(t *testing.T) {
	for _, name := range []string{"one message", "clean", "start errs"} {
		band := &effects.FakeBand{}
		w, q, o, _, clock := ar80World(t, band, "r1", "r2")
		o.Parallel = 2
		sp := &ar87Spawner{child: map[string]*ar87Child{}, err: map[string]error{}}
		var c *ar87Child
		switch name {
		case "one message":
			c = newAR87Child()
			sp.child["r1"] = c
		case "start errs":
			sp.err["r1"] = errors.New("no such file")
		}
		res := ar71Run(w, q, o, sp)
		if c != nil {
			c.say(t, "m1")
			c.end(t)
		}
		ar65End(t, res)
		if got := ar80Asks(band); got != "r1 +0s, r2 +0s" || !clock.Now().Equal(ar65T) {
			t.Fatalf("%s: asks %s, the clock at %s", name, got, clock.Now())
		}
	}
}

// TestAR87_4_HeldHeadKeepsPoll (ruling 2): r1 not-started, then admitted
// at +60 s. A child that shows at once lets r2 ask one poll after r1's
// leave, at +120 s (as TestAR80_5); one that writes one message holds r2
// past +120 s to +180 s, the leave plus BandSettle.
func TestAR87_4_HeldHeadKeepsPoll(t *testing.T) {
	for _, shows := range []bool{true, false} {
		c := newAR87Child()
		band := ar87Admitted("r1")
		w, _, clock, res := ar87Night(t, band, 2, c)
		ar80Ask(t, band, 1, clock, 60*time.Second)
		ar87Spawned(t, w, "r1")
		c.say(t, "m1")
		want := "r1 +0s, r1 +60s, r2 +120s"
		if shows {
			c.say(t, "m2")
		} else {
			want = "r1 +0s, r1 +60s, r2 +180s"
			ar69To(clock, ar65T.Add(120*time.Second))
			time.Sleep(300 * time.Millisecond)
			if got := ar80Asks(band); got != "r1 +0s, r1 +60s" {
				t.Fatalf("shows %v: asks %s at +120 s", shows, got)
			}
		}
		ar69To(clock, ar65T.Add(ar87Ask4(shows)))
		ar65Until(t, "r2's ask", func() bool { return len(band.Asks()) == 3 })
		if got := ar80Asks(band); got != want {
			t.Fatalf("shows %v: asks %s, want %s", shows, got, want)
		}
		c.end(t)
		ar65End(t, res)
	}
}

// ar87Ask4 is r2's ask in TestAR87_4: one poll after r1's leave when
// r1 shows, the leave plus BandSettle when it does not.
func ar87Ask4(shows bool) time.Duration {
	if shows {
		return 120 * time.Second
	}
	return 180 * time.Second
}

// TestAR87_5_ParallelOneAsBefore (ruling 4): at --parallel 1, r1 admitted
// on a clean child reads r1 +0s, r2 +0s; r1 not-started, then admitted at
// +60 s reads r1 +0s, r1 +60s, r2 +120s; r2's sitting band is one ask
// admitted, and no row's log says the band waits.
func TestAR87_5_ParallelOneAsBefore(t *testing.T) {
	for _, held := range []bool{false, true} {
		band, want := &effects.FakeBand{}, "r1 +0s, r2 +0s"
		if held {
			band, want = ar87Admitted("r1"), "r1 +0s, r1 +60s, r2 +120s"
		}
		w, o, clock, res := ar87Night(t, band, 1, nil)
		if held {
			ar80Ask(t, band, 1, clock, 60*time.Second)
			ar80Ask(t, band, 2, clock, 120*time.Second)
		}
		ar65End(t, res)
		if got := ar80Asks(band); got != want {
			t.Fatalf("held %v: asks %s, want %s", held, got, want)
		}
		if got := fmt.Sprint(ar80One(t, w.ledger.Written(), "sitting", "r2")["band"]); got != "map[asks:1 verdict:admitted wait_s:0]" {
			t.Fatalf("held %v: r2's sitting band %s", held, got)
		}
		if n := ar87Waits(t, o, "r1", "r2") + ar87Waits(t, o, "r2", "r1"); n != 0 {
			t.Fatalf("held %v: the logs say the band waits %d times", held, n)
		}
	}
}
