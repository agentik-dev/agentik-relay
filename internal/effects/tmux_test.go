package effects

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestRealTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	short, err := os.MkdirTemp("/tmp", "rt") // a socket path's length is capped
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(short) })
	sock, err := TmuxSocket(short)
	if err != nil {
		t.Fatal(err)
	}
	tm := OSTmux{Socket: sock} // the private socket only, never the default
	name := fmt.Sprintf("relay-test-%d", os.Getpid())
	dir := t.TempDir()
	if !tm.Present() {
		t.Fatal("Present false with tmux on PATH")
	}
	if err := tm.NewSession(name, dir, []string{"sh", "-c", "sleep 30"}); err != nil {
		t.Fatal(err)
	}
	defer exec.Command("tmux", "-S", sock, "kill-server").Run()
	if err := tm.NewWindow(name, "row", dir, []string{"sh", "-c", "sleep 30"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := tm.Alive(name); !ok || err != nil {
		t.Fatalf("Alive = %v, %v", ok, err)
	}
	if err := exec.Command("tmux", "-S", sock, "kill-session", "-t", name).Run(); err != nil {
		t.Fatal(err)
	}
	if ok, err := tm.Alive(name); ok || err != nil {
		t.Fatalf("Alive after kill = %v, %v", ok, err)
	}
}

func TestTmuxAbsent(t *testing.T) {
	tm := OSTmux{LookPath: func(string) (string, error) { return "", exec.ErrNotFound }, Socket: "/s/relay.sock"}
	if tm.Present() {
		t.Fatal("Present with no tmux")
	}
	if err := tm.NewSession("s", "/", nil); err == nil {
		t.Fatal("NewSession without tmux did not fail")
	}
	if got := tm.AttachArgv("relay-n", "a"); !reflect.DeepEqual(got, []string{"tmux", "-S", "/s/relay.sock", "attach", "-t", "=relay-n:a"}) {
		t.Fatalf("AttachArgv = %q", got)
	}
}

func TestFakeTmuxAlive(t *testing.T) {
	ft := &FakeTmux{IsPresent: true, AliveAfter: 1}
	a, _ := ft.Alive("s")
	b, _ := ft.Alive("s")
	if !a || b {
		t.Fatalf("Alive = %v then %v, want true then false", a, b)
	}
}
