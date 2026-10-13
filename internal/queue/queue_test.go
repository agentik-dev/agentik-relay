package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"
)

// fixtures is a copy of the repository's fixture trees under t.TempDir():
// the queue fixtures name packets from its root.
func fixtures(t *testing.T) string {
	return copyIn(t, "../..", "internal/queue/testdata", "internal/packet/testdata")
}

func load(t *testing.T, name string) *Queue {
	t.Helper()
	q, err := Load(fixtures(t), "internal/queue/testdata/"+name)
	if err != nil {
		t.Fatalf("Load %s: %v", name, err)
	}
	return q
}

func TestLoadValid(t *testing.T) {
	q := load(t, "valid.yaml")
	if ps := q.Validate(); len(ps) != 0 {
		t.Fatalf("problems: %+v", ps)
	}
	if q.Version != 1 || q.Words != "" || len(q.Rows) != 6 {
		t.Fatalf("version %d, words %q, %d rows", q.Version, q.Words, len(q.Rows))
	}
	src, _ := os.ReadFile("testdata/valid.yaml")
	if sum := sha256.Sum256(src); q.Hash != hex.EncodeToString(sum[:]) {
		t.Errorf("hash %s is not the file's sha256", q.Hash)
	}
	a3, a8 := q.Rows[0], q.Rows[5]
	if a3.Turns != 100 || a3.Context != 110000 || a3.USD != 10 || a3.Wall != 150*time.Minute {
		t.Errorf("a3 budget: %d turns, %d context, $%d, %s", a3.Turns, a3.Context, a3.USD, a3.Wall)
	}
	if a8.Model != "claude-sonnet-5" || a8.Turns != 100 || a8.Wall != 4*time.Hour || a8.Merge != BranchOnly || a8.Device != "never" {
		t.Errorf("a8 defaults: %+v", a8)
	}
	if a3.Model != "claude-opus-5-5" || q.Defaults.Verify != "auto" {
		t.Errorf("a3 model %q, verify %q", a3.Model, q.Defaults.Verify)
	}
	if a3.Packet == nil || a3.Packet.ID != "a3-machine" || len(a3.Packet.Acceptance) != 8 {
		t.Errorf("a3 packet: %+v", a3.Packet)
	}
	if got := strings.Join(q.External(), " "); got != "a2-relay-design ar1-relay-hooks" {
		t.Errorf("External = %s", got)
	}
	if got := strings.Join(q.ExternalOf(q.Rows[1]), " "); got != "a2-relay-design" {
		t.Errorf("ExternalOf a4 = %s", got)
	}
	order, err := q.Order()
	if err != nil || strings.Join(order, " ") != "a3-machine a4-schemas a5-effects-plan a6-stage-gate a7-spawn-stream a8-night" {
		t.Errorf("Order = %v, %v", order, err)
	}
}

func TestOrderHonoursBase(t *testing.T) {
	// b is listed first but is cut off a; c waits on b.
	q := &Queue{Version: 1, Rows: []Row{
		{ID: "b", Base: "a"}, {ID: "c", DependsOn: []string{"b", "outside"}}, {ID: "a", Base: "main"},
	}}
	order, err := q.Order()
	if err != nil || strings.Join(order, " ") != "a b c" {
		t.Fatalf("Order = %v, %v; want a b c", order, err)
	}
	if got := q.External(); len(got) != 1 || got[0] != "outside" {
		t.Errorf("External = %v; main is never external", got)
	}
}

