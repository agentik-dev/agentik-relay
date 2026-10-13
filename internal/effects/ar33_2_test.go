package effects

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// shortTmux is an OSTmux on a socket of its own in a short temp dir (a
// socket path's length is capped), the server killed at the end; skipped
// without tmux. Never the account's socket.
func shortTmux(t *testing.T) OSTmux {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	short, err := os.MkdirTemp("/tmp", "ar33")
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(short, "t.sock")
	t.Cleanup(func() {
		exec.Command("tmux", "-S", sock, "kill-server").Run()
		os.RemoveAll(short)
	})
	return OSTmux{Socket: sock}
}

// TestAR33_2_KillIsExact (AR33 act 2): Kill, Alive and Sessions take the
// exact session: killing relay-t-1 twice leaves relay-t-10 alive, and
// Alive("relay-t-1") is false once it is gone (a bare has-session target
// would match relay-t-10 by prefix).
func TestAR33_2_KillIsExact(t *testing.T) {
	tm := shortTmux(t)
	dir := t.TempDir()
	for _, s := range []string{"relay-t-1", "relay-t-10"} {
		if err := tm.NewSession(s, dir, []string{"sleep", "30"}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := tm.Sessions(); err != nil || !slices.Equal(got, []string{"relay-t-1", "relay-t-10"}) {
		t.Fatalf("Sessions = %q, %v", got, err)
	}
	if err := tm.Kill("relay-t-1"); err != nil {
		t.Fatal(err)
	}
	err := tm.Kill("relay-t-1")
	t.Logf("second kill: %v", err)
	if err == nil {
		t.Fatal("a second kill of relay-t-1 found a session")
	}
	if ok, err := tm.Alive("relay-t-1"); ok || err != nil {
		t.Fatalf("Alive(relay-t-1) = %v, %v", ok, err)
	}
	if ok, err := tm.Alive("relay-t-10"); !ok || err != nil {
		t.Fatalf("Alive(relay-t-10) = %v, %v", ok, err)
	}
	if got, err := tm.Sessions(); err != nil || !slices.Equal(got, []string{"relay-t-10"}) {
		t.Fatalf("Sessions after = %q, %v", got, err)
	}
}

// TestAR33_2_SessionsNoServer (AR33 act 2): a socket with no server, no
// file there or a stale one, lists empty with no error.
func TestAR33_2_SessionsNoServer(t *testing.T) {
	tm := shortTmux(t)
	if got, err := tm.Sessions(); err != nil || len(got) != 0 {
		t.Fatalf("no socket file: Sessions = %q, %v", got, err)
	}
	l, err := net.Listen("unix", tm.Socket)
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close() // a stale socket: the file with no server behind it
	if _, err := os.Stat(tm.Socket); err != nil {
		t.Fatal(err)
	}
	if got, err := tm.Sessions(); err != nil || len(got) != 0 {
		t.Fatalf("a stale socket: Sessions = %q, %v", got, err)
	}
}
