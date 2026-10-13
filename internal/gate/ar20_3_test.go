package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// remoteMain is the bare remote's main in w.
func (w *world) remoteMain(t *testing.T) (string, string) {
	t.Helper()
	remote := filepath.Join(filepath.Dir(w.root), "remote.git")
	return remote, gitIn(t, remote, "rev-parse", "refs/heads/main")
}

// TestAR20_3_VerifyLineCannotPush is ADV11-6 and -7: a line of the branch's
// that pushes to origin by name is denied (ChildEnv's pushurl), one that
// pushes by URL and repoints origin at a mirror is caught by the literal
// URL's ls-remote after the lines, and a branch script main's harness runs
// only on the merged tree is caught after mergedVerify: never green, never
// merged.
func TestAR20_3_VerifyLineCannotPush(t *testing.T) {
	line := packet.Line{Shell: "sh t.sh", Expect: "exit 0"}
	t.Run("by-name", func(t *testing.T) {
		w := hermeticWorld(t)
		remote, was := w.remoteMain(t)
		st := w.stage(t, "b.txt")
		write(t, w.tree, "t.sh", "git push -q origin HEAD:main\n")
		gitIn(t, w.tree, "add", "t.sh")
		gitIn(t, w.tree, "commit", "-qm", "push")
		res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt", "t.sh"}, line), nil, seen, Verify{})
		if res == relay.GreenResult {
			t.Fatalf("Run = green %q; lines %+v", why, g.Lines)
		}
		if now := gitIn(t, remote, "rev-parse", "refs/heads/main"); now != was {
			t.Fatalf("the line pushed origin's main to %s", now)
		}
	})
	t.Run("by-url-and-repoint", func(t *testing.T) {
		w := hermeticWorld(t)
		remote, _ := w.remoteMain(t)
		mirror := filepath.Join(filepath.Dir(w.root), "mirror.git")
		gitIn(t, w.root, "clone", "-q", "--bare", remote, mirror)
		st := w.stage(t, "b.txt")
		write(t, w.tree, "t.sh", "git push -q "+remote+" HEAD:main && git config remote.origin.url "+mirror+"\n")
		gitIn(t, w.tree, "add", "t.sh")
		gitIn(t, w.tree, "commit", "-qm", "push")
		res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt", "t.sh"}, line), nil, seen, Verify{})
		if res == relay.GreenResult || why != relay.OriginMoved {
			t.Fatalf("Run = %s %q, want origin-moved; origin_same %v, lines %+v", res, why, g.OriginSame, g.Lines)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
			t.Fatalf("merged after a push by URL: %v", err)
		}
	})
	t.Run("after-merged-verify", func(t *testing.T) {
		w := hermeticWorld(t)
		remote, _ := w.remoteMain(t)
		write(t, w.root, "Makefile", "check:\n\tgrep -qx a a.txt\n\tsh t.sh\n")
		write(t, w.root, "t.sh", "true\n")
		gitIn(t, w.root, "add", ".")
		gitIn(t, w.root, "commit", "-qm", "main runs t.sh")
		st := w.stage(t, "b.txt")
		// Only on the merged tree (a merge in progress) does it push.
		write(t, w.tree, "t.sh", "if test -e \"$(git rev-parse --git-path MERGE_HEAD)\"; then git push -q "+remote+" HEAD:refs/heads/evil; fi\n")
		gitIn(t, w.tree, "add", "t.sh")
		gitIn(t, w.tree, "commit", "-qm", "push when merged")
		res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt", "t.sh"}), nil, seen, Verify{Name: "auto"})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s %q, want green; verify %+v", res, why, g.Verify)
		}
		_, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto")
		if err == nil || !strings.Contains(err.Error(), "origin-moved") || w.merged() {
			t.Fatalf("merge after mergedVerify pushed = %v, merged %v", err, w.merged())
		}
		if _, err := os.Stat(filepath.Join(w.root, "b.txt")); err == nil {
			t.Fatal("b.txt is on main")
		}
	})
}
