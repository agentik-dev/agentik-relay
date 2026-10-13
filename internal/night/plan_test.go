package night

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// fixture writes a three-row queue over the packet corpus into a temp repo:
// a3 waits on an external green in the ledger, a4 on a merged branch (an
// ancestor of main), a5 on a4, which is neither.
func fixture(t *testing.T) (string, *queue.Queue) {
	t.Helper()
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "design", "packets"), 0o755)
	for _, id := range []string{"a3-machine", "a4-schemas", "a5-effects-plan"} {
		src, err := os.ReadFile(filepath.Join("..", "packet", "testdata", id+".md"))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(repo, "design", "packets", id+".md"), src, 0o644)
	}
	os.WriteFile(filepath.Join(repo, "queue.yaml"), []byte(`version: 1
defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,
           merge: branch-only, device: never, verify: auto}
words: ""
rows:
  - id: a3-machine
    packet: design/packets/a3-machine.md
    depends_on: [a2-relay-design]
  - id: a4-schemas
    packet: design/packets/a4-schemas.md
    depends_on: [ar1-relay-hooks]
  - id: a5-effects-plan
    packet: design/packets/a5-effects-plan.md
    depends_on: [a4-schemas]
    base: a4-schemas
    budget: {turns: 120}
`), 0o644)
	q, err := queue.Load(repo, "queue.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return repo, q
}

func fakes(repo string, dirty bool) (effects.Effects, *effects.FakeGit, *effects.FakeShell, *effects.FakeLedger) {
	now := time.Date(2026, 9, 22, 22, 0, 0, 0, time.UTC)
	g := &effects.FakeGit{
		Roots:     map[string]string{repo: "/root"},
		Branches:  map[string]string{"/root": "main"},
		Statuses:  map[string][]string{},
		Ancestors: map[string]bool{"ar1-relay-hooks..main": true}, Merged: map[string]bool{"ar1-relay-hooks..main": true},
		Existing: map[string]bool{"main": true, "ar1-relay-hooks": true},
		Remote:   map[string]string{"refs/remotes/origin/main": "0933461"},
	}
	if dirty {
		g.Statuses["/root"] = []string{" M README.md"}
	}
	sh := &effects.FakeShell{Results: map[string]effects.ShellResult{
		"make check; echo $?":         {Exit: 0, Last: "0"},
		"grep -c -- '-race' Makefile": {Exit: 0, Last: "1"},
	}}
	fs := &effects.FakeFS{}
	fs.MkdirAll("/state/nights/2026-09-22-1", 0o700)
	l := &effects.FakeLedger{Clock: effects.FakeClock{T: now}, Night: "2026-09-22-2",
		Prior: []map[string]any{{"night": "2026-09-20-1", "event": "gate", "row": "a2-relay-design", "result": "green"}}}
	return effects.Effects{Git: g, Shell: sh, Clock: effects.FakeClock{T: now}, FS: fs, Ledger: l}, g, sh, l
}

