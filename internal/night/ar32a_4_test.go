package night_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/gate"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

const (
	orchHead = "version: 1\ndefaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,\n" +
		"           merge: merge-on-green, device: never, verify: auto}\nwords: \"\"\nrows:\n"
	orchRow = "  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
)

// buildRow is a build row's queue lines.
func buildRow(id string) string {
	return fmt.Sprintf("  - id: %s\n    packet: design/packets/%s.md\n", id, id)
}

// charterOf is a charter whose Next act is act.
func charterOf(act string) string { return "# Sitting\n\n## Next act\n" + act + "\n" }

// orchNight commits the brief, a charter of o at main with Next act act,
// the queue (the standing row, then builds, each with its packet) and
// extra files (path to text) to a fresh root. files is the fake's
// FAKECLAUDE_FILES directory: <files>/<branch>/ is what a sitting commits.
func orchNight(t *testing.T, act string, extra map[string]string, builds ...string) (*world, *queue.Queue, string) {
	t.Helper()
	fakeClaude(t)
	w := newWorld(t)
	put := func(dir, p, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	y := orchHead + orchRow
	for _, id := range builds {
		y += buildRow(id)
		put(w.root, "design/packets/"+id+".md", fmt.Sprintf(packetSrc, id, "ok"))
	}
	put(w.root, "queue.yaml", y)
	put(w.root, "brief.md", "# Brief\n\n## How this runs\nOne act a sitting.\n")
	put(w.root, "history/sittings/2026-09-28-1-o.md", charterOf(act))
	put(w.root, ".gitignore", ".claude/\n")
	for p, text := range extra {
		put(w.root, p, text)
	}
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "the queue")
	q, err := queue.Load(w.root, "queue.yaml")
	if err != nil || len(q.Validate()) > 0 {
		t.Fatalf("queue: %v %v", err, q.Validate())
	}
	return w, q, t.TempDir()
}

// sit writes what sitting (a branch) commits: its charter, dated
// 2026-09-29 and numbered n, with act, and extra files.
func sit(t *testing.T, files, sitting string, n int, act string, extra map[string]string) {
	t.Helper()
	all := map[string]string{fmt.Sprintf("history/sittings/2026-09-29-%d-o.md", n): charterOf(act)}
	for p, text := range extra {
		all[p] = text
	}
	for p, text := range all {
		dst := filepath.Join(files, sitting, p)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// orchOpts is an orchestrated night over the fake claude: each sitting is
// gated as the standing row's against the night's queue and the one it
// began from, each build row with the queue pinned, and green merges.
func (w *world) orchOpts(t *testing.T, q0 *queue.Queue, files string, env ...string) night.NightOptions {
	o := night.NightOptions{State: t.TempDir(), Poll: 20 * time.Millisecond, Orchestrate: true, QueuePath: "queue.yaml",
		PlanOptions: night.Options{Repo: w.root, Night: "2026-09-29-1", AllowedTools: "Bash Edit Write", UUID: night.NewUUID}}
	o.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		v, pk := gate.Verify{QueuePath: "queue.yaml"}, r.Packet
		if rp.Sitting != nil {
			v.Orchestrator, pk = &gate.Orchestrated{Row: r, Night: q0, Began: rp.Sitting.Began}, &packet.Packet{ID: rp.ID}
		}
		return []byte("prompt"), night.RunOptions{Context: r.Context, Transcripts: w.transcripts,
			Env: w.env("ok.jsonl", append([]string{"FAKECLAUDE_FILES=" + files}, env...)...),
			Gate: func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
				res, why, g, _ := gate.Run(w.fx, st, pk, rp.Baseline, &gate.ExitFacts{Seen: seen, IsError: isError}, v)
				return res, why, g
			}}, nil
	}
	o.Merge = func(r queue.Row, g relay.Gate) (string, error) {
		return gate.MergeIfGreen(w.fx, w.root, g, r.ID, q0.Defaults.Verify)
	}
	return o
}

// sittings is the rows the night's sitting lines name, in order, each
// with its state.
func sittings(w *world) []string {
	var out []string
	for _, l := range w.ledger.Written() {
		if l["event"] == "sitting" {
			out = append(out, fmt.Sprintf("%v=%v", l["row"], l["state"]))
		}
	}
	return out
}

// stopOf is the night's stop line.
func stopOf(t *testing.T, w *world) map[string]any {
	t.Helper()
	s := lastOf(w.ledger.Written(), "stop")
	if s == nil {
		t.Fatal("no stop line")
	}
	return s
}

