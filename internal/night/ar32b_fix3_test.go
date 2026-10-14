package night_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// reportOf is the report of the ledger's night (the fake clock's id, not
// Night.ID) from the lines it wrote, with its markdown.
func reportOf(w *world) (night.Report, string) {
	r := night.BuildReport(w.ledger.Written(), w.ledger.Night)
	return r, string(night.Markdown(r, "", ""))
}

// mergeLineOf is the last `merge` ledger line naming row, nil when none.
func mergeLineOf(w *world, row string) map[string]any {
	var m map[string]any
	for _, l := range w.ledger.Written() {
		if l["event"] == "merge" && l["row"] == row {
			m = l
		}
	}
	return m
}

// TestAR32BFix_LandedRowReportsMerged is ar32b post-review F1 (orchestrator
// r1 §6, relay-r1 §9's report): a build row that lands through a sitting's
// merge writes a `merge` ledger line (result merged, the sitting's merge
// sha, via the sitting), so `agentik-relay report` counts it merged and never
// lists it as waiting on a hand merge, as the stop line says.
func TestAR32BFix_LandedRowReportsMerged(t *testing.T) {
	w, q, files := ar32bNight(t, "- act 1")
	sit(t, files, "o.1", 1, "- act 2: merge b", addsB())
	fakeFile(t, files, "b/a.txt", "b's work\n")
	fakeFile(t, files, "o.2.merge", "b\n")
	sit(t, files, "o.2", 2, "DONE: b is merged", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	o2 := n.Sittings["o.2"].MergeSHA
	if o2 == "" || n.Sittings["b"].MergeSHA != o2 {
		t.Fatalf("b's MergeSHA %q, o.2's %q", n.Sittings["b"].MergeSHA, o2)
	}
	if m := mergeLineOf(w, "b"); m == nil || m["result"] != "merged" || m["sha"] != o2 || m["via"] != "o.2" {
		t.Fatalf("b's merge line %v; want merged at o.2's merge %s, via o.2", m, o2)
	}
	r, md := reportOf(w)
	if len(r.Waiting) != 0 || !slices3(r.Merged, "o.1", "o.2", "b") || r.Merged[2].SHA != o2 {
		t.Fatalf("report merged %+v, waiting %v; want o.1, o.2, b merged, none waiting", r.Merged, r.Waiting)
	}
	if !strings.Contains(md, "3 merged, 0 stopped short, 0 waiting on a hand merge.") {
		t.Fatalf("report:\n%s", md)
	}
}

// TestAR32BFix_UnmergedRowStillWaits: o.1 adds b and c, both green on
// their branches; o.2 merges b only. b is merged in the report, c alone
// waits on a hand merge, as the stop line's unmerged names it.
func TestAR32BFix_UnmergedRowStillWaits(t *testing.T) {
	w, q, files := ar32bNight(t, "- act 1")
	sit(t, files, "o.1", 1, "- act 2: merge b and c", map[string]string{
		"queue.yaml":          ar32bHead + ar32bOrch + buildRow("b") + buildRow("c"),
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok"),
		"design/packets/c.md": fmt.Sprintf(packetSrc, "c", "ok")})
	fakeFile(t, files, "b/a.txt", "b's work\n")
	fakeFile(t, files, "c/a.txt", "c's work\n")
	fakeFile(t, files, "o.2.merge", "b\n")
	sit(t, files, "o.2", 2, "- act 3: merge c", nil)
	o := w.orchOpts(t, q, files)
	o.MaxSittings = 4
	n := runNight(t, w, q, o)
	b, c := n.Sittings["b"], n.Sittings["c"]
	if b == nil || c == nil || b.MergeSHA == "" || b.MergeSHA != n.Sittings["o.2"].MergeSHA || c.MergeSHA != "" {
		t.Fatalf("sittings %v; want b landed through o.2, c on its branch", sittings(w))
	}
	if stop := stopOf(t, w); fmt.Sprint(stop["unmerged"]) != "[c]" {
		t.Fatalf("stop %v: want unmerged [c]", stop)
	}
	if m := mergeLineOf(w, "c"); m != nil {
		t.Fatalf("c has a merge line %v", m)
	}
	r, md := reportOf(w)
	if !reflect.DeepEqual(r.Waiting, []string{"c (off main)"}) || !slices3(r.Merged, "o.1", "o.2", "b") {
		t.Fatalf("report merged %+v, waiting %v; want o.1, o.2, b merged, c waiting", r.Merged, r.Waiting)
	}
	if !strings.Contains(md, "3 merged, 0 stopped short, 1 waiting on a hand merge.") || strings.Contains(md, "- b (off main)") {
		t.Fatalf("report:\n%s", md)
	}
}

// slices3 reports whether merged names exactly rows, in order.
func slices3(merged []night.Merged, rows ...string) bool {
	if len(merged) != len(rows) {
		return false
	}
	for i, m := range merged {
		if m.Row != rows[i] {
			return false
		}
	}
	return true
}
