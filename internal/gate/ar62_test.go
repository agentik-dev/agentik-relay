package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar62Green is a gathered gate every check passes: one green line.
func ar62Green() relay.Gate {
	return relay.Gate{Gathered: true, ResultSeen: true, ExitOK: true, Commit: true, Clean: true, OriginSame: true,
		Lines: []relay.Check{{Status: relay.CheckGreen, Expect: "reads 1"}}}
}

// AR62 act 1: Cause names each check Red names, in its order, once, with
// its first fact; "" when Red names none.
func TestAR62_1_Cause(t *testing.T) {
	red := func(last string) relay.Check {
		return relay.Check{Status: relay.CheckRed, BranchExit: 1, BranchLast: last, Expect: "reads 1"}
	}
	green := relay.Check{Status: relay.CheckGreen, Expect: "reads 1"}
	cases := []struct {
		name string
		edit func(*relay.Gate)
		want string
	}{
		{"ungathered", func(g *relay.Gate) { *g = relay.Gate{Outside: []string{"x"}} }, ""},
		{"green", func(g *relay.Gate) {}, ""},
		{"branch", func(g *relay.Gate) { g.OffBranch = true }, "branch: off the row's branch"},
		{"no result", func(g *relay.Gate) { g.ResultSeen = false }, "exit: no result event"},
		{"is_error", func(g *relay.Gate) { g.ExitOK = false }, "exit: is_error"},
		{"commit", func(g *relay.Gate) { g.Commit = false }, "commit: none"},
		{"clean 1", func(g *relay.Gate) { g.Clean, g.Dirty = false, 1 }, "clean: 1 path"},
		{"clean 3", func(g *relay.Gate) { g.Clean, g.Dirty = false, 3 }, "clean: 3 paths"},
		{"origin", func(g *relay.Gate) { g.OriginSame = false }, "origin: a remote moved"},
		{"touch 1", func(g *relay.Gate) { g.Outside = []string{"b.txt"} }, "touch: b.txt outside may touch"},
		{"touch 3", func(g *relay.Gate) { g.Outside = []string{"b.txt", "c.txt", "d.txt"} },
			"touch: b.txt outside may touch (+2 more)"},
		{"charter", func(g *relay.Gate) { g.NoCharter = true }, "charter: none"},
		{"queue", func(g *relay.Gate) { g.QueueInvalid = true }, "queue: invalid at the head"},
		{"merges", func(g *relay.Gate) { g.Unproven = true }, "merges: unproven"},
		{"packet-source", func(g *relay.Gate) { g.PacketDiffers = true }, "packet-source: differs at the head"},
		{"acceptance 1", func(g *relay.Gate) { g.Lines = []relay.Check{red("no")} },
			`acceptance 1: last "no", want reads 1`},
		{"acceptance unrun", func(g *relay.Gate) { g.Lines = []relay.Check{{Status: relay.CheckUnrun, Expect: "reads 1"}} },
			"acceptance 1: unrun"},
		{"acceptance 3 and 5", func(g *relay.Gate) {
			g.Lines = []relay.Check{green, green, red("three"), green, red("five")}
		}, `acceptance 3: last "three", want reads 1 (+1 more)`},
		{"verify cut", func(g *relay.Gate) { g.Verify = &relay.Check{Status: relay.CheckRed, BranchExit: 124, Cut: 600} },
			"verify: exit 124, cut at 600 s"},
		{"verify red", func(g *relay.Gate) {
			g.Verify = &relay.Check{Status: relay.CheckRed, BranchExit: 2, BranchLast: "FAIL"}
		},
			`verify: exit 2, last "FAIL"`},
		{"verify unrun", func(g *relay.Gate) { g.Verify = &relay.Check{Status: relay.CheckUnrun} }, "verify: unrun"},
		{"main-moved", func(g *relay.Gate) { g.MainMoved, g.Main = true, "0123456789abcdef0123" },
			"main-moved: main at 0123456789ab"},
		{"orphans", func(g *relay.Gate) { g.Orphans = 2 }, "orphans: 2 swept"},
		{"clean before touch", func(g *relay.Gate) {
			g.Outside, g.Clean, g.Dirty = []string{"b.txt"}, false, 2
			g.Lines = []relay.Check{red("no")}
		}, `clean: 2 paths; touch: b.txt outside may touch; acceptance 1: last "no", want reads 1`},
	}
	for _, c := range cases {
		g := ar62Green()
		c.edit(&g)
		if got := Cause(g); got != c.want {
			t.Errorf("%s: Cause = %q, want %q", c.name, got, c.want)
		}
		if strings.Contains(Cause(g), "(+0 more)") {
			t.Errorf("%s: a (+0 more): %q", c.name, Cause(g))
		}
	}

	// A 300-rune last is clipped to 60, and the line to 200.
	long := strings.Repeat("é", 300)
	g := ar62Green()
	g.Lines = []relay.Check{red(long)}
	got := Cause(g)
	if want := `acceptance 1: last "` + strings.Repeat("é", 57) + `...", want reads 1`; got != want {
		t.Errorf("clipped last: Cause = %q, want %q", got, want)
	}
	g.Outside = []string{long}
	g.Verify = &relay.Check{Status: relay.CheckRed, BranchExit: 2, BranchLast: long}
	g.Clean, g.Dirty = false, 9
	got = Cause(g)
	if n := utf8.RuneCountInString(got); n > 200 || !strings.HasSuffix(got, "...") {
		t.Errorf("the line is %d runes, want at most 200, clipped: %q", n, got)
	}
}

