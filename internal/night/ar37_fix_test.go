package night_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// firstLine is PrintStatus's first line for st.
func firstLine(st night.Status) string {
	var out bytes.Buffer
	night.PrintStatus(&out, st)
	first, _, _ := strings.Cut(out.String(), "\n")
	return first
}

// TestAR37Fix_F1_RootBeforeStatus (AR37 fix F1): a night with no readable
// status.json still names its root. During the measure, WatchGroups'
// groups.json carries it; a half-written status.json falls back to it;
// with no groups.json, the ledger's night-open line for that night names
// it (not another night's).
func TestAR37Fix_F1_RootBeforeStatus(t *testing.T) {
	const id = "2026-09-30-3"
	state, root := t.TempDir(), filepath.Join(t.TempDir(), "repo")
	t.Setenv("HOME", t.TempDir())
	reg := &effects.Groups{}
	stop := night.WatchGroups(effects.Effects{FS: effects.OSFS{}}, reg, state, id, root)
	reg.Add(effects.Group{Pgid: 999999, Started: "then", Row: ""}, func(os.Signal) error { return nil })
	dir := night.NightDir(state, id)
	b, err := os.ReadFile(filepath.Join(dir, night.GroupsFile))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil || doc["root"] != root {
		t.Fatalf("groups.json during the measure = %s; want root %s", b, root)
	}
	want := "night " + id + " in " + root + ": ; 0 rows"
	if st := night.ReadStatus(effects.OSFS{}, state, id); st.Root != root || firstLine(st) != want {
		t.Fatalf("measuring: root %q, first line %q; want %q", st.Root, firstLine(st), want)
	}
	half := `{"night": "` + id + `", "state": "runn`
	if err := os.WriteFile(filepath.Join(dir, "status.json"), []byte(half), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := night.ReadStatus(effects.OSFS{}, state, id); st.Root != root || firstLine(st) != want {
		t.Fatalf("half a status.json: root %q, first line %q; want %q", st.Root, firstLine(st), want)
	}
	reg.Done(999999)
	stop()
	if _, err := os.Stat(filepath.Join(dir, night.GroupsFile)); !os.IsNotExist(err) {
		t.Fatalf("groups.json after the measure: %v; want it removed", err)
	}
	if st := night.ReadStatus(effects.OSFS{}, state, id); st.Root != "" {
		t.Fatalf("no groups.json, no ledger: root %q; want none", st.Root)
	}
	ledger := effects.LedgerDir(state)
	os.MkdirAll(ledger, 0o700)
	lines := `{"ts":"2026-09-30T20:00:00-07:00","night":"2026-09-30-2","event":"night-open","root":"/elsewhere"}` + "\n" +
		`not json` + "\n" +
		`{"ts":"2026-09-30T21:00:00-07:00","night":"` + id + `","event":"night-open","root":"` + root + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(ledger, "2026-10-01.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := night.ReadStatus(effects.OSFS{}, state, id); st.Root != root || firstLine(st) != want {
		t.Fatalf("the ledger's night-open: root %q, first line %q; want %q", st.Root, firstLine(st), want)
	}
	t.Logf("%s", want)
}

// recFS records a night's writes: every WriteFile's path and data, and
// every Rename's target.
type recFS struct {
	effects.FS
	mu      sync.Mutex
	writes  map[string][][]byte
	renamed map[string]int
}

func (r *recFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	r.mu.Lock()
	r.writes[filepath.Base(path)] = append(r.writes[filepath.Base(path)], append([]byte(nil), data...))
	r.mu.Unlock()
	return r.FS.WriteFile(path, data, perm)
}

func (r *recFS) Rename(from, to string) error {
	r.mu.Lock()
	r.renamed[filepath.Base(to)]++
	r.mu.Unlock()
	return r.FS.Rename(from, to)
}

// TestAR37Fix_F1_StatusWrittenWhole (AR37 fix F1): a one-row night writes
// status.json only by renaming a whole temp file over it, never in place,
// and leaves no temp file; every groups.json it writes names its root.
func TestAR37Fix_F1_StatusWrittenWhole(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	rec := &recFS{FS: w.fx.FS, writes: map[string][][]byte{}, renamed: map[string]int{}}
	w.fx.FS = rec
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.writes["status.json"]) != 0 || rec.renamed["status.json"] == 0 {
		t.Fatalf("status.json written in place %d times, renamed over %d times; want 0 and some",
			len(rec.writes["status.json"]), rec.renamed["status.json"])
	}
	if len(rec.writes[night.GroupsFile]) == 0 {
		t.Fatal("the night wrote no groups.json")
	}
	for _, b := range rec.writes[night.GroupsFile] {
		var doc map[string]any
		if json.Unmarshal(b, &doc) != nil || doc["root"] != w.root {
			t.Fatalf("groups.json %s; want root %s", b, w.root)
		}
	}
	names, _ := os.ReadDir(night.NightDir(o.State, n.ID))
	for _, e := range names {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("a temp file is left: %s", e.Name())
		}
	}
	if st := night.ReadStatus(effects.OSFS{}, o.State, n.ID); st.Root != w.root || st.Night != n.ID {
		t.Fatalf("status %q root %q; want %s in %s", st.Night, st.Root, n.ID, w.root)
	}
}

// TestAR37Fix_F4_HomeCleaned (AR37 fix F4): HOME with a trailing slash is
// the same home, and a root that is the home prints `~`; a sibling that
// only shares the home's prefix prints whole.
func TestAR37Fix_F4_HomeCleaned(t *testing.T) {
	for _, c := range []struct{ home, root, want string }{
		{"/u/ann/", "/u/ann/src/a", "~/src/a"},
		{"/u/ann//", "/u/ann/src/a", "~/src/a"},
		{"/u/ann", "/u/ann", "~"},
		{"/u/ann/", "/u/ann", "~"},
		{"/u/ann", "/u/ann/", "~"},
		{"/u/ann/", "/u/anna/a", "/u/anna/a"},
		{"", "/u/ann/a", "/u/ann/a"},
	} {
		t.Setenv("HOME", c.home)
		if got := night.ShowRoot(c.root); got != c.want {
			t.Errorf("HOME %q, root %q: %q; want %q", c.home, c.root, got, c.want)
		}
	}
	t.Setenv("HOME", "/u/ann/")
	st := night.Status{Night: "2026-09-30-1", State: "running", Rows: []night.RowStatus{}, Root: "/u/ann"}
	if got, want := firstLine(st), "night 2026-09-30-1 in ~: running; 0 rows"; got != want {
		t.Fatalf("first line %q; want %q", got, want)
	}
}
