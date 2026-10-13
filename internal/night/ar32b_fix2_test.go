package night_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR32BFix_RedRowIsNoMerge is ar32b review round 1 (F2), orchestrator
// r1 §5's "a red row's commit reds row-unproven": b gates green under
// merge-on-green defaults, then its own merge is refused (the root's
// config moves between the gate and the merge, then back), so b ends red.
// o.2 merges b's head anyway: its gate reds row-unproven, and b's head
// never reaches main.
func TestAR32BFix_RedRowIsNoMerge(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil)
	sit(t, files, "o.1", 1, "- act 2: merge b", map[string]string{"queue.yaml": orchHead + orchRow + buildRow("b"),
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")})
	fakeFile(t, files, "b/a.txt", "b's work\n")
	fakeFile(t, files, "o.2.merge", "b\n")
	sit(t, files, "o.2", 2, "DONE: b is merged", nil)
	o := w.orchOpts(t, q, files)
	o.MaxSittings = 3
	merge := o.Merge
	o.Merge = func(r queue.Row, g relay.Gate) (string, error) {
		if r.ID != "b" {
			return merge(r, g)
		}
		cfg := filepath.Join(w.root, ".git", "config")
		before, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatal(err)
		}
		gitIn(t, w.root, "config", "fix.moved", "1")
		defer os.WriteFile(cfg, before, 0o644)
		return merge(r, g)
	}
	n := runNight(t, w, q, o)
	if s := n.Sittings["b"]; s == nil || s.State != relay.Red {
		t.Fatalf("b %v; want red, its merge refused (sittings %v)", s, sittings(w))
	}
	if s := n.Sittings["o.2"]; s == nil || s.State != relay.Red || s.MergeSHA != "" {
		t.Errorf("o.2 %+v; want red, unmerged: it merged red b's head", s)
	}
	var result map[string]any
	for _, l := range w.ledger.Written() {
		if l["event"] == "gate" && l["row"] == "o.2" && l["result"] != nil {
			result = l
		}
	}
	if result["result"] != string(relay.RedResult) || result["reason"] != string(relay.RowUnproven) {
		t.Errorf("o.2's gate %v; want red row-unproven", result)
	}
	if exec.Command("git", "-C", w.root, "merge-base", "--is-ancestor", "refs/heads/b", "main").Run() == nil {
		t.Errorf("red b's head is on main (sittings %v)", sittings(w))
	}
}
