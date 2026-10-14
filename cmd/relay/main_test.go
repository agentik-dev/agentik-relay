package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

func TestVerbs(t *testing.T) {
	table := []struct {
		args     []string
		code     int
		out, err string
	}{
		{[]string{"version"}, exitOK, "relay 0.1.0-dev\n", ""},
		{[]string{"gate"}, exitUsage, "", "usage: relay gate <row> [--json]\n"},
		{[]string{"clean"}, exitUsage, "", "usage: relay clean --merged|--stale [--json]\n"},
		{[]string{"hook", "Stop"}, exitOK, "{}\n", ""},
		{[]string{"hook"}, exitOK, "{}\n", ""},
		{[]string{}, exitUsage, "", "usage: relay <verb> [--json] [args]\n"},
		{[]string{"bogus"}, exitUsage, "", "relay: unknown verb \"bogus\"\n"},
	}
	home := t.TempDir()
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = os.Getenv })
	measuring(t) // resume refuses under a measurement (AR17B)
	table = append(table, []struct {
		args     []string
		code     int
		out, err string
	}{
		{[]string{"attach", "nosuchrow"}, exitUsage, "", "relay attach: no night\n"},
		{[]string{"stop"}, exitUsage, "", "relay stop: no night under "},
		{[]string{"kill", "a8"}, exitUsage, "", "relay kill: no night under "},
		{[]string{"resume", "nosuchrow"}, exitUsage, "", "relay resume: row nosuchrow: no sitting in the ledger\n"},
		{[]string{"resume", "--json"}, exitOK, `{"night":"","resume":[]}` + "\n", ""},
	}...)
	for _, r := range table {
		t.Run(strings.Join(r.args, " "), func(t *testing.T) {
			var out, errb bytes.Buffer
			code := run(r.args, strings.NewReader(""), &out, &errb)
			if code != r.code || out.String() != r.out || !strings.HasPrefix(errb.String(), r.err) {
				t.Fatalf("run(%q) = %d, %q, %q; want %d, %q, %q", r.args, code, out.String(), errb.String(), r.code, r.out, r.err)
			}
		})
	}
}

// TestDoctorTable runs doctor over a fake lookPath, a fake `claude
// --max-turns 1 --version` probe and fake effects: all present, the probe
// refused, tmux absent (a warning), claude absent, and a dirty root.
func TestDoctorTable(t *testing.T) {
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	defer func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT }()
	refused := errors.New("exit status 1")
	names := "claude git git-worktree caffeinate tmux ntfy state queue root breaker template config"
	table := []struct {
		name, missing string
		probe         error
		dirty         bool
		code          int
		oks, warns    string // one letter per check: y or n
	}{
		{"all present", "", nil, false, exitOK, "yyyyyyyyyyyy", "nnnnnnnnnnn"},
		{"probe refused", "", refused, false, exitUnwell, "nyyyyyyyyyyy", "nnnnnnnnnnn"},
		{"tmux absent", "tmux", nil, false, exitOK, "yyyyyyyyyyyy", "nnnnynnnnnn"},
		{"claude absent", "claude", nil, false, exitUnwell, "nyyyyyyyyyyy", "nnnnnnnnnnn"},
		{"root dirty", "", nil, true, exitUnwell, "yyyyyyyynyyy", "nnnnnnnnnnn"},
	}
	for _, r := range table {
		t.Run(r.name, func(t *testing.T) {
			planFakes(t, "main")
			if r.dirty {
				old := effectsFor
				effectsFor = func(state, night string) effects.Effects {
					fx := old(state, night)
					top, _ := toplevel()
					fx.Git.(*effects.FakeGit).Statuses = map[string][]string{top: {" M README.md"}}
					return fx
				}
			}
			lookPath = func(name string) (string, error) {
				if name == r.missing {
					return "", errors.New("absent")
				}
				return "/fake/" + name, nil
			}
			claudeProbe = func(string) error { return r.probe }
			gitWorktree = func(string) error { return nil }
			var out, errb bytes.Buffer
			code := run([]string{"doctor", "--json"}, strings.NewReader(""), &out, &errb)
			var rep struct{ Checks []check }
			if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
				t.Fatal(err)
			}
			var got, oks, warns []string
			yn := map[bool]string{true: "y", false: "n"}
			for _, c := range rep.Checks {
				got = append(got, strings.ReplaceAll(c.Name, " ", "-"))
				oks = append(oks, yn[c.OK])
				if c.Name != "breaker" { // AR29 act 4: warns only on a host with neither /proc nor a pinned lsof
					warns = append(warns, yn[c.Warn])
				}
			}
			if code != r.code || strings.Join(got, " ") != names || strings.Join(oks, "") != r.oks || strings.Join(warns, "") != r.warns {
				t.Fatalf("doctor = %d, %s", code, out.String())
			}
			if r.probe != nil && !strings.Contains(rep.Checks[0].Detail, "--max-turns 1 --version: exit status 1") {
				t.Fatalf("the claude check does not name the probe: %s", rep.Checks[0].Detail)
			}
		})
	}
}

// TestAR4_DoctorProbesMaxTurns: doctor runs the claude on PATH with exactly
// `--max-turns 1 --version` and passes on exit 0, naming the probe.
func TestAR4_DoctorProbesMaxTurns(t *testing.T) {
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	gitWorktree = func(string) error { return nil }
	var probed []string
	claudeProbe = func(p string) error { probed = append(probed, p); return nil }
	var out, errb bytes.Buffer
	run([]string{"doctor", "--json"}, strings.NewReader(""), &out, &errb)
	var rep struct{ Checks []check }
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(probed) != 1 || probed[0] != "/fake/claude" || !rep.Checks[0].OK ||
		rep.Checks[0].Detail != "/fake/claude; accepts --max-turns 1 --version" {
		t.Fatalf("probed %v; claude check %+v", probed, rep.Checks[0])
	}
}

