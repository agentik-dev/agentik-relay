package night

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/hooks"
)

// PidFileName is the account's pid file under PidDir (relay-r1 §8, §15;
// AR17B, AR22 act 2): its one line `<pid> <started>` only names the
// account lock's holder (LockPort); the lock is the port, never the file.
const PidFileName = "relay.pid"

// LockPortLink is set at link time only (-ldflags -X), for a test's built
// binary: its account lock's port in place of the uid's. Empty in a
// release; never read from the environment. It needs PidHomeLink beside
// it, or PidDir refuses (AR36 ruling 2).
var LockPortLink string

// The account lock's port (AR22 act 2, ADV13-4): LockPortBase + uid %
// LockPortSpan, 20000 to 29999, below every ephemeral range (Linux's
// 32768, darwin's 49152), so no outgoing connection takes it by chance.
const (
	LockPortBase = 20000
	LockPortSpan = 10000
)

// AccountPort is uid's lock port.
func AccountPort(uid int) int { return LockPortBase + uid%LockPortSpan }

// LockPort is this process's lock port: LockPortLink when linked (an
// invalid one is an error, never the uid's), else AccountPort(getuid). A
// test replaces it with a free port of its own: no test binds the
// account's.
var LockPort = func() (int, error) {
	if LockPortLink != "" {
		n, err := strconv.Atoi(LockPortLink)
		if err != nil || n <= 0 || n > 65535 {
			return 0, fmt.Errorf("the linked lock port %q", LockPortLink)
		}
		return n, nil
	}
	uid := os.Getuid()
	if uid < 0 {
		return 0, errors.New("no uid")
	}
	return AccountPort(uid), nil
}

// PidHomeLink is set at link time only (-ldflags -X), for a test's built
// binary; empty in a release. Never read from the environment (ADV11-9).
var PidHomeLink string

// UserHome is the account's home from the user database (getpwuid), never
// $HOME: a child that changes HOME or XDG_STATE_HOME still finds its
// parent's pid file (ADV11-9). A test may replace it.
var UserHome = func() (string, error) {
	if PidHomeLink != "" {
		return PidHomeLink, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

// PidDir is the pid file's fixed directory, <home>/.local/state/agent-relay
// with home from UserHome; an error when that home is not absolute. A
// binary linked with LockPortLink and without PidHomeLink refuses (AR36
// ruling 2): its port is its own, and the account's real pid dir, read
// from the user database whatever HOME says, may name a live relay.
func PidDir() (string, error) {
	if LockPortLink != "" && PidHomeLink == "" {
		return "", errors.New("a linked lock port (LockPortLink) needs a linked pid home (PidHomeLink): refusing the account's real pid dir")
	}
	home, err := UserHome()
	if err != nil {
		return "", fmt.Errorf("the account's home: %w", err)
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("the account's home %q is not absolute", home)
	}
	return filepath.Join(home, ".local", "state", "agent-relay"), nil
}

// The Write door denies the pid dir wherever XDG_STATE_HOME points
// (ADV12-8); hooks imports nothing of the relay, so it is handed PidDir.
func init() { hooks.PidDir = PidDir }

// NestedError is PidFile's refusal: the account's lock port is held. Pid
// is the holder relay.pid names when that pid is live and started when its
// line says, else 0: a refusal names a live pid only (ADV13-14), and a port
// held with no live relay named (a moved pid file, another program on the
// port) refuses all the same.
type NestedError struct{ Pid, Port int }

func (e NestedError) Error() string {
	if e.Pid <= 0 {
		return fmt.Sprintf("nested: the account's lock (127.0.0.1:%d) is held and relay.pid names no live relay", e.Port)
	}
	return fmt.Sprintf("nested: relay %d is live", e.Pid)
}

// PidStarted is a pid's start time, an error when it is not live; a test
// may replace it.
var PidStarted = func(pid int) (string, error) { return effects.OSProcs{}.Started(pid) }

// heldLock is this process's account lock: the listening socket's fd,
// the wildcard binds held beside it (AR24 act 2), and the pid file it
// wrote (its path and identity, which the poll compares).
type heldLock struct {
	fd   int
	wild []int
	path string
	fi   os.FileInfo
}

// lockHeld is this process's lock, held for the process's life (and past
// an exec, ExecHolding); nil when none is held.
var (
	lockMu   sync.Mutex
	lockHeld *heldLock
)

// PidFile takes the account's lock for this process (relay-r1 §8, "one
// relay per account"; AR22 act 2, ADV13-4): a TCP listener on 127.0.0.1 at
// LockPort, which no sitting can unlink or rename, held for the process's
// life and freed by the kernel on its death; then <state>/relay.pid,
// written by a rename, whose one line `<pid> <started>` names the holder.
// While the port is held it refuses with NestedError; with the port taken
// and relay.pid naming a live holder other than this process, it frees
// the port and refuses naming that holder and the path (AR36 ruling 1). The fd is
// close-on-exec, so no spawned child inherits it; ExecHolding keeps it
// across resume's exec. release removes the pid file if it is still the
// one written, then closes the port.
func PidFile(state string) (release func(), err error) {
	if err := os.MkdirAll(state, 0o755); err != nil {
		return nil, err
	}
	port, err := LockPort()
	if err != nil {
		return nil, fmt.Errorf("the account's lock: %w", err)
	}
	path := filepath.Join(state, PidFileName)
	self := os.Getpid()
	started, err := PidStarted(self)
	if err != nil {
		return nil, err
	}
	fd, err := listenLoopback(port)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return nil, NestedError{Pid: liveHolder(path), Port: port}
		}
		return nil, fmt.Errorf("the account's lock (127.0.0.1:%d): %w", port, err)
	}
	wild, err := bindWildcard(port)
	if err != nil {
		syscall.Close(fd)
		if errors.Is(err, syscall.EADDRINUSE) {
			return nil, NestedError{Pid: liveHolder(path), Port: port}
		}
		return nil, fmt.Errorf("the account's lock: %w", err)
	}
	// AR36 ruling 1: a file naming a live holder other than this process
	// is another lock's (a probe linked to a port of its own over the
	// account's pid dir), never replaced: the port just taken goes back.
	// A dead or reused pid's file is replaced, as crash recovery; a live
	// pid whose start time cannot be read is not (maybeLiveHolder).
	if holder := maybeLiveHolder(path); holder > 0 && holder != self {
		closeAll(fd, wild)
		return nil, fmt.Errorf("%s names live relay %d, which holds another lock; this process is on port %d", path, holder, port)
	}
	fi, err := writePidLine(path, fmt.Sprintf("%d %s\n", self, started))
	if err != nil {
		closeAll(fd, wild)
		return nil, err
	}
	h := &heldLock{fd: fd, wild: wild, path: path, fi: fi}
	lockMu.Lock()
	lockHeld = h
	lockMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			lockMu.Lock()
			if lockHeld == h {
				lockHeld = nil
			}
			lockMu.Unlock()
			if now, err := os.Lstat(path); err == nil && os.SameFile(now, fi) {
				os.Remove(path)
			}
			closeAll(fd, wild) // the port goes with this process's last descriptor
		})
	}, nil
}

