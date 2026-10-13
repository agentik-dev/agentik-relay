package night_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/gate"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar45 is what one AR45 row meets. outside: the child commits b.txt, a
// path its may-touch does not hold; dirty: the gate finds an untracked
// file in the tree; unrun: a second acceptance line is `as main` with no
// baseline, so it is unrun; moved: main takes a commit before the gate
// reads it; orphan: the child leaves a sleeper in its group, which the
// sweep kills; v is the gate's verify.
type ar45 struct {
	outside, dirty, unrun, moved, orphan bool
	v                                    gate.Verify
}

// ar45Row runs row id through the fake claude and the gate itself (an
// acceptance line that holds, may touch a.txt alone) and returns the
// `gate` object of its `sitting` line.
func ar45Row(t *testing.T, w *world, id string, c ar45) map[string]any {
	t.Helper()
	st, rp := w.stage(t, id)
	p := &packet.Packet{ID: id, MayTouch: []packet.Glob{{Raw: "a.txt", Segs: []string{"a.txt"}}},
		Acceptance: []packet.Line{{Shell: "echo ok", Expect: "reads `ok`", Kind: packet.Reads, Value: "ok", N: 1}}}
	if c.unrun {
		p.Acceptance = append(p.Acceptance, packet.Line{Shell: "echo ok", Expect: "as main", Kind: packet.AsMain, N: 2})
	}
	env := w.env("ok.jsonl")
	if c.outside {
		files := t.TempDir()
		if err := os.MkdirAll(filepath.Join(files, id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(files, id, "b.txt"), []byte("b\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		env = append(env, "FAKECLAUDE_FILES="+files)
	}
	if c.orphan {
		w.fx.Procs = effects.OSProcs{}
		env = append(env, "FAKECLAUDE_MODE=orphan")
	}
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, []byte("the prompt\n"), night.RunOptions{
		Context: 130000, Wall: time.Minute, Env: env, Transcripts: w.transcripts,
		Gate: func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
			if c.dirty {
				if err := os.WriteFile(filepath.Join(st.Path, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
					t.Error(err)
				}
			}
			if c.moved {
				gitIn(t, w.root, "commit", "--allow-empty", "-qm", "main moves")
			}
			res, why, g, _ := gate.Run(w.fx, st, p, nil, &gate.ExitFacts{Seen: seen, IsError: isError}, c.v)
			return res, why, g
		}})
	return ar45Gate(t, w)
}

// ar45Gate is the `gate` object of the ledger's last `sitting` line.
func ar45Gate(t *testing.T, w *world) map[string]any {
	t.Helper()
	g, ok := lastOf(w.ledger.Written(), "sitting")["gate"].(map[string]any)
	if !ok {
		t.Fatalf("the sitting line's gate is no object: %v", lastOf(w.ledger.Written(), "sitting"))
	}
	return g
}

// TestAR45_1_SittingNamesRed: the `sitting` line's `gate` object names
// what made the row red, a check by the `check` its own `gate` event
// carries. A row whose branch changes one path outside its may-touch, all
// else green, reads `["touch"]` under result red; a row red on verify
// alone reads the one name verify's event carries; an acceptance line
// that is unrun, all else green, reads that line's name; a main that
// moved reads `main-moved` and swept orphans `orphans`, the two with no
// event; a green row and a sitting with no gate read `[]`, never an
// absent key and never null.
func TestAR45_1_SittingNamesRed(t *testing.T) {
	fakeClaude(t)
	for _, c := range []struct {
		name   string
		row    ar45
		result string
		red    []any
	}{
		{"touch", ar45{outside: true}, "red", []any{"touch"}},
		{"green", ar45{}, "green", []any{}},
		{"verify", ar45{v: gate.Verify{Name: "false"}}, "red", []any{"verify"}},
		{"unrun line", ar45{unrun: true}, "red", []any{"acceptance"}},
		{"main moved", ar45{moved: true}, "red", []any{"main-moved"}},
		{"orphans", ar45{orphan: true}, "red", []any{"orphans"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := ar45Row(t, newWorld(t), "a1", c.row)
			if g["result"] != c.result || !reflect.DeepEqual(g["red"], c.red) {
				t.Fatalf("gate = %v, want result %s and red %v", g, c.result, c.red)
			}
		})
	}
	t.Run("no gate", func(t *testing.T) {
		w := newWorld(t)
		st, rp := w.stage(t, "a1")
		night.RunRow(w.fx, effects.OSSpawner{}, st, rp, []byte("the prompt\n"), night.RunOptions{
			Context: 130000, Wall: time.Minute, Env: w.env("ok.jsonl"), Transcripts: w.transcripts})
		if g := ar45Gate(t, w); g["result"] != "unrun" || !reflect.DeepEqual(g["red"], []any{}) {
			t.Fatalf("gate = %v, want result unrun and red []", g)
		}
	})
}

// TestAR45_2_OneSource: a row failing three checks at once: touch and
// clean red, an acceptance line unrun. The names in the `sitting` line's
// `red` that a `gate` event carries (all of them here: main did not move
// and no orphan was swept) are, in order, the `check` names of that row's
// `gate` events whose status relay.Decide does not pass in the same
// ledger.
func TestAR45_2_OneSource(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	g := ar45Row(t, w, "a1", ar45{outside: true, dirty: true, unrun: true})
	passes := map[any]bool{relay.CheckGreen: true, relay.CheckSameAsMain: true, relay.CheckSkipped: true}
	var events []any
	for _, l := range w.ledger.Written() {
		if check, ok := l["check"]; ok && l["event"] == "gate" && l["row"] == "a1" && !passes[l["status"]] {
			events = append(events, check)
		}
	}
	if !reflect.DeepEqual(events, []any{"clean", "touch", "acceptance"}) {
		t.Fatalf("the ledger's failing gate events = %v, want clean, touch, acceptance", events)
	}
	if !reflect.DeepEqual(g["red"], events) {
		t.Fatalf("the sitting line's red = %v, the gate events' = %v", g["red"], events)
	}
}
