package effects

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestAR21_1_ProgramKeysRefused: each program-valued key ruling 1 names,
// in the root's local config, is one ProgramKeys returns (section and
// variable case-folded as git reads them, the subsection kept); a key in
// a linked worktree's config.worktree is one too, an include.path is seen
// and never followed, and a plain key is none (AR21 act 1, ADV12-2).
func TestAR21_1_ProgramKeysRefused(t *testing.T) {
	for _, c := range []struct{ pattern, key, want string }{
		{"filter.*", "filter.fix.smudge", "filter.fix.smudge"},
		{"filter.*", "filter.fix.process", "filter.fix.process"},
		{"merge.*.driver", "merge.m.driver", "merge.m.driver"},
		{"diff.*.textconv", "diff.d.textconv", "diff.d.textconv"},
		{"diff.*.command", "diff.d.command", "diff.d.command"},
		{"diff.external", "diff.external", "diff.external"},
		{"core.sshCommand", "core.sshCommand", "core.sshcommand"},
		{"core.gitProxy", "core.gitProxy", "core.gitproxy"},
		{"core.askPass", "core.askPass", "core.askpass"},
		{"credential.*", "credential.helper", "credential.helper"},
		{"credential.*", "credential.https://x.helper", "credential.https://x.helper"},
		{"gpg.*.program", "gpg.x509.program", "gpg.x509.program"},
		{"include.*", "include.path", "include.path"},
		{"includeIf.*", "includeIf.gitdir:/nowhere/.path", "includeif.gitdir:/nowhere/.path"},
		{"url.*.insteadOf", "url.https://evil/.insteadOf", "url.https://evil/.insteadof"},
		{"url.*.pushInsteadOf", "url.https://evil/.pushInsteadOf", "url.https://evil/.pushinsteadof"},
		{"remote.*.uploadpack", "remote.origin.uploadpack", "remote.origin.uploadpack"},
		{"remote.*.receivepack", "remote.origin.receivepack", "remote.origin.receivepack"},
		{"MiXeD case", "Filter.Fix.SMUDGE", "filter.Fix.smudge"},
	} {
		t.Run(c.key, func(t *testing.T) {
			root, _ := repoWithRemote(t)
			gitIn(t, root, "config", c.key, "sh -c true")
			got, err := ProgramKeys(root, "")
			if err != nil || !reflect.DeepEqual(got, []string{c.want}) {
				t.Fatalf("%s: ProgramKeys = %q, %v; want [%s]", c.pattern, got, err, c.want)
			}
		})
	}
	t.Run("none", func(t *testing.T) {
		root, _ := repoWithRemote(t)
		gitIn(t, root, "config", "user.name", "t")
		gitIn(t, root, "config", "remote.origin.pushurl", "/nowhere")
		gitIn(t, root, "config", "diff.renames", "true")
		if got, err := ProgramKeys(root, ""); err != nil || len(got) != 0 {
			t.Fatalf("ProgramKeys = %q, %v; want none", got, err)
		}
	})
	t.Run("raw-file", func(t *testing.T) {
		root, _ := repoWithRemote(t)
		// written by hand, as a child would: git's case folding, and an
		// include whose target would name a filter is seen, never followed
		inc := filepath.Join(t.TempDir(), "inc")
		os.WriteFile(inc, []byte("[filter \"hidden\"]\n\tclean = evil\n"), 0o644)
		f, err := os.OpenFile(filepath.Join(root, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("[CORE]\n\tSshCommand = evil\n[Include]\n\tPath = " + inc + "\n")
		f.Close()
		got, err := ProgramKeys(root, "")
		if want := []string{"core.sshcommand", "include.path"}; err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("ProgramKeys = %q, %v; want %q", got, err, want)
		}
	})
	t.Run("worktree-config", func(t *testing.T) {
		root, _ := repoWithRemote(t)
		var g OSGit
		tree := filepath.Join(root, ".claude", "worktrees", "a1")
		if err := g.WorktreeAdd(root, tree, "a1", "main"); err != nil {
			t.Fatal(err)
		}
		if err := g.InstallPrePush(tree); err != nil { // extensions.worktreeConfig on
			t.Fatal(err)
		}
		if got, err := ProgramKeys(root, tree); err != nil || len(got) != 0 {
			t.Fatalf("ProgramKeys after the stage's own writes = %q, %v; want none", got, err)
		}
		gitIn(t, tree, "config", "--worktree", "diff.d.textconv", "evil")
		for _, wt := range []string{tree, ""} { // the row's tree, and any linked worktree
			if got, err := ProgramKeys(root, wt); err != nil || !reflect.DeepEqual(got, []string{"diff.d.textconv"}) {
				t.Fatalf("ProgramKeys(%q) = %q, %v; want [diff.d.textconv]", wt, got, err)
			}
		}
	})
	t.Run("not-a-repo", func(t *testing.T) {
		if got, err := ProgramKeys(t.TempDir(), ""); err != nil || len(got) != 0 {
			t.Fatalf("ProgramKeys = %q, %v; want none", got, err)
		}
	})
}

// TestAR21_1_AlternatesDigested: objects/info/alternates is in the git
// state digest (ADV12-10): a store borrowed after stage is
// git-state-moved.
func TestAR21_1_AlternatesDigested(t *testing.T) {
	root, _ := repoWithRemote(t)
	was, err := GitStateDigest(root, "")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".git", "objects", "info", "alternates")
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(t.TempDir()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if now, err := GitStateDigest(root, ""); err != nil || now == was {
		t.Fatalf("digest with an alternates file = %s (%v), was %s", now, err, was)
	}
}
