package night_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

func TestWindowsModes(t *testing.T) {
	absent := &effects.FakeTmux{}
	if w, err := night.Open(absent, "auto", "n", "/s"); err != nil || w.Mode != "never" {
		t.Fatalf("auto without tmux = %+v, %v", w, err)
	}
	if _, err := night.Open(absent, "required", "n", "/s"); err == nil {
		t.Fatal("required without tmux opened")
	}
	if _, err := night.Open(absent, "sometimes", "n", "/s"); err == nil {
		t.Fatal("an unknown mode opened")
	}
	ft := &effects.FakeTmux{IsPresent: true}
	w, err := night.Open(ft, "never", "n", "/s")
	if err != nil || w.RowWindow("a", "/s/a.log") != nil || len(ft.Recorded()) != 0 {
		t.Fatalf("never touched tmux: %v %q", err, ft.Recorded())
	}
	w, _ = night.Open(ft, "required", "n", "/s")
	w.RowWindow("a", night.RowLog("/s", "n", "a"))
	want := []string{"Sessions", "NewSession relay-n /s " + strings.Join(night.StatusArgv("n"), " "),
		"NewWindow relay-n a /s tail -f /s/nights/n/a.log"}
	if !reflect.DeepEqual(ft.Recorded(), want) {
		t.Fatalf("calls = %q", ft.Recorded())
	}
	ft.AliveAfter = 1
	if fb, gone := w.Check(); fb || gone {
		t.Fatal("alive reported lost")
	}
	if fb, gone := w.Check(); fb || !gone {
		t.Fatalf("required lost = %v, %v", fb, gone)
	}
	if fb, gone := w.Check(); fb || gone {
		t.Fatal("gone reported twice")
	}
}

// withoutAlive drops the Alive calls from a FakeTmux record.
func withoutAlive(calls []string) []string {
	var out []string
	for _, c := range calls {
		if !strings.HasPrefix(c, "Alive ") {
			out = append(out, c)
		}
	}
	return out
}

// TestFakeTmuxWindows: a one-row night with tmux present: the sweep's list
// (AR33), one session (its first window watches the night, AR33), one row
// window tailing the row's log, in that order, and the session checked
// after the events.
func TestFakeTmuxWindows(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	ft := &effects.FakeTmux{IsPresent: true}
	o.Tmux = ft
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	want := []string{"Sessions", "NewSession relay-2026-09-22-1 " + o.State + " " + strings.Join(night.StatusArgv("2026-09-22-1"), " "),
		"NewWindow relay-2026-09-22-1 r1 " + o.State + " tail -f " + night.RowLog(o.State, n.ID, "r1")}
	calls := ft.Recorded()
	if !reflect.DeepEqual(withoutAlive(calls), want) || len(calls) == len(want) {
		t.Fatalf("tmux calls = %q", calls)
	}
	for _, c := range calls {
		t.Logf("tmux: %s", c)
	}
	if open := lastOf(w.ledger.Written(), "night-open"); open["tmux"] != "auto" || open["session"] != "relay-2026-09-22-1" {
		t.Fatalf("night-open %v", open)
	}
}

// TestTmuxGoneFallsBack: the session lost under auto is one notice (a
// LogNotify line) and file windows for the rest of the night, which ends
// queue-empty.
func TestTmuxGoneFallsBack(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	o.Tmux = &effects.FakeTmux{IsPresent: true, AliveAfter: 1}
	var log bytes.Buffer
	o.Notify = effects.LogNotify{W: &log}
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	if strings.Count(log.String(), "tmux-gone") != 1 || !strings.Contains(log.String(), "fallback=file-windows") {
		t.Fatalf("notices:\n%s", log.String())
	}
}

