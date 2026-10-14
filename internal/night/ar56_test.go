package night_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// ar56Night is the night the AR56 tests' literal ledger lines name.
const ar56Night = "2026-10-03-1"

// ar56Root is a real root under t.TempDir (ar13_landed_test.go's shape, over
// gitIn) after the owner's hand: g1 green and r1 red, each merged --no-ff
// onto main; g2 green, fast-forwarded; r2 red, its branch never moved. It
// returns the root and the two merge shas, g1's and r1's.
func ar56Root(t *testing.T) (root, g1, r1 string) {
	t.Helper()
	root = ar56Init(t)
	gitIn(t, root, "branch", "r2") // red, never moved: its tip is main's line
	sha := map[string]string{}
	for _, id := range []string{"g1", "r1"} {
		gitIn(t, root, "checkout", "-q", "-b", id)
		gitIn(t, root, "commit", "-q", "--allow-empty", "-m", id+" work")
		gitIn(t, root, "checkout", "-q", "main")
		gitIn(t, root, "merge", "-q", "--no-ff", "-m", "merge "+id, id)
		sha[id] = gitIn(t, root, "rev-parse", "HEAD")
	}
	gitIn(t, root, "checkout", "-q", "-b", "g2")
	gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "g2 work")
	gitIn(t, root, "checkout", "-q", "main")
	gitIn(t, root, "merge", "-q", "--ff-only", "g2")
	return root, sha["g1"], sha["r1"]
}

// ar56Init is a fresh root under t.TempDir, main at one empty commit, git
// held to the test's own identity and no config of the account's.
func ar56Init(t *testing.T) string {
	t.Helper()
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_GLOBAL": os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}
	root := filepath.Join(t.TempDir(), "root")
	gitIn(t, filepath.Dir(root), "init", "-q", "-b", "main", root)
	gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	return root
}

// ar56Lines is the night's ledger as the relay wrote it: four rows staged
// off main, g1 and g2 green, r1 and r2 red, no `merge` line.
func ar56Lines(root string) []map[string]any {
	lines := []map[string]any{{"event": "night-open", "night": ar56Night, "root": root}}
	for _, r := range [][2]string{{"g1", "green"}, {"r1", "red"}, {"g2", "green"}, {"r2", "red"}} {
		lines = append(lines, map[string]any{"event": "stage", "night": ar56Night, "row": r[0], "branch": r[0], "base": "main"})
		l := map[string]any{"event": "sitting", "night": ar56Night, "row": r[0], "state": r[1]}
		if r[1] == "red" {
			l["reason"] = "acceptance"
		}
		lines = append(lines, l)
	}
	return lines
}

// TestAR56_1 (ruling 1): with a git, the report reads g1 and r1 merged at
// git's merge shas: g1 leaves Waiting, r1 leaves Resume and keeps its
// Stopped entry (the night's verdict) with no resume line. g2, fast-
// forwarded, still waits (§3: a fast-forward does not land); r2, never
// moved, keeps its resume. Without a git the report is as before.
func TestAR56_1(t *testing.T) {
	root, g1, r1 := ar56Root(t)
	lines := ar56Lines(root)
	want := map[string]string{"g1": g1, "r1": r1}
	if got := night.HandMerges(effects.OSGit{}, lines, ar56Night); !reflect.DeepEqual(got, want) {
		t.Fatalf("HandMerges = %v, want %v", got, want)
	}
	r := night.BuildReport(lines, ar56Night, effects.OSGit{})
	if m := []night.Merged{{Row: "g1", SHA: g1}, {Row: "r1", SHA: r1}}; !reflect.DeepEqual(r.Merged, m) {
		t.Fatalf("Merged = %v, want %v", r.Merged, m)
	}
	if !reflect.DeepEqual(r.Waiting, []string{"g2 (off main)"}) {
		t.Fatalf("Waiting = %v, want g2 alone", r.Waiting)
	}
	if !reflect.DeepEqual(r.Resume, []string{"agentik relay resume r2"}) {
		t.Fatalf("Resume = %v, want agentik relay resume r2 alone", r.Resume)
	}
	stopped := []night.StoppedRow{{Row: "r1", State: "red", Reason: "acceptance"},
		{Row: "r2", State: "red", Reason: "acceptance", Resume: "agentik relay resume r2"}}
	if !reflect.DeepEqual(r.Stopped.Rows, stopped) {
		t.Fatalf("Stopped.Rows = %+v, want %+v", r.Stopped.Rows, stopped)
	}

	// Without a git, as before AR56.
	r = night.BuildReport(lines, ar56Night)
	if len(r.Merged) != 0 || !reflect.DeepEqual(r.Waiting, []string{"g1 (off main)", "g2 (off main)"}) ||
		!reflect.DeepEqual(r.Resume, []string{"agentik relay resume r1", "agentik relay resume r2"}) {
		t.Fatalf("no git: merged %v waiting %v resume %v", r.Merged, r.Waiting, r.Resume)
	}
	// A git error (the root gone) leaves every row as it is.
	if got := night.HandMerges(effects.OSGit{}, ar56Lines(filepath.Join(root, "gone")), ar56Night); len(got) != 0 {
		t.Fatalf("HandMerges over a missing root = %v", got)
	}
	// A `merge` line that says merged is the night's own: no hand merge.
	own := append(ar56Lines(root), map[string]any{"event": "merge", "night": ar56Night, "row": "g1", "result": "merged", "sha": g1})
	if got := night.HandMerges(effects.OSGit{}, own, ar56Night); !reflect.DeepEqual(got, map[string]string{"r1": r1}) {
		t.Fatalf("HandMerges with g1's merge line = %v", got)
	}
}

