package gate

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR24_2_WildcardAndGowork is ADV14-3 and -4 (review test 3): while
// the account's lock is held (a free port of 49152 or above), a `:P`,
// `0.0.0.0:P` or `[::]:P` listener fails to bind; and a go.work above
// the fresh clone (a sitting's ~/go.work naming a dir that is not there)
// changes no gate line's exit: `go test ./...` there is green, GOWORK=off.
func TestAR24_2_WildcardAndGowork(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go")
	}
	port := 0
	for port < 49152 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port = l.Addr().(*net.TCPAddr).Port
		l.Close()
	}
	old := night.LockPort
	t.Cleanup(func() { night.LockPort = old })
	night.LockPort = func() (int, error) { return port, nil }
	w := hermeticWorld(t)
	release, err := night.PidFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, addr := range []string{fmt.Sprintf(":%d", port), fmt.Sprintf("0.0.0.0:%d", port), fmt.Sprintf("[::]:%d", port)} {
		if l, err := net.Listen("tcp", addr); err == nil {
			l.Close()
			t.Fatalf("a listener bound %s while the lock held 127.0.0.1:%d", addr, port)
		}
	}
	write(t, os.Getenv("XDG_STATE_HOME"), "go.work", "go 1.26\n\nuse ./nowhere\n")
	st := w.stage(t, "b.txt")
	write(t, w.tree, "go.mod", "module x\n\ngo 1.26\n")
	write(t, w.tree, "x_test.go", "package x\n\nimport \"testing\"\n\nfunc TestPasses(t *testing.T) {}\n")
	gitIn(t, w.tree, "add", ".")
	gitIn(t, w.tree, "commit", "-qm", "a passing test")
	res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt", "go.mod", "x_test.go"}), nil, seen, Verify{Name: "go test ./..."})
	if res != relay.GreenResult || g.Verify == nil || g.Verify.Status != relay.CheckGreen {
		t.Fatalf("Run with a go.work above the clone = %s %q; verify %+v, want green", res, why, g.Verify)
	}
}
