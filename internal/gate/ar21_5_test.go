package gate

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR21_5_LedgerRowBoundAfresh is ADV12-7's first half: a forged
// `stage` line (the last one wins) with a wrong main_sha, base_sha,
// state_digest, remote_urls and origin_refs does not become the hand
// gate's main, digest or remotes, nor a Staged any reader of the ledger
// (resume, a restarted process) is handed: the gate binds them from git
// now (AR21 act 5). The forged origin_refs is a claim that can only red
// (AR22 act 4, ADV13-6): origin holds another main, so the row is failed
// origin-moved, never green, on bindings still git's.
func TestAR21_5_LedgerRowBoundAfresh(t *testing.T) {
	w := hermeticWorld(t)
	st := w.stage(t, "b.txt")
	st.Unlock() // an earlier process staged it: no lock is held now
	head := gitIn(t, w.tree, "rev-parse", "HEAD")
	main := gitIn(t, w.root, "rev-parse", "main")
	remote := gitIn(t, w.root, "remote", "get-url", "origin")
	decoy := filepath.Join(t.TempDir(), "decoy.git")
	gitIn(t, w.root, "init", "-q", "--bare", "-b", "main", decoy)
	forged := "1111111111111111111111111111111111111111"
	w.ledger.Append("stage", map[string]any{"row": "a1", "worktree": st.Path, "branch": "a1", "base": "main",
		"head_before": st.HeadBefore, "origin": st.Origin, "base_sha": head, "main_sha": forged,
		"state_digest": "forged", "remote_urls": map[string]any{"origin": []any{decoy}},
		"origin_refs": map[string]any{"origin refs/heads/main": forged}})

	back := night.StagedFromLedger(w.ledger.Written(), "a1")
	if back == nil || back.MainSha != "" || back.BaseSha != "" || back.StateDigest != "" || back.RemoteURLs != nil || back.OriginRefs != nil {
		t.Errorf("the ledger's claims became a Staged's bindings: %+v", back)
	}

	row := queue.Row{ID: "a1", Base: "main", Packet: pk([]string{"b.txt"},
		packet.Line{Shell: "cat b.txt", Expect: "reads `a1 b.txt`", Kind: packet.Reads, Value: "a1 b.txt"})}
	res, why, g, err := Hand(w.fx, w.root, Verify{}, row)
	if err != nil || res != relay.FailedResult || why != relay.OriginMoved {
		t.Fatalf("Hand = %s %q %v; gate %+v; want failed origin-moved", res, why, err, g)
	}
	digest, err := effects.GitStateDigest(w.root, st.Path)
	if err != nil {
		t.Fatal(err)
	}
	if g.Main != main || g.MainMoved || g.StateMoved || g.StateDigest != digest {
		t.Errorf("main %s moved %v, digest %s moved %v; want main %s, digest %s", g.Main, g.MainMoved, g.StateDigest, g.StateMoved, main, digest)
	}
	if !reflect.DeepEqual(g.RemoteURLs, map[string][]string{"origin": {remote}}) {
		t.Errorf("remote URLs %v, want origin at %s", g.RemoteURLs, remote)
	}
	held := strings.Fields(gitIn(t, w.root, "ls-remote", remote, "refs/heads/main"))[0]
	if g.RemoteRefs["origin refs/heads/main"] != held || g.OriginSame {
		t.Errorf("remote refs %v (same %v), want origin's main at %s, not the claim's", g.RemoteRefs, g.OriginSame, held)
	}
	if len(g.Outside) != 0 {
		t.Errorf("outside %v", g.Outside)
	}
}