// TestTmuxRequiredStops: the session lost under required stops the night
// tmux-gone.
func TestTmuxRequiredStops(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	o.Tmux, o.TmuxMode = &effects.FakeTmux{IsPresent: true, AliveAfter: 1}, "required"
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.TmuxGone) || night.Clean(n) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	if stop := lastOf(w.ledger.Written(), "stop"); stop["reason"] != "tmux-gone" {
		t.Fatalf("stop %v", stop)
	}
	o.Tmux = &effects.FakeTmux{}
	if _, err := night.Run(w.fx, effects.OSSpawner{}, q, o); err == nil {
		t.Fatal("required without tmux ran")
	}
}

// secret is a prompt no notice may carry.
var secret = strings.Repeat("the owner's words: zebra-quartz-7731 never leave the machine. ", 20)

// notifyNight is one row with an 80k ceiling (ok.jsonl peaks near 71k: a
// ceiling line, no kill), the secret as its prompt, notices to a FakeNotify
// and a LogNotify.
func notifyNight(t *testing.T) ([]effects.Notice, string) {
	t.Helper()
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		_, ro, err := row(r, rp)
		ro.Context = 80000
		return []byte(secret), ro, err
	}
	fn := &effects.FakeNotify{}
	var log bytes.Buffer
	o.Notify = effects.Multi(fn, effects.LogNotify{W: &log})
	n := runNight(t, w, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	return fn.Notices(), log.String()
}

// TestNotifyEvents: night-open, sitting, ceiling, stop and night-close are
// five notices; turn, spawn, exit and stage are none.
func TestNotifyEvents(t *testing.T) {
	sent, log := notifyNight(t)
	var events []string
	for _, nt := range sent {
		events = append(events, nt.Event)
	}
	if got := strings.Join(events, " "); got != "night-open ceiling sitting stop night-close" {
		t.Fatalf("notices %q", got)
	}
	for _, nt := range sent {
		if want := map[bool]int{true: 4, false: 3}[nt.Event == "ceiling" || nt.Event == "stop"]; nt.Priority != want || nt.Night != "2026-09-22-1" {
			t.Fatalf("notice %+v", nt)
		}
	}
	t.Logf("LogNotify:\n%s", log)
}

// TestNotifyNoContent: no notice carries 16 bytes running of the prompt,
// and no body passes the cap.
func TestNotifyNoContent(t *testing.T) {
	sent, _ := notifyNight(t)
	for _, nt := range sent {
		text := nt.Title + "\n" + nt.Body
		for i := 0; i+16 <= len(secret); i++ {
			if strings.Contains(text, secret[i:i+16]) {
				t.Fatalf("notice %s carries the prompt: %q", nt.Event, text)
			}
		}
		if len(nt.Body) > effects.NoticeCap {
			t.Fatalf("notice %s body is %d bytes", nt.Event, len(nt.Body))
		}
	}
}

// TestAttachWithoutTmux: with no live session attach reads `tail -f` over
// the row's log (every row's with none), with one `tmux attach -t
// '=relay-<night>:<row>'` (quoted for zsh); an unknown row is an error.
func TestAttachWithoutTmux(t *testing.T) {
	st := night.Status{Night: "n1", Rows: []night.RowStatus{{ID: "a"}, {ID: "b"}}}
	ft := &effects.FakeTmux{}
	argv, err := night.Attach(ft, st, "/s", "a")
	if err != nil || night.Quote(argv) != "tail -f /s/nights/n1/a.log" {
		t.Fatalf("attach a = %q, %v", argv, err)
	}
	t.Logf("without tmux: %s", night.Quote(argv))
	if argv, _ := night.Attach(nil, st, "/s", ""); len(argv) != 4 || argv[3] != "/s/nights/n1/b.log" {
		t.Fatalf("attach = %q", argv)
	}
	if _, err := night.Attach(ft, st, "/s", "nosuchrow"); err == nil {
		t.Fatal("an unknown row attached")
	}
	ft.IsPresent = true
	argv, _ = night.Attach(ft, st, "/s", "a")
	if night.Quote(argv) != "tmux attach -t '=relay-n1:a'" {
		t.Fatalf("attach with a live session = %q", argv)
	}
	t.Logf("with a live session: %s", night.Quote(argv))
}
