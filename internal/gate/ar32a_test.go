package gate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

const (
	ar32aBrief = "history/sessions/brief.md"
	ar32aHead  = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nwords: go on\nrows:\n"
	ar32aOrch  = "  - id: o\n    kind: orchestrator\n    prompt: " + ar32aBrief + "\n    sections: [How this runs]\n" +
		"    may_touch: [Makefile]\n    budget: {turns: 400, context: 400000, usd: 150, wall: 6h}\n"
	ar32aRows    = "  - {id: b1, packet: design/packets/b1.md, parked: acceptance}\n  - {id: b2, packet: design/packets/b2.md}\n"
	ar32aCharter = "# Sitting\n\n## Next act\n- act 2: the guards\n"
)

// ar32aPacket is ok-minimal's packet under id.
func ar32aPacket(t *testing.T, id string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "packet", "testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(string(b), "ok-minimal", id, 1)
}

// orchWorld is a world whose main holds the brief, the queue (head then
// rows) and b1 to b3's packets, and whose queue is loaded as the night's
// and as the one the sitting began from.
func orchWorld(t *testing.T, rows string) (*world, *Orchestrated) {
	t.Helper()
	return orchWorldWith(t, ar32aOrch, rows)
}

// orchWorldWith is orchWorld with orch as the standing row.
func orchWorldWith(t *testing.T, orch, rows string) (*world, *Orchestrated) {
	t.Helper()
	w := newWorld(t)
	write(t, w.root, ar32aBrief, "# Brief\n")
	write(t, w.root, effectsQueue, ar32aHead+orch+rows)
	for _, id := range []string{"b1", "b2", "b3"} {
		write(t, w.root, "design/packets/"+id+".md", ar32aPacket(t, id))
	}
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "the queue")
	q, err := queue.Load(w.root, effectsQueue)
	if err != nil || len(q.Validate()) > 0 {
		t.Fatalf("the night's queue: %v %v", err, q.Validate())
	}
	r, _ := q.Orchestrator()
	return w, &Orchestrated{Row: r, Night: q, Began: q}
}

const effectsQueue = "relay/queue.yaml"

// queueDetail is the last queue check's detail in w's ledger.
func queueDetail(w *world) string {
	d := ""
	for _, l := range w.ledger.Written() {
		if l["check"] == "queue" {
			d, _ = l["detail"].(string)
		}
	}
	return d
}

// orchSitting stages o.1 and commits files (path to bytes) on it, then
// gates it with o.
func orchSitting(t *testing.T, w *world, o *Orchestrated, files map[string]string, extra ...func(tree string)) (relay.Result, relay.StopReason, relay.Gate) {
	t.Helper()
	tree := filepath.Join(w.root, ".claude", "worktrees", "o.1")
	st, err := night.Stage(w.fx, &night.RowPlan{ID: "o.1", Worktree: tree, Branch: "o.1", Base: "main", Row: "o"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Unlock() })
	for f, b := range files {
		if b == "" {
			gitIn(t, tree, "rm", "-q", f)
			continue
		}
		write(t, tree, f, b)
	}
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-qm", "o.1's work")
	for _, e := range extra {
		e(tree)
	}
	res, why, g, _ := Run(w.fx, st, &packet.Packet{ID: "o.1"}, nil, seen, Verify{Orchestrator: o, QueuePath: effectsQueue})
	return res, why, g
}

