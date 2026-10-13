package effects

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Tmux is the night's windows (§9, D16): a session per night, a window per
// row. A window is a viewer over a log, never a child's terminal.
type Tmux interface {
	Present() bool
	NewSession(name, dir string, argv []string) error
	NewWindow(session, window, dir string, argv []string) error
	Alive(session string) (bool, error)
	AttachArgv(session, window string) []string
	Sessions() ([]string, error)
	Kill(session string) error
}

// OSTmux runs the machine's tmux. LookPath finds it (nil: exec.LookPath).
// Every tmux command runs under DropSitting(os.Environ()): tmux is never a
// sitting, and a server new-session starts keeps the env it was started
// with for every later window, the owner's interactive ones too; a
// depth-1 relay's CLAUDE_SESSION_DEPTH or RELAY_ROW there would make the
// window's claude a sitting to the relay's Stop hook (AR24 act 3,
// ADV14-5). The row's sitting is not a tmux child: its window is a viewer
// over the log, and the spawn's `claude -p` gets ChildEnv from the spawner.
//
// Socket is the relay's own server (AR25 act 2, ADV15-2, -5): every
// command passes `-S Socket` (TmuxSocket), never the default socket a
// sitting's or the owner's tmux reaches. With no Socket, tmux is absent:
// Present is false and every command an error.
type OSTmux struct {
	LookPath func(string) (string, error)
	Socket   string
}

// errNoSocket is every OSTmux command's error with no Socket.
var errNoSocket = errors.New("tmux: no socket of the relay's own (effects.TmuxSocket)")

// TmuxSocket is the relay's tmux socket, <state>/tmux/relay.sock (AR25
// act 2). The dir is made 0700 and read by Lstat: a symlink or a non-dir
// there is refused, and a looser mode is set back to 0700.
func TmuxSocket(state string) (string, error) {
	dir := filepath.Join(state, "tmux")
	if err := os.MkdirAll(state, 0o700); err != nil {
		return "", err
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	fi, err := os.Lstat(dir)
	switch {
	case err != nil:
		return "", err
	case fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir():
		return "", fmt.Errorf("tmux: %s is not a dir of the relay's own (%s)", dir, fi.Mode())
	case fi.Mode().Perm() != 0o700:
		if err := os.Chmod(dir, 0o700); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "relay.sock"), nil
}

// command is tmux at p with `-S Socket` and args, under DropSitting's env.
func (t OSTmux) command(p string, args ...string) *exec.Cmd {
	cmd := exec.Command(p, append([]string{"-S", t.Socket}, args...)...)
	cmd.Env = DropSitting(os.Environ())
	return cmd
}

func (t OSTmux) path() (string, error) {
	if t.Socket == "" {
		return "", errNoSocket
	}
	look := t.LookPath
	if look == nil {
		look = exec.LookPath
	}
	return look("tmux")
}

func (t OSTmux) Present() bool {
	_, err := t.path()
	return err == nil
}

func (t OSTmux) run(args ...string) error {
	p, err := t.path()
	if err != nil {
		return err
	}
	out, err := t.command(p, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %v: %s", args[0], err, LastLine(string(out)))
	}
	return nil
}

func (t OSTmux) NewSession(name, dir string, argv []string) error {
	return t.run(append([]string{"new-session", "-d", "-s", name, "-c", dir}, argv...)...)
}

func (t OSTmux) NewWindow(session, window, dir string, argv []string) error {
	return t.run(append([]string{"new-window", "-d", "-t", session + ":", "-n", window, "-c", dir}, argv...)...)
}

// Exact is a session target tmux matches whole (AR33 act 2): `=<session>`.
// A bare name is also a prefix, so `relay-X-1` alone would reach
// `relay-X-10` once `relay-X-1` is gone.
func Exact(session string) string { return "=" + session }

// Alive is has-session on the exact session: a non-zero exit is a session
// gone, not an error.
func (t OSTmux) Alive(session string) (bool, error) {
	p, err := t.path()
	if err != nil {
		return false, err
	}
	err = t.command(p, "has-session", "-t", Exact(session)).Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee):
		return false, nil
	default:
		return false, err
	}
}

// Sessions is every session's name on the relay's socket (AR33 act 2),
// `list-sessions -F '#{session_name}'`; no server there, or none of its
// sessions, is an empty list, not an error.
func (t OSTmux) Sessions() ([]string, error) {
	p, err := t.path()
	if err != nil {
		return nil, err
	}
	out, err := t.command(p, "list-sessions", "-F", "#{session_name}").CombinedOutput()
	if err != nil {
		if noServer(string(out)) {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %v: %s", err, LastLine(string(out)))
	}
	var names []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

// noServer is tmux's word for a socket with no server behind it (no file,
// or a stale one) or a server with no sessions.
func noServer(out string) bool {
	for _, s := range []string{"no server running", "no sessions", "No such file or directory", "Connection refused"} {
		if strings.Contains(out, s) {
			return true
		}
	}
	return false
}

// Kill is `kill-session -t =<session>` (AR33 act 2): the exact session,
// never another by prefix.
func (t OSTmux) Kill(session string) error {
	return t.run("kill-session", "-t", Exact(session))
}

func (t OSTmux) AttachArgv(session, window string) []string {
	p, err := t.path()
	if err != nil {
		p = "tmux"
	}
	return AttachArgv(p, t.Socket, session, window)
}

// AttachArgv is `tmux [-S <socket>] attach -t =<session>[:<window>]` with
// tmux at p (no socket: no -S, a fake's); the session exact (AR33 act 2).
func AttachArgv(p, socket, session, window string) []string {
	target := Exact(session)
	if window != "" {
		target += ":" + window
	}
	argv := []string{p}
	if socket != "" {
		argv = append(argv, "-S", socket)
	}
	return append(argv, "attach", "-t", target)
}
