package gate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// globalWorld is newWorld without its GIT_CONFIG_GLOBAL=/dev/null pin (the
// pin hid ADV12-1): HOME and XDG_CONFIG_HOME are temp dirs, and
// $HOME/.gitconfig holds what a sitting can write there, a smudge filter,
// an insteadOf from origin's URL to a mirror, and an excludesFile; the
// default $XDG_CONFIG_HOME/git/ignore hides one more name.
func globalWorld(t *testing.T) (w *world, remote string) {
	t.Helper()
	home, xdg := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	w = newWorld(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	os.Unsetenv("GIT_CONFIG_GLOBAL") // t.Setenv restores it
	remote = filepath.Join(filepath.Dir(w.root), "remote.git")
	mirror := filepath.Join(filepath.Dir(w.root), "mirror.git")
	gitIn(t, w.root, "clone", "-q", "--bare", remote, mirror)
	write(t, home, "ignore", "junk.txt\n")
	write(t, xdg, "git/ignore", "junk2.txt\n")
	write(t, home, ".gitconfig", fmt.Sprintf("[filter \"fix\"]\n\tsmudge = cat >/dev/null && echo a\n"+
		"[url %q]\n\tinsteadOf = %s\n[core]\n\texcludesFile = %s\n", mirror, remote, filepath.Join(home, "ignore")))
	return w, remote
}

// TestAR21_0_GlobalConfigIgnored is ADV12-1 (review test 1) and ADV12-10's
// warning red: the gate's and the merge's git never read the global config.
// A global smudge filter cannot repair a broken a.txt at the fresh checkout
// (never green, never merged); a push by URL is seen though a global
// insteadOf points origin's URL at a mirror (origin_same false); an
// untracked file a global excludesFile, or the default XDG ignore, hides is
// still seen by clean; a global config git cannot parse leaves a clean
// branch green; and the merge commits as the identity read at start (the
// verify's merge too), never the global one, and is unrun, no-identity,
// with none.
func TestAR21_0_GlobalConfigIgnored(t *testing.T) {
	t.Run("smudge", func(t *testing.T) {
		w, _ := globalWorld(t)
		st := w.stage(t, "a.txt") // a.txt is "a1 a.txt": main's check wants "a"
		write(t, w.tree, ".gitattributes", "a.txt filter=fix\n")
		gitIn(t, w.tree, "add", ".gitattributes")
		gitIn(t, w.tree, "commit", "-qm", "the filter's attribute")
		p := pk([]string{"a.txt", ".gitattributes"}, packet.Line{Shell: "cat a.txt", Expect: "reads `a`", Kind: packet.Reads, Value: "a"})
		res, why, g, _ := Run(w.fx, st, p, nil, seen, Verify{})
		if res == relay.GreenResult {
			t.Fatalf("Run = green %q: the global smudge repaired a.txt; lines %+v", why, g.Lines)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || w.merged() {
			t.Fatalf("merged a broken a.txt: %v", err)
		}
	})
	t.Run("insteadOf", func(t *testing.T) {
		w, remote := globalWorld(t)
		st := w.stage(t, "b.txt")
		// The push by URL a child can make, past the insteadOf and the hook.
		c := exec.Command("git", "-C", w.tree, "push", "-q", "--no-verify", remote, "HEAD:refs/heads/evil")
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("push by URL: %v\n%s", err, out)
		}
		res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if g.OriginSame || why != relay.OriginMoved {
			t.Fatalf("Run = %s %q, origin_same %v: the insteadOf read the mirror", res, why, g.OriginSame)
		}
	})
	for _, junk := range []string{"junk.txt", "junk2.txt"} {
		t.Run("excludes-"+junk, func(t *testing.T) {
			w, _ := globalWorld(t)
			st := w.stage(t, "b.txt")
			write(t, w.tree, junk, "untracked\n")
			res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
			if g.Clean || why != relay.DirtyTree {
				t.Fatalf("Run = %s %q, clean %v: a global ignore hid %s", res, why, g.Clean, junk)
			}
		})
	}
	t.Run("no-identity", func(t *testing.T) {
		w, _ := globalWorld(t)
		st := w.stage(t, "b.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s", res)
		}
		w.fx.Identity = func() (string, string) { return "", "" }
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || w.merged() {
			t.Fatalf("merged with no identity: %v", err)
		}
		last := w.ledger.Written()[len(w.ledger.Written())-1]
		if last["event"] != "merge" || last["result"] != string(relay.Unrun) || last["reason"] != string(relay.NoIdentity) {
			t.Fatalf("the merge line is %v, want unrun no-identity", last)
		}
	})
	t.Run("identity", func(t *testing.T) {
		w, _ := globalWorld(t)
		for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}
		f, _ := os.OpenFile(filepath.Join(os.Getenv("HOME"), ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0)
		fmt.Fprint(f, "[user]\n\tname = global\n\temail = global@x\n")
		f.Close()
		write(t, w.root, "Makefile", "check:\n\tgrep -qx a a.txt\n")
		gitIn(t, w.root, "add", ".")
		gitIn(t, w.root, "commit", "-qm", "main's check")
		st := w.stage(t, "b.txt")
		res, _, g, _ := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s", res)
		}
		w.fx.Identity = func() (string, string) { return "owner", "owner@x" }
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err != nil || !w.merged() {
			t.Fatalf("MergeIfGreen = %v, merged %v", err, w.merged())
		}
		if who := gitIn(t, w.root, "log", "-1", "--format=%an <%ae>, %cn <%ce>"); who != "owner <owner@x>, owner <owner@x>" {
			t.Fatalf("the merge is by %s, want the identity read at start", who)
		}
	})
	t.Run("broken-global", func(t *testing.T) {
		w, _ := globalWorld(t)
		st := w.stage(t, "b.txt")
		write(t, os.Getenv("XDG_CONFIG_HOME"), "git/config", "[core\n")
		res, why, g, err := Run(w.fx, st, pk([]string{"b.txt"}), nil, seen, Verify{})
		if res != relay.GreenResult {
			t.Fatalf("Run = %s %q (%v): a global config the child wrote turned the gate; gate %+v", res, why, err, g)
		}
	})
}
