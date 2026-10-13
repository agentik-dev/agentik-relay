package night_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/gate"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

var (
	fakeOnce sync.Once
	fakeBin  string
	fakeErr  error
)

// fakeClaude builds internal/effects/testdata/fakeclaude once per test
// binary and puts it first on PATH as `claude`.
func fakeClaude(t *testing.T) {
	t.Helper()
	fakeOnce.Do(func() {
		fakeBin, fakeErr = os.MkdirTemp("", "fakeclaude")
		if fakeErr == nil {
			out, err := exec.Command("go", "build", "-o", filepath.Join(fakeBin, "claude"), "../effects/testdata/fakeclaude").CombinedOutput()
			if err != nil {
				fakeErr = &buildError{string(out)}
			}
		}
	})
	if fakeErr != nil {
		t.Fatal(fakeErr)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

type buildError struct{ out string }

func (e *buildError) Error() string { return "build fakeclaude: " + e.out }

// TestMain points XDG_STATE_HOME, the one state root (§5), at a temp
// directory before any test, so no test reads or writes the machine's
// ledger, locks or nights; and removes the fake claude after.
func TestMain(m *testing.M) {
	// The tests push only into t.TempDir() remotes; the deny is advisory (relay-r1 §10; ADV11-8 deferred).
	effects.DropEnvConfig()
	state, err := os.MkdirTemp("", "relay-state-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Setenv("XDG_STATE_HOME", state)
	// The go tool keeps its cache, modules and env file under the real
	// HOME: pinned before HOME moves, so a test's go build stays warm.
	if out, err := exec.Command("go", "env", "GOCACHE", "GOPATH", "GOMODCACHE", "GOENV").Output(); err == nil {
		keys := []string{"GOCACHE", "GOPATH", "GOMODCACHE", "GOENV"}
		for i, v := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			if i < len(keys) && os.Getenv(keys[i]) == "" {
				os.Setenv(keys[i], v)
			}
		}
	}
	// AR21 act 0 (ADV12-1): HOME and XDG_CONFIG_HOME are temp dirs, so no
	// test reads the owner's ~/.gitconfig or ~/.config/git; the tests' own
	// git commits as t.
	for k, v := range map[string]string{"HOME": filepath.Join(state, "home"), "XDG_CONFIG_HOME": filepath.Join(state, "config"),
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		if strings.HasPrefix(v, state) {
			os.MkdirAll(v, 0o755)
		}
		os.Setenv(k, v)
	}
	// AR22 act 2: the account lock is a free port of this binary's, never
	// the account's.
	if err := useTestLockPort(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// AR22 act 1: every gate line and runner git runs under a pinned env,
	// built under a temp dir of its own.
	code := effects.RunPinned(m)
	os.RemoveAll(state)
	if fakeBin != "" {
		os.RemoveAll(fakeBin)
	}
	os.Exit(code)
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// world is a temp root on main (no remote), the machine's effects with a
// fake ledger, the locks in a temp dir.
type world struct {
	root, transcripts string
	fx                effects.Effects
	ledger            *effects.FakeLedger
}

func newWorld(t *testing.T) *world {
	t.Helper()
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}
	oldDir, oldLive, oldGrace := night.LockDir, night.LiveClaude, night.KillGrace
	locks := t.TempDir()
	night.LockDir = func() string { return locks }
	night.LiveClaude = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() { night.LockDir, night.LiveClaude, night.KillGrace = oldDir, oldLive, oldGrace })
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	w := &world{root: filepath.Join(dir, "root"), transcripts: filepath.Join(dir, "projects")}
	gitIn(t, dir, "init", "-q", "-b", "main", w.root)
	os.WriteFile(filepath.Join(w.root, "a.txt"), []byte("a\n"), 0o644)
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "one")
	clock := effects.FakeClock{T: time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)}
	w.ledger = &effects.FakeLedger{Clock: clock, Night: "2026-09-22-1"}
	w.fx = effects.Effects{Git: effects.OSGit{}, Shell: effects.OSShell{}, Clock: clock, FS: effects.OSFS{}, Ledger: w.ledger}
	return w
}

