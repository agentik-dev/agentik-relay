package night_test

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR33_2_OpenClosesOldNights (AR33 act 2): a night's open lists the
// relay's socket and kills exactly the other nights' sessions (relay-a,
// relay-b; not `other`, not its own) before it starts its own, and the
// night writes one `windows-closed` line naming them. A failed list or kill
// is one `relay: tmux: <err>` line, and the night goes on.
func TestAR33_2_OpenClosesOldNights(t *testing.T) {
	const id = "2026-09-22-1"
	own := night.SessionName(id)
	ft := &effects.FakeTmux{IsPresent: true, List: []string{"relay-a", "relay-b", "other", own}}
	w, err := night.Open(ft, "auto", id, "/s")
	if err != nil || w.Mode != "auto" || w.Session != own {
		t.Fatalf("open = %+v, %v", w, err)
	}
	want := []string{"Sessions", "Kill relay-a", "Kill relay-b", "NewSession " + own + " /s " + strings.Join(night.StatusArgv(id), " ")}
	if !slices.Equal(ft.Recorded(), want) || !slices.Equal(w.Closed, []string{"relay-a", "relay-b"}) || len(w.Warn) != 0 {
		t.Fatalf("calls = %q, closed %q, warn %q", ft.Recorded(), w.Closed, w.Warn)
	}

	nw, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := nw.opts(t, by)
	o.Tmux = &effects.FakeTmux{IsPresent: true, List: []string{"relay-2026-09-21-1", "relay-2026-09-21-10", "work"}}
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, nw, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || states(n) != "r1=green" {
		t.Fatalf("night %s: %s", n.State, states(n))
	}
	lines := nw.ledger.Written()
	closed := lastOf(lines, "windows-closed")
	if got := fmt.Sprint(closed["sessions"]); got != "[relay-2026-09-21-1 relay-2026-09-21-10]" {
		t.Fatalf("windows-closed = %v", closed)
	}
	var events []string
	for _, l := range lines {
		events = append(events, fmt.Sprint(l["event"]))
	}
	if i := slices.Index(events, "windows-closed"); i < 1 || events[i-1] != "night-open" || strings.Count(strings.Join(events, " "), "windows-closed") != 1 {
		t.Fatalf("events %q", events)
	}
	t.Logf("windows-closed: %v", closed["sessions"])

	for _, ft := range []*effects.FakeTmux{
		{IsPresent: true, List: []string{"relay-a"}, KillErr: errors.New("tmux kill-session: exit status 1: can't find session")},
		{IsPresent: true, ListErr: errors.New("tmux list-sessions: exit status 1: lost server")},
	} {
		w, err := night.Open(ft, "required", id, "/s")
		if err != nil || w.Session != own || len(w.Closed) != 0 || len(w.Warn) != 1 || !strings.HasPrefix(w.Warn[0], "relay: tmux: tmux ") {
			t.Fatalf("a failed sweep = %+v, %v", w, err)
		}
		t.Logf("warn: %s", w.Warn[0])
	}
}

// TestAR33_2_NeverTouchesNothing (AR33 act 2): under never the open
// neither lists nor kills, and the night writes no `windows-closed`.
func TestAR33_2_NeverTouchesNothing(t *testing.T) {
	ft := &effects.FakeTmux{IsPresent: true, List: []string{"relay-a", "relay-b"}}
	if w, err := night.Open(ft, "never", "n", "/s"); err != nil || w.Mode != "never" || len(ft.Recorded()) != 0 || len(w.Closed) != 0 {
		t.Fatalf("never = %+v, %v, calls %q", w, err, ft.Recorded())
	}
	nw, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := nw.opts(t, by)
	o.Tmux, o.NoTmux = ft, true
	n := runNight(t, nw, q, o)
	if n.State != relay.Stopped(relay.QueueEmpty) || len(ft.Recorded()) != 0 || !slices.Equal(ft.List, []string{"relay-a", "relay-b"}) {
		t.Fatalf("night %s, calls %q, list %q", n.State, ft.Recorded(), ft.List)
	}
	if l := lastOf(nw.ledger.Written(), "windows-closed"); l != nil {
		t.Fatalf("windows-closed under never: %v", l)
	}
}
