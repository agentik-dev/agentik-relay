package gate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// world is a temp root on main with a bare file remote as origin, fetched;
// the effects are the machine's with a fake ledger and the locks in a temp
// dir. stage adds the row a1.
type world struct {
	root, tree string
	fx         effects.Effects
	ledger     *effects.FakeLedger
}

func newWorld(t *testing.T) *world {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}
	oldDir, oldLive := night.LockDir, night.LiveClaude
	locks := t.TempDir()
	night.LockDir = func() string { return locks }
	night.LiveClaude = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() { night.LockDir, night.LiveClaude = oldDir, oldLive })

	dir, _ := filepath.EvalSymlinks(t.TempDir())
	w := &world{root: filepath.Join(dir, "root")}
	remote := filepath.Join(dir, "remote.git")
	gitIn(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	gitIn(t, dir, "init", "-q", "-b", "main", w.root)
	write(t, w.root, "a.txt", "a\n")
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "one")
	gitIn(t, w.root, "remote", "add", "origin", remote)
	gitIn(t, w.root, "push", "-q", "origin", "main")
	gitIn(t, w.root, "fetch", "-q", "origin")

	clock := effects.FakeClock{T: time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)}
	w.ledger = &effects.FakeLedger{Clock: clock, Night: "2026-09-22-1"}
	w.fx = effects.Effects{Git: effects.OSGit{}, Shell: effects.OSShell{}, Clock: clock, FS: effects.OSFS{}, Ledger: w.ledger}
	return w
}

// stage stages a1 off main through night.Stage and commits touch in it.
func (w *world) stage(t *testing.T, touch ...string) *night.Staged {
	t.Helper()
	w.tree = filepath.Join(w.root, ".claude", "worktrees", "a1")
	st, err := night.Stage(w.fx, &night.RowPlan{ID: "a1", Worktree: w.tree, Branch: "a1", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Unlock() })
	for _, f := range touch {
		write(t, w.tree, f, "a1 "+f+"\n")
	}
	gitIn(t, w.tree, "add", ".")
	gitIn(t, w.tree, "commit", "-qm", "a1's work")
	return st
}

func pk(may []string, lines ...packet.Line) *packet.Packet {
	p := &packet.Packet{ID: "a1", Acceptance: lines}
	for _, m := range may {
		p.MayTouch = append(p.MayTouch, glob(m))
	}
	return p
}

var seen = &ExitFacts{Seen: true}

func TestGateGreen(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt", "history/sittings/2026-09-22-1.md")
	p := pk([]string{"a.txt"}, packet.Line{Shell: "cat a.txt", Expect: "reads `a1 a.txt`", Kind: packet.Reads, Value: "a1 a.txt"})
	res, why, g, _ := Run(w.fx, st, p, nil, seen, Verify{})
	if res != relay.GreenResult || why != "" {
		t.Fatalf("Run = %s %s; gate %+v", res, why, g)
	}
	lines := w.ledger.Written()
	last := lines[len(lines)-1]
	if !effects.Green(last, "a1") {
		t.Fatalf("the last ledger line is not a green result: %v", last)
	}
	for _, l := range lines {
		for _, v := range l {
			if s, ok := v.(string); ok && strings.Contains(s, "cat a.txt") {
				t.Fatalf("a ledger line holds the shell text: %v", l)
			}
		}
	}
}

func TestTouchSet(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt", "b.txt", "history/sittings/2026-09-22-1.md")
	res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
	if res != relay.RedResult || !reflect.DeepEqual(g.Outside, []string{"b.txt"}) {
		t.Fatalf("Run = %s, outside %q; want red naming b.txt only", res, g.Outside)
	}
	var named bool
	for _, l := range w.ledger.Written() {
		if l["event"] == "touch" && reflect.DeepEqual(l["paths"], []any{"b.txt"}) {
			named = true
		}
	}
	if !named {
		t.Fatalf("no touch line names b.txt: %v", w.ledger.Written())
	}
}

func TestOriginMovedStops(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	// Someone pushes to origin between stage and gate.
	write(t, w.root, "c.txt", "c\n")
	gitIn(t, w.root, "add", ".")
	gitIn(t, w.root, "commit", "-qm", "the owner's")
	gitIn(t, w.root, "push", "-q", "origin", "main")
	gitIn(t, w.root, "fetch", "-q", "origin")
	res, why, _, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
	if res != relay.FailedResult || why != relay.OriginMoved {
		t.Fatalf("Run = %s %s, want failed origin-moved", res, why)
	}
}

