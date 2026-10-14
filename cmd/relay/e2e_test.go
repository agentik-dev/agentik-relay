//go:build e2e

// The night end to end (relay-r1 §11, AR12): the built relay over a fixture
// repo and the fake claude, `make e2e` only. Every path a case opens lies
// under t.TempDir(); the binary's environment is built from scratch.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fx is one case's world: the fixture repo, the state root, the home, the
// relay binary's dir, the fakes' dir (claude, sysctl), the load file and
// the fake's record dir (prompts, pids, env dumps).
type fx struct {
	Root, State, Home, Bin, Fakes, Load, Rec string

	queue       strings.Builder
	mode, acts  []string
	env         []string // extra, per case
	words       string
	lastRunArgs []string
}

type rowOpts struct {
	Deps         []string
	Parked, Mode string
	Acts         int
	Wall         string
}

const e2eQueueHead = "version: 1\n" +
	"defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 2m,\n" +
	"           merge: merge-on-green, device: never, verify: auto}\n" +
	"words: %q\nrows:\n"

const e2ePacket = "# Packet %[1]s: an e2e row\n\n| item | value |\n|---|---|\n| from | `design/relay-r1.md` §11 |\n" +
	"| pin | branch `main` |\n| wall | this repo only |\n| may touch | `acts/*.txt`. Nothing else |\n" +
	"| acceptance | `test -f acts/%[1]s-1.txt && echo ok` reads `ok` |\n| budget | 10 turns, 20k context, $5 |\n\n" +
	"## 1. What the source says (quoted)\nQuoted.\n\n## 2. Rulings\nNone.\n\n## 3. Interfaces (signatures, not prose)\nNone.\n\n" +
	"## 4. Findings that name this row\nNone.\n\n## 5. Forbidden reads\nNone.\n\n## 6. Report (capped)\nA table.\n"

// The fake sysctl: the load from $FAKE_LOAD's file, else the real one.
const fakeSysctl = "#!/bin/sh\n" +
	"if [ \"$1 $2\" = \"-n vm.loadavg\" ] && [ -n \"$FAKE_LOAD\" ]; then cat \"$FAKE_LOAD\"; exit 0; fi\n" +
	"exec /usr/sbin/sysctl \"$@\"\n"

const lowLoad = "{ 0.5 0.5 0.5 }\n"

// The fake top (AR47), beside the fake sysctl: a faked load has a faked
// idle share, never this machine's. With $FAKE_LOAD set it prints two
// `CPU usage:` samples at once, as `top -l 2 -n 0 -s 1` does over a
// second: the first is one the relay must not read, the second's idle
// share is the percent in $FAKE_IDLE's file, else 5 (a load the CPU
// confirms); with no $FAKE_LOAD, the real one.
const e2eFakeTop = "#!/bin/sh\n" +
	"if [ -n \"$FAKE_LOAD\" ]; then\n" +
	"  idle=5.00\n" +
	"  if [ -n \"$FAKE_IDLE\" ] && [ -f \"$FAKE_IDLE\" ]; then idle=$(cat \"$FAKE_IDLE\"); fi\n" +
	"  echo 'CPU usage: 0.50% user, 0.50% sys, 99.00% idle'\n" +
	"  echo \"CPU usage: 50.00% user, 45.00% sys, ${idle}% idle\"\n" +
	"  exit 0\n" +
	"fi\n" +
	"exec /usr/bin/top \"$@\"\n"

