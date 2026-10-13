// Command fakeclaude is a stand-in for the claude CLI used by the effects
// tests: it replays a stream-json fixture, optionally writes a transcript
// tree, makes an empty commit and exits with a chosen code.
//
// The end-to-end knobs (relay-r1 §11, AR12), each read for the row the
// relay names in RELAY_ROW: FAKECLAUDE_MODE is one mode or a list
// `row=mode,...` (ok, red, stall, orphan, hang, hold, next); FAKECLAUDE_ACTS
// is a count or `row=n,...`, n files acts/<row>-<k>.txt, one commit each
// instead of the empty one; FAKECLAUDE_PROMPT, _PIDFILE and _ENVDUMP name
// directories the fake writes <row>.prompt (stdin), <row>.pid (and
// <row>.sleeper for a forked sleeper) and <row>.env into. FAKECLAUDE_RESULT
// is a result text or `row=text,...` (run 10 ruling 1): the fixture's
// result event carries it with is_error true, the fake leaves
// acts/<row>-wip.txt uncommitted, makes no commit and exits 1.
// FAKECLAUDE_FILES names a directory (AR32A act 4): the fake copies
// <dir>/<branch>/ over its tree, <branch> its worktree's, and commits it
// instead of the empty commit; a branch with no directory there is as
// before. Before it copies, it merges each branch <dir>/<branch>.merge
// lists, one a line, `--no-ff --no-edit` (ar32b act 3).
// FAKECLAUDE_RESULT_DELAY_MS (AR35) is a further wait before the
// fixture's result event: a kill that lands in it leaves no result, so a
// test of a kill before the result holds however slow the relay is.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func fail(code int, format string, a ...any) {
	fmt.Fprintf(os.Stderr, "fakeclaude: "+format+"\n", a...)
	os.Exit(code)
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fail(2, "%s: %v", name, err)
	}
	return n
}

func writeTranscript(dir, session string) {
	top := strings.Join([]string{
		`{"type":"user","n":1}`,
		`{"type":"assistant","n":2}`,
		`{"type":"user","n":3}`,
	}, "\n") + "\n"
	sub := `{"type":"assistant","message":{"id":"msg_sub1","usage":{"input_tokens":5,"cache_creation_input_tokens":100,"cache_read_input_tokens":200,"output_tokens":7}}}` + "\n" +
		`{"type":"assistant","message":{"id":"msg_sub2","usage":{"input_tokens":5,"cache_creation_input_tokens":100,"cache_read_input_tokens":200,"output_tokens":9}}}` + "\n"
	subDir := filepath.Join(dir, session, "subagents")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		fail(2, "transcript: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte(top), 0o600); err != nil {
		fail(2, "transcript: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(sub), 0o600); err != nil {
		fail(2, "transcript: %v", err)
	}
}

func main() {
	session := "fake-session"
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--session-id" && i+1 < len(args) {
			session = args[i+1]
			i++
		}
	}
	if os.Getenv("FAKECLAUDE_IGNORE_TERM") != "" {
		signal.Ignore(syscall.SIGTERM)
	}
	row := os.Getenv("RELAY_ROW")
	mode := perRow(os.Getenv("FAKECLAUDE_MODE"), row, "ok")
	prompt, _ := io.ReadAll(os.Stdin)
	record("FAKECLAUDE_PROMPT", row, ".prompt", prompt)
	record("FAKECLAUDE_ENVDUMP", row, ".env", []byte(strings.Join(os.Environ(), "\n")+"\n"))
	record("FAKECLAUDE_PIDFILE", row, ".pid", []byte(strconv.Itoa(os.Getpid())))

	path := os.Getenv("FAKECLAUDE_FIXTURE")
	if path == "" {
		fail(2, "FAKECLAUDE_FIXTURE is unset")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fail(2, "fixture: %v", err)
	}
	delay := time.Duration(envInt("FAKECLAUDE_DELAY_MS", 0)) * time.Millisecond
	resultDelay := time.Duration(envInt("FAKECLAUDE_RESULT_DELAY_MS", 0)) * time.Millisecond
	code := envInt("FAKECLAUDE_EXIT", 0)

	out := bufio.NewWriter(os.Stdout)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	switch mode {
	case "stall", "hang": // the init line, then silence: no result event
		lines = lines[:1]
	case "ok", "red", "orphan", "hold", "next":
	default:
		fail(2, "FAKECLAUDE_MODE %q: unknown", mode)
	}
	result := perRow(os.Getenv("FAKECLAUDE_RESULT"), row, "")
	commitAt := envInt("FAKECLAUDE_COMMIT_AT", 0) // a commit before that line (AR28's head move)
	for i, line := range lines {
		if commitAt > 0 && i == commitAt {
			if msg, err := exec.Command("git", "commit", "--allow-empty", "-qm", "fake mid act").CombinedOutput(); err != nil {
				fail(3, "git commit: %v: %s", err, msg)
			}
		}
		var ev map[string]any
		if result != "" && json.Unmarshal([]byte(line), &ev) == nil && ev["type"] == "result" {
			ev["is_error"], ev["result"] = true, result
			b, _ := json.Marshal(ev)
			line = string(b)
		}
		if i > 0 && delay > 0 {
			time.Sleep(delay)
		}
		var kind struct{ Type string }
		if resultDelay > 0 && json.Unmarshal([]byte(line), &kind) == nil && kind.Type == "result" {
			time.Sleep(resultDelay)
		}
		fmt.Fprintln(out, strings.ReplaceAll(line, "SESSION", session))
		if err := out.Flush(); err != nil {
			fail(2, "stdout: %v", err)
		}
	}

	if dir := os.Getenv("FAKECLAUDE_TRANSCRIPT"); dir != "" {
		writeTranscript(dir, session)
	}
	switch mode {
	case "stall": // no commit, no write, no CPU
		time.Sleep(600 * time.Second)
	case "hang":
		sleeper(row, false)
		time.Sleep(600 * time.Second)
	}
	if result != "" { // the account's failure: work in flight, never committed
		wip := filepath.Join("acts", row+"-wip.txt")
		if err := os.MkdirAll(filepath.Dir(wip), 0o755); err != nil {
			fail(2, "wip: %v", err)
		}
		if err := os.WriteFile(wip, []byte("wip\n"), 0o644); err != nil {
			fail(2, "wip: %v", err)
		}
		os.Exit(1)
	}
	acts, err := strconv.Atoi(perRow(os.Getenv("FAKECLAUDE_ACTS"), row, "0"))
	if err != nil {
		fail(2, "FAKECLAUDE_ACTS: %v", err)
	}
	for k := 1; k <= acts; k++ {
		act(filepath.Join("acts", fmt.Sprintf("%s-%d.txt", row, k)), fmt.Sprintf("act %d", k))
	}
	switch mode {
	case "red": // a file outside the packet's may-touch list
		act("outside.txt", "outside")
	case "next":
		act(filepath.Join("history", "sessions", "NEXT"), "next")
	case "orphan":
		sleeper(row, false)
	case "hold": // the sleeper keeps stdout open past the fake's exit
		sleeper(row, true)
	}
	if acts == 0 && files() {
		acts = -1 // committed: no empty commit
	}
	if acts == 0 && os.Getenv("FAKECLAUDE_NO_COMMIT") == "" {
		cmd := exec.Command("git", "commit", "--allow-empty", "-qm", "fake act")
		if msg, err := cmd.CombinedOutput(); err != nil {
			fail(3, "git commit: %v: %s", err, msg)
		}
	}
	os.Exit(code)
}

