package gate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// replaceWorld is hermeticWorld (main's `make check` holds a.txt to `a`)
// with XDG_CONFIG_HOME a temp dir too, a1 staged with a broken a.txt, and
// the child's `git replace` in its own tree: a.txt's committed blob reads
// as main's `a` through refs/replace/, which the common dir shares.
func replaceWorld(t *testing.T) (*world, *night.Staged, *packet.Packet) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// The test's own git reads replace refs, even when this process runs
	// under a pinned env (a gate's make check: AR22 act 1).
	t.Setenv("GIT_NO_REPLACE_OBJECTS", "")
	os.Unsetenv("GIT_NO_REPLACE_OBJECTS")
	w := hermeticWorld(t)
	st := w.stage(t, "a.txt") // a.txt is "a1 a.txt": main's check wants "a"
	bad := gitIn(t, w.tree, "rev-parse", "HEAD:a.txt")
	good := gitIn(t, w.tree, "rev-parse", "main:a.txt")
	gitIn(t, w.tree, "replace", bad, good)
	if got := gitIn(t, w.tree, "show", "HEAD:a.txt"); got != "a" {
		t.Fatalf("the replace ref does not bend a.txt: %q", got)
	}
	return w, st, pk([]string{"a.txt"}, packet.Line{Shell: "cat a.txt", Expect: "reads `a`", Kind: packet.Reads, Value: "a"})
}

// TestAR22_0_ReplaceRefs is ADV13-1 (review test 1): a replace ref the
// child writes in its tree judges one tree and merges another. The gate's
// lines and verify line run in a clone that fetched the branch by its full
// refname (no refs/replace/), and the runner sets GIT_NO_REPLACE_OBJECTS:
// Run is never green, and MergeIfGreen never merges, whether the gate
// reads a.txt or only the merge's verify line does.
func TestAR22_0_ReplaceRefs(t *testing.T) {
	t.Run("gate", func(t *testing.T) {
		w, st, p := replaceWorld(t)
		res, why, g, _ := Run(w.fx, st, p, nil, seen, Verify{Name: "auto"})
		if res == relay.GreenResult {
			t.Fatalf("Run = green %q: the replace ref judged a.txt as `a`; lines %+v, verify %+v", why, g.Lines, g.Verify)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
			t.Fatalf("merged a broken a.txt through a replace ref: %v", err)
		}
	})
	t.Run("merge", func(t *testing.T) {
		w, st, _ := replaceWorld(t)
		res, why, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run with no line = %s %q, want green", res, why)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
			t.Fatalf("the merge's verify judged a.txt through a replace ref and merged: %v", err)
		}
		if got := gitIn(t, w.root, "--no-replace-objects", "show", "main:a.txt"); got != "a" {
			t.Fatalf("main's a.txt is %q", got)
		}
	})
}

// TestAR22_0_CloneHoldsNoRootState: the tree the gate's lines run in is a
// new repository (its own .git directory), holding the refs it fetched by
// full refname and nothing of the root's: no config key, no replace or
// remote ref, no alternates, no hooks, no info/, no shallow; and it is no
// worktree of the root, so `clean --merged` never sees it (ADV13-1, -14).
func TestAR22_0_CloneHoldsNoRootState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	w := hermeticWorld(t)
	gitIn(t, w.root, "config", "relay.probe", "root")
	hook(t, filepath.Join(w.root, ".git", "hooks"), "post-checkout", "true\n")
	other := filepath.Join(filepath.Dir(w.root), "other.git")
	gitIn(t, w.root, "init", "-q", "--bare", other)
	write(t, w.root, ".git/objects/info/alternates", filepath.Join(other, "objects")+"\n")
	write(t, w.root, "x.txt", "x\n")
	write(t, w.root, "y.txt", "y\n")
	x, y := gitIn(t, w.root, "hash-object", "-w", "x.txt"), gitIn(t, w.root, "hash-object", "-w", "y.txt")
	os.Remove(filepath.Join(w.root, "x.txt"))
	os.Remove(filepath.Join(w.root, "y.txt"))
	gitIn(t, w.root, "replace", x, y)
	st := w.stage(t, "b.txt")
	probe := `c=$(git rev-parse --path-format=absolute --git-common-dir); ` +
		`echo "$(test -d .git && echo dir || echo gitfile) $(git config --get relay.probe || echo noconfig)` +
		` $(git for-each-ref --format='%(refname)' | tr '\n' ,)` +
		` $(test -e "$c/objects/info/alternates" && echo alternates || echo noalternates)` +
		` $(ls -A "$c/hooks" 2>/dev/null | wc -l | tr -d ' ')hooks $(test -e "$c/info" && echo info || echo noinfo)` +
		` $(test -e "$c/shallow" && echo shallow || echo noshallow)` +
		` $(git -C '` + w.root + `' worktree list --porcelain | grep -c /fresh-)worktrees"`
	want := "dir noconfig refs/heads/a1,refs/heads/main, noalternates 0hooks noinfo noshallow 0worktrees"
	p := pk([]string{"b.txt"}, packet.Line{Shell: probe, Expect: "reads `" + want + "`", Kind: packet.Reads, Value: want})
	res, why, g, err := Run(w.fx, st, p, nil, seen, Verify{})
	if res != relay.GreenResult {
		last := ""
		if len(g.Lines) > 0 {
			last = g.Lines[0].BranchLast
		}
		t.Fatalf("Run = %s %q (%v): the fresh tree reads\n %q\nwant %q", res, why, err, last, want)
	}
	if out := gitIn(t, w.root, "worktree", "list", "--porcelain"); strings.Contains(out, "fresh-") {
		t.Fatalf("a fresh tree is left in the root's worktree list:\n%s", out)
	}

	// Each ref is fetched at its bound sha, else ref-moved and nothing is
	// left under the state root's gate/; done removes the clone.
	state := t.TempDir()
	fx := w.fx
	fx.Getenv = func(k string) string { return map[string]string{"XDG_STATE_HOME": state}[k] }
	gates := filepath.Join(state, "agent-relay", "gate")
	a1, main := gitIn(t, w.root, "rev-parse", "a1"), gitIn(t, w.root, "rev-parse", "main")
	if _, _, err := Fresh(fx, w.root, effects.Ref{Name: "refs/heads/a1", Sha: main}); !errors.Is(err, effects.ErrRefMoved) {
		t.Fatalf("Fresh with a1 bound at main's sha: %v, want ref-moved", err)
	}
	if es, _ := os.ReadDir(gates); len(es) != 0 {
		t.Fatalf("a refused clone left %d entries under gate/", len(es))
	}
	dir, done, err := Fresh(fx, w.root, effects.Ref{Name: "refs/heads/a1", Sha: a1}, effects.Ref{Name: "refs/heads/main", Sha: main})
	if err != nil {
		t.Fatal(err)
	}
	if head := gitIn(t, dir, "rev-parse", "HEAD"); head != a1 || gitIn(t, dir, "branch", "--show-current") != "" {
		t.Fatalf("the clone is at %s, want a1's %s detached", head, a1)
	}
	done()
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("done left the clone: %v", err)
	}
}
