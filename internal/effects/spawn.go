package effects

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// Spawn is one child: its working tree, argv, stdin (the prompt), its whole
// environment, and Stdout, when set, a copy of every byte the child writes
// to stdout (the stream itself is the reader Start returns).
type Spawn struct {
	Dir    string
	Argv   []string
	Stdin  []byte
	Env    []string
	Stdout io.Writer
}

// Process is a started child. Signal reaches its whole process group; Wait
// is the exit code (128 plus the signal for a signalled child, its error
// then `signal: <name>`, run 10 ruling 7) at the child's exit, whoever
// still holds its stream (ADV6-7: the reader closes a held one); Pid is its pid, which is its process group's id and its
// session's too (Setsid, AR17C; the watchdog's kill, §15).
type Process interface {
	Signal(os.Signal) error
	Wait() (exit int, err error)
	Pid() int
}

// RelayBin is the running binary's absolute path (os.Executable), "" when
// the OS cannot say; ChildEnv passes it as RELAY_BIN.
func RelayBin() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// Spawner starts a child and hands back its stdout.
type Spawner interface {
	Start(Spawn) (Process, io.ReadCloser, error)
}

// DropSitting is environ with a sitting's marks gone: every CLAUDE*
// variable (a nested spawn is its own session with its own hooks), every
// RELAY_* but RELAY_DEVICE_VERBS too (the child's hook reads the owner's
// device verbs, ADV6-11). ChildEnv's drop, and every helper's env the relay
// starts that is not a sitting: each tmux command (a tmux server the relay
// starts would hand CLAUDE_SESSION_DEPTH and RELAY_ROW to every later
// window, and a window's claude would be taken for a sitting) and doctor's
// claude probe (AR24 act 3, ADV14-5).
func DropSitting(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if strings.HasPrefix(kv, "CLAUDE") || strings.HasPrefix(kv, "RELAY_") && !strings.HasPrefix(kv, "RELAY_DEVICE_VERBS=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// ChildEnv is §7's environment: DropSitting's drop (every CLAUDE* variable,
// every RELAY_* but RELAY_DEVICE_VERBS), and RELAY_ROW=<row> added, and
// RELAY_MEASURE_DEPTH=1 (§15: a row's own `agentik-relay plan` measures nothing); the child knows nothing of the
// plugin beyond its row (R25-91 (3)). Each of remotes gets its pushurl list
// reset and set to DeniedPushURL through GIT_CONFIG_COUNT, after any the
// parent set, so a push past the hook (`--no-verify`) lands nothing (ADV6-6).
// After the CLAUDE* drop the child gets the session plugin's budget:
// SessionDepthVar=1 and SessionTurnsVar=<turns>, the spawn line's
// --max-turns (S10's handover; a parent's values never pass), and
// SessionProfileVar=<profile>, the row's role (`builder`, or `orchestrator`
// for an orchestrator sitting: AR41), so the plugin meters the child in
// its role, never the default profile; "" sets none. SessionModelVar is
// model, the spawn line's --model as the queue row writes it, its window
// marker intact (`claude-opus-5-5[1m]`), since under `claude -p` the
// SessionStart payload names no model and the plugin would meter the
// child at its profile's window (AR74; the session plugin's S28 reads
// it); "" sets none. RELAY_BIN is
// bin, the running relay's absolute path, so the plugin's hook shim finds
// the guards without PATH (ADV10-15); "" sets none.
func ChildEnv(environ []string, row, profile, model string, turns int, remotes []string, bin string) []string {
	out := append(DropSitting(environ), "RELAY_ROW="+row, MeasureDepthVar+"=1",
		SessionDepthVar+"=1", fmt.Sprintf("%s=%d", SessionTurnsVar, turns))
	if profile != "" {
		out = append(out, SessionProfileVar+"="+profile)
	}
	if model != "" {
		out = append(out, SessionModelVar+"="+model)
	}
	if bin != "" {
		out = append(out, "RELAY_BIN="+bin)
	}
	return PushDenyEnv(out, remotes)
}

// PushDenyEnv is environ with each of remotes' pushurl list reset and set
// to DeniedPushURL through GIT_CONFIG_COUNT, after any entries environ
// already counts (ADV6-6): ChildEnv's deny, and the gate's lines' (AR20
// act 3, ADV11-6).
func PushDenyEnv(environ []string, remotes []string) []string {
	n := 0
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "GIT_CONFIG_COUNT="); ok {
			if c, err := strconv.Atoi(v); err == nil && c > 0 {
				n = c
			}
		}
	}
	out := make([]string, 0, len(environ)+1+2*len(remotes))
	for _, kv := range environ {
		switch {
		case strings.HasPrefix(kv, "GIT_CONFIG_COUNT="),
			n == 0 && (strings.HasPrefix(kv, "GIT_CONFIG_KEY_") || strings.HasPrefix(kv, "GIT_CONFIG_VALUE_")):
			continue
		}
		out = append(out, kv)
	}
	if len(remotes) == 0 {
		if n > 0 {
			out = append(out, fmt.Sprintf("GIT_CONFIG_COUNT=%d", n))
		}
		return out
	}
	for _, r := range remotes {
		// An empty value resets the multi-valued list (git 2.46), so a
		// pushurl in the repo's own config is not pushed to as well.
		for _, v := range []string{"", DeniedPushURL} {
			out = append(out, fmt.Sprintf("GIT_CONFIG_KEY_%d=remote.%s.pushurl", n, r), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", n, v))
			n++
		}
	}
	return append(out, fmt.Sprintf("GIT_CONFIG_COUNT=%d", n))
}

// DropEnvConfig unsets every GIT_CONFIG_COUNT, GIT_CONFIG_KEY_* and
// GIT_CONFIG_VALUE_* in this process's environment: PushDenyEnv's entries
// among them. Only the test suites call it (TestMain): their pushes go into
// t.TempDir() remotes alone, and the deny is advisory (relay-r1 §10; the
// sandbox is ADV11-8, deferred), so the suite reads the same under a
// sitting's or a gate's env as under the owner's.
func DropEnvConfig() {
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k == "GIT_CONFIG_COUNT" || strings.HasPrefix(k, "GIT_CONFIG_KEY_") || strings.HasPrefix(k, "GIT_CONFIG_VALUE_") {
			os.Unsetenv(k)
		}
	}
}