// stage stages row id off main with the planned spawn line.
func (w *world) stage(t *testing.T, id string) (*night.Staged, *night.RowPlan) {
	t.Helper()
	rp := &night.RowPlan{ID: id, Worktree: filepath.Join(w.root, ".claude", "worktrees", id), Branch: id, Base: "main",
		SessionID: night.NewUUID()}
	rp.Spawn = night.SpawnArgv(queue.Row{ID: id, Model: "claude-opus-5-5", Turns: 100, USD: 15}, rp.SessionID, "Bash Edit Write", false)
	st, err := night.Stage(w.fx, rp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Unlock() })
	return st, rp
}

// env is the child's environment for a fixture.
func (w *world) env(fixture string, extra ...string) []string {
	abs, _ := filepath.Abs(filepath.Join("..", "effects", "testdata", "fakeclaude", "fixtures", fixture))
	return append(append(os.Environ(), "FAKECLAUDE_FIXTURE="+abs, "FAKECLAUDE_TRANSCRIPT="+w.transcripts), extra...)
}

// gateWith is A6's gate with one acceptance line that holds.
func (w *world) gateWith() func(*night.Staged, bool, bool) (relay.Result, relay.StopReason, relay.Gate) {
	p := &packet.Packet{ID: "a1", Acceptance: []packet.Line{{Shell: "echo ok", Expect: "reads `ok`", Kind: packet.Reads, Value: "ok", N: 1}}}
	return func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
		res, why, g, _ := gate.Run(w.fx, st, p, nil, &gate.ExitFacts{Seen: seen, IsError: isError}, gate.Verify{})
		return res, why, g
	}
}

func events(lines []map[string]any, row string) []string {
	var out []string
	for _, l := range lines {
		if l["row"] == row {
			out = append(out, l["event"].(string))
		}
	}
	return out
}

func lastOf(lines []map[string]any, event string) map[string]any {
	var m map[string]any
	for _, l := range lines {
		if l["event"] == event {
			m = l
		}
	}
	return m
}

// TestRunOneRowFake: the fake claude over ok.jsonl in a temp repo is green
// with one commit, and the ledger reads stage, spawn, turn, exit, gate,
// sitting.
func TestRunOneRowFake(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	var log bytes.Buffer
	s, g := night.RunRow(w.fx, effects.OSSpawner{}, st, rp, []byte("the prompt\n"), night.RunOptions{
		Context: 130000, Wall: time.Minute, Log: &log, Gate: w.gateWith(), Env: w.env("ok.jsonl"), Transcripts: w.transcripts})
	if s.State != relay.Green || !g.Commit || !g.ResultSeen {
		t.Fatalf("sitting %+v gate %+v\n%s", s, g, log.String())
	}
	if n := gitIn(t, st.Path, "rev-list", "--count", st.HeadBefore+"..HEAD"); n != "1" {
		t.Fatalf("%s commits", n)
	}
	seq := strings.Join(events(w.ledger.Written(), "a1"), " ")
	for _, want := range []string{"stage", "spawn", "turn", "exit", "gate", "sitting"} {
		if !strings.Contains(seq, want) {
			t.Fatalf("no %s line in %s", want, seq)
		}
	}
	sit := lastOf(w.ledger.Written(), "sitting")
	if sit["builders_source"] != "subagents" || sit["state"] != "green" || sit["session_id"] != rp.SessionID {
		t.Fatalf("sitting line %v", sit)
	}
	if !strings.Contains(log.String(), `turn 3: Bash "git commit -m act"; context 71k/130k; $1.20`) {
		t.Fatalf("live line:\n%s", log.String())
	}
}

// TestNoCommitFails: a sitting that commits nothing fails no-commit.
func TestNoCommitFails(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	s, _ := night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 130000, Gate: w.gateWith(), Env: w.env("ok.jsonl", "FAKECLAUDE_NO_COMMIT=1"), Transcripts: w.transcripts})
	if s.State != relay.Failed || s.Reason != relay.NoCommit {
		t.Fatalf("sitting %+v", s)
	}
}

