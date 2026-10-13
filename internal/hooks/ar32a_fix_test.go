package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR32AFix_RootWriteJudgesTheCommandsCd is ar32a review round 1 (F3):
// root-write reads a cd target from the hook's env and the disk, so a
// command that changes what its cd means is one the guard cannot read,
// and git after it denies: HOME or CDPATH (zsh's cdpath too) assigned,
// exported or declared before the cd, an `ln` or `mv` before it, a
// target not there when the hook runs, and a relative operand under the
// hook's own CDPATH. A cd by a literal path that is there still passes.
func TestAR32AFix_RootWriteJudgesTheCommandsCd(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	tree := filepath.Join(root, ".claude", "worktrees", "orient.2")
	for _, d := range []string{tree, filepath.Join(root, "sub"), filepath.Join(tree, "sub"), filepath.Join(other, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(other, filepath.Join(other, "was")); err != nil {
		t.Fatal(err)
	}
	kv := map[string]string{"RELAY_ROW": "orient", "CLAUDE_SESSION_DEPTH": "1", "CLAUDE_SESSION_MAX_TURNS": "100",
		"CLAUDE_PROJECT_DIR": tree, "HOME": other}
	row := ar27bEnv(kv)
	for _, tc := range []struct {
		cmd  string
		deny bool
	}{
		{"HOME=" + root + " cd && git merge orient.1", true},
		{"HOME=" + root + "; cd && git merge orient.1", true},
		{"export HOME=" + root + "; cd ~ && git status", true},
		{"export CDPATH=" + root + "; cd sub && git status", true},
		{"CDPATH=" + root + " cd sub && git status", true},
		{"declare -x CDPATH=" + root + "; cd sub && git status", true},
		{"cdpath=(" + root + "); cd sub && git status", true},
		{"ln -s " + root + " " + other + "/r && cd " + other + "/r && git merge orient.1", true},
		{"ln -sfn " + root + " " + other + "/was && cd " + other + "/was && git merge orient.1", true},
		{"cd " + other + "/absent && git status", true},
		{"cd " + other + " && git status", false},
		{"cd " + other + "/was && git status", false},
		{"cd && git status", false},
		{"cd sub && git status", false},
		{"cd " + tree + " && git status", false},
		{"git status; HOME=" + root + " cd", false},
	} {
		deny, why := ar32aHook(t, "Bash", map[string]any{"command": tc.cmd}, tree, row)
		if deny != tc.deny || deny && !strings.HasPrefix(why, "relay: root-write: ") {
			t.Errorf("%q = %v, %q; want denied %v (root-write)", tc.cmd, deny, why, tc.deny)
		}
	}
	kv["CDPATH"] = root // the hook's own CDPATH: a bare relative operand may land in the root
	cd := ar27bEnv(kv)
	for _, tc := range []struct {
		cmd  string
		deny bool
	}{
		{"cd sub && git status", true},
		{"cd ./sub && git status", false},
		{"cd " + tree + " && git status", false},
	} {
		deny, why := ar32aHook(t, "Bash", map[string]any{"command": tc.cmd}, tree, cd)
		if deny != tc.deny {
			t.Errorf("CDPATH set, %q = %v, %q; want denied %v", tc.cmd, deny, why, tc.deny)
		}
	}
}
