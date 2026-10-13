package effects

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestAR24_2_GoworkOff is ADV14-3: the pinned env carries GOWORK=off, so
// a go.work above a gate line's dir (a sitting's ~/go.work) is never
// read: `go env GOWORK` there reads `off`.
func TestAR24_2_GoworkOff(t *testing.T) {
	p, ok := Current()
	if !ok || !slices.Contains(p.Env, "GOWORK=off") {
		t.Fatalf("the pinned env %v lacks GOWORK=off", p.Env)
	}
	if _, ok := p.Tools["go"]; !ok {
		t.Skip("no go pinned")
	}
	top := t.TempDir()
	os.WriteFile(filepath.Join(top, "go.work"), []byte("go 1.26\n\nuse ./nowhere\n"), 0o644)
	dir := filepath.Join(top, "clone")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.26\n"), 0o644)
	if exit, last, err := (OSShell{}).Run(dir, "go env GOWORK", 30*time.Second); err != nil || exit != 0 || last != "off" {
		t.Fatalf("go env GOWORK under a go.work = %q (exit %d, %v), want off", last, exit, err)
	}
}