// TestLedgerBrackets: stage before and exit after every spawn, whatever
// the stream does (§11), including a child that never starts.
func TestLedgerBrackets(t *testing.T) {
	w := newWorld(t)
	for i, fx := range []string{"ok.jsonl", "error.jsonl", "noresult.jsonl", "over.jsonl", ""} {
		id := "b" + string(rune('1'+i))
		st, rp := w.stage(t, id)
		sp := &effects.FakeSpawner{}
		if fx == "" {
			sp.Err = os.ErrNotExist
		} else {
			sp.Out, _ = os.ReadFile(filepath.Join("..", "effects", "testdata", "fakeclaude", "fixtures", fx))
		}
		night.RunRow(w.fx, sp, st, rp, nil, night.RunOptions{Context: 130000, Transcripts: w.transcripts})
	}
	staged, spawned := map[string]bool{}, map[string]bool{}
	for _, l := range w.ledger.Written() {
		row, _ := l["row"].(string)
		switch l["event"] {
		case "stage":
			staged[row] = true
		case "spawn":
			if !staged[row] || spawned[row] {
				t.Fatalf("spawn for %s without its stage, or twice", row)
			}
			spawned[row] = true
		case "exit":
			if !spawned[row] {
				t.Fatalf("exit for %s without a spawn", row)
			}
			delete(spawned, row)
		}
	}
	if len(spawned) != 0 || len(staged) != 5 {
		t.Fatalf("unbracketed spawns %v, staged %d", spawned, len(staged))
	}
}

// TestCeilingKill: over.jsonl with a delay passes the context on turn two;
// the child is signalled (a SIGTERM it ignores, then SIGKILL after the
// grace) and the sitting fails ceiling-kill. The fake holds its result a
// minute (AR35), so the kill landed before the child's work would finish
// when no result was read, however loaded the machine; the time bound
// only guards against a hang.
func TestCeilingKill(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	night.KillGrace = 300 * time.Millisecond
	st, rp := w.stage(t, "a1")
	var notes []string
	start := time.Now()
	s, _ := night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Notify: func(n string) { notes = append(notes, n) },
		Env: w.env("over.jsonl", "FAKECLAUDE_DELAY_MS=200", "FAKECLAUDE_RESULT_DELAY_MS=60000", "FAKECLAUDE_IGNORE_TERM=1"), Transcripts: w.transcripts})
	took := time.Since(start)
	if s.State != relay.Failed || s.Reason != relay.CeilingKill {
		t.Fatalf("sitting %+v", s)
	}
	exit := lastOf(w.ledger.Written(), "exit")
	if exit["reason"] != "ceiling-kill" || exit["child_exit"] != float64(128+int(syscall.SIGKILL)) || exit["cost_read"] != false {
		t.Fatalf("exit line %v", exit)
	}
	if lastOf(w.ledger.Written(), "ceiling") == nil || len(notes) != 1 {
		t.Fatalf("no ceiling line or notify: %v", notes)
	}
	if took > 20*time.Second { // a hang guard: the kill is due in about a second, the result in a minute
		t.Fatalf("the kill took %v", took)
	}
}

// TestWallClockKill: the wall clock kills the same way, wall-clock.
func TestWallClockKill(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	s, _ := night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 1 << 30, Wall: 300 * time.Millisecond, Env: w.env("ok.jsonl", "FAKECLAUDE_DELAY_MS=500"), Transcripts: w.transcripts})
	if s.State != relay.Failed || s.Reason != relay.WallClock {
		t.Fatalf("sitting %+v", s)
	}
}

// TestAR4_SittingLineKeys: the `sitting` line carries every key of the
// stand-in's (24, `row` included), `gate` an object holding `result` and
// `verify`; repo is the root, cwd the worktree, the kill reason empty.
func TestAR4_SittingLineKeys(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, []byte("the prompt\n"), night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Env: w.env("ok.jsonl"), Transcripts: w.transcripts})
	sit := lastOf(w.ledger.Written(), "sitting")
	for _, k := range []string{"row", "repo", "cwd", "mode", "branch", "base", "prompt_hash", "rules_hash", "model",
		"max_turns", "budget_usd", "head_before", "head_after", "session_id", "turns", "cost_usd", "duration_ms",
		"wall_s", "child_exit", "is_error", "denials", "gate", "stop", "next_after"} {
		if _, ok := sit[k]; !ok {
			t.Errorf("the sitting line lacks %q", k)
		}
	}
	gate, ok := sit["gate"].(map[string]any)
	if !ok {
		t.Fatalf("gate = %#v, want an object", sit["gate"])
	}
	if _, ok := gate["verify"]; !ok || gate["result"] != "green" {
		t.Fatalf("gate = %v", gate)
	}
	if sit["repo"] != w.root || sit["cwd"] != st.Path || sit["mode"] != "worktree" || sit["stop"] != "" ||
		sit["next_after"] != "" || sit["head_before"] != st.HeadBefore || sit["head_after"] == st.HeadBefore {
		t.Fatalf("sitting line %v", sit)
	}
}

