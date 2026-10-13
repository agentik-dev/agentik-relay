package effects

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// pinUnderIgnore is PinAt under temp dirs whose XDG_CONFIG_HOME/git/ignore
// holds ignore ("" writes none), the package's pin restored at cleanup; the
// owner's real config and ignore are never read (HOME and XDG_CONFIG_HOME
// are temp dirs, GIT_CONFIG_GLOBAL is /dev/null by repoWithRemote).
func pinUnderIgnore(t *testing.T, ignore string) (Pinned, string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", xdg)
	file := filepath.Join(xdg, "git", "ignore")
	if ignore != "" {
		os.MkdirAll(filepath.Dir(file), 0o755)
		if err := os.WriteFile(file, []byte(ignore), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saved, _ := Current()
	tools := PinTools
	PinTools = nil
	p, err := PinAt(filepath.Join(t.TempDir(), "state"), t.TempDir())
	PinTools = tools
	t.Cleanup(func() { p.Release(); Use(saved) })
	if err != nil {
		t.Fatal(err)
	}
	return p, file
}

// TestAR28_4_WorktreeStaysStrict (AR28B act 1, run 15's decision 4): the
// pin copies the owner's ignore to <Home>/excludes; RootStatus reads it,
// Status (a worktree's) never does, and an ignore edited after the pin
// hides nothing.
func TestAR28_4_WorktreeStaysStrict(t *testing.T) {
	root, _ := repoWithRemote(t)
	p, file := pinUnderIgnore(t, ".claude/settings.local.json\n")
	if want := filepath.Join(p.Home, "excludes"); p.Excludes != want {
		t.Fatalf("Excludes = %q, want %q", p.Excludes, want)
	}
	if fi, err := os.Stat(p.Excludes); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the pinned ignore: %v, %v; want 0600", fi, err)
	}
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	os.WriteFile(filepath.Join(root, ".claude", "settings.local.json"), []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "stray"), []byte("s\n"), 0o644)
	var g OSGit
	both := []string{"?? .claude/settings.local.json", "?? stray"}
	if s, err := g.Status(root); err != nil || !reflect.DeepEqual(s, both) {
		t.Fatalf("Status = %q, %v; want %q", s, err, both)
	}
	if s, err := g.RootStatus(root); err != nil || !reflect.DeepEqual(s, []string{"?? stray"}) {
		t.Fatalf("RootStatus = %q, %v; want the stray alone", s, err)
	}
	if err := os.WriteFile(file, []byte(".claude/settings.local.json\nstray\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err := g.RootStatus(root); err != nil || !reflect.DeepEqual(s, []string{"?? stray"}) {
		t.Fatalf("RootStatus after the ignore's edit = %q, %v; want the stray still", s, err)
	}
	// No ignore at the pin: Excludes is "" and RootStatus is Status.
	p, _ = pinUnderIgnore(t, "")
	if p.Excludes != "" {
		t.Fatalf("Excludes with no ignore = %q", p.Excludes)
	}
	if s, err := g.RootStatus(root); err != nil || !reflect.DeepEqual(s, both) {
		t.Fatalf("RootStatus with no ignore = %q, %v; want %q", s, err, both)
	}
}