// fixture builds the case's world: the relay and the fake claude built into
// it, a git repo on main with a Makefile whose check fails while
// history/sessions/NEXT exists, and no rows yet (row adds them).
func fixture(t *testing.T) *fx {
	t.Helper()
	tmp := t.TempDir()
	f := &fx{Root: filepath.Join(tmp, "repo"), State: filepath.Join(tmp, "state"), Home: filepath.Join(tmp, "home"),
		Bin: filepath.Join(tmp, "bin"), Fakes: filepath.Join(tmp, "fakes"), Load: filepath.Join(tmp, "load"),
		Rec: filepath.Join(tmp, "rec"), words: "e2e words " + filepath.Base(tmp)}
	for _, d := range []string{f.Root, f.State, filepath.Join(f.Home, ".config", "agentik", "relay"), f.Bin, f.Fakes, f.Rec} {
		must(t, os.MkdirAll(d, 0o755))
	}
	build(t, filepath.Join(f.Bin, "relay"), ".")
	build(t, filepath.Join(f.Fakes, "claude"), "../../internal/effects/testdata/fakeclaude")
	must(t, os.WriteFile(filepath.Join(f.Fakes, "sysctl"), []byte(fakeSysctl), 0o755))
	must(t, os.WriteFile(filepath.Join(f.Fakes, "top"), []byte(e2eFakeTop), 0o755))
	f.setLoad(t, lowLoad)
	must(t, os.WriteFile(filepath.Join(f.Home, ".gitconfig"), []byte("[user]\n\tname = E2E\n\temail = e2e@example.invalid\n"), 0o644))
	// The fake must be the relay's direct child: caffeinate turns a signal
	// into exit 137 (the charter's choice 1).
	must(t, os.WriteFile(filepath.Join(f.Home, ".config", "agentik", "relay", "config.yaml"), []byte("caffeinate: false\n"), 0o644))
	f.git(t, "init", "-q", "-b", "main")
	must(t, os.WriteFile(filepath.Join(f.Root, "Makefile"), []byte("check: ; @test ! -e history/sessions/NEXT\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(f.Root, ".gitignore"), []byte(".claude/\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(f.Root, "design", "packets"), 0o755))
	f.git(t, "add", ".")
	f.git(t, "commit", "-qm", "fixture")
	fmt.Fprintf(&f.queue, e2eQueueHead, f.words)
	return f
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func build(t *testing.T, out, pkg string) {
	t.Helper()
	// The pid file's home is the case's temp dir, set at link time (ADV11-9).
	flag := pidHomeFlag(filepath.Dir(filepath.Dir(out)))
	if msg, err := exec.Command("go", "build", "-ldflags", flag, "-o", out, pkg).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, msg)
	}
}

func (f *fx) setLoad(t *testing.T, load string) {
	t.Helper()
	tmp := f.Load + ".tmp"
	must(t, os.WriteFile(tmp, []byte(load), 0o644))
	must(t, os.Rename(tmp, f.Load))
}

// row adds a row to the queue with its packet; run commits them.
func (f *fx) row(id string, o rowOpts) {
	os.WriteFile(filepath.Join(f.Root, "design", "packets", id+".md"), []byte(fmt.Sprintf(e2ePacket, id)), 0o644)
	fmt.Fprintf(&f.queue, "  - id: %s\n    packet: design/packets/%s.md\n", id, id)
	if len(o.Deps) > 0 {
		fmt.Fprintf(&f.queue, "    depends_on: [%s]\n", strings.Join(o.Deps, ", "))
	}
	if o.Parked != "" {
		fmt.Fprintf(&f.queue, "    parked: %q\n", o.Parked)
	}
	if o.Wall != "" {
		fmt.Fprintf(&f.queue, "    budget: {wall: %s}\n", o.Wall)
	}
	if o.Mode != "" {
		f.mode = append(f.mode, id+"="+o.Mode)
	}
	if o.Acts > 0 {
		f.acts = append(f.acts, id+"="+strconv.Itoa(o.Acts))
	}
}

// environ is the binary's environment, from scratch: no RELAY_*, no CLAUDE*.
func (f *fx) environ() []string {
	env := []string{
		"PATH=" + f.Fakes + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + f.Home, "GIT_CONFIG_NOSYSTEM=1", "XDG_STATE_HOME=" + f.State, "TMPDIR=" + os.TempDir(),
		"FAKE_LOAD=" + f.Load, "FAKECLAUDE_FIXTURE=" + fakeFixture("ok.jsonl"),
		"FAKECLAUDE_MODE=" + strings.Join(f.mode, ","), "FAKECLAUDE_ACTS=" + strings.Join(f.acts, ","),
		"FAKECLAUDE_PROMPT=" + f.Rec, "FAKECLAUDE_PIDFILE=" + f.Rec, "FAKECLAUDE_ENVDUMP=" + f.Rec,
	}
	return append(env, f.env...)
}

func fakeFixture(name string) string {
	p, _ := filepath.Abs(filepath.Join("..", "..", "internal", "effects", "testdata", "fakeclaude", "fixtures", name))
	return p
}

func (f *fx) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = f.Root, f.environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes the queue and commits what the rows added.
func (f *fx) commit(t *testing.T) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Join(f.Root, "relay"), 0o755))
	must(t, os.WriteFile(filepath.Join(f.Root, "relay", "queue.yaml"), []byte(f.queue.String()), 0o644))
	if f.git(t, "status", "--porcelain") != "" {
		f.git(t, "add", ".")
		f.git(t, "commit", "-qm", "queue")
	}
}

// started is a relay run in flight; wait returns its exit and output.
type started struct {
	cmd  *exec.Cmd
	out  *strings.Builder
	done chan struct{}
}

func (s *started) wait() (int, string) {
	<-s.done
	return s.cmd.ProcessState.ExitCode(), s.out.String()
}

