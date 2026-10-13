package packet

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ar18Packet is a packet doclint passes but for its acceptance line.
func ar18Packet(accept string) string {
	var b strings.Builder
	b.WriteString("# Packet X\n\n| item | value |\n|---|---|\n")
	for _, r := range []string{"from", "pin", "wall", "may touch"} {
		b.WriteString("| " + r + " | x |\n")
	}
	b.WriteString("| acceptance | `" + accept + "` reads `ok` |\n| budget | x |\n| verify | `make check` |\n\n")
	for i, s := range []string{"What the source says", "Rulings", "Interfaces",
		"Findings that name this row", "Forbidden reads", "Report"} {
		b.WriteString("## " + string(rune('1'+i)) + ". " + s + "\n\nx\n\n")
	}
	return b.String()
}

// TestAR18_3_CanonicalDoclintRefuses (ADV10-4, conventions §6): the branch's
// scripts/doclint, run in a temp repo, refuses an acceptance line running a
// whole suite in each of the four spellings it reads, exit 2 naming each.
func TestAR18_3_CanonicalDoclintRefuses(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	dir := copyIn(t, "../..", "scripts/doclint")
	spellings := []string{"make test", "make check-unlocked", "go test ./...", "go test -race ./..."}
	for i, s := range spellings {
		p := filepath.Join(dir, "design", "packets", "p"+string(rune('a'+i))+".md")
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(ar18Packet(s)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	cmd := exec.Command(py, "-B", "scripts/doclint")
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState.ExitCode() != 2 {
		t.Fatalf("doclint exit %d, want 2: %s", cmd.ProcessState.ExitCode(), out)
	}
	for i, s := range spellings {
		want := "p" + string(rune('a'+i)) + ".md: packet: acceptance runs a whole suite: " + s
		if !strings.Contains(string(out), want) {
			t.Errorf("doclint never named %q:\n%s", want, out)
		}
	}
}

// TestAR18_3_GoRefusesSuperset (ADV10-4): packet.Unbounded refuses every
// spelling of a whole suite, including the six the canonical doclint passes.
func TestAR18_3_GoRefusesSuperset(t *testing.T) {
	for _, l := range []string{"make test", "make check-unlocked", "make e2e", "make vet",
		"gmake check", "$(echo make) check", "m=make; $m check", "echo 'make check' | sh",
		"go test ./...", "go test -race ./..."} {
		if Unbounded(l) == "" {
			t.Errorf("Unbounded(%q) passes it", l)
		}
	}
}