// TestAR56_2 (ruling 2): TakeMerges gives a row in the map with no merge
// sha git's sha and no resume line, and takes its line out of Resume; its
// state and reason stay the night's. A row the night merged keeps its sha.
func TestAR56_2(t *testing.T) {
	st := night.Status{Night: ar56Night, State: "stopped", Reason: "queue-empty", Rows: []night.RowStatus{
		{ID: "g1", State: "green"},
		{ID: "r1", State: "red", Reason: "acceptance", Resume: "agentik relay resume r1"},
		{ID: "r2", State: "red", Reason: "acceptance", Resume: "agentik relay resume r2"},
		{ID: "g3", State: "green", MergeSHA: "c0ffee"},
	}, Resume: []string{"agentik relay resume r1", "agentik relay resume r2"}}
	st.TakeMerges(map[string]string{"g1": "a11ce5", "r1": "b0b0b0", "g3": "d00d00"})
	got := map[string]night.RowStatus{}
	for _, r := range st.Rows {
		got[r.ID] = r
	}
	if g1, r1 := got["g1"], got["r1"]; g1.MergeSHA != "a11ce5" || r1.MergeSHA != "b0b0b0" || r1.Resume != "" ||
		r1.State != "red" || r1.Reason != "acceptance" || g1.State != "green" {
		t.Fatalf("g1 %+v, r1 %+v", g1, r1)
	}
	if r2, g3 := got["r2"], got["g3"]; r2.MergeSHA != "" || r2.Resume != "agentik relay resume r2" || g3.MergeSHA != "c0ffee" {
		t.Fatalf("r2 %+v, g3 %+v", r2, g3)
	}
	if !reflect.DeepEqual(st.Resume, []string{"agentik relay resume r2"}) {
		t.Fatalf("Resume = %v, want agentik relay resume r2 alone", st.Resume)
	}
	var b bytes.Buffer
	night.PrintStatus(&b, st)
	if out := b.String(); strings.Contains(out, "agentik relay resume r1") || !strings.Contains(out, "agentik relay resume r2") {
		t.Fatalf("PrintStatus:\n%s", out)
	}
}

// TestAR56_3 (ruling 3): the night-open line names the relay's revision,
// the build's vcs.revision with +dirty when vcs.modified is true; a build
// with no vcs.revision (a test binary) has none, "".
func TestAR56_3(t *testing.T) {
	old := night.ReadBuildInfo
	t.Cleanup(func() { night.ReadBuildInfo = old })
	night.ReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.modified", Value: "true"}}}, true
	}
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	runNight(t, w, q, w.opts(t, by))
	var opens []any
	for _, l := range w.ledger.Written() {
		if l["event"] == "night-open" {
			opens = append(opens, l["revision"])
		}
	}
	if !reflect.DeepEqual(opens, []any{"abc123+dirty"}) {
		t.Fatalf("night-open revisions %v, want one abc123+dirty", opens)
	}
	night.ReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}}}, true
	}
	if r := night.Revision(); r != "" {
		t.Fatalf("Revision() with no vcs.revision = %q", r)
	}
	night.ReadBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }
	if r := night.Revision(); r != "" {
		t.Fatalf("Revision() with no build info = %q", r)
	}
}

// ar56Row is one row's `stage` and `sitting` lines in night n, staged off
// main at before; after, when not "", is the sitting's head_after.
func ar56Row(n, id, state, before, after string) []map[string]any {
	sit := map[string]any{"event": "sitting", "night": n, "row": id, "state": state, "head_before": before}
	if state != "green" {
		sit["reason"] = "acceptance"
	}
	if after != "" {
		sit["head_after"] = after
	}
	return []map[string]any{
		{"event": "stage", "night": n, "row": id, "branch": id, "base": "main", "head_before": before},
		sit,
	}
}