// start runs the relay binary in the fixture, after committing the queue.
func (f *fx) start(t *testing.T, args ...string) *started {
	t.Helper()
	f.commit(t)
	s := &started{out: &strings.Builder{}, done: make(chan struct{})}
	s.cmd = exec.Command(filepath.Join(f.Bin, "relay"), args...)
	s.cmd.Dir, s.cmd.Env = f.Root, f.environ()
	s.cmd.Stdout, s.cmd.Stderr = s.out, s.out
	must(t, s.cmd.Start())
	go func() { s.cmd.Wait(); close(s.done) }()
	t.Cleanup(func() {
		select {
		case <-s.done:
		default:
			s.cmd.Process.Kill()
			<-s.done
		}
	})
	return s
}

// run is start then wait, the whole run held to 5 minutes.
func (f *fx) run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	s := f.start(t, args...)
	select {
	case <-s.done:
	case <-time.After(5 * time.Minute):
		s.cmd.Process.Kill()
		<-s.done
		t.Fatalf("relay %v: still running after 5m\n%s", args, s.out.String())
	}
	code, out := s.wait()
	t.Logf("relay %s: exit %d\n%s", strings.Join(args, " "), code, out)
	return code, out
}

// ledger is every line of the state's ledger, the day files in order.
func (f *fx) ledger(t *testing.T) []map[string]any {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(f.State, "agentik-relay", "ledger", "*"))
	sort.Strings(files)
	var lines []map[string]any
	for _, p := range files {
		data, err := os.ReadFile(p)
		must(t, err)
		sc := bufio.NewScanner(strings.NewReader(string(data)))
		sc.Buffer(nil, 1<<22)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Fatalf("ledger %s: %v: %s", p, err, sc.Text())
			}
			lines = append(lines, m)
		}
	}
	return lines
}

// await polls the ledger for a line of event that pred holds of.
func (f *fx) await(t *testing.T, event string, pred func(map[string]any) bool, within time.Duration) map[string]any {
	t.Helper()
	for end := time.Now().Add(within); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
		for _, l := range f.ledger(t) {
			if l["event"] == event && (pred == nil || pred(l)) {
				return l
			}
		}
	}
	t.Fatalf("no %s line within %s", event, within)
	return nil
}

// awaitFile waits for path to exist and returns its contents.
func awaitFile(t *testing.T, path string, within time.Duration) string {
	t.Helper()
	for end := time.Now().Add(within); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return string(b)
		}
	}
	t.Fatalf("%s: absent after %s", path, within)
	return ""
}

func (f *fx) pid(t *testing.T, name string) int {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimSpace(awaitFile(t, filepath.Join(f.Rec, name), 30*time.Second)))
	must(t, err)
	return n
}

// firstParentMerges is main's first-parent merge commits, oldest first.
func firstParentMerges(t *testing.T, root string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-list", "--first-parent", "--merges", "--reverse", "main").Output()
	must(t, err)
	return strings.Fields(string(out))
}

func alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }

// awaitDead: a swept process may take a moment to be reaped.
func awaitDead(t *testing.T, pid int, within time.Duration) {
	t.Helper()
	for end := time.Now().Add(within); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if !alive(pid) {
			return
		}
	}
	t.Fatalf("pid %d still alive after %s", pid, within)
}

// lines picks the ledger's lines of event (and of row, when row is not "").
func lines(all []map[string]any, event, row string) []map[string]any {
	var out []map[string]any
	for _, l := range all {
		if l["event"] == event && (row == "" || l["row"] == row) {
			out = append(out, l)
		}
	}
	return out
}

// field walks a dotted path through nested objects.
func field(l map[string]any, path string) any {
	var v any = l
	for _, k := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// TestE2E_0_FixtureOnly: the wall (D7). Every path is outside the checkout,
// the child's env is §7's (of CLAUDE*, only the session budget), the fixture has no remote, the real queue is not
// touched and the prompt names the row's packet and the queue's words.
func TestE2E_0_FixtureOnly(t *testing.T) {
	realQueue := filepath.Join(checkout, "relay", "queue.yaml")
	before, err := os.Stat(realQueue)
	must(t, err)
	f := fixture(t)
	f.row("r1", rowOpts{Acts: 1})
	code, _ := f.run(t, "run", "--no-tmux", "--rows", "r1")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, p := range []string{f.Root, f.State, f.Home, f.Bin, f.Fakes, f.Load, f.Rec} {
		if err := outsideCheckout(p); err != nil {
			t.Fatal(err)
		}
	}
	env := awaitFile(t, filepath.Join(f.Rec, "r1.env"), time.Second)
	for _, want := range []string{"RELAY_ROW=r1", "RELAY_MEASURE_DEPTH=1"} {
		if !strings.Contains("\n"+env, "\n"+want+"\n") {
			t.Errorf("env dump lacks %s", want)
		}
	}
	// Of CLAUDE*, exactly the session budget (AR14 act 3): depth 1, the row's turns, and the row's role (AR41).
	var claude []string
	for _, kv := range strings.Split(env, "\n") {
		if strings.HasPrefix(kv, "CLAUDE") {
			claude = append(claude, kv)
		}
	}
	if got := strings.Join(claude, " "); got != "CLAUDE_SESSION_DEPTH=1 CLAUDE_SESSION_MAX_TURNS=100 CLAUDE_SESSION_PROFILE=builder CLAUDE_SESSION_MODEL=claude-opus-5-5" {
		t.Errorf("the env dump's CLAUDE* = %q", got)
	}
	if r := f.git(t, "remote"); r != "" {
		t.Errorf("the fixture has a remote: %s", r)
	}
	after, err := os.Stat(realQueue)
	must(t, err)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("the real queue's mtime moved: %s to %s", before.ModTime(), after.ModTime())
	}
	prompt := awaitFile(t, filepath.Join(f.Rec, "r1.prompt"), time.Second)
	for _, want := range []string{"# Packet r1: an e2e row", f.words} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}