func TestAcceptanceSameAsMain(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	fails := "echo broken; exit 3"
	m := gitIn(t, w.root, "rev-parse", "main") // the head the baseline was measured at (R6 §4)
	main := []relay.Check{{MainExit: 3, MainLast: "broken", Status: relay.CheckRed, Head: m},
		{MainExit: 3, MainLast: "broken", Status: relay.CheckRed, Head: m},
		{MainExit: 0, MainLast: "fine", Status: relay.CheckGreen, Head: m}}
	p := pk([]string{"a.txt"},
		packet.Line{Shell: fails, Expect: "as main", Kind: packet.AsMain},
		packet.Line{Shell: fails, Expect: "reads 0", Kind: packet.Reads, Value: "0"},
		packet.Line{Shell: "echo changed", Expect: "as main", Kind: packet.AsMain})
	g := Gather(w.fx, st, p, main, seen)
	var got []string
	for _, c := range g.Lines {
		got = append(got, c.Status)
	}
	want := []string{relay.CheckSameAsMain, relay.CheckRed, relay.CheckRed}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses %q, want %q", got, want)
	}
	if g := Gather(w.fx, st, pk([]string{"a.txt"}, p.Acceptance[0]), nil, seen); g.Lines[0].Status != relay.CheckUnrun {
		t.Fatalf("an as main line with no baseline is %s, want unrun", g.Lines[0].Status)
	}
	if res, _ := relay.Decide(Gather(w.fx, st, pk([]string{"a.txt"}, p.Acceptance[0]), main, seen)); res != relay.GreenResult {
		t.Fatalf("a gate whose only line is same-as-main is %s", res)
	}
}

func TestMergeNeedsGreen(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt", "b.txt")
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	red := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	if _, err := MergeIfGreen(w.fx, w.root, red, "a1", "", nil); err == nil || !strings.Contains(err.Error(), "not green") {
		t.Fatalf("MergeIfGreen on red = %v", err)
	}
	if _, err := MergeIfGreen(w.fx, w.root, relay.Gate{}, "a1", ""); err == nil {
		t.Fatal("MergeIfGreen merged an unrun gate")
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("a refused merge moved the root to %s", head)
	}
	green := Gather(w.fx, st, pk([]string{"a.txt", "b.txt"}), nil, seen)
	sha, err := MergeIfGreen(w.fx, w.root, green, "a1", "test -f b.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != sha || len(strings.Fields(gitIn(t, w.root, "rev-list", "--parents", "-n1", "HEAD"))) != 3 {
		t.Fatalf("the merge %s is not the root's head %s, or not a merge commit", sha, head)
	}
}

func TestMergeAbortOnRedVerify(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	branch := gitIn(t, w.root, "rev-parse", "a1")
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	if _, err := MergeIfGreen(w.fx, w.root, green, "a1", "echo nope; exit 1", nil); err == nil {
		t.Fatal("MergeIfGreen committed over a red verify")
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("the root is at %s, want its pre-merge %s", head, before)
	}
	if s := gitIn(t, w.root, "status", "--porcelain", "--untracked-files=no"); s != "" {
		t.Fatalf("the root is dirty after the abort: %q", s)
	}
	if b := gitIn(t, w.root, "rev-parse", "a1"); b != branch {
		t.Fatalf("the branch moved to %s", b)
	}
	if l := w.ledger.Written(); l[len(l)-1]["result"] != "aborted" {
		t.Fatalf("no aborted merge line: %v", l[len(l)-1])
	}
}

func TestGateTempWorktreeRemoved(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	// Only the branch remains: its worktree is gone, so the hand gate
	// makes its own.
	gitIn(t, w.root, "worktree", "remove", st.Path)
	for _, line := range []string{"cat a.txt", "exit 1"} {
		row := queue.Row{ID: "a1", Base: "main", Packet: pk([]string{"a.txt"},
			packet.Line{Shell: line, Expect: "reads `a1 a.txt`", Kind: packet.Reads, Value: "a1 a.txt"})}
		res, _, g, err := Hand(w.fx, w.root, Verify{}, row)
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string]relay.Result{"cat a.txt": relay.GreenResult, "exit 1": relay.RedResult}[line]; res != want {
			t.Fatalf("%s: Hand = %s, want %s; %+v", line, res, want, g)
		}
		if l := gitIn(t, w.root, "worktree", "list"); strings.Contains(l, "gate-") {
			t.Fatalf("%s: a temporary worktree survived:\n%s", line, l)
		}
		if _, err := os.Stat(filepath.Join(w.root, ".claude", "worktrees", "gate-a1")); !os.IsNotExist(err) {
			t.Fatalf("%s: gate-a1 is still on disk: %v", line, err)
		}
	}
	// The ledger's stage line gives the hand gate head_before and origin;
	// exit is still skipped, and so is verify: the row has no verify line.
	var skipped []any
	for _, l := range w.ledger.Written() {
		if l["event"] == "gate" && l["status"] == relay.CheckSkipped {
			skipped = append(skipped, l["check"])
		}
	}
	if !reflect.DeepEqual(skipped, []any{"exit", "verify", "exit", "verify"}) {
		t.Fatalf("skipped checks %v, want exit and verify twice", skipped)
	}
	if _, _, _, err := Hand(w.fx, w.root, Verify{}, queue.Row{ID: "nosuch", Packet: pk([]string{"a.txt"})}); err == nil {
		t.Fatal("Hand gated a row with no branch")
	}
}

