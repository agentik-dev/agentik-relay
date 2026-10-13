package gate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR24_0_ToolchainTree is ADV14-2 (review test 1): the pinned go's
// launcher is bound, and so is the GOROOT it names (a fake go over a
// small tree, never the account's). One byte changed under src/testing
// (the review's `Fail` returns) and Moved names go: the next gate is
// unrun, tool-moved, and the merge refused.
func TestAR24_0_ToolchainTree(t *testing.T) {
	old, _ := effects.Current()
	tools := effects.PinTools
	t.Cleanup(func() { effects.Use(old); effects.PinTools = tools })
	root := t.TempDir()
	src := filepath.Join(root, "src", "testing", "testing.go")
	os.MkdirAll(filepath.Dir(src), 0o755)
	os.MkdirAll(filepath.Join(root, "pkg", "tool"), 0o755)
	write(t, root, "src/testing/testing.go", "package testing\n\nfunc (c *common) Fail() { c.failed = true }\n")
	write(t, root, "pkg/tool/compile", "compile")
	fakeAhead(t, map[string]string{"go": "[ \"$1 $2\" = \"env GOROOT\" ] && { echo " + root + "; exit 0; }\necho v1"})
	effects.PinTools = []string{"go"}
	p, err := effects.PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })
	if !slices.Contains(p.Env, "GOROOT="+root) {
		t.Fatalf("the pinned env %v fixes no GOROOT=%s", p.Env, root)
	}
	w := hermeticWorld(t)
	st := w.stage(t, "b.txt")
	line := pk([]string{"b.txt"}, packet.Line{Shell: "go test", Expect: "reads `v1`", Kind: packet.Reads, Value: "v1"})
	res, why, g, _ := Run(w.fx, st, line, nil, seen, Verify{})
	if res != relay.GreenResult || len(p.Moved()) != 0 {
		t.Fatalf("Run with GOROOT as pinned = %s %q, moved %v; lines %+v", res, why, p.Moved(), g.Lines)
	}
	b, _ := os.ReadFile(src)
	os.WriteFile(src, []byte(strings.Replace(string(b), "true", "fals", 1)), 0o644)
	if moved := p.Moved(); !slices.Equal(moved, []string{"go"}) {
		t.Fatalf("Moved after a byte under GOROOT/src/testing = %v, want [go]", moved)
	}
	if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || !strings.Contains(err.Error(), string(relay.ToolMoved)) || w.merged() {
		t.Fatalf("the merge after GOROOT moved: %v", err)
	}
	if res, why, _, _ = Run(w.fx, st, line, nil, seen, Verify{}); res != relay.Unrun || why != relay.ToolMoved {
		t.Fatalf("Run after GOROOT moved = %s %q, want unrun tool-moved", res, why)
	}
}
