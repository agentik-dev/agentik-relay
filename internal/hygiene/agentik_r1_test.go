package hygiene

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentikR1_2_VendoredPinned (agentik-r1 §8.2, §7.3): the three files
// vendored from agentik-commons are the bytes scripts/vendored.sha256 pins,
// as `make hygiene` checks them: `shasum -a 256 -c` reads three OK lines.
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
	for _, name := range []string{"agentik", "hygiene", "bounded"} {
		if strings.Contains(string(out), "scripts/"+name+": OK\n") {
			ok++
		}
	}
	if ok != 3 || strings.Count(string(out), "\n") != 3 {
		t.Errorf("shasum -c read %d of 3 OK lines:\n%s", ok, out)
	}
}
