package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// ar32bGit runs git in dir with a fixed author and no global config.
func ar32bGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestAR32B_4_OrchestrateAcceptsBranchOnly is ar32b act 4 (orchestrator
// r1 §6, §11): `relay run --orchestrate` starts over branch-only
// defaults, the standing row naming no merge and so merging on green; one
// naming branch-only is still refused, as a queue problem.
func TestAR32B_4_OrchestrateAcceptsBranchOnly(t *testing.T) {
	_, l := planFakes(t, "main")
	measuring(t)
	top, _ := toplevel()
	put := func(path, text string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const head = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: branch-only, device: never}\nwords: \"\"\nrows:\n"
	const orch = "  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
	put(filepath.Join(getenv("HOME"), ".config", "agent-relay", "config.yaml"), "ntfy:\n  url: https://ntfy.invalid\n  topic: relay-test\n")
	put(filepath.Join(top, "relay", "queue.yaml"), head+orch)
	l.Prior = l.Prior[:1]
	if why := orchestrateRefusal(); why != "" {
		t.Fatalf("branch-only defaults refused: %q", why)
	}
	put(filepath.Join(top, "relay", "queue.yaml"), head+orch+"    merge: branch-only\n")
	var out, errb bytes.Buffer
	if code := run([]string{"run", "--orchestrate", "--max-usd", "50"}, strings.NewReader(""), &out, &errb); code != exitUsage ||
		!strings.Contains(errb.String(), "an orchestrator row merges merge-on-green, not \"branch-only\"") {
		t.Errorf("a row naming branch-only: exit %d, stderr %q; want 2, a queue problem", code, errb.String())
	}
}

// TestAR32B_3_CleanMergedTakesTheHop is ar32b act 3 (orchestrator r1 §6),
// in real git: b's worktree, merged into sitting o.2 that landed on main,
// goes with o.2's under `relay clean --merged`, since ancestry covers the
// hop; c, green on its branch and never merged, stays.
func TestAR32B_3_CleanMergedTakesTheHop(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(dir, "root")
	ar32bGit(t, dir, "init", "-q", "-b", "main", root)
	ar32bGit(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	trees := filepath.Join(root, ".claude", "worktrees")
	for _, b := range []string{"b", "c", "o.2"} {
		ar32bGit(t, root, "worktree", "add", "-q", "-b", b, filepath.Join(trees, b), "main")
		ar32bGit(t, filepath.Join(trees, b), "commit", "-q", "--allow-empty", "-m", b+"'s work")
	}
	ar32bGit(t, filepath.Join(trees, "o.2"), "merge", "-q", "--no-ff", "--no-edit", "b")
	ar32bGit(t, root, "merge", "-q", "--no-ff", "--no-edit", "o.2")
	if ok, err := (effects.OSGit{}).FirstParentMerged(root, "b", "main"); err != nil || ok {
		t.Fatalf("b is merged on main's own first-parent line (%v): no hop to test", err)
	}

	oldEnv, oldFx := getenv, effectsFor
	t.Cleanup(func() { getenv, effectsFor = oldEnv, oldFx })
	fakeTop(t, root)
	getenv = func(string) string { return "" }
	effectsFor = func(string, string) effects.Effects { return effects.Effects{Git: effects.OSGit{}} }
	var out, errb bytes.Buffer
	if code := run([]string{"clean", "--merged", "--json"}, strings.NewReader(""), &out, &errb); code != exitOK {
		t.Fatalf("clean --merged = %d, %s %s", code, out.String(), errb.String())
	}
	var rep struct{ Removed []string }
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	slices.Sort(rep.Removed)
	if want := []string{filepath.Join(trees, "b"), filepath.Join(trees, "o.2")}; !slices.Equal(rep.Removed, want) {
		t.Fatalf("removed %v, want %v", rep.Removed, want)
	}
	if _, err := os.Stat(filepath.Join(trees, "c")); err != nil {
		t.Fatalf("c's worktree went: %v", err)
	}
}