// listenLoopback is a listening TCP socket on 127.0.0.1:port, its fd
// close-on-exec (set under syscall.ForkLock, so no fork meanwhile inherits
// it) and no SO_REUSEADDR: a port anything else holds, a wildcard bind
// included, is EADDRINUSE.
func listenLoopback(port int) (int, error) {
	syscall.ForkLock.RLock()
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err == nil {
		syscall.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return -1, fmt.Errorf("socket: %w", err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Port: port, Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err := syscall.Listen(fd, 1); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// bindWildcard binds 0.0.0.0:port and, when the stack has IPv6,
// [::]:port (v6-only), each close-on-exec and never listening, beside the
// held 127.0.0.1:port (AR24 act 2, ADV14-4): on BSD a wildcard bind with
// SO_REUSEADDR (Go's net.Listen(":P"), python's http.server) passes a
// specific holder, so a listener planted during a night would refuse
// every later one; held here, it is EADDRINUSE. The tcp4 bind sets
// SO_REUSEADDR to pass this process's own 127.0.0.1:port, never
// SO_REUSEPORT, so no other bind shares it. Linux refuses an overlapping
// wildcard bind beside a listener already: none is held there.
func bindWildcard(port int) ([]int, error) {
	if runtime.GOOS == "linux" {
		return nil, nil
	}
	var fds []int
	for _, c := range []struct {
		family int
		sa     syscall.Sockaddr
		name   string
	}{
		{syscall.AF_INET, &syscall.SockaddrInet4{Port: port}, fmt.Sprintf("0.0.0.0:%d", port)},
		{syscall.AF_INET6, &syscall.SockaddrInet6{Port: port}, fmt.Sprintf("[::]:%d", port)},
	} {
		syscall.ForkLock.RLock()
		fd, err := syscall.Socket(c.family, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
		if err == nil {
			syscall.CloseOnExec(fd)
		}
		syscall.ForkLock.RUnlock()
		if err != nil && c.family == syscall.AF_INET6 && (errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EPROTONOSUPPORT)) {
			break // no IPv6 stack: nothing to hold
		}
		if err == nil {
			if c.family == syscall.AF_INET {
				err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			} else {
				err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
			}
			if err == nil {
				err = syscall.Bind(fd, c.sa)
			}
			if err != nil {
				syscall.Close(fd)
			}
		}
		if err != nil {
			closeAll(-1, fds)
			return nil, fmt.Errorf("%s: %w", c.name, err)
		}
		fds = append(fds, fd)
	}
	return fds, nil
}

// closeAll closes fd (when not -1) and each of wild.
func closeAll(fd int, wild []int) {
	for _, w := range wild {
		syscall.Close(w)
	}
	if fd >= 0 {
		syscall.Close(fd)
	}
}

// writePidLine puts line at path by a rename, so a reader never sees it
// truncated or half-written (ADV13-14's pid 0), and returns the new file's
// identity.
func writePidLine(path, line string) (os.FileInfo, error) {
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	os.Remove(tmp)
	fh, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	_, err = fh.WriteString(line)
	if cerr := fh.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return nil, err
	}
	return os.Lstat(path)
}

// liveHolder is the pid relay.pid names when that pid is live and started
// when the line says (its start read back, ADV13-14); 0 otherwise: a
// refusal never names a dead pid, a reused one, or pid 0.
func liveHolder(path string) int {
	pid, started := readPidLine(path)
	if pid <= 0 {
		return 0
	}
	if now, err := PidStarted(pid); err != nil || now != started {
		return 0
	}
	return pid
}

// maybeLiveHolder is PidFile's guard before its write (AR36 ruling 1): the
// pid relay.pid names when that pid is live and started when the line
// says, or when its start time cannot be read (PidStarted failed: a ps
// that could not run) but kill(pid, 0) finds it, so a live holder's file
// is never replaced on a failed read; 0 for a dead pid, a reused one, or
// no line.
func maybeLiveHolder(path string) int {
	pid, started := readPidLine(path)
	if pid <= 0 {
		return 0
	}
	now, err := PidStarted(pid)
	if err == nil {
		if now == started {
			return pid
		}
		return 0
	}
	if kerr := syscall.Kill(pid, 0); kerr == nil || errors.Is(kerr, syscall.EPERM) {
		return pid
	}
	return 0
}

// readPidLine is relay.pid's pid and start time as its one line gives
// them; 0 when path is not a small regular file whose line starts with a
// pid above 0.
func readPidLine(path string) (int, string) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 4096 {
		return 0, ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, ""
	}
	line, _, _ := strings.Cut(string(b), "\n")
	pidText, started, _ := strings.Cut(strings.TrimSpace(line), " ")
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		return 0, ""
	}
	return pid, strings.TrimSpace(started)
}