// TestAR32A_4_RestartsAfterMerge is ar32a act 4 (orchestrator r1 §4): the
// standing row runs as o.1, then fresh as o.2 after o.1's merge, each on
// its own branch and session with RELAY_ROW=o, and DONE stops the night
// done.
func TestAR32A_4_RestartsAfterMerge(t *testing.T) {
	w, q, files := orchNight(t, "- act 1: the row", nil)
	sit(t, files, "o.1", 1, "- act 2: the loop", nil)
	sit(t, files, "o.2", 2, "DONE: the row is built", nil)
	envs := t.TempDir()
	n := runNight(t, w, q, w.orchOpts(t, q, files, "FAKECLAUDE_ENVDUMP="+envs))
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "o.2=green"}) || n.Reason != relay.Done {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	env, err := os.ReadFile(filepath.Join(envs, "o.env"))
	if err != nil || !strings.Contains(string(env), "\nRELAY_ROW=o\n") {
		t.Fatalf("RELAY_ROW: %v %q", err, env)
	}
	if _, err := os.Stat(filepath.Join(envs, "o.1.env")); err == nil {
		t.Fatal("a sitting ran as RELAY_ROW=o.1")
	}
	spawns := map[string]string{}
	for _, l := range w.ledger.Written() {
		if l["event"] == "spawn" {
			spawns[fmt.Sprint(l["row"])] = fmt.Sprint(l["session_id"])
		}
	}
	if len(spawns) != 2 || spawns["o.1"] == spawns["o.2"] {
		t.Fatalf("spawns %v: want o.1 and o.2, fresh sessions", spawns)
	}
	if main := gitIn(t, w.root, "rev-parse", "main"); n.Sittings["o.2"].MergeSHA != main {
		t.Fatalf("o.2 merged at %q, main %s", n.Sittings["o.2"].MergeSHA, main)
	}
	if stop := stopOf(t, w); stop["detail"] != "DONE: the row is built" {
		t.Fatalf("stop %v", stop)
	}
}

