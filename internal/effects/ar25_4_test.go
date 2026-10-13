package effects

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestAR25_4_CommFromPS is the breaker's reading (AR25 act 4, ADV15-1):
// OSProcs.Comm is the pinned `ps -o comm= -p pid`, so a process whose
// executable is named claude reads as one (IsClaude) whatever its env; a
// pid that is no process is an error. IsClaude takes the base name
// `claude` or the pinned claude's real file, never a claude's renamed
// worker or a longer name.
func TestAR25_4_CommFromPS(t *testing.T) {
	// A link named claude, as the account's bin/claude links its version
	// file (a copied system binary is killed at exec on darwin).
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.Symlink("/bin/sleep", bin); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(bin, "30")
	c.Env = []string{"RELAY_ROW=", "CLAUDE_SESSION_HELPER=0"}
	if err := c.Start(); err != nil {
		t.Skipf("a copied sleep does not run here: %v", err)
	}
	defer func() { c.Process.Kill(); c.Wait() }()
	comm, err := OSProcs{}.Comm(c.Process.Pid)
	if err != nil || filepath.Base(comm) != "claude" || !IsClaude(comm, "") {
		t.Fatalf("Comm = %q, %v; want a claude", comm, err)
	}
	if comm, err := (OSProcs{}).Comm(os.Getpid()); err != nil || IsClaude(comm, "") {
		t.Fatalf("this test's Comm = %q, %v", comm, err)
	}
	if _, err := (OSProcs{}).Comm(1 << 30); err == nil {
		t.Fatal("a pid that is no process has a comm")
	}
	for comm, want := range map[string]bool{
		"claude": true, "/usr/local/bin/claude": true, " claude\n": true,
		"claude bg-pty-host": false, "claudex": false, "/bin/zsh": false, "": false,
	} {
		if IsClaude(comm, "") != want {
			t.Errorf("IsClaude(%q) = %v, want %v", comm, !want, want)
		}
	}
	if p, ok := Current(); ok && p.Tools["claude"] != "" && !IsClaude(p.Tools["claude"], "") {
		t.Errorf("the pinned claude's real file %q is not a claude", p.Tools["claude"])
	}
}