// TestAR32A_3_GateRedsNoCharter is ar32a act 3 (orchestrator r1 §5): an
// orchestrator sitting is green only when the newest
// history/sittings/*-o.md at its head was added on its branch, is under
// 4,096 bytes and has a non-empty Next act; else red no-charter. No
// acceptance line runs.
func TestAR32A_3_GateRedsNoCharter(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		main  string // a charter already at main
		res   relay.Result
		why   relay.StopReason
	}{
		{"a good charter", map[string]string{"history/sittings/2026-09-29-1-o.md": ar32aCharter}, "", relay.GreenResult, ""},
		{"none", map[string]string{"notes.md": "x\n"}, "", relay.RedResult, relay.NoCharter},
		{"another row's", map[string]string{"history/sittings/2026-09-29-1-b2.md": ar32aCharter}, "", relay.RedResult, relay.NoCharter},
		{"not added here", map[string]string{"notes.md": "x\n"}, "history/sittings/2026-09-29-1-o.md", relay.RedResult, relay.NoCharter},
		{"an older one added", map[string]string{"history/sittings/2026-09-28-9-o.md": ar32aCharter}, "history/sittings/2026-09-29-1-o.md", relay.RedResult, relay.NoCharter},
		{"4 KB", map[string]string{"history/sittings/2026-09-29-1-o.md": ar32aCharter + strings.Repeat("x", 4096)}, "", relay.RedResult, relay.NoCharter},
		{"an empty Next act", map[string]string{"history/sittings/2026-09-29-1-o.md": "# Sitting\n\n## Next act\n\n## Handover\n- x\n"}, "", relay.RedResult, relay.NoCharter},
		{"-10- over -9-", map[string]string{"history/sittings/2026-09-29-9-o.md": "# S\n", "history/sittings/2026-09-29-10-o.md": ar32aCharter}, "", relay.GreenResult, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, o := orchWorld(t, ar32aRows)
			if c.main != "" {
				write(t, w.root, c.main, ar32aCharter)
				gitIn(t, w.root, "add", ".")
				gitIn(t, w.root, "commit", "-qm", "a charter")
			}
			res, why, g := orchSitting(t, w, o, c.files)
			if res != c.res || why != c.why || g.NoCharter != (c.why == relay.NoCharter) || len(g.Lines) != 0 {
				t.Errorf("%s, %q, no_charter %v, %d lines; want %s, %q", res, why, g.NoCharter, len(g.Lines), c.res, c.why)
			}
		})
	}
}

// TestAR32A_3_GateRedsEditedRow is ar32a act 3 (§5): a sitting only
// appends rows. A row at main when it began that it changed, dropped or
// unparked reds queue-invalid, and so does a queue that no longer loads;
// an appended row whose packet parses is green.
func TestAR32A_3_GateRedsEditedRow(t *testing.T) {
	good := map[string]string{"history/sittings/2026-09-29-1-o.md": ar32aCharter}
	with := func(q string) map[string]string {
		m := map[string]string{effectsQueue: q}
		for k, v := range good {
			m[k] = v
		}
		return m
	}
	for _, c := range []struct {
		name  string
		files map[string]string
		why   relay.StopReason
	}{
		{"appended", with(ar32aHead + ar32aOrch + ar32aRows + "  - {id: b3, packet: design/packets/b3.md}\n"), ""},
		{"changed", with(ar32aHead + ar32aOrch + strings.Replace(ar32aRows, "id: b2,", "id: b2, model: m2,", 1)), relay.QueueInvalid},
		{"dropped", with(ar32aHead + ar32aOrch + "  - {id: b1, packet: design/packets/b1.md, parked: acceptance}\n"), relay.QueueInvalid},
		{"unparked", with(ar32aHead + ar32aOrch + strings.Replace(ar32aRows, ", parked: acceptance", "", 1)), relay.QueueInvalid},
		{"reordered", with(ar32aHead + ar32aOrch + "  - {id: b2, packet: design/packets/b2.md}\n  - {id: b1, packet: design/packets/b1.md, parked: acceptance}\n"), relay.QueueInvalid},
		{"no packet", with(ar32aHead + ar32aOrch + ar32aRows + "  - {id: b4, packet: design/packets/b4.md}\n"), relay.QueueInvalid},
		{"unparsable", with("version: [\n"), relay.QueueInvalid},
		{"deleted", map[string]string{"history/sittings/2026-09-29-1-o.md": ar32aCharter, effectsQueue: ""}, relay.QueueInvalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, o := orchWorld(t, ar32aRows)
			res, why, g := orchSitting(t, w, o, c.files)
			want := relay.GreenResult
			if c.why != "" {
				want = relay.RedResult
			}
			if res != want || why != c.why || g.QueueInvalid != (c.why != "") {
				t.Errorf("%s, %q, queue_invalid %v, %q; want %s, %q", res, why, g.QueueInvalid, queueDetail(w), want, c.why)
			}
		})
	}
}