// LockMoved is the holder's poll (AR22 act 2): "" while relay.pid is the
// file this process wrote, or when it holds no lock; else why not (the
// file gone, or a new inode at its path). The night stops lock-moved.
func LockMoved() string {
	lockMu.Lock()
	h := lockHeld
	lockMu.Unlock()
	if h == nil {
		return ""
	}
	now, err := os.Lstat(h.path)
	if err != nil {
		return h.path + " is gone"
	}
	if !os.SameFile(now, h.fi) {
		return h.path + " is a new inode"
	}
	return ""
}

// ExecHolding replaces this process with bin (argv, env), keeping the
// account's lock across the exec (resume): close-on-exec is cleared on the
// port's fd and its wildcard binds (AR24 act 2) just before the exec, under syscall.ForkLock so no child
// forked meanwhile inherits it, and set again if the exec fails. With no
// lock held it is syscall.Exec.
func ExecHolding(bin string, argv, env []string) error {
	lockMu.Lock()
	h := lockHeld
	lockMu.Unlock()
	if h == nil {
		return syscall.Exec(bin, argv, env)
	}
	syscall.ForkLock.RLock() // a fork takes it for writing: none runs until the exec
	defer syscall.ForkLock.RUnlock()
	fds := append([]int{h.fd}, h.wild...)
	for _, fd := range fds {
		if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, 0); errno != 0 {
			for _, fd := range fds {
				syscall.CloseOnExec(fd)
			}
			return fmt.Errorf("the account's lock across the exec: %w", errno)
		}
	}
	err := syscall.Exec(bin, argv, env)
	for _, fd := range fds {
		syscall.CloseOnExec(fd)
	}
	return err
}
