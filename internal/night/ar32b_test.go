package night_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar32bHead is orchHead over branch-only defaults, as a product repo's
// queue may have them; ar32bOrch is the standing row naming
// merge-on-green, which act 4 makes its reading when it names no merge.
var (
	ar32bHead = strings.Replace(orchHead, "merge: merge-on-green", "merge: branch-only", 1)
	ar32bOrch = orchRow + "    merge: merge-on-green\n"
)

// ar32bNight is orchNight over ar32bHead and ar32bOrch with builds: the
// world, the night's queue, the fake's files directory.
func ar32bNight(t *testing.T, act string, builds ...string) (*world, *queue.Queue, string) {
	t.Helper()
	w, _, files := orchNight(t, act, nil, builds...)
	y := ar32bHead + ar32bOrch
	for _, id := range builds {
		y += buildRow(id)
	}
	if err := os.WriteFile(filepath.Join(w.root, "queue.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, w.root, "commit", "-qam", "branch-only defaults")
	q, err := queue.Load(w.root, "queue.yaml")
	if err != nil || len(q.Validate()) > 0 {
		t.Fatalf("queue: %v %v", err, q.Validate())
	}
	return w, q, files
}

// addsB is what o.1 commits to add build row b: the queue with b appended
// and b's packet.
func addsB() map[string]string {
	return map[string]string{"queue.yaml": ar32bHead + ar32bOrch + buildRow("b"),
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")}
}

// recordStdin wraps o.Row so each sitting's stdin, as RenderSitting makes
// it, is kept by sitting id.
func recordStdin(o *night.NightOptions) map[string]string {
	got, row := map[string]string{}, o.Row
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		b, ro, err := row(r, rp)
		if rp.Sitting != nil {
			s, _, _ := night.RenderSitting(&night.Plan{}, r, rp, "")
			got[rp.ID] = string(s)
		}
		return b, ro, err
	}
	return got
}

// TestAR32B_2_SittingNamesGreenRows is ar32b act 2 (orchestrator r1 §3):
// o.1 adds b, which runs green on its branch under branch-only defaults
// and stays unmerged; o.2's stdin names b among the green branches to
// merge, and o.1's names none.
func TestAR32B_2_SittingNamesGreenRows(t *testing.T) {
	w, q, files := ar32bNight(t, "- act 1")
	sit(t, files, "o.1", 1, "- act 2: merge b", addsB())
	sit(t, files, "o.2", 2, "- act 3", nil)
	o := w.orchOpts(t, q, files)
	o.MaxSittings = 3
	stdins := recordStdin(&o)
	n := runNight(t, w, q, o)
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "b=green", "o.2=green"}) {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	if n.Sittings["b"].MergeSHA != "" {
		t.Fatalf("b merged at %s under branch-only", n.Sittings["b"].MergeSHA)
	}
	if s := stdins["o.2"]; !strings.Contains(s, "green branches to merge: b;") {
		t.Errorf("o.2's stdin does not name b: %s", s)
	}
	if s := stdins["o.1"]; !strings.Contains(s, "green branches to merge: none;") {
		t.Errorf("o.1's stdin names a green branch: %s", s)
	}
}

// TestAR32B_4_StopNamesUnmerged is ar32b act 4 (orchestrator r1 §6): the
// standing row names no merge over branch-only defaults, so o.1 lands; it
// adds b, b runs green and stays on its branch, and --max-sittings stops
// the night with the stop line naming b as unmerged.
func TestAR32B_4_StopNamesUnmerged(t *testing.T) {
	w, _, files := orchNight(t, "- act 1", nil)
	y := ar32bHead + orchRow
	if err := os.WriteFile(filepath.Join(w.root, "queue.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, w.root, "commit", "-qam", "branch-only defaults")
	q, err := queue.Load(w.root, "queue.yaml")
	if err != nil || len(q.Validate()) > 0 {
		t.Fatalf("queue: %v %v", err, q.Validate())
	}
	sit(t, files, "o.1", 1, "- act 2: merge b", map[string]string{"queue.yaml": y + buildRow("b"),
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")})
	o := w.orchOpts(t, q, files)
	o.MaxSittings = 2
	n := runNight(t, w, q, o)
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "b=green"}) || n.Reason != relay.CountReached {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	if n.Sittings["o.1"].MergeSHA == "" || n.Sittings["b"].MergeSHA != "" {
		t.Fatalf("o.1 merged at %q, b at %q; want o.1 landed, b on its branch", n.Sittings["o.1"].MergeSHA, n.Sittings["b"].MergeSHA)
	}
	if stop := stopOf(t, w); fmt.Sprint(stop["unmerged"]) != "[b]" {
		t.Fatalf("stop %v: want unmerged [b]", stop)
	}
}

// fakeFile writes text to <files>/<branch>/<p>: what the fake commits on
// branch; a branch "<b>.merge" file is the branches it merges first.
func fakeFile(t *testing.T, files, p, text string) {
	t.Helper()
	dst := filepath.Join(files, p)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAR32B_3_DoneAfterItsRowsMerged is ar32b act 3 (orchestrator r1 §6)
// over branch-only defaults and the real gate: o.1 adds b, b runs green
// and stays on its branch, o.2 merges b and writes DONE; o.2's merge is
// proven, the relay lands o.2, b takes o.2's merge as its MergeSHA, and
// the night stops done, clean.
func TestAR32B_3_DoneAfterItsRowsMerged(t *testing.T) {
	w, q, files := ar32bNight(t, "- act 1")
	sit(t, files, "o.1", 1, "- act 2: merge b", addsB())
	fakeFile(t, files, "b/a.txt", "b's work\n")
	fakeFile(t, files, "o.2.merge", "b\n")
	sit(t, files, "o.2", 2, "DONE: b is merged", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "b=green", "o.2=green"}) || n.Reason != relay.Done || !night.Clean(n) {
		t.Fatalf("sittings %v, stop %s, clean %v", got, n.Reason, night.Clean(n))
	}
	o2, b := n.Sittings["o.2"].MergeSHA, n.Sittings["b"].MergeSHA
	if o2 == "" || b != o2 || o2 != gitIn(t, w.root, "rev-parse", "main") {
		t.Fatalf("b's MergeSHA %q, o.2's %q; want o.2's merge, main", b, o2)
	}
	if parents := strings.Fields(gitIn(t, w.root, "log", "-1", "--format=%P", "refs/heads/o.2")); len(parents) != 1 ||
		!strings.Contains(gitIn(t, w.root, "log", "--first-parent", "--merges", "--format=%P", "refs/heads/o.2"), gitIn(t, w.root, "rev-parse", "refs/heads/b")) {
		t.Fatalf("o.2 did not merge b before its charter: %v", parents)
	}
	if stop := stopOf(t, w); stop["detail"] != "DONE: b is merged" {
		t.Fatalf("stop %v", stop)
	}
}