// dump logs the ledger compactly, for a failing case.
func dump(t *testing.T, all []map[string]any) {
	t.Helper()
	for _, l := range all {
		b, _ := json.Marshal(l)
		t.Log(string(b))
	}
}

// TestE2E_1_FullNight: two rows landed in order, a parked one skipped, the
// ledger's whole shape, main's first-parent merges, report, status, locks.
func TestE2E_1_FullNight(t *testing.T) {
	f := fixture(t)
	f.row("r1", rowOpts{Acts: 2})
	f.row("r2", rowOpts{Deps: []string{"r1"}, Acts: 1})
	f.row("r3", rowOpts{Parked: "e2e", Acts: 1})
	code, _ := f.run(t, "run", "--no-tmux", "--rows", "r1,r2,r3")
	all := f.ledger(t)
	defer func() {
		if t.Failed() {
			dump(t, all)
		}
	}()
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// The shape: a plan line per unparked row, then the night's lines.
	var planned []string
	for _, l := range lines(all, "plan", "") {
		planned = append(planned, l["row"].(string))
	}
	if strings.Join(planned, ",") != "r1,r2" {
		t.Errorf("plan lines for %v, want r1,r2", planned)
	}
	want := "night-open stage spawn exit gate sitting merge stage spawn exit gate sitting merge stop night-close"
	if got := shape(all); got != want {
		t.Errorf("the ledger's shape:\n got %s\nwant %s", got, want)
	}
	if s := lines(all, "stop", ""); len(s) != 1 || s[0]["reason"] != "queue-empty" {
		t.Errorf("stop lines %v", s)
	}
	merges := lines(all, "merge", "")
	fp := firstParentMerges(t, f.Root)
	if len(fp) != 2 || len(merges) != 2 {
		t.Fatalf("first-parent merges %v, merge lines %d", fp, len(merges))
	}
	for i, row := range []string{"r1", "r2"} {
		m, st := merges[i], lines(all, "sitting", row)
		if m["row"] != row || m["result"] != "merged" || m["sha"] != fp[i] || len(st) != 1 {
			t.Errorf("merge %d: %v (main's %s)", i, m, fp[i])
			continue
		}
		sitting(t, st[0], "green")
		if tip := f.git(t, "rev-parse", fp[i]+"^2"); tip != st[0]["head_after"] {
			t.Errorf("%s: the merge's second parent %s, the row's tip %v", row, tip, st[0]["head_after"])
		}
	}
	if code, out := f.run(t, "report"); code != 0 || !strings.Contains(out, fp[0]) || !strings.Contains(out, fp[1]) {
		t.Errorf("report: exit %d, want both merge shas", code)
	}
	st := f.status(t)
	if st["state"] != "stopped:queue-empty" {
		t.Errorf("status %v", st["state"])
	}
	for _, r := range []string{"r1=green", "r2=green", "r3=skipped/parked"} {
		if !strings.Contains(rowStates(st), r) {
			t.Errorf("status rows %s, want %s", rowStates(st), r)
		}
	}
	f.noLocks(t)
}

// shape is the ledger's events in order, less plan and turn lines and the
// gate's per-check lines.
func shape(all []map[string]any) string {
	var out []string
	for _, l := range all {
		ev, _ := l["event"].(string)
		if ev == "plan" || ev == "turn" || ev == "breaker" || (ev == "gate" && l["check"] != nil) {
			continue
		}
		out = append(out, ev)
	}
	return strings.Join(out, " ")
}