// TestAR32A_4_RereadsQueueAfterMerge: o.1 appends row b; after its merge
// the relay reads the queue at main, writes queue-reread with the new
// hash and b, plans b and runs it before o.2.
func TestAR32A_4_RereadsQueueAfterMerge(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil)
	y := orchHead + orchRow + buildRow("b")
	sit(t, files, "o.1", 1, "- act 2", map[string]string{"queue.yaml": y,
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")})
	sit(t, files, "o.2", 2, "DONE: b is in", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "b=green", "o.2=green"}) || n.Reason != relay.Done {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	var re []map[string]any // o.1's re-read adds b, o.2's nothing
	for _, l := range w.ledger.Written() {
		if l["event"] == "queue-reread" {
			re = append(re, l)
		}
	}
	nq, err := queue.Load(w.root, "queue.yaml")
	if err != nil || len(re) != 2 || re[0]["hash"] != nq.Hash || fmt.Sprint(re[0]["added"]) != "[b]" || fmt.Sprint(re[1]["added"]) != "[]" {
		t.Fatalf("queue-reread lines %v, main's queue %v", re, err)
	}
	if n.Sittings["b"].MergeSHA == "" {
		t.Fatal("b did not land")
	}
}

// TestAR32A_4_BuildRowsFirst: a ready build row runs before the standing
// row's first sitting.
func TestAR32A_4_BuildRowsFirst(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil, "b1")
	sit(t, files, "o.1", 1, "DONE: nothing left", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	if got := sittings(w); !slices.Equal(got, []string{"b1=green", "o.1=green"}) || n.Reason != relay.Done {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
}

// TestAR32A_4_StuckTwiceStops: a sitting whose merge changed only its
// charter and left the key (RESPAWN: stripped) as it began is stuck; the
// first is allowed, the second stops no-progress.
func TestAR32A_4_StuckTwiceStops(t *testing.T) {
	w, q, files := orchNight(t, "- act 1: the loop", nil)
	sit(t, files, "o.1", 1, "RESPAWN: - act 1: the loop", nil)
	sit(t, files, "o.2", 2, "- act 1:   the loop", nil)
	sit(t, files, "o.3", 3, "DONE: never reached", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "o.2=green"}) || n.Reason != relay.NoProgress {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	if stop := stopOf(t, w); !strings.Contains(fmt.Sprint(stop["detail"]), "stuck twice") {
		t.Fatalf("stop %v", stop)
	}
}

// TestAR32A_4_DoneWaitsForItsRows: o.1 adds b and writes DONE; b is
// ready, so DONE reads as WAITING, b runs and lands, o.2 writes DONE
// again and the night stops done.
func TestAR32A_4_DoneWaitsForItsRows(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil)
	sit(t, files, "o.1", 1, "DONE: b is the last", map[string]string{"queue.yaml": orchHead + orchRow + buildRow("b"),
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")})
	sit(t, files, "o.2", 2, "DONE: b landed", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "b=green", "o.2=green"}) || n.Reason != relay.Done {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	if stop := stopOf(t, w); stop["detail"] != "DONE: b landed" {
		t.Fatalf("stop %v", stop)
	}
}

// TestAR32A_4_WaitingStops: WAITING with no ready build row stops the
// night waiting (ar32c adds the idle hold).
func TestAR32A_4_WaitingStops(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil)
	sit(t, files, "o.1", 1, "WAITING: the owner's answer to question 3", nil)
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green"}) || n.Reason != relay.StillWaiting || night.Clean(n) {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	if stop := stopOf(t, w); !strings.Contains(fmt.Sprint(stop["detail"]), "WAITING: the owner's answer") {
		t.Fatalf("stop %v", stop)
	}
}

// TestAR32A_4_RedRunsReadyRowsThenStops: o.1 lands b; o.2 edits the
// brief and reads red; it is parked (no o.3, agentik relay resume o.2), the
// rows ready run, and the night stops orchestrator-red.
func TestAR32A_4_RedRunsReadyRowsThenStops(t *testing.T) {
	w, q, files := orchNight(t, "- act 1", nil)
	sit(t, files, "o.1", 1, "- act 2", map[string]string{"queue.yaml": orchHead + orchRow + buildRow("b"),
		"design/packets/b.md": fmt.Sprintf(packetSrc, "b", "ok")})
	sit(t, files, "o.2", 2, "- act 3", map[string]string{"brief.md": "# Brief, rewritten\n"})
	sit(t, files, "o.3", 3, "DONE: never reached", nil)
	o := w.orchOpts(t, q, files)
	var out strings.Builder
	o.Out = &out
	n := runNight(t, w, q, o)
	if got := sittings(w); !slices.Equal(got, []string{"o.1=green", "b=green", "o.2=red"}) || n.Reason != relay.OrchestratorRed {
		t.Fatalf("sittings %v, stop %s", got, n.Reason)
	}
	if !strings.Contains(out.String(), "agentik relay resume o.2") {
		t.Fatalf("no resume line:\n%s", out.String())
	}
}

// TestAR32A_4_MergeHeldStops: a green sitting whose merge fails with a
// plain error stops merge-held and names the branch; a refused merge
// reads red, so the night stops orchestrator-red.
func TestAR32A_4_MergeHeldStops(t *testing.T) {
	for _, c := range []struct {
		err  error
		want relay.StopReason
	}{
		{errors.New("the root is dirty"), relay.MergeHeld},
		{&relay.MergeRefused{Reason: relay.MergeTreeDiffers, Err: errors.New("the merge tree differs")}, relay.OrchestratorRed},
	} {
		w, q, files := orchNight(t, "- act 1", nil)
		sit(t, files, "o.1", 1, "- act 2", nil)
		o := w.orchOpts(t, q, files)
		o.Merge = func(queue.Row, relay.Gate) (string, error) { return "", c.err }
		n := runNight(t, w, q, o)
		stop := stopOf(t, w)
		if n.Reason != c.want || len(sittings(w)) != 1 {
			t.Fatalf("%v: stop %v, sittings %v", c.err, stop, sittings(w))
		}
		if c.want == relay.MergeHeld && !strings.Contains(fmt.Sprint(stop["detail"]), "branch o.1") {
			t.Fatalf("merge-held detail %v", stop["detail"])
		}
	}
}

// TestAR32A_4_QuestionOnRise: a sitting that raises OPEN-QUESTIONS.md's
// open count at main writes one question line: the count, the file and
// main's sha12, never the text.
func TestAR32A_4_QuestionOnRise(t *testing.T) {
	const qs = "# Open questions\n\n## 1. The first?\nText one.\n\n%s## Answered\n\n## 7. Old?\n"
	if got := night.OpenQuestions([]byte(fmt.Sprintf(qs, "## 2. The second?\nText two.\n\n"))); got != 2 {
		t.Fatalf("OpenQuestions %d, want 2", got)
	}
	w, q, files := orchNight(t, "- act 1", map[string]string{"OPEN-QUESTIONS.md": fmt.Sprintf(qs, "")})
	sit(t, files, "o.1", 1, "DONE: one question asked", map[string]string{
		"OPEN-QUESTIONS.md": fmt.Sprintf(qs, "## 2. The secret question?\nText two.\n\n")})
	n := runNight(t, w, q, w.orchOpts(t, q, files))
	if n.Reason != relay.Done {
		t.Fatalf("stop %s", n.Reason)
	}
	var lines []map[string]any
	for _, l := range w.ledger.Written() {
		if l["event"] == "question" {
			lines = append(lines, l)
		}
	}
	main := gitIn(t, w.root, "rev-parse", "main")
	if len(lines) != 1 || number(lines[0]["open"]) != 2 || lines[0]["file"] != "OPEN-QUESTIONS.md" || lines[0]["at"] != main[:12] ||
		strings.Contains(fmt.Sprint(lines[0]), "secret") {
		t.Fatalf("question lines %v, main %s", lines, main)
	}
}
