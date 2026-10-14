// Package hooks is agent-relay's side of the plugin's hooks (overhaul-r1
// §2.5): the PreToolUse deny guards, the Stop guard on a dirty tree or an
// act with no commit, SessionStart's state file and packet injection, and
// PostToolUse's soft-ceiling note (AR28). The Stop guard, the state file
// and the note act only in a sitting (inSitting).
//
// Every hook exits 0 and prints one JSON object; a hook that cannot decide
// (unknown event, empty or malformed stdin, an effect that fails, a panic)
// prints {}. The one exception is the Bash door (AR44 act 2): a panic while
// a sitting's guards or the helper guard read a command is a deny
// (bashDoor), since {} there would let the command through unread.
package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/xdg"
)

// Effects is every effect a hook has, so a test can fake them all.
type Effects interface {
	StatusPorcelain(dir string) (string, error)
	Head(dir string) (string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, b []byte) error
	Now() time.Time
}

// empty is the answer of a hook that has nothing to say or cannot decide.
type empty struct{}

// Run answers one hook event read from stdin and returns 0 always.
func Run(event string, stdin io.Reader, stdout io.Writer, env func(string) string, fx Effects) int {
	writeJSON(stdout, answer(event, stdin, env, fx))
	return 0
}

// answer is one event's answer. A panic is the empty answer, but for the one
// bashDoor recovers first and answers with a deny.
func answer(event string, stdin io.Reader, env func(string) string, fx Effects) (out any) {
	defer func() {
		if recover() != nil {
			out = empty{}
		}
	}()
	b, err := io.ReadAll(stdin)
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return empty{}
	}
	var r any
	switch event {
	case "PreToolUse":
		r = preToolUse(b, env)
	case "Stop":
		r = stop(b, env, fx)
	case "SessionStart":
		r = sessionStart(b, env, fx)
	case "PostToolUse":
		r = postToolUse(b, env, fx)
	}
	if r == nil {
		return empty{}
	}
	return r
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// inSitting is the sitting test the Stop guard and SessionStart's state
// share (the owner's ruling, 2026-09-24): RELAY_ROW non-empty (ChildEnv's
// rows) or CLAUDE_SESSION_DEPTH an integer >= 1 (`claude-session start`
// and the stand-in's children). Anything else is the owner's own session,
// which the Stop guard never blocks and SessionStart never records. A
// helper (CLAUDE_SESSION_HELPER non-empty, AR25 act 0) is no sitting: it
// keeps RELAY_ROW, so the PreToolUse row guards stay on, but its Stop and
// start.json are its sitting's (s15-resync: Stop blocked the README
// renderer).
func inSitting(env func(string) string) bool {
	if env("CLAUDE_SESSION_HELPER") != "" {
		return false
	}
	if env("RELAY_ROW") != "" {
		return true
	}
	n, err := strconv.Atoi(env("CLAUDE_SESSION_DEPTH"))
	return err == nil && n >= 1
}

// postToolUse answers the relay's soft request (AR28): in a sitting whose
// session has a soft file (SoftPath), the note that asks the child to land
// the act in flight and stop; nil (so {}) with no sitting, no file, or one
// that does not parse. Advisory: nothing is denied.
func postToolUse(b []byte, env func(string) string, fx Effects) any {
	var in struct {
		SessionID string `json:"session_id"`
	}
	if !inSitting(env) || json.Unmarshal(b, &in) != nil {
		return nil
	}
	path := SoftPath(env, in.SessionID)
	if path == "" {
		return nil
	}
	raw, err := fx.ReadFile(path)
	var soft struct{ Context, Ceiling int }
	if err != nil || json.Unmarshal(raw, &soft) != nil || soft.Ceiling <= 0 {
		return nil
	}
	return struct {
		HookSpecificOutput contextOutput `json:"hookSpecificOutput"`
	}{contextOutput{"PostToolUse", fmt.Sprintf("relay: soft ceiling: context %dk of %dk. Land the act in flight now "+
		"(its gate, its commit, the charter's exit line), then end your turn; start no new act.", soft.Context/1000, soft.Ceiling/1000)}}
}

