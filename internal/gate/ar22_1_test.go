package gate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// fakeAhead writes each of bodies as an executable script in a temp dir
// and puts that dir first on this process's PATH: the account-writable
// dir a sitting writes (ADV13-2's `~/.local/bin`).
func fakeAhead(t *testing.T, bodies map[string]string) {
	t.Helper()
	bin := t.TempDir()
	for name, body := range bodies {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestAR22_1_PinnedEnv is ADV13-2 and -3 (review test 2): a fake `make`
// and `cat` ahead on PATH, and `go env -w GOFLAGS=-run=^$` in the temp
// HOME, each alone turns a red gate green when a line reads the relay's
// env. Every gate line runs under the pinned env (the root-owned dirs'
// make and cat, GOENV=off, a relay home): Run is never green, and
// MergeIfGreen never merges.
func TestAR22_1_PinnedEnv(t *testing.T) {
	t.Run("make and cat ahead on PATH", func(t *testing.T) {
		w := hermeticWorld(t) // main's check holds a.txt to `a`
		st := w.stage(t, "a.txt")
		fakeAhead(t, map[string]string{"make": "exit 0", "cat": "echo a"})
		p := pk([]string{"a.txt"}, packet.Line{Shell: "cat a.txt", Expect: "reads `a`", Kind: packet.Reads, Value: "a"})
		res, why, g, _ := Run(w.fx, st, p, nil, seen, Verify{Name: "auto"})
		if res == relay.GreenResult {
			t.Fatalf("Run = green %q: a PATH the sitting writes judged the gate; lines %+v, verify %+v", why, g.Lines, g.Verify)
		}
		if _, err := MergeIfGreen(w.fx, w.root, g, "a1", "auto"); err == nil || w.merged() {
			t.Fatalf("merged a.txt = `a1 a.txt` through a fake make and cat: %v", err)
		}
	})
	t.Run("GOFLAGS in the HOME's go env", func(t *testing.T) {
		gobin, err := exec.LookPath("go")
		if err != nil {
			t.Skip("no go")
		}
		w := hermeticWorld(t) // HOME and XDG_STATE_HOME are temp dirs
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		for _, k := range []string{"GOENV", "GOFLAGS"} {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}
		if out, err := exec.Command(gobin, "env", "-w", "GOFLAGS=-run=^$").CombinedOutput(); err != nil {
			t.Fatalf("go env -w: %v\n%s", err, out)
		}
		if out, _ := exec.Command(gobin, "env", "GOFLAGS").Output(); strings.TrimSpace(string(out)) != "-run=^$" {
			t.Fatalf("the temp HOME's go env holds GOFLAGS=%q, want -run=^$", out)
		}
		st := w.stage(t, "b.txt")
		write(t, w.tree, "go.mod", "module x\n\ngo 1.21\n")
		write(t, w.tree, "x_test.go", "package x\n\nimport \"testing\"\n\nfunc TestFails(t *testing.T) { t.Fatal(\"red\") }\n")
		gitIn(t, w.tree, "add", ".")
		gitIn(t, w.tree, "commit", "-qm", "a failing test")
		res, why, g, _ := Run(w.fx, st, pk([]string{"b.txt", "go.mod", "x_test.go"}), nil, seen, Verify{Name: "go test ./..."})
		if res == relay.GreenResult {
			t.Fatalf("Run = green %q: the HOME's go env ran no test; verify %+v", why, g.Verify)
		}
		if g.Verify == nil || g.Verify.Status != relay.CheckRed || !strings.HasPrefix(g.Verify.BranchLast, "FAIL") {
			t.Fatalf("verify = %+v, want red by the failing test (the pinned go, GOENV=off)", g.Verify)
		}
	})
}

// TestAR22_1_ToolMoved is AR22 act 1 (ADV13-2): an account tool the
// pinned PATH reaches is bound by its sha256 at start; the file rewritten,
// its link in the relay home retargeted, or an entry added to bin/, and
// the next gate is unrun, tool-moved, and the merge refused.
func TestAR22_1_ToolMoved(t *testing.T) {
	old, _ := effects.Current()
	tools := effects.PinTools
	t.Cleanup(func() { effects.Use(old); effects.PinTools = tools })
	fakeAhead(t, map[string]string{"ar22tool": "echo v1"})
	tool, err := exec.LookPath("ar22tool")
	if err != nil {
		t.Fatal(err)
	}
	// bin/ is first on the pinned PATH, so an entry added there (a `make`)
	// would be every line's make: Moved names it.
	for _, c := range []struct {
		name, want string
		move       func(p effects.Pinned)
	}{
		{"rewritten", "ar22tool", func(effects.Pinned) { os.WriteFile(tool, []byte("#!/bin/sh\necho a\n"), 0o755) }},
		{"relinked", "ar22tool", func(p effects.Pinned) {
			link := filepath.Join(p.Home, "bin", "ar22tool")
			os.Remove(link)
			os.Symlink("/bin/echo", link)
		}},
		{"added make", "make", func(p effects.Pinned) { os.Symlink("/usr/bin/true", filepath.Join(p.Home, "bin", "make")) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			os.WriteFile(tool, []byte("#!/bin/sh\necho v1\n"), 0o755)
			effects.PinTools = []string{"ar22tool"}
			p, err := effects.PinAt(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { p.Release() })
			w := hermeticWorld(t)
			st := w.stage(t, "b.txt")
			line := pk([]string{"b.txt"}, packet.Line{Shell: "ar22tool", Expect: "reads `v1`", Kind: packet.Reads, Value: "v1"})
			res, why, g, _ := Run(w.fx, st, line, nil, seen, Verify{})
			if res != relay.GreenResult || len(p.Moved()) != 0 {
				t.Fatalf("Run with the tool as pinned = %s %q, moved %v; lines %+v", res, why, p.Moved(), g.Lines)
			}
			c.move(p)
			if moved := p.Moved(); len(moved) != 1 || moved[0] != c.want {
				t.Fatalf("Moved = %v, want [%s]", moved, c.want)
			}
			if !strings.HasPrefix(p.Make, "/") || strings.HasPrefix(p.Make, p.Home) {
				t.Fatalf("the pinned make %q moved with bin/", p.Make)
			}
			if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || !strings.Contains(err.Error(), string(relay.ToolMoved)) || w.merged() {
				t.Fatalf("the merge after a moved tool: %v", err)
			}
			res, why, _, _ = Run(w.fx, st, line, nil, seen, Verify{})
			if res != relay.Unrun || why != relay.ToolMoved {
				t.Fatalf("Run after the tool moved = %s %q, want unrun tool-moved", res, why)
			}
		})
	}
}