// TestAR4_MergeVerifyAsMain, as ADV6-5 left it: a verify line red on main
// never merges. With no baseline a red verify aborts; one red on main at the
// root's head refuses the merge before it starts (`branch-only`, main red),
// whatever the merged tree's last line; one measured at another head is no
// anchor, and the red merged tree aborts: the verify passes green only.
func TestAR4_MergeVerifyAsMain(t *testing.T) {
	w := newWorld(t)
	st := w.stage(t, "a.txt")
	before := gitIn(t, w.root, "rev-parse", "HEAD")
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	verify := "echo 2 lint findings; exit 2"
	if _, err := MergeIfGreen(w.fx, w.root, green, "a1", verify, nil); err == nil {
		t.Fatal("no baseline: a red verify merged")
	}
	for _, last := range []string{"1 lint finding", "2 lint findings"} {
		red := &relay.Check{Name: verify, MainExit: 2, MainLast: last, Status: relay.CheckRed, Head: before}
		if _, err := MergeIfGreen(w.fx, w.root, green, "a1", verify, red); err == nil || !strings.Contains(err.Error(), "main red") {
			t.Fatalf("main red (last %q): %v", last, err)
		}
	}
	stale := &relay.Check{Name: verify, MainExit: 2, MainLast: "2 lint findings", Status: relay.CheckRed, Head: "0000000"}
	if _, err := MergeIfGreen(w.fx, w.root, green, "a1", verify, stale); err == nil || strings.Contains(err.Error(), "main red") {
		t.Fatalf("a stale anchor: %v, want an aborted merge", err)
	}
	if head := gitIn(t, w.root, "rev-parse", "HEAD"); head != before {
		t.Fatalf("a refused merge moved the root to %s", head)
	}
	sha, err := MergeIfGreen(w.fx, w.root, green, "a1", "true", stale)
	if err != nil {
		t.Fatal(err)
	}
	if msg := gitIn(t, w.root, "log", "-1", "--format=%s"); sha == before || !strings.Contains(msg, "verify green") {
		t.Fatalf("the merge commit says %q", msg)
	}
}

// TestMain points XDG_STATE_HOME, the one state root (§5), at a temp
// directory before any test, so no test reads or writes the machine's
// ledger, locks or nights.
func TestMain(m *testing.M) {
	// The tests push only into t.TempDir() remotes; the deny is advisory (relay-r1 §10; ADV11-8 deferred).
	effects.DropEnvConfig()
	state, err := os.MkdirTemp("", "relay-state-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Setenv("XDG_STATE_HOME", state)
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
	// AR22 act 1: every gate line and runner git runs under a pinned env,
	// built under a temp dir of its own.
	code := effects.RunPinned(m)
	os.RemoveAll(state)
	os.Exit(code)
}

// TestAR5_MergeOnlyInMergeIfGreen (AR2-16): a scan of every non-test Go file
// under internal/ and cmd/ finds MergeNoCommit called in MergeIfGreen and
// nowhere else, so no path merges without the gate; mergedVerify, which
// MergeIfGreen alone calls after the green check, merges only its own fresh
// tree to run the verify line (AR17A act 2).
func TestAR5_MergeOnlyInMergeIfGreen(t *testing.T) {
	callers := map[string]int{}
	root := copyIn(t, "../..", "internal", "cmd")
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.Contains(path, "testdata") {
				return err
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				ast.Inspect(fd, func(n ast.Node) bool {
					if c, ok := n.(*ast.CallExpr); ok {
						if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "MergeNoCommit" {
							callers[filepath.Base(path)+":"+fd.Name.Name]++
						}
					}
					return true
				})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(callers, map[string]int{"gate.go:MergeIfGreen": 1, "gate.go:mergedVerify": 1}) {
		t.Fatalf("MergeNoCommit callers %v", callers)
	}
}
