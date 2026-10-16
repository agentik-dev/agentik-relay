package gate

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// flakyRemote is OSGit whose LsRemote, once armed, fails its first fails
// calls with err (every call when fails is negative) and counts them all.
type flakyRemote struct {
	effects.OSGit
	mu    sync.Mutex
	armed bool
	fails int
	err   error
	reads int
}

func (g *flakyRemote) LsRemote(dir, url string) (map[string]string, error) {
	g.mu.Lock()
	if g.armed {
		g.reads++
		if g.fails != 0 {
			g.fails--
			g.mu.Unlock()
			return nil, g.err
		}
	}
	g.mu.Unlock()
	return g.OSGit.LsRemote(dir, url)
}

// ar88Timeout is the error OSGit.LsRemote returns at its timeout.
func ar88Timeout(url string) error {
	return fmt.Errorf("git ls-remote %s: %w after %s", url, effects.ErrLsRemoteTimeout, effects.LsRemoteTimeout)
}

// ar88Gate runs TestGateGreen's row with the remote read failing as g
// says, the backoff a millisecond, and returns the gate's end line.
func ar88Gate(t *testing.T, g *flakyRemote) (relay.Result, relay.StopReason, map[string]any, error) {
	t.Helper()
	was := LsRemoteBackoff
	LsRemoteBackoff = time.Millisecond
	t.Cleanup(func() { LsRemoteBackoff = was })
	w := newWorld(t)
	st := w.stage(t, "a.txt", "history/sittings/2026-09-22-1.md")
	w.fx.Git, g.armed = g, true
	p := pk([]string{"a.txt"}, packet.Line{Shell: "cat a.txt", Expect: "reads `a1 a.txt`", Kind: packet.Reads, Value: "a1 a.txt"})
	res, why, _, err := Run(w.fx, st, p, nil, seen, Verify{})
	var end map[string]any
	for _, l := range w.ledger.Written() {
		if _, ok := l["result"]; ok && l["event"] == "gate" {
			end = l
		}
	}
	if end == nil {
		t.Fatalf("no end line: %v", w.ledger.Written())
	}
	return res, why, end, err
}

// TestAR88_1_TimeoutOnceProceeds: the gate's first ls-remote times out and
// the retry reads: the row proceeds, green, its end line naming no reason.
func TestAR88_1_TimeoutOnceProceeds(t *testing.T) {
	g := &flakyRemote{fails: 1, err: ar88Timeout("origin")}
	res, why, end, err := ar88Gate(t, g)
	if res != relay.GreenResult || why != "" || err != nil || end["reason"] != "" {
		t.Fatalf("a read that timed out once: %s %q %v; end line %v; want green", res, why, err, end)
	}
	if g.reads != 3 { // the first site's two reads, the second site's one
		t.Fatalf("reads %d, want 3", g.reads)
	}
}

// TestAR88_2_TimeoutEveryRead: every read times out: unrun, remote-timeout,
// after LsRemoteTries reads; the end line names the reason and the cause.
func TestAR88_2_TimeoutEveryRead(t *testing.T) {
	g := &flakyRemote{fails: -1, err: ar88Timeout("origin")}
	res, why, end, err := ar88Gate(t, g)
	if res != relay.Unrun || why != relay.RemoteTimeout || err == nil || !strings.HasPrefix(err.Error(), "ls-remote: ") || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("every read timing out: %s %q %v; want unrun remote-timeout", res, why, err)
	}
	if g.reads != LsRemoteTries || LsRemoteTries != 3 {
		t.Fatalf("reads %d, tries %d; want 3", g.reads, LsRemoteTries)
	}
	if end["result"] != "unrun" || end["reason"] != "remote-timeout" || end["cause"] != "ls-remote: remote-timeout" {
		t.Fatalf("end line %v; want unrun, reason and cause remote-timeout", end)
	}
	if d, _ := end["detail"].(string); !strings.Contains(d, "timed out after") {
		t.Fatalf("end line detail %q; want the timeout's", d)
	}
}

// TestAR88_3_OtherErrorAtOnce: a read that fails but not on its timeout is
// not retried: unrun, remote-error, one read.
func TestAR88_3_OtherErrorAtOnce(t *testing.T) {
	g := &flakyRemote{fails: -1, err: errors.New("git ls-remote origin: fatal: repository not found")}
	res, why, end, err := ar88Gate(t, g)
	if res != relay.Unrun || why != relay.RemoteError || err == nil || g.reads != 1 {
		t.Fatalf("a plain error: %s %q %v, reads %d; want unrun remote-error after one read", res, why, err, g.reads)
	}
	if end["reason"] != "remote-error" || end["cause"] != "ls-remote: remote-error" {
		t.Fatalf("end line %v; want reason and cause remote-error", end)
	}
}