// AR62 act 1: the gate's end line gains `cause`; `reason` stays the stop
// reason.
func TestAR62_2_EndLineCause(t *testing.T) {
	end := func(t *testing.T, w *world) map[string]any {
		t.Helper()
		var last map[string]any
		for _, l := range w.ledger.Written() {
			if _, ok := l["result"]; ok && l["event"] == "gate" {
				last = l
			}
		}
		if last == nil {
			t.Fatalf("no end line: %v", w.ledger.Written())
		}
		return last
	}

	// TestTouchSet's world: red on touch, no reason, the cause names b.txt.
	w := newWorld(t)
	st := w.stage(t, "a.txt", "b.txt", "history/sittings/2026-09-22-1.md")
	Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
	if e := end(t, w); e["result"] != "red" || e["reason"] != "" || e["cause"] != "touch: b.txt outside may touch" {
		t.Errorf("touch: end line %v", e)
	}

	// TestGateGreen's world: no cause.
	w = newWorld(t)
	st = w.stage(t, "a.txt", "history/sittings/2026-09-22-1.md")
	p := pk([]string{"a.txt"}, packet.Line{Shell: "cat a.txt", Expect: "reads `a1 a.txt`", Kind: packet.Reads, Value: "a1 a.txt"})
	Run(w.fx, st, p, nil, seen, Verify{})
	if e := end(t, w); e["result"] != "green" {
		t.Errorf("green: end line %v", e)
	} else if _, ok := e["cause"]; ok {
		t.Errorf("green: the end line carries a cause: %v", e)
	}

	// An untracked file: dirty-tree, one path.
	w = newWorld(t)
	st = w.stage(t, "a.txt", "history/sittings/2026-09-22-1.md")
	if err := os.WriteFile(filepath.Join(w.tree, "stray.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, Verify{})
	if e := end(t, w); e["reason"] != "dirty-tree" || e["cause"] != "clean: 1 path" {
		t.Errorf("dirty: end line %v", e)
	}
}

// TestAR62Fix_3_CauseRewritesBeforeClip: Cause writes the row's root `.`
// and the home `~` before it clips a last line (round one's F1), so no
// clip leaves a fragment of either.
func TestAR62Fix_3_CauseRewritesBeforeClip(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	root := "/home/u/src/repo"
	g := ar62Green()
	g.Worktree = root + "/.claude/worktrees/a1"
	g.Lines = []relay.Check{{Status: relay.CheckRed, BranchExit: 1, Expect: "reads 1",
		BranchLast: "FAIL " + strings.Repeat("x", 40) + " " + g.Worktree + "/x_test.go:3"}}
	g.Verify = &relay.Check{Status: relay.CheckRed, BranchExit: 2,
		BranchLast: strings.Repeat("z", 45) + " /home/u/.cache/go-build/ab"}
	got := Cause(g)
	if strings.Contains(got, "/home/u") || !strings.Contains(got, " ./.claude/") || !strings.Contains(got, " ~/.cache/") {
		t.Fatalf("Cause = %q; want the root `.` and the home `~`, no fragment of either", got)
	}
}
