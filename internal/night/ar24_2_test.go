package night_test

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// freePort is a TCP port of 49152 or above, free on 127.0.0.1 when asked.
func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if port >= 49152 {
			return port
		}
	}
	t.Fatal("no free port of 49152 or above")
	return 0
}

// wildcards are the binds ADV14-4 planted beside a held 127.0.0.1:P.
func wildcards(port int) []string {
	return []string{fmt.Sprintf(":%d", port), fmt.Sprintf("0.0.0.0:%d", port), fmt.Sprintf("[::]:%d", port)}
}

// TestAR24_2_WildcardHeld is ADV14-4: while the lock is held, a wildcard
// listener (Go's `:P`, `0.0.0.0:P`, `[::]:P`, each SO_REUSEADDR as
// python's http.server) fails to bind; after release, 127.0.0.1:P and
// all three are free.
func TestAR24_2_WildcardHeld(t *testing.T) {
	port := freePort(t)
	old := night.LockPort
	t.Cleanup(func() { night.LockPort = old })
	night.LockPort = func() (int, error) { return port, nil }
	release, err := night.PidFile(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range wildcards(port) {
		if l, err := net.Listen("tcp", addr); err == nil {
			l.Close()
			release()
			t.Fatalf("a listener bound %s while the lock held 127.0.0.1:%d", addr, port)
		}
	}
	release()
	for _, addr := range append([]string{fmt.Sprintf("127.0.0.1:%d", port)}, wildcards(port)...) {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("%s after release: %v", addr, err)
		}
		l.Close()
	}
}
