package hygiene

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentikR1_2_VendoredPinned (agentik-r1 §8.2, §7.3): the four files
// vendored from agentik-commons are the bytes scripts/vendored.sha256 pins,
// as `make hygiene` checks them: `shasum -a 256 -c` reads four OK lines.
func TestAgentikR1_2_VendoredPinned(t *testing.T) {
	sum, err := exec.LookPath("shasum")
	if err != nil {
		t.Skip("shasum not on PATH")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sum, "-a", "256", "-c", "scripts/vendored.sha256")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shasum -c scripts/vendored.sha256: %v\n%s", err, out)
	}
	ok := 0
	for _, name := range []string{"agentik", "hygiene", "bounded", "plugin-version-check"} {
		if strings.Contains(string(out), "scripts/"+name+": OK\n") {
			ok++
		}
	}
	if ok != 4 || strings.Count(string(out), "\n") != 4 {
		t.Errorf("shasum -c read %d of 4 OK lines:\n%s", ok, out)
	}
}

// TestAgentikR1_6_RenamedZero (agentik-r1 §8.2; ruling 12): the vendored
// scan over this repo finds no `renamed` hit outside the allowlist: its
// --json reads groups.renamed.outside_files 0 and no renamed row in files.
// The private list is kept absent (HOME and XDG_CONFIG_HOME a temp dir):
// the group is generic.
func TestAgentikR1_6_RenamedZero(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(py, "-B", filepath.Join(root, "scripts", "hygiene"), "--json", "--repo", root)
	tmp := t.TempDir()
	for _, kv := range os.Environ() {
		if k := kv[:strings.IndexByte(kv, '=')+1]; k != "AGENTIK_HYGIENE_PRIVATE=" && k != "XDG_CONFIG_HOME=" && k != "HOME=" {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+tmp, "XDG_CONFIG_HOME="+tmp, "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		t.Fatalf("hygiene --json: %v", err)
	}
	var d struct {
		Groups map[string]struct {
			OutsideFiles int `json:"outside_files"`
		}
		Files []struct{ Group, Path string }
	}
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("hygiene --json: %v\n%s", err, out)
	}
	g, ok := d.Groups["renamed"]
	if !ok || g.OutsideFiles != 0 {
		t.Errorf("renamed outside_files = %d (group present %v)", g.OutsideFiles, ok)
	}
	for _, f := range d.Files {
		if f.Group == "renamed" {
			t.Errorf("renamed outside the allowlist: %s", f.Path)
		}
	}
}
