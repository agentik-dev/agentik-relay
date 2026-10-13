package effects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR25_2_OwnSocket is ADV15-2, -5: the relay's tmux runs on its own
// socket. A fake tmux (OSTmux.LookPath) logs its argv: every call passes
// `-S <state>/tmux/relay.sock`, and the attach line names it. With no
// socket tmux is absent. TmuxSocket's dir is 0700; a symlink or a file
// there is refused.
func TestAR25_2_OwnSocket(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	sock, err := TmuxSocket(state)
	if err != nil || sock != filepath.Join(state, "tmux", "relay.sock") {
		t.Fatalf("TmuxSocket = %q, %v", sock, err)
	}
	if fi, err := os.Lstat(filepath.Dir(sock)); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("the socket dir: %v %v", fi, err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	bin := filepath.Join(dir, "tmux")
	os.WriteFile(bin, []byte("#!/bin/sh\necho \"$*\" >> "+log+"\nexit 0\n"), 0o755)
	tm := OSTmux{LookPath: func(string) (string, error) { return bin, nil }, Socket: sock}
	if !tm.Present() {
		t.Fatal("Present false with a socket and a tmux")
	}
	if err := tm.NewSession("relay-n", dir, []string{"tail", "-f", "x"}); err != nil {
		t.Fatal(err)
	}
	if err := tm.NewWindow("relay-n", "r1", dir, []string{"tail", "-f", "y"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := tm.Alive("relay-n"); !ok || err != nil {
		t.Fatalf("Alive = %v, %v", ok, err)
	}
	b, _ := os.ReadFile(log)
	calls := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(calls) != 3 {
		t.Fatalf("tmux calls %q, want 3", calls)
	}
	for _, c := range calls {
		if !strings.HasPrefix(c, "-S "+sock+" ") {
			t.Fatalf("a tmux call without its own socket: %q", c)
		}
	}
	if got := strings.Join(tm.AttachArgv("relay-n", "r1"), " "); got != bin+" -S "+sock+" attach -t =relay-n:r1" {
		t.Fatalf("AttachArgv = %q", got)
	}
	none := OSTmux{LookPath: tm.LookPath}
	if none.Present() || none.NewSession("s", dir, nil) == nil {
		t.Fatal("a tmux with no socket is present")
	}
	if ok, err := none.Alive("s"); ok || err == nil {
		t.Fatalf("Alive with no socket = %v, %v", ok, err)
	}
	link := filepath.Join(t.TempDir(), "s1")
	os.MkdirAll(link, 0o700)
	os.Symlink(t.TempDir(), filepath.Join(link, "tmux"))
	if _, err := TmuxSocket(link); err == nil {
		t.Fatal("TmuxSocket followed a symlinked dir")
	}
	file := filepath.Join(t.TempDir(), "s2")
	os.MkdirAll(file, 0o700)
	os.WriteFile(filepath.Join(file, "tmux"), nil, 0o600)
	if _, err := TmuxSocket(file); err == nil {
		t.Fatal("TmuxSocket took a file for its dir")
	}
	loose := filepath.Join(t.TempDir(), "s3")
	os.MkdirAll(filepath.Join(loose, "tmux"), 0o755)
	os.Chmod(filepath.Join(loose, "tmux"), 0o755)
	if _, err := TmuxSocket(loose); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(filepath.Join(loose, "tmux")); fi.Mode().Perm() != 0o700 {
		t.Fatalf("a loose socket dir kept %v", fi.Mode().Perm())
	}
}
