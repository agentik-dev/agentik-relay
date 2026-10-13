package night

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR28_4_RootReadsOwnerIgnore (AR28B act 1, run 15's decision 4): a
// file the owner's global ignore names, untracked in the root, is not
// dirt to the baseline; one it does not name still is. The ignore is a
// temp XDG_CONFIG_HOME's, pinned by PinAt under temp dirs; the package's
// pin is restored.
func TestAR28_4_RootReadsOwnerIgnore(t *testing.T) {
	tempHomes(t)
	root, _ := landedRepo(t)
	ignore := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "git", "ignore")
	os.MkdirAll(filepath.Dir(ignore), 0o755)
	if err := os.WriteFile(ignore, []byte(".claude/settings.local.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	saved, _ := effects.Current()
	tools := effects.PinTools
	effects.PinTools = nil
	p, err := effects.PinAt(filepath.Join(t.TempDir(), "state"), t.TempDir())
	effects.PinTools = tools
	t.Cleanup(func() { p.Release(); effects.Use(saved) })
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	os.WriteFile(filepath.Join(root, ".claude", "settings.local.json"), []byte("{}\n"), 0o644)
	g := effects.OSGit{}
	if why := baselineBlocked(g, root); why != "" {
		t.Fatalf("baselineBlocked with an ignored file = %q, want \"\"", why)
	}
	os.WriteFile(filepath.Join(root, "stray"), []byte("s\n"), 0o644)
	if why := baselineBlocked(g, root); !strings.Contains(why, "dirty (1 paths)") {
		t.Fatalf("baselineBlocked with a stray file = %q, want dirty", why)
	}
}
