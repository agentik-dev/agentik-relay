package gate

import (
	"slices"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR22_4_HandGateAfterPush is ADV13-6, the regression: after the
// child's `git push <url> HEAD:refs/heads/evil` to the local bare origin,
// the hand gate (ls-remote bound now, AR21 act 5) reads the stage line's
// origin_refs as a claim that can only red: failed origin-moved, never
// green. With no push the claim is the same, and ls-remote is listed
// skipped, never proven from the ledger (AR22 act 4).
func TestAR22_4_HandGateAfterPush(t *testing.T) {
	for _, push := range []bool{true, false} {
		t.Run(map[bool]string{true: "push", false: "no-push"}[push], func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			w := hermeticWorld(t)
			st := w.stage(t, "b.txt")
			st.Unlock() // an earlier process staged it
			if push {
				url := gitIn(t, w.root, "remote", "get-url", "origin")
				gitIn(t, w.tree, "push", "-q", "--no-verify", url, "HEAD:refs/heads/evil")
			}
			row := queue.Row{ID: "a1", Base: "main", Packet: pk([]string{"b.txt"},
				packet.Line{Shell: "cat b.txt", Expect: "reads `a1 b.txt`", Kind: packet.Reads, Value: "a1 b.txt"})}
			res, why, g, err := Hand(w.fx, w.root, Verify{}, row)
			switch {
			case push && (res != relay.FailedResult || why != relay.OriginMoved || g.OriginSame):
				t.Fatalf("after a push by URL: Hand = %s %q %v, origin_same %v; want failed origin-moved", res, why, err, g.OriginSame)
			case !push && (res != relay.GreenResult || !slices.Contains(g.Skipped, "ls-remote")):
				t.Fatalf("no push: Hand = %s %q %v, skipped %v; want green, ls-remote skipped", res, why, err, g.Skipped)
			}
		})
	}
}