// TestAR32A_3_GateHoldsTrustFieldsToNightStart is ar32a act 3 (§5): the
// defaults, words and orchestrator row equal the queue as the night began,
// never main's; an added row's model, merge, device and harness are unset
// or the defaults', and its budget within theirs; else queue-invalid.
func TestAR32A_3_GateHoldsTrustFieldsToNightStart(t *testing.T) {
	charter := "history/sittings/2026-09-29-1-o.md"
	for _, c := range []struct {
		name, queue string
		mainMoved   bool // main's queue raised the defaults' turns after the night began
		why         relay.StopReason
		detail      string
	}{
		{"as the night began", ar32aHead + ar32aOrch + ar32aRows, false, "", ""},
		{"main's raised turns kept", strings.Replace(ar32aHead, "turns: 10", "turns: 90", 1) + ar32aOrch + ar32aRows, true, relay.QueueInvalid, "defaults"},
		{"defaults' model", strings.Replace(ar32aHead, "model: m,", "model: m2,", 1) + ar32aOrch + ar32aRows, false, relay.QueueInvalid, "defaults"},
		{"words", strings.Replace(ar32aHead, "words: go on", "words: push it", 1) + ar32aOrch + ar32aRows, false, relay.QueueInvalid, "words"},
		{"the orchestrator's budget", ar32aHead + strings.Replace(ar32aOrch, "usd: 150", "usd: 900", 1) + ar32aRows, false, relay.QueueInvalid, "orchestrator row"},
		{"the orchestrator's may_touch", ar32aHead + strings.Replace(ar32aOrch, "[Makefile]", "[Makefile, scripts/]", 1) + ar32aRows, false, relay.QueueInvalid, "orchestrator row"},
		{"an added row's model", ar32aHead + ar32aOrch + ar32aRows + "  - {id: b3, packet: design/packets/b3.md, model: m2}\n", false, relay.QueueInvalid, "model, merge, device or harness"},
		{"an added row's device", ar32aHead + ar32aOrch + ar32aRows + "  - {id: b3, packet: design/packets/b3.md, device: the owner's words}\n", false, relay.QueueInvalid, "model, merge, device or harness"},
		{"an added row's merge", ar32aHead + ar32aOrch + ar32aRows + "  - {id: b3, packet: design/packets/b3.md, merge: branch-only}\n", false, relay.QueueInvalid, "model, merge, device or harness"},
		{"an added row's harness", ar32aHead + ar32aOrch + ar32aRows + "  - id: b3\n    packet: design/packets/b3.md\n    harness: [go.mod]\n", false, relay.QueueInvalid, "model, merge, device or harness"},
		{"an added row's budget", ar32aHead + ar32aOrch + ar32aRows + "  - id: b3\n    packet: design/packets/b3.md\n    budget: {turns: 11}\n", false, relay.QueueInvalid, "budget is above"},
		{"an added row within", ar32aHead + ar32aOrch + ar32aRows + "  - id: b3\n    packet: design/packets/b3.md\n    model: m\n    budget: {turns: 9, usd: 5}\n", false, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, o := orchWorld(t, ar32aRows)
			if c.mainMoved {
				write(t, w.root, effectsQueue, c.queue)
				gitIn(t, w.root, "commit", "-qam", "main's queue moved")
				began, err := queue.Load(w.root, effectsQueue)
				if err != nil {
					t.Fatal(err)
				}
				o.Began = began
			}
			res, why, g := orchSitting(t, w, o, map[string]string{charter: ar32aCharter, effectsQueue: c.queue})
			want := relay.GreenResult
			if c.why != "" {
				want = relay.RedResult
			}
			if d := queueDetail(w); res != want || why != c.why || g.QueueInvalid != (c.why != "") || !strings.Contains(d, c.detail) {
				t.Errorf("%s, %q, queue_invalid %v, %q; want %s, %q, %q", res, why, g.QueueInvalid, d, want, c.why, c.detail)
			}
		})
	}
}