// TestAR56_5 (ruling 6, the review's F1 and F2). F1: r2 is red with no
// commit, so its tip is the cut X; while main is at X the owner runs `git
// merge main` on an older night's waiting row g0, then lands g0 with a
// plain `git merge`, which fast-forwards: X is now a later parent of a
// merge on main's first-parent line. r2 keeps `agentik relay resume r2`; g1,
// merged --no-ff by hand with its head_after its tip, still reads merged.
// F2: night N1's red r1 committed A; the branch name is reused, and night
// N2's r1 (B, off main) is merged by the relay. N1's r1 gets no sha, since
// A is not in B's history.
func TestAR56_5(t *testing.T) {
	const n1, n2 = "2026-10-03-1", "2026-10-03-2"
	t.Run("F1", func(t *testing.T) {
		root := ar56Init(t)
		gitIn(t, root, "checkout", "-q", "-b", "g0") // an older night's row, cut before X
		gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "g0 work")
		gitIn(t, root, "checkout", "-q", "main")
		gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "X")
		x := gitIn(t, root, "rev-parse", "HEAD")
		gitIn(t, root, "branch", "r2") // staged at X; red, no commit
		gitIn(t, root, "checkout", "-q", "-b", "g1")
		gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "g1 work")
		g1tip := gitIn(t, root, "rev-parse", "HEAD")
		gitIn(t, root, "checkout", "-q", "g0")
		gitIn(t, root, "merge", "-q", "--no-edit", "main") // g0 brought up to X
		gitIn(t, root, "checkout", "-q", "main")
		gitIn(t, root, "merge", "-q", "g0") // a plain merge: fast-forward
		gitIn(t, root, "merge", "-q", "--no-ff", "-m", "merge g1", "g1")
		g1 := gitIn(t, root, "rev-parse", "HEAD")
		lines := []map[string]any{{"event": "night-open", "night": n1, "root": root}}
		lines = append(lines, ar56Row(n1, "r2", "red", x, x)...)
		lines = append(lines, ar56Row(n1, "g1", "green", x, g1tip)...)
		if got, want := night.HandMerges(effects.OSGit{}, lines, n1), map[string]string{"g1": g1}; !reflect.DeepEqual(got, want) {
			t.Fatalf("HandMerges = %v, want %v (r2's tip is its cut)", got, want)
		}
		if r := night.BuildReport(lines, n1, effects.OSGit{}); !reflect.DeepEqual(r.Resume, []string{"agentik relay resume r2"}) {
			t.Fatalf("Resume = %v, want agentik relay resume r2", r.Resume)
		}
	})
	t.Run("F2", func(t *testing.T) {
		root := ar56Init(t)
		cut := gitIn(t, root, "rev-parse", "HEAD")
		gitIn(t, root, "checkout", "-q", "-b", "r1")
		gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "N1 r1 work A")
		a := gitIn(t, root, "rev-parse", "HEAD")
		gitIn(t, root, "checkout", "-q", "main")
		gitIn(t, root, "branch", "-D", "r1")
		gitIn(t, root, "checkout", "-q", "-b", "r1") // the name reused by N2, off main
		gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "N2 r1 work B")
		b := gitIn(t, root, "rev-parse", "HEAD")
		gitIn(t, root, "checkout", "-q", "main")
		gitIn(t, root, "merge", "-q", "--no-ff", "-m", "relay merge r1", "r1")
		m2 := gitIn(t, root, "rev-parse", "HEAD")
		lines := []map[string]any{{"event": "night-open", "night": n1, "root": root}}
		lines = append(lines, ar56Row(n1, "r1", "red", cut, a)...)
		lines = append(lines, map[string]any{"event": "night-open", "night": n2, "root": root})
		lines = append(lines, ar56Row(n2, "r1", "green", cut, b)...)
		lines = append(lines, map[string]any{"event": "merge", "night": n2, "row": "r1", "result": "merged", "sha": m2})
		if got := night.HandMerges(effects.OSGit{}, lines, n1); len(got) != 0 {
			t.Fatalf("N1's HandMerges = %v, want none (N1's commit is not in main)", got)
		}
		if r := night.BuildReport(lines, n1, effects.OSGit{}); len(r.Merged) != 0 ||
			!reflect.DeepEqual(r.Resume, []string{"agentik relay resume r1"}) {
			t.Fatalf("N1's report: merged %v, resume %v", r.Merged, r.Resume)
		}
	})
}
