package night_test

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/gate"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// stalledRemote is OSGit whose LsRemote times out once armed (the gate's
// reads, never the stage's).
type stalledRemote struct {
	effects.OSGit
	armed atomic.Bool
}

func (g *stalledRemote) LsRemote(dir, url string) (map[string]string, error) {
	if g.armed.Load() {
		return nil, fmt.Errorf("git ls-remote %s: %w after %s", url, effects.ErrLsRemoteTimeout, effects.LsRemoteTimeout)
	}
	return g.OSGit.LsRemote(dir, url)
}

// TestAR88_4_StatusAndReport: one row over a root with an origin, every
// gate read of it timing out: the row is red remote-timeout, never
// gate-unrun, and status.json's row and the report's stopped row carry the
// cause `ls-remote: remote-timeout`.
func TestAR88_4_StatusAndReport(t *testing.T) {
	was := gate.LsRemoteBackoff
	gate.LsRemoteBackoff = time.Millisecond
	t.Cleanup(func() { gate.LsRemoteBackoff = was })
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	remote := filepath.Join(filepath.Dir(w.root), "remote.git")
	gitIn(t, filepath.Dir(w.root), "init", "-q", "--bare", "-b", "main", remote)
	gitIn(t, w.root, "remote", "add", "origin", remote)
	gitIn(t, w.root, "push", "-q", "origin", "main")
	gitIn(t, w.root, "fetch", "-q", "origin")
	g := &stalledRemote{}
	w.fx.Git = g
	o := w.opts(t, by)
	row := o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := row(r, rp)
		inner := ro.Gate
		ro.Gate = func(st *night.Staged, seen, isError bool, gw night.GateWait) (relay.Result, relay.StopReason, relay.Gate) {
			g.armed.Store(true)
			return inner(st, seen, isError, gw)
		}
		return b, ro, err
	}
	n := runNight(t, w, q, o)
	if s := n.Sittings["r1"]; s == nil || s.State != relay.Red || s.Reason != relay.RemoteTimeout {
		t.Fatalf("r1's sitting %+v; want red remote-timeout", s)
	}
	const want = "ls-remote: remote-timeout"
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	if len(st.Rows) != 1 || st.Rows[0].Reason != "remote-timeout" || st.Rows[0].Cause != want {
		t.Fatalf("status rows %+v; want r1 remote-timeout, cause %q", st.Rows, want)
	}
	r := night.BuildReport(w.ledger.Written(), n.ID)
	if len(r.Stopped.Rows) != 1 || r.Stopped.Rows[0].Reason != "remote-timeout" || r.Stopped.Rows[0].Cause != want {
		t.Fatalf("report stopped %+v; want r1 remote-timeout, cause %q", r.Stopped.Rows, want)
	}
}