// TestExitCodes pins §9's numbers, so a constant cannot drift under the
// tables that use it.
func TestExitCodes(t *testing.T) {
	for _, r := range []struct {
		name      string
		got, want int
	}{{"ok", exitOK, 0}, {"red", exitRed, 1}, {"usage", exitUsage, 2}, {"unwell", exitUnwell, 2}, {"not built", exitNotBuilt, 3}} {
		if r.got != r.want {
			t.Errorf("exit %s = %d; want %d", r.name, r.got, r.want)
		}
	}
}

// planFakes points plan at fixture queues: the repo root a temp tree whose
// internal/packet/testdata links the packet corpus, so their packet paths
// resolve without git or the checkout; the effects faked (the root top
// itself, on another branch, so no baseline runs), the state and config under a temp dir, and
// RELAY_MEASURE_DEPTH set (measuring clears it).
func planFakes(t *testing.T, branch string) (*effects.FakeShell, *effects.FakeLedger) {
	oldTop, oldEnv, oldFx := toplevel, getenv, effectsFor
	t.Cleanup(func() { toplevel, getenv, effectsFor = oldTop, oldEnv, oldFx })
	home := t.TempDir()
	top := filepath.Join(home, "top") // under home: printed as ~/top
	corpus, err := filepath.Abs(filepath.Join("..", "..", "internal", "packet", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(top, "internal", "packet"), 0o755)
	if err := os.Symlink(corpus, filepath.Join(top, "internal", "packet", "testdata")); err != nil {
		t.Fatal(err)
	}
	fakeTop(t, top)
	getenv = fakeEnv(home)
	clock := effects.FakeClock{T: time.Date(2026, 9, 22, 22, 0, 0, 0, time.UTC)}
	sh := &effects.FakeShell{}
	// a2-relay-design landed in git: AR22 act 3 (ADV13-7) took away the
	// ledger green line that satisfied it; the line stays, a claim only.
	l := &effects.FakeLedger{Clock: clock, Prior: []map[string]any{{"row": "a2-relay-design", "result": "green"}}}
	effectsFor = func(state, night string) effects.Effects {
		l.Night = night
		return effects.Effects{Git: &effects.FakeGit{Roots: map[string]string{top: top},
			Branches: map[string]string{top: branch}, Existing: map[string]bool{"ar1-relay-hooks": true, "a2-relay-design": true, "main": true},
			Ancestors: map[string]bool{"ar1-relay-hooks..main": true},
			Merged:    map[string]bool{"ar1-relay-hooks..main": true, "a2-relay-design..main": true}},
			Shell: sh, Clock: clock, FS: &effects.FakeFS{}, Ledger: l, Getenv: lateEnv}
	}
	return sh, l
}

// TestPlan runs plan over three fixture queues.
func TestPlan(t *testing.T) {
	const dir = "../../internal/queue/testdata/"
	table := []struct {
		queue    string
		code     int
		valid    bool
		states   string
		problems int
	}{
		{"valid.yaml", exitOK, true, "ready pending pending pending pending pending", 0},
		{"cycle.yaml", exitUsage, false, "invalid invalid invalid ready", 3},
		{"missing-packet.yaml", exitUsage, false, "ready invalid", 1},
	}
	for _, r := range table {
		t.Run(r.queue, func(t *testing.T) {
			sh, l := planFakes(t, "feature")
			var out, errb bytes.Buffer
			if code := run([]string{"plan", "--queue", dir + r.queue, "--json"}, strings.NewReader(""), &out, &errb); code != r.code {
				t.Fatalf("plan --json = %d; want %d; %s", code, r.code, errb.String())
			}
			var p night.Plan
			if err := json.Unmarshal(out.Bytes(), &p); err != nil {
				t.Fatalf("json: %v: %s", err, out.String())
			}
			var states []string
			for _, row := range p.Rows {
				states = append(states, row.State)
			}
			if p.Valid != r.valid || strings.Join(states, " ") != r.states || len(p.Problems) != r.problems ||
				len(p.TemplateHash) != 64 || p.Night != "2026-09-22-1" || len(sh.Calls()) != 0 || len(l.Written()) != len(p.Rows) {
				t.Errorf("plan: valid %v, states %q, problems %v, night %s, shell %d, ledger %d",
					p.Valid, states, p.Problems, p.Night, len(sh.Calls()), len(l.Written()))
			}
			out.Reset()
			if code := run([]string{"plan", "--queue=" + dir + r.queue}, strings.NewReader(""), &out, &errb); code != r.code {
				t.Fatalf("plan = %d; want %d", code, r.code)
			}
			if n := strings.Count(out.String(), "spawn: "); n != len(p.Rows) {
				t.Errorf("plan printed %d spawn lines, want %d:\n%s", n, len(p.Rows), out.String())
			}
		})
	}
	sh, _ := planFakes(t, "main")
	var out, errb bytes.Buffer
	if code := run([]string{"plan", "--queue", dir + "valid.yaml", "--rows", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb); code != exitOK {
		t.Fatalf("nested plan --rows = %d: %s", code, errb.String())
	}
	var nested night.Plan
	json.Unmarshal(out.Bytes(), &nested)
	if !nested.Nested || len(sh.Calls()) != 0 || len(nested.Rows) != 1 || nested.Rows[0].Baseline[0].MainLast != effects.NestedReason {
		t.Errorf("nested plan --rows a3-machine measured (%d calls): %s", len(sh.Calls()), out.String())
	}
	out.Reset()
	measuring(t)
	if code := run([]string{"plan", "--queue", dir + "valid.yaml", "--rows", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb); code != exitOK {
		t.Fatalf("plan --rows = %d: %s", code, errb.String())
	}
	var p night.Plan
	json.Unmarshal(out.Bytes(), &p)
	if len(p.Rows) != 1 || len(p.Rows[0].Baseline) != 8 || p.Rows[0].Baseline[0].Status == relay.CheckUnrun {
		t.Errorf("plan --rows a3-machine = %s", out.String())
	}
	out.Reset()
	if code := run([]string{"plan", "--queue", dir + "absent.yaml", "--json"}, strings.NewReader(""), &out, &errb); code != exitUsage ||
		!strings.HasPrefix(out.String(), `{"valid":false,`) {
		t.Errorf("unreadable queue = %d, %s", code, out.String())
	}
	if code := run([]string{"plan", "--bogus"}, strings.NewReader(""), &out, &errb); code != exitUsage {
		t.Errorf("plan --bogus = %d; want %d", code, exitUsage)
	}
}

// gateFakes points gate at a fixture queue in a temp tree (relay/queue.yaml
// holding a3-machine off ar1-relay-hooks, its packet beside it; the fake
// root's file system holds a3's too), with git faked: a3-machine's branch
// and worktree exist when branch is set, checked out at its tip. The root
// is top itself (Git.Root: the gate reads the queue there). The gate
// measures (the depth cleared), over the fake shell.
func gateFakes(t *testing.T, branch bool) (*effects.FakeShell, *effects.FakeLedger) {
	t.Helper()
	oldTop, oldEnv, oldFx := toplevel, getenv, effectsFor
	t.Cleanup(func() { toplevel, getenv, effectsFor = oldTop, oldEnv, oldFx })
	home := t.TempDir()
	getenv = fakeEnv(home)
	measuring(t)
	src, err := os.ReadFile("../../internal/packet/testdata/a3-machine.md")
	if err != nil {
		t.Fatal(err)
	}
	top := t.TempDir()
	os.MkdirAll(filepath.Join(top, "relay"), 0o755)
	os.MkdirAll(filepath.Join(top, "design", "packets"), 0o755)
	os.WriteFile(filepath.Join(top, "design", "packets", "a3-machine.md"), src, 0o644)
	os.WriteFile(filepath.Join(top, "relay", "queue.yaml"), []byte(`version: 1
defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,
           merge: branch-only, device: never, verify: auto}
words: ""
rows:
  - id: a3-machine
    packet: design/packets/a3-machine.md
    depends_on: [a2-relay-design, ar1-relay-hooks]
    base: ar1-relay-hooks
`), 0o644)
	fakeTop(t, top)
	clock := effects.FakeClock{T: time.Date(2026, 9, 22, 22, 0, 0, 0, time.UTC)}
	sh := &effects.FakeShell{Results: map[string]effects.ShellResult{}}
	l := &effects.FakeLedger{Clock: clock}
	g := &effects.FakeGit{Roots: map[string]string{top: top, "/wt": top},
		Branches: map[string]string{"/wt": "a3-machine"}, Heads: map[string]string{"/wt": "a3sha"},
		Tips:  map[string]string{"a3-machine": "a3sha", "main": "m0", "ar1-relay-hooks": "r1sha"},
		Diffs: map[string][]string{"r1sha...a3sha": {"internal/relay/gate.go", "README.md"}},
		// Both dependencies merged into main: the hand gate's invariants read them (§15).
		Ancestors: map[string]bool{"a2-relay-design..main": true, "ar1-relay-hooks..main": true}, Merged: map[string]bool{"a2-relay-design..main": true, "ar1-relay-hooks..main": true}}
	if branch {
		g.Existing = map[string]bool{"a3-machine": true}
		g.WTs = []effects.Worktree{{Path: top}, {Path: "/wt", Branch: "a3-machine"}}
	}
	fs := &effects.FakeFS{Files: map[string][]byte{filepath.Join(top, "design", "packets", "a3-machine.md"): src}}
	effectsFor = func(state, night string) effects.Effects {
		return effects.Effects{Git: g, Shell: sh, Clock: clock, FS: fs, Ledger: l, Getenv: lateEnv}
	}
	return sh, l
}

func TestGateVerb(t *testing.T) {
	var out, errb bytes.Buffer
	gateFakes(t, true)
	if code := run([]string{"gate", "nosuchrow", "--json"}, strings.NewReader(""), &out, &errb); code != exitUsage ||
		!strings.Contains(out.String(), `"result":"unrun"`) {
		t.Fatalf("gate nosuchrow = %d, %s", code, out.String())
	}
	out.Reset()
	gateFakes(t, false)
	if code := run([]string{"gate", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb); code != exitUsage ||
		!strings.Contains(out.String(), "no branch") {
		t.Fatalf("gate with no branch = %d, %s", code, out.String())
	}
	out.Reset()
	sh, l := gateFakes(t, true)
	code := run([]string{"gate", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb)
	var rep struct {
		Result  string
		Outside []string
		Lines   []relay.Check
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if code != exitRed || rep.Result != "red" || !reflect.DeepEqual(rep.Outside, []string{"README.md"}) || len(rep.Lines) != 8 {
		t.Fatalf("gate a3-machine = %d, %s", code, out.String())
	}
	top, _ := toplevel()
	for _, c := range sh.Calls() {
		// verify: auto is probed on main, once (ADV6-10); every line runs in
		// a fresh tree under the state root's gate/ (AR17A act 1).
		if !strings.Contains(c, "/agentik-relay/gate/fresh-") && c != top+": make -n check >/dev/null 2>&1" {
			t.Fatalf("an acceptance line ran outside the row's worktree: %s", c)
		}
	}
	if w := l.Written(); len(w) == 0 || w[len(w)-1]["result"] != "red" {
		t.Fatalf("no red result line: %v", w)
	}
}

func TestClean(t *testing.T) {
	oldTop, oldEnv, oldFx := toplevel, getenv, effectsFor
	t.Cleanup(func() { toplevel, getenv, effectsFor = oldTop, oldEnv, oldFx })
	fakeTop(t, "/root")
	getenv = func(k string) string { return "" }
	wt := "/root/.claude/worktrees/"
	g := &effects.FakeGit{Roots: map[string]string{"/root": "/root"},
		WTs: []effects.Worktree{{Path: "/root", Branch: "main"}, {Path: wt + "a", Branch: "a"},
			{Path: wt + "b", Branch: "b"}, {Path: wt + "gate-c"}, {Path: "/elsewhere/d", Branch: "d"}},
		Ancestors: map[string]bool{"a..main": true, "d..main": true, "main..main": true}, Merged: map[string]bool{"a..main": true, "d..main": true, "main..main": true}}
	effectsFor = func(state, night string) effects.Effects { return effects.Effects{Git: g} }
	var out, errb bytes.Buffer
	if code := run([]string{"clean", "--merged", "--json"}, strings.NewReader(""), &out, &errb); code != exitOK ||
		strings.TrimSpace(out.String()) != `{"removed":["/root/.claude/worktrees/a"],"kept":[]}` {
		t.Fatalf("clean --merged = %d, %s %s", code, out.String(), errb.String())
	}
	for _, c := range g.Calls() {
		if strings.HasPrefix(c, "WorktreeRemove") && c != "WorktreeRemove /root "+wt+"a" {
			t.Fatalf("clean removed more than the merged worktree: %s", c)
		}
	}
	if code := run([]string{"clean", "--all"}, strings.NewReader(""), &out, &errb); code != exitUsage {
		t.Fatalf("clean --all = %d", code)
	}
}

// TestRunVerb: run takes exactly one row until A8; --dry stages and spawns
// nothing and prints the plan's spawn line with the prompt's hashes.
func TestRunVerb(t *testing.T) {
	// The root is the temp repo itself (plan reads its queue there), so a
	// short temp base keeps the spawn line's worktree path under 100 columns.
	t.Setenv("TMPDIR", "/tmp")
	planFakes(t, "feature")
	measuring(t) // the run guard (AR11) refuses under the depth
	oldTmux := newTmux
	newTmux = func(string) effects.Tmux { return &effects.FakeTmux{} } // never a real session
	t.Cleanup(func() { newTmux = oldTmux })
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "relay"), 0o755)
	os.MkdirAll(filepath.Join(repo, "design", "packets"), 0o755)
	src, err := os.ReadFile("../../internal/packet/testdata/a3-machine.md")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "design", "packets", "a3-machine.md"), src, 0o644)
	os.WriteFile(filepath.Join(repo, "relay", "queue.yaml"), []byte(`version: 1
defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,
           merge: branch-only, device: never, verify: auto}
words: ""
rows:
  - id: a3-machine
    packet: design/packets/a3-machine.md
`), 0o644)
	fakeTop(t, repo)
	faked := effectsFor
	effectsFor = func(state, night string) effects.Effects {
		fx := faked(state, night)
		g := fx.Git.(*effects.FakeGit)
		g.Roots[repo], g.Branches[repo] = repo, "feature"
		return fx
	}
	for _, r := range []struct {
		args []string
		err  string
	}{
		{[]string{"run", "--rows", "a3-machine", "--bogus"}, "relay run: unknown argument"},
		{[]string{"run", "--parallel", "two"}, "relay run: --parallel \"two\": want a whole number"},
		{[]string{"run", "--max-usd", "lots"}, "relay run: --max-usd \"lots\": want dollars"},
		{[]string{"run", "--rows", "a3-machine,a4-schemas", "--dry"}, "relay run: the plan is not valid"},
	} {
		var out, errb bytes.Buffer
		if code := run(r.args, strings.NewReader(""), &out, &errb); code != exitUsage || !strings.HasPrefix(errb.String(), r.err) {
			t.Fatalf("run(%q) = %d, %q", r.args, code, errb.String())
		}
	}
	var out, errb bytes.Buffer
	if code := run([]string{"run", "--rows", "nosuchrow", "--dry"}, strings.NewReader(""), &out, &errb); code != exitUsage {
		t.Fatalf("run nosuchrow = %d, %q", code, errb.String())
	}
	out.Reset()
	if code := run([]string{"run", "--rows=a3-machine", "--dry", "--json"}, strings.NewReader(""), &out, &errb); code != exitOK {
		t.Fatalf("run --dry = %d, %s %s", code, out.String(), errb.String())
	}
	var ro runOutput
	if err := json.Unmarshal(out.Bytes(), &ro); err != nil || len(ro.Rows) != 1 {
		t.Fatalf("%v %s", err, out.String())
	}
	rep := ro.Rows[0]
	if len(rep.PromptHash) != 64 || len(rep.RulesHash) != 64 || rep.State != "" || rep.Spawn[len(rep.Spawn)-1] != rep.SessionID {
		t.Fatalf("dry report %+v", rep)
	}
	out.Reset()
	run([]string{"plan", "--rows", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb)
	var p night.Plan
	if err := json.Unmarshal(out.Bytes(), &p); err != nil || len(p.Rows) != 1 {
		t.Fatalf("plan: %v %s", err, out.String())
	}
	planned, dry := p.Rows[0].Spawn, rep.Spawn
	if night.Quote(planned[:len(planned)-1]) != night.Quote(dry[:len(dry)-1]) {
		t.Fatalf("plan's spawn %q, run's %q", planned, dry)
	}
	out.Reset()
	run([]string{"run", "--rows", "a3-machine", "--dry"}, strings.NewReader(""), &out, &errb)
	capture(t, "run-dry", out.String())
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if !strings.HasPrefix(lines[0], "run --dry: night ") || len(lines) != 5+strings.Count(out.String(), " \\\n") {
		t.Fatalf("run --dry:\n%s", out.String())
	}
	for _, l := range lines {
		if len(l) >= 100 {
			t.Fatalf("%d columns: %s", len(l), l)
		}
	}
}

// capture writes a verb's printed output to $UX1_CAPTURE/<name>.txt when
// that is set (UX1's captures).
func capture(t *testing.T, name, text string) {
	if dir := os.Getenv("UX1_CAPTURE"); dir != "" {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, name+".txt"), []byte(text), 0o644)
	}
}

// TestNightVerbs: status, stop, kill, resume and report over a night's
// status.json and ledger lines in a temp state directory.
func TestNightVerbs(t *testing.T) {
	home := t.TempDir()
	gitRoot, _ := resumeRepo(t, home) // resume's door is git's (AR22 act 3)
	getenv = fakeEnv(home)
	oldExec, oldTop := execResume, toplevel
	t.Cleanup(func() { getenv, execResume, toplevel = os.Getenv, oldExec, oldTop })
	measuring(t) // resume refuses under a measurement (AR17B)
	state := effects.StateDir(getenv)
	const id = "2026-09-22-1"
	root := filepath.Join(home, "repo")
	os.MkdirAll(night.NightDir(state, id), 0o700)
	os.WriteFile(filepath.Join(night.NightDir(state, id), "status.json"),
		[]byte(`{"night":"`+id+`","state":"running","rows":[{"id":"r1","state":"red","resume":"relay resume r1"}],"resume":["relay resume r1"]}`), 0o600)
	l := effects.NewLedger(state, effects.FakeClock{T: time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)}, id)
	l.Append("night-open", map[string]any{"root": root})
	l.Append("stage", map[string]any{"row": "r1", "worktree": filepath.Join(root, ".claude", "worktrees", "r1"), "branch": "r1", "base": "main"})
	l.Append("sitting", map[string]any{"row": "r1", "state": "red", "session_id": "s-1", "turns": 3, "cost_usd": 1.5})
	l.Append("stop", map[string]any{"reason": "queue-empty", "state": "stopped:queue-empty"})

	do := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	if code, out, _ := do("status", "--json"); code != exitOK || !strings.Contains(out, `"night":"`+id+`"`) {
		t.Fatalf("status = %d %s", code, out)
	}
	if code, out, _ := do("status"); code != exitOK || !strings.HasSuffix(out, "\nrelay resume r1\n") {
		t.Fatalf("status = %d %s", code, out)
	}
	if code, _, e := do("stop"); code != exitOK {
		t.Fatalf("stop = %d %s", code, e)
	}
	if code, _, e := do("kill", "r1"); code != exitOK {
		t.Fatalf("kill = %d %s", code, e)
	}
	stop, kills := night.ReadControl(effects.OSFS{}, night.ControlPath(state, id))
	if !stop || len(kills) != 1 || kills[0] != "r1" {
		t.Fatalf("control %v %v", stop, kills)
	}
	var ran []string
	execResume = func(dir string, argv []string) error { ran = append([]string{dir}, argv...); return nil }
	// AR22 act 3 (ADV13-8): the ledger's s-1 is printed, never run; with
	// no transcript for r1's tree the door is the picker in the root.
	if code, out, _ := do("resume", "r1"); code != exitOK || !reflect.DeepEqual(ran, []string{gitRoot, "claude", "--resume"}) ||
		!strings.Contains(out, "&& claude --resume\n") || !strings.Contains(out, `session "s-1"`) {
		t.Fatalf("resume r1 = %d %q %v", code, out, ran)
	}
	_, rout, _ := do("resume", "r1")
	capture(t, "resume-r1", strings.ReplaceAll(rout, home, "$HOME"))
	_, rout, _ = do("resume")
	capture(t, "resume", rout)
	if code, out, _ := do("resume"); code != exitOK || out != "night "+id+": 1 red or failed row(s) to resume\nrelay resume r1\n" {
		t.Fatalf("resume = %d %q", code, out)
	}
	code, out, _ := do("report", "--json")
	var r night.Report
	if err := json.Unmarshal([]byte(out), &r); code != exitOK || err != nil || len(r.Sittings) != 1 || r.Stopped.Reason != "queue-empty" {
		t.Fatalf("report = %d %s", code, out)
	}
	if code, out, _ := do("report"); code != exitOK || strings.Contains(out, home) || !strings.Contains(out, "\nrelay resume r1\n") {
		t.Fatalf("report = %d %s", code, out)
	}
	fakeTop(t, root)
	if code, _, e := do("report", "--write", "history/nights/"+id+".md"); code != exitOK {
		t.Fatalf("report --write = %d %s", code, e)
	}
	if _, err := os.Stat(filepath.Join(root, "history", "nights", id+".md")); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := do("report", "--write", "/etc/x.md"); code != exitUsage {
		t.Fatalf("an absolute --write = %d", code)
	}
}

