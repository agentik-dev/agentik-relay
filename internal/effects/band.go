package effects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/xdg"
)

// The session band (band-pacing-r1 §2, §3; AR80): a night's row asks the
// session plugin's admission verdict at its first spawn and holds while it
// says no room. The relay writes none of the plugin's files: it runs the
// tool and reads band.json's reset.

// BandTool is the session plugin's command line tool, resolved at the
// night's open outside PinTools (band-pacing-r1 §2.1).
const BandTool = "agentik-session"

// BandTimeout is an ask's limit (band-pacing-r1 §2.2).
const BandTimeout = 10 * time.Second

// BandRequest is an ask's stdin: the part's model, the relay row's
// subagent type, background and not a fork, the tool use id
// `<night>/<row>/<part>`. The fields marshal in the order §2.2 quotes.
type BandRequest struct {
	Model        string `json:"model"`
	SubagentType string `json:"subagentType"`
	Background   bool   `json:"background"`
	Fork         bool   `json:"fork"`
	ToolUseID    string `json:"tool_use_id"`
}

// BandVerdict is the plugin's answer: its verdict, mode and text, and the
// peak it projects (nil for `unread` and `stale`).
type BandVerdict struct {
	Verdict string   `json:"verdict"`
	Mode    string   `json:"mode"`
	Text    string   `json:"text"`
	Peak    *float64 `json:"peak"`
}

// Band is the session band: Ask runs one admission for session, Reset
// reads the five-hour window's reset.
type Band interface {
	Ask(ctx context.Context, session string, req BandRequest) (BandVerdict, error)
	Reset() (time.Time, error)
}

// OSBand runs File (`agentik-session`) as `admission admit --session <id>`
// with the request on stdin under Timeout (zero: BandTimeout), and reads
// band.json under Env's state home (nil: the process's).
type OSBand struct {
	File    string
	Env     func(string) string
	Timeout time.Duration
}

// Ask runs the admission: a non-zero exit (3: the plugin's lock wait), the
// timeout, or stdout that is not one JSON object errs.
func (b OSBand) Ask(ctx context.Context, session string, req BandRequest) (BandVerdict, error) {
	limit := b.Timeout
	if limit <= 0 {
		limit = BandTimeout
	}
	in, err := json.Marshal(req)
	if err != nil {
		return BandVerdict{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.File, "admission", "admit", "--session", session)
	cmd.Stdin = bytes.NewReader(in)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	switch {
	case ctx.Err() != nil:
		return BandVerdict{}, fmt.Errorf("%s admission admit: %v after %s", filepath.Base(b.File), ctx.Err(), limit)
	case err != nil:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return BandVerdict{}, fmt.Errorf("%s admission admit: exit %d: %s", filepath.Base(b.File), ee.ExitCode(), lastLine(errb.String()))
		}
		return BandVerdict{}, fmt.Errorf("%s admission admit: %v", filepath.Base(b.File), err)
	}
	return oneVerdict(out.Bytes())
}

// oneVerdict reads stdout as exactly one JSON object.
func oneVerdict(out []byte) (BandVerdict, error) {
	dec := json.NewDecoder(bytes.NewReader(out))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return BandVerdict{}, fmt.Errorf("the admission's output: not one JSON object: %v", err)
	}
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '{' {
		return BandVerdict{}, errors.New("the admission's output: not one JSON object")
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return BandVerdict{}, errors.New("the admission's output: more than one JSON value")
	}
	var v BandVerdict
	if err := json.Unmarshal(raw, &v); err != nil {
		return BandVerdict{}, fmt.Errorf("the admission's output: %v", err)
	}
	return v, nil
}

// lastLine is s's last non-blank line.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// The band file under the state home: the plugin's new folder, else its
// old one, the folder the old session layer names (agentik-r1).
var bandFiles = []string{
	filepath.Join(BandTool, "admission", "band.json"),
	filepath.Join(strings.SplitN(oldSessionConfig, "/", 2)[0], "admission", "band.json"),
}