// perRow reads a knob: one value for every row, or `row=value,...`
// (def when the row is not named).
func perRow(v, row, def string) string {
	if v == "" {
		return def
	}
	if !strings.Contains(v, "=") {
		return v
	}
	for _, kv := range strings.Split(v, ",") {
		if k, val, ok := strings.Cut(kv, "="); ok && k == row {
			return val
		}
	}
	return def
}

// record writes data to <$name>/<row><ext> when $name names a directory.
func record(name, row, ext string, data []byte) {
	if dir := os.Getenv(name); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, row+ext), data, 0o600); err != nil {
			fail(2, "%s: %v", name, err)
		}
	}
}

// files merges the branches $FAKECLAUDE_FILES/<branch>.merge lists, then
// copies $FAKECLAUDE_FILES/<branch>/ over the tree and commits it; false
// when the knob is absent or neither file is there.
func files() bool {
	dir := os.Getenv("FAKECLAUDE_FILES")
	if dir == "" {
		return false
	}
	b, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		fail(3, "git rev-parse: %v", err)
	}
	branch, merged := strings.TrimSpace(string(b)), false
	if list, err := os.ReadFile(filepath.Join(dir, branch+".merge")); err == nil {
		for _, m := range strings.Fields(string(list)) {
			if msg, err := exec.Command("git", "merge", "-q", "--no-ff", "--no-edit", m).CombinedOutput(); err != nil {
				fail(3, "git merge %s: %v: %s", m, err, msg)
			}
			merged = true
		}
	}
	src := filepath.Join(dir, branch)
	if _, err := os.Stat(src); err != nil {
		return merged
	}
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return err
		}
		return os.WriteFile(rel, data, 0o644)
	})
	if err != nil {
		fail(2, "FAKECLAUDE_FILES: %v", err)
	}
	for _, argv := range [][]string{{"add", "-A"}, {"commit", "-qm", "fake files"}} {
		if msg, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
			fail(3, "git %s: %v: %s", argv[0], err, msg)
		}
	}
	return true
}

// act writes path and commits it.
func act(path, text string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(2, "act: %v", err)
	}
	if err := os.WriteFile(path, []byte(text+"\n"), 0o644); err != nil {
		fail(2, "act: %v", err)
	}
	for _, argv := range [][]string{{"add", path}, {"commit", "-qm", "fake " + path}} {
		if msg, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
			fail(3, "git %s: %v: %s", argv[0], err, msg)
		}
	}
}

// sleeper forks `sleep 300` in the fake's process group, its stdout the
// fake's when hold, else /dev/null, and records its pid.
func sleeper(row string, hold bool) {
	cmd := exec.Command("sleep", "300")
	if hold {
		cmd.Stdout = os.Stdout
	}
	if err := cmd.Start(); err != nil {
		fail(2, "sleeper: %v", err)
	}
	record("FAKECLAUDE_PIDFILE", row, ".sleeper", []byte(strconv.Itoa(cmd.Process.Pid)))
}