// TestAttachVerb: attach --print reads `tail -f <log>` with the
// row's path when no session is alive, `tmux attach -t '=relay-<night>:<row>'`
// when one is, and exits 2 for a row the night does not hold.
func TestAttachVerb(t *testing.T) {
	home := t.TempDir()
	getenv = fakeEnv(home)
	oldTmux, oldExec := newTmux, execAttach
	t.Cleanup(func() { getenv, newTmux, execAttach = os.Getenv, oldTmux, oldExec })
	state := effects.StateDir(getenv)
	const id = "2026-09-22-1"
	os.MkdirAll(night.NightDir(state, id), 0o700)
	os.WriteFile(filepath.Join(night.NightDir(state, id), "status.json"),
		[]byte(`{"night":"`+id+`","state":"running","rows":[{"id":"r1","state":"running"},{"id":"r2","state":"ready"}]}`), 0o600)
	ft := &effects.FakeTmux{}
	newTmux = func(string) effects.Tmux { return ft }
	var ran []string
	execAttach = func(argv []string) error { ran = argv; return nil }
	do := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := run(append([]string{"attach"}, args...), strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	log := night.RowLog(state, id, "r1")
	_, aout, _ := do("r1", "--print")
	capture(t, "attach-r1", aout)
	if code, out, _ := do("r1", "--print"); code != exitOK || out != "tail -f ~/"+night.Quote([]string{strings.TrimPrefix(log, home+"/")})+"\n" || ran != nil {
		t.Fatalf("attach r1 --print = %d %q, ran %q", code, out, ran)
	}
	if code, _, _ := do("r1"); code != exitOK || strings.Join(ran, " ") != "tail -f "+log {
		t.Fatalf("attach r1 ran %q (%d)", ran, code)
	}
	if code, out, _ := do("--print"); code != exitOK || !strings.Contains(out, "r2.log") {
		t.Fatalf("attach --print = %d %q", code, out)
	}
	if code, _, errs := do("nosuchrow"); code != exitUsage || !strings.Contains(errs, "no row nosuchrow") {
		t.Fatalf("attach nosuchrow = %d %q", code, errs)
	}
	ft.IsPresent = true
	if code, out, _ := do("r1", "--print"); code != exitOK || out != "tmux attach -t '=relay-"+id+":r1'\n" {
		t.Fatalf("attach with a live session = %d %q", code, out)
	}
	ft.AliveAfter = 1 // the next Alive is the second: the session is gone
	if _, out, _ := do("r1", "--print"); !strings.HasPrefix(out, "tail -f ") {
		t.Fatalf("attach after the session went = %q", out)
	}
}

// jsonKeys is every key path a type marshals to, one per line: a nested
// struct's keys under `parent.`, a slice's element under `parent[].`, a
// map's values under `parent{}`.
func jsonKeys(prefix string, t reflect.Type, out *[]string) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		switch t.Kind() {
		case reflect.Slice:
			prefix += "[]"
		case reflect.Map:
			prefix += "{}"
		}
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == "time" {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" || !f.IsExported() {
			continue
		}
		if f.Anonymous && name == "" {
			jsonKeys(prefix, f.Type, out)
			continue
		}
		if name == "" {
			name = f.Name
		}
		*out = append(*out, prefix+name)
		jsonKeys(prefix+name+".", f.Type, out)
	}
}