// Reset reads band.json's `resetsAt` under xdg.StateHome, the new folder
// else the old: no file, no `resetsAt`, or one not RFC 3339 errs.
func (b OSBand) Reset() (time.Time, error) {
	env := b.Env
	if env == nil {
		env = os.Getenv
	}
	home := xdg.StateHome(env)
	if home == "" {
		return time.Time{}, errors.New("band.json: no state home")
	}
	var data []byte
	var path string
	for _, rel := range bandFiles {
		path = filepath.Join(home, rel)
		var err error
		if data, err = os.ReadFile(path); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return time.Time{}, fmt.Errorf("band.json: %v", err)
		}
		data = nil
	}
	if data == nil {
		return time.Time{}, fmt.Errorf("band.json: none under %s", home)
	}
	var f struct {
		ResetsAt *string `json:"resetsAt"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return time.Time{}, fmt.Errorf("%s: %v", path, err)
	}
	if f.ResetsAt == nil || *f.ResetsAt == "" {
		return time.Time{}, fmt.Errorf("%s: no resetsAt", path)
	}
	at, err := time.Parse(time.RFC3339, *f.ResetsAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: resetsAt %q: not RFC 3339", path, *f.ResetsAt)
	}
	return at, nil
}

// ResolveBand is the night's band under the config's mode: `off` gives nil
// and "off"; else BandTool resolved as resolveTool resolves it, an OSBand
// and its file, or nil and "absent" when it resolves to none.
func ResolveBand(mode string, env func(string) string) (Band, string) {
	if mode == BandOff {
		return nil, BandOff
	}
	file, ok := resolveTool(BandTool)
	if !ok {
		return nil, "absent"
	}
	return OSBand{File: file, Env: env, Timeout: BandTimeout}, file
}

// BandVersion is `<file> --version`'s first line, run under BandTimeout
// (doctor's band check, band-pacing-r1 §3; AR81 ruling 6): it never asks,
// since an ask writes a verdict line. A non-zero exit, the timeout or no
// output errs.
func BandVersion(file string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), BandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, file, "--version")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	switch {
	case ctx.Err() != nil:
		return "", fmt.Errorf("%v after %s", ctx.Err(), BandTimeout)
	case err != nil:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("exit %d: %s", ee.ExitCode(), lastLine(errb.String()))
		}
		return "", err
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out.String()), "\n")
	if first = strings.TrimSpace(first); first == "" {
		return "", errors.New("no output")
	}
	return first, nil
}

// The `band` key's values (band-pacing-r1 §2.1): pace, the default, or off.
const (
	BandPace = "pace"
	BandOff  = "off"
)

// FakeAnswer is one scripted answer: the verdict, or Err.
type FakeAnswer struct {
	BandVerdict
	Err error
}

// FakeBand scripts each row's answers by the row in the tool use id
// (`<night>/<row>/<part>`), the last repeating (none: `admitted`), and
// Reset (ResetErr, else ResetAt; zero: no resetsAt). It records each ask
// as `<tool_use_id> <session> <model>`, then the time when Clock is set.
type FakeBand struct {
	Answers  map[string][]FakeAnswer
	ResetAt  time.Time
	ResetErr error
	Clock    Clock

	mu   sync.Mutex
	asks []string
	n    map[string]int
}

func (f *FakeBand) Ask(_ context.Context, session string, req BandRequest) (BandVerdict, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := req.ToolUseID
	if p := strings.Split(row, "/"); len(p) == 3 {
		row = p[1]
	}
	rec := req.ToolUseID + " " + session + " " + req.Model
	if f.Clock != nil {
		rec += " " + f.Clock.Now().UTC().Format(time.RFC3339)
	}
	f.asks = append(f.asks, rec)
	if f.n == nil {
		f.n = map[string]int{}
	}
	as := f.Answers[row]
	k := f.n[row]
	f.n[row]++
	if len(as) == 0 {
		return BandVerdict{Verdict: "admitted"}, nil
	}
	a := as[min(k, len(as)-1)]
	return a.BandVerdict, a.Err
}

func (f *FakeBand) Reset() (time.Time, error) {
	if f.ResetErr != nil {
		return time.Time{}, f.ResetErr
	}
	if f.ResetAt.IsZero() {
		return time.Time{}, errors.New("band.json: no resetsAt")
	}
	return f.ResetAt, nil
}

// Asks is every ask so far, in order.
func (f *FakeBand) Asks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asks...)
}
