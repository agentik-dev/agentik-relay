package night_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR37_1_StatusCarriesRoot (AR37 act 1): a one-row night's status.json
// carries `root`, the plan's root, and the dashboard's first line names it,
// `night <id> in <root>: …`, the home directory written `~` (read from
// HOME); with HOME elsewhere the root prints whole.
func TestAR37_1_StatusCarriesRoot(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	b, err := os.ReadFile(filepath.Join(night.NightDir(o.State, n.ID), "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["root"] != w.root || n.Plan.Root != w.root {
		t.Fatalf("status.json root %v, plan root %q, want %q\n%s", doc["root"], n.Plan.Root, w.root, b)
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	for _, c := range []struct{ home, want string }{
		{filepath.Dir(w.root), "night " + n.ID + " in ~/" + filepath.Base(w.root) + ": " + st.State + "; 1 row"},
		{t.TempDir(), "night " + n.ID + " in " + w.root + ": " + st.State + "; 1 row"},
	} {
		t.Setenv("HOME", c.home)
		var out bytes.Buffer
		night.PrintStatus(&out, st)
		first, _, _ := strings.Cut(out.String(), "\n")
		if !strings.HasPrefix(first, c.want) {
			t.Fatalf("HOME %s: first line %q, want it to start %q", c.home, first, c.want)
		}
	}
}

// TestAR37_1_OldStatusPrintsAsBefore (AR37 act 1): a status.json written
// before AR37, with no root, prints its first line as it did.
func TestAR37_1_OldStatusPrintsAsBefore(t *testing.T) {
	const id = "2026-09-29-11"
	state := t.TempDir()
	dir := night.NightDir(state, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"night": "` + id + `", "state": "running", "reason": "", "rows": [], "burn": {}, "next": "queue-empty", "resume": [], "pid": 0, "started": ""}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "status.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	night.PrintStatus(&out, night.ReadStatus(effects.OSFS{}, state, id))
	first, _, _ := strings.Cut(out.String(), "\n")
	if want := "night " + id + ": running; 0 rows"; first != want {
		t.Fatalf("first line %q, want %q\n%s", first, want, out.String())
	}
}