// TestJSONShapesFrozen (UX1): the key sets of `status --json`, `report
// --json`, `plan --json`, `run --json` and `doctor --json` equal the golden taken before
// the printed lines were reworded; the plugin reads them (§10).
// UX1_GOLDEN=1 rewrites the golden.
func TestJSONShapesFrozen(t *testing.T) {
	var got []string
	for _, v := range []struct {
		verb string
		t    reflect.Type
	}{{"status", reflect.TypeOf(night.Status{})}, {"report", reflect.TypeOf(night.Report{})}, {"plan", reflect.TypeOf(night.Plan{})}, {"run", reflect.TypeOf(runOutput{})}, {"gate", reflect.TypeOf(gateReport{})}} {
		var keys []string
		jsonKeys("", v.t, &keys)
		for _, k := range keys {
			got = append(got, v.verb+" "+k)
		}
	}
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	var out, errb bytes.Buffer
	run([]string{"doctor", "--json"}, strings.NewReader(""), &out, &errb)
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				got = append(got, "doctor "+prefix+k)
				walk(prefix+k+".", e)
			}
		case []any:
			if len(x) > 0 {
				walk(strings.TrimSuffix(prefix, ".")+"[].", x[0])
			}
		}
	}
	walk("", doc)
	sort.Strings(got)
	const golden = "../../internal/night/testdata/json-keys.golden"
	text := strings.Join(got, "\n") + "\n"
	if os.Getenv("UX1_GOLDEN") != "" {
		os.MkdirAll(filepath.Dir(golden), 0o755)
		os.WriteFile(golden, []byte(text), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != text {
		t.Fatalf("the --json key sets moved:\n got %s\nwant %s", text, want)
	}
}

// TestDoctorPrinted (UX1): the first line is the verdict, home reads `~`,
// and a long detail wraps under its column so no line reaches 100.
func TestDoctorPrinted(t *testing.T) {
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	home := getenv("HOME")
	lookPath = func(name string) (string, error) { return home + "/" + strings.Repeat("long/", 12) + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	var out, errb bytes.Buffer
	run([]string{"doctor"}, strings.NewReader(""), &out, &errb)
	capture(t, "doctor", out.String())
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if !strings.HasPrefix(lines[0], "doctor: ") || strings.Contains(out.String(), home) || !strings.Contains(out.String(), "~/long/") {
		t.Fatalf("doctor:\n%s", out.String())
	}
	for _, l := range lines {
		if len(l) >= 100 {
			t.Fatalf("%d columns: %s", len(l), l)
		}
	}
}

// TestUsageOrder (UX1): usage names status and resume first, and no line
// reaches 100 columns.
func TestUsageOrder(t *testing.T) {
	var b bytes.Buffer
	usage(&b)
	capture(t, "help", b.String())
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if !strings.HasPrefix(lines[1], "  status ") || !strings.HasPrefix(lines[2], "  resume ") {
		t.Fatalf("usage:\n%s", b.String())
	}
	for _, l := range lines {
		if len(l) >= 100 {
			t.Fatalf("%d columns: %s", len(l), l)
		}
	}
}

// TestAR4_JSONKeysSnake: every --json key the golden freezes is snake_case
// (relay.Check, relay.Gate, queue.Problem and `gate --json` included), and
// the golden carries status rows' worktree and the gate verb.
func TestAR4_JSONKeysSnake(t *testing.T) {
	b, err := os.ReadFile("../../internal/night/testdata/json-keys.golden")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.ToLower(l) != l {
			t.Errorf("a capitalised key: %q", l)
		}
	}
	for _, want := range []string{"\nstatus rows.[]worktree\n", "\ngate lines.[]main_exit\n", "\nplan problems.[]msg\n", "\nplan verify.main_last\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("the golden lacks %q", strings.TrimSpace(want))
		}
	}
}