// TestAR32A_3_TouchBounds is ar32a act 3 (§5): an orchestrator sitting may
// change every unguarded path and the guarded ones its may_touch names;
// the brief is guarded unless may_touch names it; a settings file is
// never inside; any merge commit on its first-parent line reds
// row-unproven (ar32b proves them).
func TestAR32A_3_TouchBounds(t *testing.T) {
	charter := "history/sittings/2026-09-29-1-o.md"
	for _, c := range []struct {
		name    string
		orch    string
		files   map[string]string
		outside []string
	}{
		{"unguarded anywhere", ar32aOrch, map[string]string{"internal/x/x.go": "package x\n", "design/packets/b9.md": "x\n", "docs/a.md": "a\n"}, nil},
		{"a guarded path named", ar32aOrch, map[string]string{"Makefile": "all:\n"}, nil},
		{"a guarded path not named", ar32aOrch, map[string]string{"scripts/x.sh": "true\n", ".hygiene-allow": "x\n"}, []string{".hygiene-allow", "scripts/x.sh"}},
		{"the brief", ar32aOrch, map[string]string{ar32aBrief: "# Brief, rewritten\n"}, []string{ar32aBrief}},
		{"the brief named", strings.Replace(ar32aOrch, "[Makefile]", "[Makefile, "+ar32aBrief+"]", 1), map[string]string{ar32aBrief: "# Brief, rewritten\n"}, nil},
		{"settings", ar32aOrch, map[string]string{"x/.claude/settings.json": "{}\n"}, []string{"x/.claude/settings.json"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, o := orchWorldWith(t, c.orch, ar32aRows)
			files := map[string]string{charter: ar32aCharter}
			for k, v := range c.files {
				files[k] = v
			}
			res, why, g := orchSitting(t, w, o, files)
			slices.Sort(g.Outside)
			want := relay.GreenResult
			if len(c.outside) > 0 {
				want = relay.RedResult
			}
			if res != want || why != "" || !slices.Equal(g.Outside, c.outside) && !(len(g.Outside) == 0 && len(c.outside) == 0) {
				t.Errorf("%s, %q, outside %q, %q; want %s, %q", res, why, g.Outside, queueDetail(w), want, c.outside)
			}
		})
	}
	t.Run("a merge commit", func(t *testing.T) {
		w, o := orchWorld(t, ar32aRows)
		res, why, g := orchSitting(t, w, o, map[string]string{charter: ar32aCharter}, func(tree string) {
			gitIn(t, tree, "checkout", "-q", "-b", "side", "HEAD~1")
			write(t, tree, "side.txt", "side\n")
			gitIn(t, tree, "add", ".")
			gitIn(t, tree, "commit", "-qm", "a helper's branch")
			gitIn(t, tree, "checkout", "-q", "o.1")
			gitIn(t, tree, "merge", "-q", "--no-ff", "-m", "merge side", "side")
		})
		if res != relay.RedResult || why != relay.RowUnproven || !g.Unproven {
			t.Errorf("%s, %q, unproven %v; want red, row-unproven", res, why, g.Unproven)
		}
	})
}

// TestAR32A_3_BuildRowQueueEditRed is ar32a act 3 (§5): in an orchestrated
// night the queue is pinned for every build row: one that changes it is
// outside its bounds, whatever its packet's may-touch says; with no
// QueuePath the gate is today's.
func TestAR32A_3_BuildRowQueueEditRed(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, effectsQueue, "a.txt")
	p := pk([]string{"relay/", "a.txt"})
	res, _, g, _ := Run(w.fx, st, p, nil, seen, Verify{QueuePath: effectsQueue})
	if res != relay.RedResult || !slices.Equal(g.Outside, []string{effectsQueue}) {
		t.Errorf("pinned: %s, outside %q; want red, the queue", res, g.Outside)
	}
	res, _, g, _ = Run(w.fx, st, p, nil, seen, Verify{})
	if res != relay.GreenResult || len(g.Outside) != 0 {
		t.Errorf("unpinned: %s, outside %q; want green", res, g.Outside)
	}
}