// state is start.json, one per session, written at SessionStart.
type state struct {
	Head    string `json:"head"`
	Cwd     string `json:"cwd"`
	Started string `json:"started"`
	Packet  string `json:"packet"`
	Blocks  int    `json:"blocks"`
}

// statePath is ${XDG_STATE_HOME:-~/.local/state}/agent-relay/sessions/<id>/start.json;
// "" when the session id is unsafe as a path segment or no base is known.
func statePath(env func(string) string, session string) string {
	if session == "" || session == "." || session == ".." || strings.ContainsAny(session, `/\`) {
		return ""
	}
	root := stateRoot(env)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "sessions", session, "start.json")
}

// SoftPath is the soft request beside start.json (AR28):
// <state>/sessions/<id>/soft.json, written by the relay at its soft line
// and read by the PostToolUse hook; "" when statePath is.
func SoftPath(env func(string) string, session string) string {
	p := statePath(env, session)
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "soft.json")
}

// stateNames are the state root's names under its base, the new first
// (agentik-r1 §6.3 c2): the folder moves only in the cold window, so the
// root is the new one when it exists, else the old when it exists, else
// the new; the file door denies both.
var stateNames = [2]string{"agentik-relay", "agent-relay"}

// stateBase is ${XDG_STATE_HOME:-~/.local/state}, `~` expanded under HOME,
// as the relay reads it: xdg.StateHome (AR76); "" when no base is known.
func stateBase(env func(string) string) string {
	return xdg.StateHome(env)
}

// stateRoot is <stateBase>/agentik-relay, else the old agent-relay when
// only it exists: the relay's state root as effects.StateDir resolves it;
// "" when no base is known.
func stateRoot(env func(string) string) string {
	base := stateBase(env)
	if base == "" {
		return ""
	}
	return newElseOld(filepath.Join(base, stateNames[0]), filepath.Join(base, stateNames[1]))
}

// newElseOld is §6.3's rule for a renamed path, as effects.NewElseOld
// reads it on the real file system (hooks imports nothing of the relay but
// the stdlib-only xdg).
func newElseOld(nw, old string) string {
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	if !exists(nw) && exists(old) {
		return old
	}
	return nw
}

// loadState reads start.json; ok is false when it is absent or unreadable.
func loadState(fx Effects, path string) (st state, ok bool) {
	if path == "" {
		return st, false
	}
	b, err := fx.ReadFile(path)
	if err != nil || json.Unmarshal(b, &st) != nil {
		return state{}, false
	}
	return st, true
}

func saveState(fx Effects, path string, st state) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return fx.WriteFile(path, append(b, '\n'))
}

// OS is the real Effects: git on PATH, the filesystem, the wall clock.
type OS struct{}

func (OS) StatusPorcelain(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return string(out), err
}

func (OS) Head(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	return strings.TrimSpace(string(out)), err
}

func (OS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (OS) WriteFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (OS) Now() time.Time { return time.Now() }

// Fake is the test Effects: porcelain and head per directory, files by path.
// A directory absent from Porcelain or Heads fails as git would outside a repo.
type Fake struct {
	Porcelain map[string]string
	Heads     map[string]string
	Files     map[string][]byte
	WriteErr  error
	Clock     time.Time
}

var errNotRepo = errors.New("fake: not a git repository")

func (f *Fake) StatusPorcelain(dir string) (string, error) {
	p, ok := f.Porcelain[dir]
	if !ok {
		return "", errNotRepo
	}
	return p, nil
}

func (f *Fake) Head(dir string) (string, error) {
	h, ok := f.Heads[dir]
	if !ok {
		return "", errNotRepo
	}
	return h, nil
}

func (f *Fake) ReadFile(path string) ([]byte, error) {
	b, ok := f.Files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return b, nil
}

func (f *Fake) WriteFile(path string, b []byte) error {
	if f.WriteErr != nil {
		return f.WriteErr
	}
	if f.Files == nil {
		f.Files = map[string][]byte{}
	}
	f.Files[path] = append([]byte(nil), b...)
	return nil
}

func (f *Fake) Now() time.Time { return f.Clock }