// TestMain points XDG_STATE_HOME, the one state root (§5), at a temp
// directory before any test, so no test reads or writes the machine's
// ledger, locks or nights.
func TestMain(m *testing.M) {
	// AR22 act 2: the account lock is a free port of this binary's (or its
	// parent test binary's), never the account's.
	if err := useTestLockPort(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if dir := os.Getenv(pidHolderEnv); dir != "" {
		pidHolder(dir) // TestAR21_2_SecondNightRefused's holder; never returns
	}
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
	// The pid file's home is the account's, not the env's (ADV11-9): a temp one.
	night.UserHome = func() (string, error) { return state, nil }
	// The recursion guard (§15): whatever this process starts is nested.
	os.Setenv(effects.MeasureDepthVar, "1")
	// A sitting's row is never this process's: the run guard reads it (AR11).
	os.Unsetenv(night.RowVar)
	// The checkout is read once, to guard against; every test's toplevel is
	// a temp tree (fakeTop), the default an empty one.
	if checkout, err = toplevel(); err != nil || checkout == "" {
		checkout, _ = filepath.Abs(filepath.Join("..", ".."))
	}
	empty, err := os.MkdirTemp("", "relay-top-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	toplevel = func() (string, error) { return empty, nil }
	// The pin is the package's (RunPinned), built once under a temp dir;
	// a run() keeps it (AR22 act 1).
	pinFor = func() (func(), error) { return func() {}, nil }
	// AR22 act 1: every gate line and runner git runs under a pinned env,
	// built under a temp dir of its own.
	code := effects.RunPinned(m)
	os.RemoveAll(empty)
	os.RemoveAll(state)
	os.Exit(code)
}

// TestAR7_GateSkippedChecks: a hand gate has no stream and no stage
// snapshot, so exit and origin are skipped: printed so, never `true`, and
// named in --json's skipped.
func TestAR7_GateSkippedChecks(t *testing.T) {
	var out, errb bytes.Buffer
	gateFakes(t, true)
	run([]string{"gate", "a3-machine"}, strings.NewReader(""), &out, &errb)
	if s := out.String(); !strings.Contains(s, "exit skipped") || !strings.Contains(s, "origin skipped") {
		t.Fatalf("gate a3-machine printed:\n%s", s)
	}
	out.Reset()
	gateFakes(t, true)
	run([]string{"gate", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb)
	var rep struct{ Skipped []string }
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || !reflect.DeepEqual(rep.Skipped, []string{"exit", "origin", "ls-remote", "base"}) {
		t.Fatalf("skipped %v (%v): %s", rep.Skipped, err, out.String())
	}
}

// TestAR7_ReportAndResumeLedgerError: a ledger that cannot be read (a
// directory where a day file belongs) is `relay <verb>: the ledger: <err>`
// on stderr and exit 2, never an empty morning.
func TestAR7_ReportAndResumeLedgerError(t *testing.T) {
	home := t.TempDir()
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = os.Getenv })
	measuring(t) // resume refuses under a measurement (AR17B)
	dir := effects.LedgerDir(effects.StateDir(getenv))
	if err := os.MkdirAll(filepath.Join(dir, "2026-09-22.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, verb := range [][]string{{"report"}, {"report", "--json"}, {"resume"}, {"resume", "r1"}} {
		var out, errb bytes.Buffer
		code := run(verb, strings.NewReader(""), &out, &errb)
		if want := "relay " + verb[0] + ": the ledger: "; code != exitUsage || !strings.HasPrefix(errb.String(), want) || out.Len() != 0 {
			t.Errorf("%v = %d, stdout %q, stderr %q; want %q...", verb, code, out.String(), errb.String(), want)
		}
	}
}

// checkout is the real checkout's root, set by TestMain; no test's
// toplevel lies under it outside a testdata directory.
var checkout string

// outsideCheckout is nil when path lies outside the real checkout, or in a
// testdata directory under it (a fixture); else an error naming it.
func outsideCheckout(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	root := checkout
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "testdata" {
			return nil
		}
	}
	return fmt.Errorf("%s lies under the real checkout %s", path, checkout)
}

// fakeTop is the one way a test sets toplevel: to dir, which must lie
// outside the real checkout (outsideCheckout); restored on cleanup.
func fakeTop(t *testing.T, dir string) {
	t.Helper()
	if err := outsideCheckout(dir); err != nil {
		t.Fatalf("toplevel: %v", err)
	}
	old := toplevel
	t.Cleanup(func() { toplevel = old })
	toplevel = func() (string, error) { return dir, nil }
}

// fakeEnv is a test's environment: HOME, and RELAY_MEASURE_DEPTH set, so
// run() never measures unless the test says measuring.
func fakeEnv(home string) func(string) string {
	m := map[string]string{"HOME": home, effects.MeasureDepthVar: "1"}
	return func(k string) string { return m[k] }
}

// lateEnv reads getenv at the call, so a fake bundle sees measuring.
func lateEnv(k string) string { return getenv(k) }

// measuring clears RELAY_MEASURE_DEPTH from getenv, for a test that
// measures over a fake shell.
func measuring(t *testing.T) {
	old := getenv
	t.Cleanup(func() { getenv = old })
	getenv = func(k string) string {
		if k == effects.MeasureDepthVar {
			return ""
		}
		return old(k)
	}
}

// TestAR10_0_NoTestOpensRealCheckout: the toplevel seam is the guard. Every
// test's toplevel goes through fakeTop, which fails a test whose tree lies
// under the real checkout outside testdata; the default TestMain leaves is
// an empty temp tree; this file sets toplevel nowhere else; and no _test.go
// under cmd or internal names a `"../..` path in the repository but a
// fixture or a copyIn copy (realTreeReads, AR17D act 4).
func TestAR10_0_NoTestOpensRealCheckout(t *testing.T) {
	if checkout == "" {
		t.Fatal("TestMain read no checkout")
	}
	for path, under := range map[string]bool{
		checkout: true, filepath.Join(checkout, "relay", "queue.yaml"): true,
		filepath.Join(checkout, "design", "packets", "a3-machine.md"): true, filepath.Join("..", ".."): true,
		filepath.Join(checkout, "internal", "packet", "testdata", "a3-machine.md"): false,
		t.TempDir(): false, "/root": false,
	} {
		if err := outsideCheckout(path); (err != nil) != under {
			t.Errorf("outsideCheckout(%s) = %v; under the checkout %v", path, err, under)
		}
	}
	if top, err := toplevel(); err != nil || outsideCheckout(top) != nil {
		t.Fatalf("the default toplevel %q (%v) lies under the checkout", top, err)
	}
	src, err := os.ReadFile("main_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(regexp.MustCompile(`(?m)^\s*toplevel = `).FindAll(src, -1)); n != 2 {
		t.Errorf("toplevel is set %d times outside a restore; want 2 (TestMain, fakeTop)", n)
	}
	if bad, err := realTreeReads(copyIn(t, "../..", "cmd", "internal")); err != nil || len(bad) != 0 {
		t.Errorf("a test reads the real tree outside copyIn: %q, %v", bad, err)
	}
}

// TestAR10_0_NestedPlanMeasuresOnce: a temp repo whose one row's one
// acceptance line runs the built binary's `plan --rows` on itself (through
// a `go test` of its own, since AR20 act 5). The
// outer plan measures that line once (the counter file holds one line) and
// reads it green; the inner plan, under RELAY_MEASURE_DEPTH from OSShell,
// measures nothing: its ledger's baseline checks are all `unrun: nested`.
func TestAR10_0_NestedPlanMeasuresOnce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	bin := filepath.Join(dir, "relay")
	if out, err := exec.Command("go", "build", "-ldflags", pidHomeFlag(dir), "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	root, home := filepath.Join(dir, "root"), filepath.Join(dir, "home")
	outer, inner, count := filepath.Join(dir, "outer"), filepath.Join(dir, "inner"), filepath.Join(dir, "count")
	os.MkdirAll(filepath.Join(root, "relay"), 0o755)
	os.MkdirAll(filepath.Join(root, "design", "packets"), 0o755)
	os.MkdirAll(home, 0o755)
	os.WriteFile(filepath.Join(root, "relay", "queue.yaml"), []byte(`version: 1
defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,
           merge: branch-only, device: never}
words: ""
rows:
  - id: n1-nested
    packet: design/packets/n1-nested.md
`), 0o644)
	// An acceptance line is `go test` or a read-only verb (AR20 act 5): the
	// line reaches the binary through a test in the fixture's own module.
	os.MkdirAll(filepath.Join(root, "n1"), 0o755)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module n1\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(root, "n1", "n1_test.go"), []byte(fmt.Sprintf(`package n1

import (
	"os"
	"os/exec"
	"testing"
)

func TestNested(t *testing.T) {
	f, err := os.OpenFile(%q, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("x\n")
	f.Close()
	c := exec.Command(%q, "plan", "--rows", "n1-nested")
	c.Dir, c.Env = "..", append(os.Environ(), "XDG_STATE_HOME="+%q)
	c.Run()
}
`, count, bin, inner)), 0o644)
	line := "go test ./n1 -run TestNested -count=1 >/dev/null 2>&1; echo $?"
	os.WriteFile(filepath.Join(root, "design", "packets", "n1-nested.md"), []byte("# Packet n1-nested: a plan inside a plan\n\n"+
		"| item | value |\n|---|---|\n| from | `design/relay-r1.md` §15 |\n| pin | branch `main` |\n| wall | this repo only |\n"+
		"| may touch | `relay/`. Nothing else |\n| acceptance | `"+line+"` reads 0 |\n| budget | 10 turns, 20k context, $5 |\n\n"+
		"## 1. What the source says (quoted)\nQuoted.\n\n## 2. Rulings\nNone.\n\n## 3. Interfaces (signatures, not prose)\nNone.\n\n"+
		"## 4. Findings that name this row\nNone.\n\n## 5. Forbidden reads\nNone.\n\n## 6. Report (capped)\nA table.\n"), 0o644)
	env := []string{"HOME=" + home, "XDG_STATE_HOME=" + outer, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k == "PATH" || k == "GOCACHE" || k == "GOPATH" || k == "TMPDIR" {
			env = append(env, kv) // never RELAY_*, CLAUDE*, the real HOME or state
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "fixture"}} {
		c := exec.Command("git", args...)
		c.Dir, c.Env = root, env
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	c := exec.Command(bin, "plan", "--rows", "n1-nested", "--json")
	c.Dir, c.Env = root, env
	out, err := c.Output()
	if err != nil {
		t.Fatalf("outer plan: %v\n%s", err, out)
	}
	var p night.Plan
	if err := json.Unmarshal(out, &p); err != nil || p.Nested || len(p.Rows) != 1 || len(p.Rows[0].Baseline) != 1 ||
		p.Rows[0].Baseline[0].Status != relay.CheckGreen {
		t.Fatalf("outer plan (%v): %s", err, out)
	}
	if b, _ := os.ReadFile(count); string(b) != "x\n" {
		t.Fatalf("the line ran %d times; want once", strings.Count(string(b), "x"))
	}
	days, _ := filepath.Glob(filepath.Join(effects.LedgerDir(filepath.Join(inner, "agentik-relay")), "*.jsonl"))
	plans := 0
	for _, d := range days {
		b, _ := os.ReadFile(d)
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var m map[string]any
			if json.Unmarshal([]byte(l), &m) != nil || m["event"] != "plan" {
				continue
			}
			plans++
			for _, c := range m["baseline"].([]any) {
				if c := c.(map[string]any); c["status"] != relay.CheckUnrun || c["last"] != effects.NestedReason {
					t.Fatalf("the inner plan measured: %v", c)
				}
			}
		}
	}
	if plans != 1 {
		t.Fatalf("the inner ledger holds %d plan lines; want 1", plans)
	}
}