// sitting checks a sitting line's state and the bounds every one carries:
// gate.verify, orphans and breaker.opens.
func sitting(t *testing.T, l map[string]any, state string) {
	t.Helper()
	if l["state"] != state || field(l, "gate.verify") == nil || l["orphans"] == nil || field(l, "breaker.opens") != 0.0 {
		b, _ := json.Marshal(l)
		t.Errorf("sitting %v: want %s with gate.verify, orphans, breaker.opens 0: %s", l["row"], state, b)
	}
}

// status is `relay status --json`.
func (f *fx) status(t *testing.T) map[string]any {
	t.Helper()
	code, out := f.run(t, "status", "--json")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); code != 0 || err != nil {
		t.Fatalf("status: exit %d, %v", code, err)
	}
	return m
}

// rowStates is status's rows as row=state[/reason] words.
func rowStates(st map[string]any) string {
	var out []string
	rows, _ := st["rows"].([]any)
	for _, r := range rows {
		m, _ := r.(map[string]any)
		w := fmt.Sprintf("%v=%v", m["id"], m["state"])
		if why, _ := m["reason"].(string); why != "" {
			w += "/" + why
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

// noLocks: no stage lock left under the state. The root lock's file
// (root-<sha1>, AR13 act 4) is a flock anchor that outlives its holder.
func (f *fx) noLocks(t *testing.T) {
	t.Helper()
	locks, _ := filepath.Glob(filepath.Join(f.State, "agentik-relay", "locks", "*"))
	var left []string
	for _, l := range locks {
		if !strings.HasPrefix(filepath.Base(l), "root-") {
			left = append(left, l)
		}
	}
	if len(left) > 0 {
		t.Errorf("locks left: %v", left)
	}
}

// TestE2E_2_Red: a file outside may-touch reds the gate's touch check; the
// row is red and resumable, its dependent skipped, the night stopped.
func TestE2E_2_Red(t *testing.T) {
	f := fixture(t)
	f.row("r1", rowOpts{Mode: "red", Acts: 1})
	f.row("r2", rowOpts{Deps: []string{"r1"}, Acts: 1})
	code, out := f.run(t, "run", "--no-tmux", "--rows", "r1,r2")
	all := f.ledger(t)
	defer func() {
		if t.Failed() {
			dump(t, all)
		}
	}()
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(out, "agentik relay resume r1") {
		t.Errorf("no `agentik relay resume r1` printed")
	}
	touch := false
	for _, g := range lines(all, "gate", "r1") {
		touch = touch || (g["check"] == "touch" && g["status"] == "red")
	}
	if !touch {
		t.Errorf("no red touch check")
	}
	st := lines(all, "sitting", "r1")
	if len(st) != 1 {
		t.Fatalf("%d r1 sitting lines", len(st))
	}
	sitting(t, st[0], "red")
	if stop, ok := st[0]["stop"]; ok && stop != "" {
		t.Errorf("the red sitting line carries stop: %v", st[0]["stop"])
	}
	if n := len(lines(all, "spawn", "r2")) + len(lines(all, "merge", "")); n != 0 {
		t.Errorf("r2 spawned or something merged (%d lines)", n)
	}
	s := f.status(t)
	if s["state"] != "stopped:gate-failed-dependency" || !strings.Contains(rowStates(s), "r1=red") || !strings.Contains(rowStates(s), "r2=skipped") {
		t.Errorf("status %v: %s", s["state"], rowStates(s))
	}
	if len(firstParentMerges(t, f.Root)) != 0 {
		t.Errorf("main has merges")
	}
}

// TestE2E_8_Next: a sitting that commits history/sessions/NEXT passes the
// touch check (the set admits it), but the gate's verify, run in the
// worktree, reads it red against main's green baseline: the row red, no
// merge line, the branch kept, main unchanged. The merge's own verify never
// runs (the charter's choice 3; its aborted merge is gate_test's).
func TestE2E_8_Next(t *testing.T) {
	f := fixture(t)
	f.row("r1", rowOpts{Mode: "next", Acts: 1})
	f.commit(t)
	main := f.git(t, "rev-parse", "main")
	code, out := f.run(t, "run", "--no-tmux", "--rows", "r1")
	all := f.ledger(t)
	defer func() {
		if t.Failed() {
			dump(t, all)
		}
	}()
	if code != 1 || !strings.Contains(out, "agentik relay resume r1") {
		t.Errorf("exit %d, want 1 and `agentik relay resume r1`", code)
	}
	st := lines(all, "sitting", "r1")
	if len(st) != 1 {
		t.Fatalf("%d sitting lines", len(st))
	}
	sitting(t, st[0], "red")
	checks := map[string]any{}
	for _, g := range lines(all, "gate", "r1") {
		if c, ok := g["check"].(string); ok {
			checks[c] = g["status"]
		}
	}
	if checks["touch"] != "green" || checks["acceptance"] != "green" || checks["verify"] != "red" {
		t.Errorf("gate checks %v: want touch and acceptance green, verify red", checks)
	}
	if v := field(st[0], "gate.verify.status"); v != "red" || field(st[0], "gate.verify.baseline_exit") != 0.0 {
		t.Errorf("gate.verify %v", field(st[0], "gate.verify"))
	}
	if m := lines(all, "merge", ""); len(m) != 0 {
		t.Errorf("merge lines %v", m)
	}
	if tip := f.git(t, "rev-parse", "r1"); tip != st[0]["head_after"] {
		t.Errorf("branch r1 at %s, the sitting's tip %v", tip, st[0]["head_after"])
	}
	if f.git(t, "cat-file", "-t", "r1:history/sessions/NEXT") != "blob" {
		t.Errorf("the branch lost NEXT")
	}
	if now := f.git(t, "rev-parse", "main"); now != main {
		t.Errorf("main moved: %s to %s", main, now)
	}
}

// one runs a single row to its end and returns the ledger and its sitting
// line; the ledger is dumped when the case fails.
func (f *fx) one(t *testing.T, o rowOpts, within time.Duration, args ...string) (int, []map[string]any, map[string]any) {
	t.Helper()
	f.row("r1", o)
	start := time.Now()
	code, _ := f.run(t, append([]string{"run", "--no-tmux", "--rows", "r1"}, args...)...)
	if d := time.Since(start); d > within {
		t.Errorf("the night took %s, want under %s", d.Round(time.Second), within)
	}
	all := f.ledger(t)
	t.Cleanup(func() {
		if t.Failed() {
			dump(t, all)
		}
	})
	st := lines(all, "sitting", "r1")
	if len(st) != 1 {
		t.Fatalf("%d sitting lines", len(st))
	}
	return code, all, st[0]
}

// TestE2E_3_Stall: no commit, no write, no CPU past --stall-seconds: the
// watchdog's next poll kills the sitting, red stalled, with the evidence.
func TestE2E_3_Stall(t *testing.T) {
	t.Parallel()
	f := fixture(t)
	_, _, st := f.one(t, rowOpts{Mode: "stall"}, 90*time.Second, "--stall-seconds", "1")
	if st["state"] != "red" || st["reason"] != "stalled" {
		t.Errorf("sitting %v/%v, want red/stalled", st["state"], st["reason"])
	}
	for _, k := range []string{"head", "mtime", "cpu_s", "at"} {
		if field(st, "stall."+k) == nil {
			t.Errorf("stall.%s absent: %v", k, st["stall"])
		}
	}
	awaitDead(t, f.pid(t, "r1.pid"), 5*time.Second)
}

// TestE2E_4_Orphan: a green exit that leaves a process in the group: the
// sweep counts and kills it, and the row is red.
func TestE2E_4_Orphan(t *testing.T) {
	t.Parallel()
	f := fixture(t)
	_, _, st := f.one(t, rowOpts{Mode: "orphan", Acts: 1}, time.Minute)
	if st["state"] != "red" || st["orphans"] != 1.0 {
		t.Errorf("sitting %v/%v orphans %v, want red, 1", st["state"], st["reason"], st["orphans"])
	}
	awaitDead(t, f.pid(t, "r1.sleeper"), 5*time.Second)
}

// TestE2E_5_Chaos: the child SIGKILLed from outside mid-sitting: exit 137
// and no result event, so stream-error; its sleeper swept; the night stopped.
func TestE2E_5_Chaos(t *testing.T) {
	t.Parallel()
	f := fixture(t)
	f.row("r1", rowOpts{Mode: "hang"})
	s := f.start(t, "run", "--no-tmux", "--rows", "r1")
	sleeper := f.pid(t, "r1.sleeper") // written after the pid, before the hang
	must(t, syscall.Kill(f.pid(t, "r1.pid"), syscall.SIGKILL))
	select {
	case <-s.done:
	case <-time.After(2 * time.Minute):
		t.Fatalf("the night outlived its killed child by 2m")
	}
	code, out := s.wait()
	t.Logf("exit %d\n%s", code, out)
	all := f.ledger(t)
	defer func() {
		if t.Failed() {
			dump(t, all)
		}
	}()
	ex, st := lines(all, "exit", "r1"), lines(all, "sitting", "r1")
	if len(ex) != 1 || len(st) != 1 {
		t.Fatalf("%d exit, %d sitting lines", len(ex), len(st))
	}
	// A signalled child is 128+sig with no error field (effects' Wait), not
	// the packet's `error: signal:...` (the charter's choice 4); no result.
	if ex[0]["child_exit"] != float64(128+int(syscall.SIGKILL)) || ex[0]["subtype"] != nil {
		t.Errorf("exit line child_exit %v subtype %v, want 137 and no result", ex[0]["child_exit"], ex[0]["subtype"])
	}
	if st[0]["state"] != "failed" || st[0]["reason"] != "stream-error" || st[0]["orphans"] != 1.0 {
		t.Errorf("sitting %v/%v orphans %v, want failed/stream-error, 1", st[0]["state"], st[0]["reason"], st[0]["orphans"])
	}
	if s := f.status(t); s["state"] != "stopped:stream-error" {
		t.Errorf("status %v", s["state"])
	}
	awaitDead(t, sleeper, 5*time.Second)
}

// TestE2E_6_Hold (ar10's gap 2, ADV6-7): the fake exits green but its
// sleeper holds stdout open, so no EOF: the Wait returns at the exit, the
// sweep kills the sleeper (the stream then ends; one held past PipeGrace
// would be closed from the read end, failed orphans) and the row is red
// with `orphans: 1` well inside 30 s, never waiting on the wall.
func TestE2E_6_Hold(t *testing.T) {
	t.Parallel()
	f := fixture(t)
	_, _, st := f.one(t, rowOpts{Mode: "hold", Acts: 1, Wall: "90s"}, 30*time.Second)
	if st["orphans"] != 1.0 || (st["state"] != "red" && st["reason"] != "orphans") {
		t.Errorf("sitting %v/%v orphans %v, want red (or failed orphans), orphans 1", st["state"], st["reason"], st["orphans"])
	}
	awaitDead(t, f.pid(t, "r1.pid"), 5*time.Second)
	awaitDead(t, f.pid(t, "r1.sleeper"), 5*time.Second)
}

// TestE2E_7_Soak: five independent rows at --parallel 1 under a load of 99
// that falls to 0.5 once the breaker opens: the plan waits one poll, every
// plan line carries the open, and then five sittings land in order.
func TestE2E_7_Soak(t *testing.T) {
	t.Parallel()
	f := fixture(t)
	f.setLoad(t, "{ 99.0 1.0 1.0 }\n")
	rows := []string{"s1", "s2", "s3", "s4", "s5"}
	for _, r := range rows {
		f.row(r, rowOpts{Acts: 1})
	}
	s := f.start(t, "run", "--no-tmux", "--parallel", "1", "--rows", strings.Join(rows, ","))
	f.await(t, "breaker", func(l map[string]any) bool { return l["state"] == "open" }, time.Minute)
	f.setLoad(t, lowLoad)
	select {
	case <-s.done:
	case <-time.After(4 * time.Minute):
		t.Fatalf("the soak still running after 4m")
	}
	code, out := s.wait()
	t.Logf("exit %d\n%s", code, out)
	all := f.ledger(t)
	defer func() {
		if t.Failed() {
			dump(t, all)
		}
	}()
	if code != 0 {
		t.Errorf("exit %d", code)
	}
	plans := lines(all, "plan", "")
	if len(plans) != len(rows) {
		t.Errorf("%d plan lines", len(plans))
	}
	for _, p := range plans {
		if field(p, "breaker.opens") != 1.0 || field(p, "breaker.load") != 99.0 {
			t.Errorf("plan %v: breaker %v, want opens 1, load 99", p["row"], p["breaker"])
		}
		if w, _ := field(p, "breaker.wait_s").(float64); w < 30 {
			t.Errorf("plan %v: wait_s %v, want 30 or more", p["row"], w)
		}
	}
	t.Logf("the breaker waited %v s", field(plans[0], "breaker.wait_s"))
	merges, fp := lines(all, "merge", ""), firstParentMerges(t, f.Root)
	if len(merges) != len(rows) || len(fp) != len(rows) {
		t.Fatalf("%d merge lines, %d merges on main", len(merges), len(fp))
	}
	for i, r := range rows {
		st := lines(all, "sitting", r)
		if len(st) != 1 || merges[i]["row"] != r || merges[i]["sha"] != fp[i] {
			t.Errorf("%s: %d sitting lines, merge %v (main's %s)", r, len(st), merges[i], fp[i])
			continue
		}
		sitting(t, st[0], "green")
		if st[0]["orphans"] != 0.0 {
			t.Errorf("%s: orphans %v", r, st[0]["orphans"])
		}
		if _, err := os.Stat(filepath.Join(f.Rec, r+".env")); err != nil {
			t.Errorf("%s: no env dump", r)
		}
	}
	if sj, _ := filepath.Glob(filepath.Join(f.State, "agentik-relay", "*", "*", "status.json")); len(sj) == 0 {
		t.Errorf("no status.json under the state")
	}
	f.noLocks(t)
}

// TestE2E_10_UsageLimit (AR17D act 0, run 10 ruling 1): through the built
// fake, r2's result names a usage limit while r1 (spawned first, stalled)
// runs beside it: the night stops usage-limit, r1 is killed and reads failed
// usage-limit, neither row's gate runs, relay run exits 1, and r2's
// uncommitted file stays in its worktree.
func TestE2E_10_UsageLimit(t *testing.T) {
	f := fixture(t)
	f.row("r1", rowOpts{Mode: "stall"})
	f.row("r2", rowOpts{})
	f.env = append(f.env, "FAKECLAUDE_RESULT=r2=You've hit your weekly limit · resets Sep 28 at 4pm (America/Los_Angeles)")
	start := time.Now()
	code, _ := f.run(t, "run", "--no-tmux", "--parallel", "2", "--rows", "r1,r2")
	all := f.ledger(t)
	defer func() {
		if t.Failed() {
			dump(t, all)
		}
	}()
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if d := time.Since(start); d > time.Minute {
		t.Errorf("the night took %s: r1 was not killed", d.Round(time.Second))
	}
	st := f.status(t)
	if st["state"] != "stopped:usage-limit" || !strings.Contains(rowStates(st), "r1=failed") || !strings.Contains(rowStates(st), "r2=failed") {
		t.Errorf("status %v: %s", st["state"], rowStates(st))
	}
	for _, row := range []string{"r1", "r2"} {
		if s := lines(all, "sitting", row); len(s) != 1 || s[0]["reason"] != "usage-limit" {
			t.Errorf("%s sitting lines %v", row, s)
		}
		if g := lines(all, "gate", row); len(g) != 0 {
			t.Errorf("%s: the gate ran", row)
		}
	}
	if b, err := os.ReadFile(filepath.Join(f.Root, ".claude", "worktrees", "r2", "acts", "r2-wip.txt")); err != nil || string(b) != "wip\n" {
		t.Errorf("r2's uncommitted file: %q, %v", b, err)
	}
	if k := lines(all, "kill", "r1"); len(k) != 1 || k[0]["reason"] != "usage-limit" {
		t.Errorf("r1 kill lines %v", k)
	}
	awaitDead(t, f.pid(t, "r1.pid"), 5*time.Second)
}

// TestE2E_11_Unconfirmed (AR47; breaker-signal-r1 §2): a load of 99 with
// the CPU 70% idle (the fake top's second sample, from $FAKE_IDLE) opens
// nothing. The plan and the spawn wait nothing, every `breaker` line is
// `state: unconfirmed` with the idle share, the plan and sitting lines
// count them, and the row lands green.
func TestE2E_11_Unconfirmed(t *testing.T) {
	t.Parallel()
	f := fixture(t)
	f.setLoad(t, "{ 99.0 1.0 1.0 }\n")
	idle := filepath.Join(filepath.Dir(f.Load), "idle")
	must(t, os.WriteFile(idle, []byte("70.00\n"), 0o644))
	f.env = append(f.env, "FAKE_IDLE="+idle)
	code, all, st := f.one(t, rowOpts{Acts: 1}, 2*time.Minute)
	if code != 0 {
		t.Errorf("exit %d", code)
	}
	sitting(t, st, "green")
	breakers, spawned := lines(all, "breaker", ""), 0
	for _, l := range breakers {
		if l["state"] != "unconfirmed" || l["load"] != 99.0 || l["idle"] != 0.7 {
			t.Errorf("breaker line %v, want unconfirmed, load 99, idle 0.7", l)
		}
		if l["row"] == "r1" {
			spawned++
		}
	}
	if len(breakers) == 0 || spawned != 1 {
		t.Errorf("%d breaker lines, %d of the spawn's; want some, and one", len(breakers), spawned)
	}
	plans := lines(all, "plan", "r1")
	if len(plans) != 1 {
		t.Fatalf("%d plan lines", len(plans))
	}
	if n, _ := field(plans[0], "breaker.unconfirmed").(float64); n < 1 || field(plans[0], "breaker.opens") != 0.0 || field(plans[0], "breaker.wait_s") != 0.0 || field(plans[0], "breaker.load") != 99.0 {
		t.Errorf("plan breaker %v, want opens 0, wait_s 0, load 99, unconfirmed 1 or more", plans[0]["breaker"])
	}
	if field(st, "breaker.unconfirmed") != 1.0 || field(st, "breaker.opens") != 0.0 {
		t.Errorf("sitting breaker %v, want opens 0, unconfirmed 1", st["breaker"])
	}
}