func TestPlanOverFakes(t *testing.T) {
	repo, q := fixture(t)
	fx, g, sh, l := fakes(repo, false)
	// a2-relay-design landed in git: AR22 act 3 (ADV13-7) took away the
	// ledger green line that satisfied it.
	g.Existing["a2-relay-design"], g.Merged["a2-relay-design..main"] = true, true
	n := 0
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Caffeinate: true, Until: "06:30",
		UUID: func() string { n++; return strings.Repeat(string(rune('0'+n)), 8) }})
	if err != nil {
		t.Fatal(err)
	}
	if p.Night != "2026-09-22-2" || len(p.TemplateHash) != 64 || !p.Valid || p.Origin["refs/remotes/origin/main"] != "0933461" {
		t.Fatalf("plan = %+v", p)
	}
	want := []struct{ id, state, sat, pend string }{
		{"a3-machine", StateReady, "a2-relay-design", ""},
		{"a4-schemas", StateReady, "ar1-relay-hooks", ""},
		{"a5-effects-plan", StatePending, "", "a4-schemas"},
	}
	for i, w := range want {
		r := p.Rows[i]
		if r.ID != w.id || r.State != w.state || strings.Join(r.Satisfied, ",") != w.sat || strings.Join(r.Pending, ",") != w.pend {
			t.Errorf("row %d = %s %s sat %v pend %v; want %+v", i, r.ID, r.State, r.Satisfied, r.Pending, w)
		}
	}
	a3 := p.Rows[0]
	if len(a3.Baseline) != 8 || a3.Baseline[0].Status != relay.CheckGreen || a3.Baseline[2].Status != relay.CheckGreen ||
		a3.Baseline[1].Status != relay.CheckRed {
		t.Fatalf("a3 baseline = %+v", a3.Baseline)
	}
	for _, c := range sh.Calls() {
		if !strings.HasPrefix(c, "/root: ") {
			t.Fatalf("a baseline ran outside the root: %q", c)
		}
	}
	wantSpawn := []string{"caffeinate", "-i", "claude", "-p", "--permission-mode", "auto", "--allowedTools",
		effects.DefaultAllowedTools, "--model", "claude-opus-5-5", "--max-turns", "120", "--max-budget-usd", "15",
		"--output-format", "stream-json", "--verbose", "--session-id", "33333333"}
	if r := p.Rows[2]; !reflect.DeepEqual(r.Spawn, wantSpawn) || r.Worktree != "/root/.claude/worktrees/a5-effects-plan" || r.Base != "a4-schemas" {
		t.Fatalf("a5 = %+v", r)
	}
	if v := p.Verify; v == nil || v.Name != "make check" || v.Status != relay.CheckGreen || v.MainExit != 0 {
		t.Fatalf("verify baseline = %+v", p.Verify)
	}
	if lines := l.Written(); len(lines) != 3 || lines[0]["event"] != "plan" || lines[0]["row"] != "a3-machine" || lines[0]["verify"] == nil {
		t.Fatalf("ledger = %v", lines)
	}
	var out strings.Builder
	Print(&out, p)
	if c := strings.Count(out.String(), "spawn: "); c != 3 || !strings.Contains(out.String(), "until-reached 06:30") {
		t.Fatalf("Print:\n%s", out.String())
	}
}

func TestPlanUnrunAndUnknown(t *testing.T) {
	repo, q := fixture(t)
	fx, g, sh, _ := fakes(repo, true)
	fx.Ledger.(*effects.FakeLedger).Prior = nil
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a3-machine", "zz"}, Until: "25:99"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 1 || p.Rows[0].State != StateInvalid || p.Valid || len(sh.Calls()) != 0 {
		t.Fatalf("plan = %+v; shell %v", p, sh.Calls())
	}
	for _, c := range p.Rows[0].Baseline {
		if c.Status != relay.CheckUnrun || c.MainExit != -1 || c.MainLast != InvalidReason { // AR10 act 1: invalid before dirty
			t.Fatalf("check = %+v", c)
		}
	}
	msgs := ""
	for _, pr := range p.Problems {
		msgs += pr.Row + ": " + pr.Msg + "\n"
	}
	for _, want := range []string{"a3-machine: depends on a2-relay-design: unknown", "--rows zz", "--until \"25:99\""} {
		if !strings.Contains(msgs, want) {
			t.Errorf("problems lack %q:\n%s", want, msgs)
		}
	}
	if why := baselineBlocked(g, "/root"); !strings.Contains(why, "dirty") {
		t.Fatalf("baselineBlocked = %q", why)
	}
	g.Branches["/root"] = "a4-schemas"
	g.Statuses["/root"] = nil
	if why := baselineBlocked(g, "/root"); !strings.Contains(why, "not main") {
		t.Fatalf("baselineBlocked = %q", why)
	}
}

func TestNightName(t *testing.T) {
	fs := &effects.FakeFS{}
	now := time.Date(2026, 9, 22, 22, 0, 0, 0, time.UTC)
	if n := NightName(fs, "/s", now); n != "2026-09-22-1" {
		t.Fatalf("NightName = %q", n)
	}
	fs.MkdirAll("/s/nights/2026-09-22-1", 0o700)
	fs.MkdirAll("/s/nights/2026-09-22-3", 0o700)
	if n := NightName(fs, "/s", now); n != "2026-09-22-2" {
		t.Fatalf("NightName = %q", n)
	}
}

