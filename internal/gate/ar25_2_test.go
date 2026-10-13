package gate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR25_2_RestoredBeforeMerge is ADV15-2 (review test 3): a fake
// GOROOT file written and restored (the same bytes, a new ctime) after
// the gate's Moved reads unmoved on the fast path and moved on the
// merge's strict read, so the merge is refused tool-moved. A touch the
// gate's own Moved read (re-hashed, equal) is its record: the merge
// after it is not refused.
func TestAR25_2_RestoredBeforeMerge(t *testing.T) {
	old, _ := effects.Current()
	tools := effects.PinTools
	t.Cleanup(func() { effects.Use(old); effects.PinTools = tools })
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src", "testing"), 0o755)
	write(t, root, "src/testing/testing.go", "package testing\n")
	src := filepath.Join(root, "src", "testing", "testing.go")
	fakeAhead(t, map[string]string{"go": "[ \"$1 $2\" = \"env GOROOT\" ] && { echo " + root + "; exit 0; }\necho v1"})
	effects.PinTools = []string{"go"}
	p, err := effects.PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })
	restore := func() {
		time.Sleep(10 * time.Millisecond) // a ctime the clock can tell apart
		os.WriteFile(src, []byte("package testing\n\nfunc init() { panic(1) }\n"), 0o644)
		os.WriteFile(src, []byte("package testing\n"), 0o644)
	}
	w := hermeticWorld(t)
	line := pk([]string{"b.txt"}, packet.Line{Shell: "go test", Expect: "reads `v1`", Kind: packet.Reads, Value: "v1"})
	restore() // before the gate: its Moved re-hashes, reads equal, records the stat
	st := w.stage(t, "b.txt")
	res, why, g, _ := Run(w.fx, st, line, nil, seen, Verify{})
	if res != relay.GreenResult {
		t.Fatalf("Run = %s %q; lines %+v", res, why, g.Lines)
	}
	if moved := p.MovedStrict(); len(moved) != 0 {
		t.Fatalf("MovedStrict right after the gate's read = %v", moved)
	}
	restore() // after the gate: the bytes equal, the ctime new
	if moved := p.MovedStrict(); !slices.Equal(moved, []string{"go"}) {
		t.Fatalf("MovedStrict after a restore = %v, want [go]", moved)
	}
	if _, err := MergeIfGreen(w.fx, w.root, g, "a1", ""); err == nil || !strings.Contains(err.Error(), string(relay.ToolMoved)) || w.merged() {
		t.Fatalf("the merge after a write restored since the gate: %v", err)
	}
	// The fast path alone reads it unmoved (equal bytes): the finding. Its
	// read is the record a later strict read compares with.
	if moved := p.Moved(); len(moved) != 0 {
		t.Fatalf("the fast Moved after a restore = %v, want none (equal bytes)", moved)
	}
	if moved := p.MovedStrict(); len(moved) != 0 {
		t.Fatalf("MovedStrict after the fast read's record = %v", moved)
	}
}
