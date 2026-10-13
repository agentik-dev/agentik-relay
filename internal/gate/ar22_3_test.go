package gate

import (
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR22_3_HandGateBase is ADV13-5: the hand gate's base is the queue
// row's (refs/heads/<base>, else main), never the last `stage` line's. A
// child that writes evil.txt outside may-touch, points a branch `decoy`
// at its HEAD and appends a stage line naming `decoy` its base is
// refused, unrun ledger-base-differs, never green (AR22 act 3).
func TestAR22_3_HandGateBase(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	w := hermeticWorld(t)
	st := w.stage(t, "b.txt")
	st.Unlock()
	write(t, w.tree, "evil.txt", "evil\n")
	gitIn(t, w.tree, "add", ".")
	gitIn(t, w.tree, "commit", "-qm", "outside may-touch")
	gitIn(t, w.root, "branch", "decoy", gitIn(t, w.tree, "rev-parse", "HEAD"))
	w.ledger.Append("stage", map[string]any{"row": "a1", "worktree": st.Path, "branch": "a1", "base": "decoy",
		"head_before": st.HeadBefore, "origin": st.Origin})

	row := queue.Row{ID: "a1", Base: "main", Packet: pk([]string{"b.txt"},
		packet.Line{Shell: "cat b.txt", Expect: "reads `a1 b.txt`", Kind: packet.Reads, Value: "a1 b.txt"})}
	res, why, g, err := Hand(w.fx, w.root, Verify{}, row)
	if res == relay.GreenResult || res != relay.Unrun || string(why) != "ledger-base-differs" {
		t.Fatalf("a ledger base decoy: Hand = %s %q %v, outside %v; want unrun ledger-base-differs", res, why, err, g.Outside)
	}
	// The queue row's base, with the ledger agreeing: red, evil.txt outside.
	w.ledger.Append("stage", map[string]any{"row": "a1", "worktree": st.Path, "branch": "a1", "base": "main",
		"head_before": st.HeadBefore, "origin": st.Origin})
	res, _, g, _ = Hand(w.fx, w.root, Verify{}, row)
	if res != relay.RedResult || len(g.Outside) != 1 || g.Outside[0] != "evil.txt" {
		t.Fatalf("the queue's base: Hand = %s, outside %v; want red [evil.txt]", res, g.Outside)
	}
}