// TestAR5_OneLiveLinePerMessage (UX1): real.jsonl streams one assistant
// message as two events; the live log writes it once, and the result once.
func TestAR5_OneLiveLinePerMessage(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	var log bytes.Buffer
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, []byte("the prompt\n"), night.RunOptions{
		Context: 130000, Wall: time.Minute, Log: &log, Gate: w.gateWith(),
		Env: w.env("../../../../stream/testdata/real.jsonl"), Transcripts: w.transcripts})
	lines := strings.Split(strings.TrimRight(log.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "turn 1: ") || !strings.HasSuffix(lines[1], "$0.03") {
		t.Fatalf("live log:\n%s", log.String())
	}
}

// TestAR5_SpawnLineAppendErrorStops (AR2-20): a `spawn` line that fails to
// append spawns nothing; the sitting fails ledger-error, which stops the
// night.
func TestAR5_SpawnLineAppendErrorStops(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	w.ledger.Err, w.ledger.FailOn = fmt.Errorf("disk full"), "spawn"
	sp := &recorder{inner: effects.OSSpawner{}}
	s, _ := night.RunRow(w.fx, sp, st, rp, []byte("the prompt\n"), night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Env: w.env("ok.jsonl"), Transcripts: w.transcripts})
	if s.State != relay.Failed || s.Reason != relay.LedgerError || len(sp.spawns) != 0 {
		t.Fatalf("sitting %+v, %d spawns", s, len(sp.spawns))
	}
	if seq := strings.Join(events(w.ledger.Written(), "a1"), " "); strings.Contains(seq, "spawn") || !strings.Contains(seq, "sitting") {
		t.Fatalf("ledger %s", seq)
	}
	if sit := lastOf(w.ledger.Written(), "sitting"); sit["state"] != "failed" || sit["reason"] != "ledger-error" {
		t.Fatalf("sitting line %v", sit)
	}
	if ns, err := relay.NightStep(relay.NightRunning, relay.OnSittingFailed, s.Reason); err != nil || ns != relay.Stopped(relay.LedgerError) {
		t.Fatalf("night %s %v", ns, err)
	}
}

// TestAR5_SpawnLineHashes (AR2-16): the `spawn` line RunRow writes carries
// prompt_hash, the sha256 of the stdin the child read, and rules_hash, the
// template hash prompt.Render gives.
func TestAR5_SpawnLineHashes(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	stdin, rulesHash, promptHash := prompt.Render(prompt.Context{Row: "a1", Tree: st.Path, Branch: "a1", Base: "main"}, nil, []byte("# Packet a1\n"))
	sp := &recorder{inner: effects.OSSpawner{}}
	night.RunRow(w.fx, sp, st, rp, stdin, night.RunOptions{
		Context: 130000, Wall: time.Minute, Gate: w.gateWith(), Env: w.env("ok.jsonl"), Transcripts: w.transcripts})
	sum := sha256.Sum256(stdin)
	line := lastOf(w.ledger.Written(), "spawn")
	if len(sp.spawns) != 1 || !bytes.Equal(sp.spawns[0].Stdin, stdin) {
		t.Fatalf("%d spawns", len(sp.spawns))
	}
	if line["prompt_hash"] != hex.EncodeToString(sum[:]) || line["prompt_hash"] != promptHash || line["rules_hash"] != rulesHash {
		t.Fatalf("spawn line %v; want prompt %s rules %s", line, promptHash, rulesHash)
	}
}

// TestAR7_LiveLineNamesOwnTool (ar5's line): split-tool.jsonl streams its
// first message as two events, the tool_use in the second; the live line is
// held until the next message begins, so it names that message's own tool,
// one line per message and the result once.
func TestAR7_LiveLineNamesOwnTool(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	st, rp := w.stage(t, "a1")
	var log bytes.Buffer
	night.RunRow(w.fx, effects.OSSpawner{}, st, rp, []byte("the prompt\n"), night.RunOptions{
		Context: 130000, Wall: time.Minute, Log: &log, Gate: w.gateWith(),
		Env: w.env("../../../../stream/testdata/split-tool.jsonl"), Transcripts: w.transcripts})
	lines := strings.Split(strings.TrimRight(log.String(), "\n"), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "turn 1: Read ") || !strings.HasPrefix(lines[1], "turn 2: Bash ") ||
		!strings.HasSuffix(lines[2], "$0.50") {
		t.Fatalf("live log:\n%s", log.String())
	}
}
