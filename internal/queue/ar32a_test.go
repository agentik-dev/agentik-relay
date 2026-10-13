package queue

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// orchHead is a queue whose defaults an orchestrator row may take.
const orchHead = "version: 1\ndefaults: {model: m, turns: 10, context: 1000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nrows:\n"

// orchRow is a well-formed orchestrator row.
const orchRow = "  - id: o\n    kind: orchestrator\n    prompt: history/sessions/brief.md\n" +
	"    sections: [How this runs, \"Settled; record\"]\n    may_touch: [Makefile, scripts/]\n" +
	"    budget: {turns: 400, context: 400000, usd: 150, wall: 6h}\n"

func loadAR32A(t *testing.T, src string) (*Queue, error) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "design", "packets"), 0o755)
	for _, id := range []string{"a3-machine", "a4-schemas"} {
		src, err := os.ReadFile(filepath.Join("..", "packet", "testdata", id+".md"))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "design", "packets", id+".md"), src, 0o644)
	}
	if err := os.WriteFile(filepath.Join(dir, "queue.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(dir, "queue.yaml")
}

// TestAR32A_1_OrchestratorRowParses (orchestrator r1 §2): `kind`,
// `prompt`, `sections` and `may_touch` parse; a row without `kind` is a
// build row; the standing row has no packet, depends_on or base, merges
// on green and never touches a device; a build row carries none of its
// keys; any other kind, or a may_touch path that leaves the repo, is
// refused.
func TestAR32A_1_OrchestratorRowParses(t *testing.T) {
	q, err := loadAR32A(t, orchHead+orchRow+"  - {id: a3-machine, packet: design/packets/a3-machine.md}\n")
	if err != nil {
		t.Fatal(err)
	}
	if ps := q.Validate(); len(ps) != 0 {
		t.Fatalf("a well-formed queue: %v", ps)
	}
	o, b := q.Rows[0], q.Rows[1]
	if o.Kind != KindOrchestrator || o.Prompt != "history/sessions/brief.md" || !o.IsOrchestrator() ||
		!reflect.DeepEqual(o.Sections, []string{"How this runs", "Settled; record"}) ||
		!reflect.DeepEqual(o.MayTouch, []string{"Makefile", "scripts/"}) || o.Turns != 400 || o.Wall != 6*time.Hour {
		t.Fatalf("the orchestrator row: %+v", o)
	}
	if b.Kind != KindBuild || b.IsOrchestrator() || b.Prompt != "" || b.Sections != nil || b.MayTouch != nil {
		t.Fatalf("the build row: %+v", b)
	}
	if got, ok := q.Orchestrator(); !ok || got.ID != "o" {
		t.Fatalf("Orchestrator() = %v %v", got.ID, ok)
	}
	for _, c := range []struct{ name, src, want string }{
		{"a packet", strings.Replace(orchRow, "kind:", "packet: design/packets/a3-machine.md\n    kind:", 1), "no packet, depends_on or base"},
		{"depends_on", strings.Replace(orchRow, "kind:", "depends_on: [a3-machine]\n    kind:", 1), "no packet, depends_on or base"},
		{"a base", strings.Replace(orchRow, "kind:", "base: a3-machine\n    kind:", 1), "no packet, depends_on or base"},
		{"branch-only", strings.Replace(orchRow, "kind:", "merge: branch-only\n    kind:", 1), "merges merge-on-green"},
		{"a device", strings.Replace(orchRow, "kind:", "device: flash it\n    kind:", 1), "device is never"},
		{"no prompt", strings.Replace(orchRow, "    prompt: history/sessions/brief.md\n", "", 1), "prompt \"\""},
		{"a prompt outside", strings.Replace(orchRow, "history/sessions/brief.md", "../brief.md", 1), "prompt \"../brief.md\""},
		{"no sections", strings.Replace(orchRow, "    sections: [How this runs, \"Settled; record\"]\n", "", 1), "sections its body line reads"},
		{"another kind", strings.Replace(orchRow, "kind: orchestrator", "kind: helper", 1) + "    packet: design/packets/a4-schemas.md\n", "kind \"helper\""},
		{"a build row's prompt", "  - {id: a3-machine, packet: design/packets/a3-machine.md, prompt: x.md}\n", "belong to an orchestrator row"},
		{"a build row's may_touch", "  - id: a3-machine\n    packet: design/packets/a3-machine.md\n    may_touch: [x]\n", "belong to an orchestrator row"},
	} {
		q, err := loadAR32A(t, orchHead+c.src)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if ps := q.Validate(); len(ps) == 0 || !strings.Contains(ps[0].Msg, c.want) {
			t.Errorf("%s: problems %v; want %q", c.name, ps, c.want)
		}
	}
	if _, err := loadAR32A(t, orchHead+strings.Replace(orchRow, "scripts/", "/etc", 1)); err == nil || !strings.Contains(err.Error(), "may_touch") {
		t.Errorf("may_touch [/etc] = %v; want refused at parse", err)
	}
	if _, err := loadAR32A(t, orchHead+strings.Replace(orchRow, "[How this runs, \"Settled; record\"]", "How this runs", 1)); err == nil || !strings.Contains(err.Error(), "sections") {
		t.Errorf("sections as a string = %v; want refused at parse", err)
	}
}

// TestAR32A_1_OneOrchestratorPerQueue (§2, §4): at most one orchestrator
// row per queue, parked or not; no row depends on it or builds on it; no
// row takes the id of one of its sittings, `<id>.<n>` in any case.
func TestAR32A_1_OneOrchestratorPerQueue(t *testing.T) {
	two := orchRow + strings.Replace(orchRow, "id: o\n", "id: p\n    parked: later\n", 1)
	for _, c := range []struct{ name, src, row, want string }{
		{"two", two, "p", "a second orchestrator row; o is"},
		{"a dependant", orchRow + "  - id: a3-machine\n    packet: design/packets/a3-machine.md\n    depends_on: [o]\n", "a3-machine", "depends on the orchestrator row o"},
		{"a base", orchRow + "  - {id: a3-machine, packet: design/packets/a3-machine.md, base: o}\n", "a3-machine", "depends on the orchestrator row o"},
		{"a sitting's id", orchRow + "  - {id: O.12, packet: design/packets/a3-machine.md}\n", "O.12", "the name of a sitting of o"},
	} {
		q, err := loadAR32A(t, orchHead+c.src)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		found := false
		for _, p := range q.Validate() {
			found = found || p.Row == c.row && strings.Contains(p.Msg, c.want)
		}
		if !found {
			t.Errorf("%s: problems %v; want %s: %q", c.name, q.Validate(), c.row, c.want)
		}
	}
	q, err := loadAR32A(t, orchHead+"  - {id: a3-machine, packet: design/packets/a3-machine.md}\n  - {id: o.1x, packet: design/packets/a4-schemas.md}\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q.Orchestrator(); ok {
		t.Fatalf("a queue of build rows has an orchestrator")
	}
}