// TestAR10_4_MeasureSecondsFlag: `--measure-seconds` takes a whole number
// of seconds over 0 on plan and run; anything else is a usage error.
func TestAR10_4_MeasureSecondsFlag(t *testing.T) {
	measuring(t) // the run guard (AR11) reads before the flags
	for _, verb := range []string{"plan", "run"} {
		for _, v := range []string{"0", "-5", "1.5", "x"} {
			var out, errb bytes.Buffer
			if code := run([]string{verb, "--measure-seconds", v}, strings.NewReader(""), &out, &errb); code != exitUsage ||
				!strings.Contains(errb.String(), "--measure-seconds") {
				t.Fatalf("%s --measure-seconds %s = %d %q", verb, v, code, errb.String())
			}
		}
	}
	if n, ok := measureSeconds("600"); !ok || n != 600 {
		t.Fatalf("measureSeconds(600) = %d %v", n, ok)
	}
}

// pidHomeFlag is the -ldflags that points a built binary's pid file at dir
// and its account lock at a free port of its own (night.PidHomeLink,
// night.LockPortLink): link time, never the environment (ADV11-9; AR22
// act 2: no built binary binds the account's port).
func pidHomeFlag(dir string) string { return lockFlags(dir, freeLockPort()) }

// lockFlags is pidHomeFlag at a given lock port (the test holds it).
func lockFlags(dir string, port int) string {
	return "-X github.com/agentik-dev/agentik-relay/internal/night.PidHomeLink=" + dir +
		" -X github.com/agentik-dev/agentik-relay/internal/night.LockPortLink=" + strconv.Itoa(port)
}