func TestQuote(t *testing.T) {
	if s := Quote([]string{"claude", "--allowedTools", "Bash Edit", "it's"}); s != `claude --allowedTools 'Bash Edit' 'it'\''s'` {
		t.Fatalf("Quote = %s", s)
	}
	// zsh expands a word that begins with `=` as a command lookup; one
	// with `=` inside is plain (AR33 fix 1).
	if s := Quote([]string{"tmux", "attach", "-t", "=relay-n:r1", "--x=y"}); s != `tmux attach -t '=relay-n:r1' --x=y` {
		t.Fatalf("Quote = %s", s)
	}
}

// TestAR4_DepNeverSatisfiedByOwnBase: a5 is cut off a4-schemas and depends on
// it; the branch exists and is its own ancestor, but only ancestry of main
// satisfies, so a5 stays pending until a4 is green or merged.
func TestAR4_DepNeverSatisfiedByOwnBase(t *testing.T) {
	repo, q := fixture(t)
	fx, g, _, _ := fakes(repo, false)
	g.Existing["a4-schemas"] = true
	g.Ancestors["a4-schemas..a4-schemas"] = true
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a5-effects-plan"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := p.Rows[0]; r.State != StatePending || strings.Join(r.Pending, ",") != "a4-schemas" {
		t.Fatalf("a5 = %s pend %v sat %v", r.State, r.Pending, r.Satisfied)
	}
	g.Ancestors["a4-schemas..main"] = true
	g.Merged["a4-schemas..main"] = true
	p, err = BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a5-effects-plan"}})
	if err != nil {
		t.Fatal(err)
	}
	if r := p.Rows[0]; r.State != StateReady {
		t.Fatalf("a5 merged into main = %s pend %v", r.State, r.Pending)
	}
}

// TestAR5_SpawnLineWrapped (UX1): every line plan prints is under 100
// columns; the spawn line's pieces, continuations joined and `~` expanded,
// are the argv run in the worktree.
func TestAR5_SpawnLineWrapped(t *testing.T) {
	home := "/home/owner"
	wt := home + "/src/agent-relay/.claude/worktrees/a5-effects-plan-with-a-long-name"
	argv := SpawnArgv(queue.Row{Model: "claude-opus-5-5", Turns: 120, USD: 15}, NewUUID(), "Bash Edit Write MultiEdit NotebookEdit", true)
	pieces := WrapSpawn(wt, argv, home)
	if len(pieces) < 2 || !strings.HasPrefix(pieces[0], "  spawn: cd ~/src/") {
		t.Fatalf("pieces %q", pieces)
	}
	var joined []string
	for i, p := range pieces {
		if n := len([]rune(p)); n >= 100 {
			t.Errorf("%d columns: %s", n, p)
		}
		p, more := strings.CutSuffix(p, " \\")
		if more != (i < len(pieces)-1) {
			t.Errorf("piece %d continuation %v: %s", i, more, p)
		}
		joined = append(joined, strings.TrimLeft(strings.TrimPrefix(p, "  spawn: "), " "))
	}
	got := strings.ReplaceAll(strings.Join(joined, " "), "~/", home+"/")
	if want := "cd " + Quote([]string{wt}) + " && " + Quote(argv) + " < prompt"; got != want {
		t.Fatalf("joined\n%s\nwant\n%s", got, want)
	}
	p := &Plan{Night: "2026-09-22-1", Valid: true, Rows: []RowPlan{{ID: "a5", State: StateReady, Branch: "a5", Base: "main",
		Worktree: wt, Spawn: argv, SessionID: argv[len(argv)-1]}}}
	t.Setenv("HOME", home)
	var out strings.Builder
	Print(&out, p)
	for _, l := range strings.Split(out.String(), "\n") {
		if n := len([]rune(l)); n >= 100 {
			t.Errorf("plan line of %d columns: %s", n, l)
		}
	}
}