// SessionDepthVar, SessionTurnsVar, SessionProfileVar and SessionModelVar
// are the session plugin's budget in a child: its depth (a relay's child
// is depth 1), its turn ceiling, its profile (the row's role; a repo's own
// session config outranks it), and its model (the spawn line's --model,
// so the plugin reads the child's window: AR74).
const (
	SessionDepthVar   = "CLAUDE_SESSION_DEPTH"
	SessionTurnsVar   = "CLAUDE_SESSION_MAX_TURNS"
	SessionProfileVar = "CLAUDE_SESSION_PROFILE"
	SessionModelVar   = "CLAUDE_SESSION_MODEL"
)

// DeniedPushURL is every remote's pushurl in a child: not a repository, so
// a push fails before it sends a byte.
const DeniedPushURL = "relay-denies-push"

// OSSpawner starts children with os/exec, each in its own session and so
// its own process group (Setsid, AR17C, ADV10-12).
// With Caffeinate, an argv not already under `caffeinate` gets `caffeinate
// -i` in front when caffeinate is on PATH.
type OSSpawner struct{ Caffeinate bool }

// StderrCap is how much of a child's stderr Wait's error carries.
const StderrCap = 400

func (s OSSpawner) Start(sp Spawn) (Process, io.ReadCloser, error) {
	argv := sp.Argv
	if len(argv) == 0 {
		return nil, nil, fmt.Errorf("spawn: an empty argv")
	}
	if s.Caffeinate && argv[0] != "caffeinate" {
		if _, err := exec.LookPath("caffeinate"); err == nil {
			argv = append([]string{"caffeinate", "-i"}, argv...)
		}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = sp.Dir, sp.Env
	cmd.Stdin = strings.NewReader(string(sp.Stdin))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	p := &osProcess{cmd: cmd}
	cmd.Stderr = &p.stderr
	// stdout is a file pipe the caller reads and closes, so the Wait
	// neither waits on it nor closes it; stdin's and stderr's copies are
	// cut off PipeGrace after the exit.
	out, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdout, cmd.WaitDelay = w, PipeGrace
	err = cmd.Start()
	w.Close()
	if err != nil {
		out.Close()
		return nil, nil, err
	}
	var r io.ReadCloser = out
	if sp.Stdout != nil {
		r = teeCloser{io.TeeReader(out, sp.Stdout), out}
	}
	return p, r, nil
}

type teeCloser struct {
	io.Reader
	io.Closer
}

type osProcess struct {
	cmd    *exec.Cmd
	stderr tailBuffer
}

// Signal sends sig to the child's process group, so `caffeinate` and the
// claude under it both get it.
func (p *osProcess) Signal(sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return p.cmd.Process.Signal(sig)
	}
	return syscall.Kill(-p.cmd.Process.Pid, s)
}

func (p *osProcess) Pid() int { return p.cmd.Process.Pid }

func (p *osProcess) Wait() (int, error) {
	err := p.cmd.Wait()
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil // a holder of stderr or stdin: the exit is still the child's
	}
	st := p.cmd.ProcessState
	if st == nil {
		return -1, err
	}
	if ws, ok := st.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		// The signal named whole (run 10 ruling 7): `signal: killed`.
		return 128 + int(ws.Signal()), fmt.Errorf("signal: %s", ws.Signal())
	}
	if code := st.ExitCode(); code != 0 {
		return code, fmt.Errorf("exit %d: %s", code, strings.TrimSpace(p.stderr.String()))
	}
	return 0, nil
}

// tailBuffer keeps the last StderrCap bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > StderrCap {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-StderrCap:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	// The tail was cut by count; drop leading continuation bytes so it
	// starts on a rune.
	s := b.buf
	for len(s) > 0 && s[0]&0xC0 == 0x80 {
		s = s[1:]
	}
	return capText(string(s), StderrCap)
}