func TestCycle(t *testing.T) {
	q := load(t, "cycle.yaml")
	_, err := q.Order()
	want := "cycle: a3-machine -> a5-effects-plan -> a4-schemas -> a3-machine"
	if err == nil || err.Error() != want {
		t.Fatalf("Order = %v; want %s", err, want)
	}
	ps := q.Validate()
	if len(ps) != 3 || ps[0].Row != "a3-machine" || ps[1].Row != "a5-effects-plan" || ps[2].Row != "a4-schemas" || ps[2].Msg != want {
		t.Fatalf("Validate = %+v", ps)
	}
	self := &Queue{Version: 1, Rows: []Row{{ID: "a", DependsOn: []string{"a"}}}}
	if _, err := self.Order(); err == nil || err.Error() != "cycle: a -> a" {
		t.Errorf("self Order = %v", err)
	}
}

func TestMissingPacket(t *testing.T) {
	ps := load(t, "missing-packet.yaml").Validate()
	if len(ps) != 1 || ps[0].Row != "a4-schemas" || ps[0].Msg != "packet internal/packet/testdata/absent/a4-schemas.md: missing" {
		t.Fatalf("Validate = %+v", ps)
	}
}

func TestFaults(t *testing.T) {
	got := map[string][]string{}
	for _, p := range load(t, "faults.yaml").Validate() {
		got[p.Row] = append(got[p.Row], p.Msg)
	}
	want := map[string]string{
		"a3-machine":      "duplicate id",
		"a4-other":        "id is not the packet's file stem a4-schemas",
		"a5-effects-plan": "merge-on-green needs base main, not a4-other",
		"a6-stage-gate":   "device is empty; want never or the owner's words",
		"a7-spawn-stream": "packet refuse-budget: row budget: want",
		"a9-merge":        "packet internal/packet/testdata/a9-merge.md: missing | merge \"fast-forward\"; want branch-only or merge-on-green",
	}
	for row, w := range want {
		if g := strings.Join(got[row], " | "); !strings.HasPrefix(g, w) {
			t.Errorf("%s: %q; want prefix %q", row, g, w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("rows with problems: %v; the parked a8-night must have none", got)
	}
}

func TestLoadRefuses(t *testing.T) {
	for _, r := range []struct{ file, want string }{
		{"unknown-key.yaml", "line 4: row 1: unknown key \"turns\""},
		{"tab.yaml", "line 3: tab in indentation"},
		{"absent.yaml", "no such file"},
	} {
		_, err := Load(fixtures(t), "internal/queue/testdata/"+r.file)
		if err == nil || !strings.Contains(err.Error(), r.want) {
			t.Errorf("Load %s = %v; want %q", r.file, err, r.want)
		}
	}
}

// TestAR7_QueueIDShape: an id is [A-Za-z0-9][A-Za-z0-9._-]* (never `.` or
// `..`), and ids are unique case-folded (this Mac's filesystem).
func TestAR7_QueueIDShape(t *testing.T) {
	got := map[string][]string{}
	for _, p := range load(t, "ids.yaml").Validate() {
		got[p.Row] = append(got[p.Row], p.Msg)
	}
	for row, w := range map[string]string{
		"..":         `id "..": not [A-Za-z0-9][A-Za-z0-9._-]*`,
		"-x":         `id "-x": not [A-Za-z0-9][A-Za-z0-9._-]*`,
		"A3-machine": "duplicate id",
	} {
		if g := strings.Join(got[row], " | "); !strings.Contains(g, w) {
			t.Errorf("%s: %q; want %q", row, g, w)
		}
	}
	if g := got["a3-machine"]; len(g) != 0 {
		t.Errorf("a3-machine: %q; want none", g)
	}
}

// TestAR7_PacketPathInRepo: a packet path that is absolute or climbs out of
// the repo is a problem, never read.
func TestAR7_PacketPathInRepo(t *testing.T) {
	got := map[string][]string{}
	for _, p := range load(t, "escape.yaml").Validate() {
		got[p.Row] = append(got[p.Row], p.Msg)
	}
	for _, row := range []string{"x", "y", "z"} {
		if g := strings.Join(got[row], " | "); g != "packet path leaves the repo" {
			t.Errorf("%s: %q; want %q", row, g, "packet path leaves the repo")
		}
	}
}
