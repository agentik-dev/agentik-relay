package effects

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestAR27A_1_StaticcheckBound is AR27A act 1 (ADV16-5): staticcheck is
// one of PinTools and of GoBinTools, so a fixture staticcheck first on
// the start PATH is bound like go (its link in the relay home's bin/, its
// sha256 read before each gate), and a scratched line's GOBIN is the
// scratch's bin/, holding a link to the bound one: `go env GOBIN` prints
// <scratch>/bin and <that>/staticcheck runs the fixture. The fixture
// rewritten, Moved names it. Skipped without a go the pin reaches.
func TestAR27A_1_StaticcheckBound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if !slices.Contains(PinTools, "staticcheck") || !slices.Contains(GoBinTools, "staticcheck") {
		t.Errorf("PinTools %q, GoBinTools %q: want staticcheck in both", PinTools, GoBinTools)
	}
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { PinTools = tools; Use(old) })
	bin := t.TempDir()
	fixture := filepath.Join(bin, "staticcheck")
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\necho 'staticcheck 2026.2.1 (ar27a fixture)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	PinTools = []string{"go", "gofmt", "staticcheck"}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := PinAt(filepath.Join(dir, "state"), filepath.Join(dir, "account"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()
	if _, ok := p.Tools["go"]; !ok {
		t.Skip("no go the pin reaches")
	}
	want, _ := filepath.EvalSymlinks(fixture)
	if p.Tools["staticcheck"] != want {
		t.Fatalf("Tools[staticcheck] = %q, want the fixture first on PATH, %q", p.Tools["staticcheck"], want)
	}
	sh, done, err := Scratched(OSShell{})
	if err != nil {
		t.Fatal(err)
	}
	scratch := sh.(OSShell).Scratch
	gobin := filepath.Join(scratch, "bin")
	if to, err := os.Readlink(filepath.Join(gobin, "staticcheck")); err != nil || to != filepath.Join(p.Home, "bin", "staticcheck") {
		t.Errorf("<scratch>/bin/staticcheck -> %q (%v), want the relay home's bin/staticcheck", to, err)
	}
	for _, c := range []struct{ line, want string }{
		{"go env GOBIN", gobin},
		{`"$(go env GOBIN)/staticcheck" --version`, "staticcheck 2026.2.1 (ar27a fixture)"},
	} {
		exit, last, err := sh.Run(dir, c.line, time.Minute)
		if err != nil || exit != 0 || last != c.want {
			t.Errorf("%s: %q (exit %d, %v), want %q", c.line, last, exit, err, c.want)
		}
	}
	done()
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Errorf("the scratch home %s outlived its func: %v", scratch, err)
	}
	if moved := p.Moved(); len(moved) != 0 {
		t.Fatalf("Moved = %q before any change", moved)
	}
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\necho 'staticcheck 0.0.0 (moved)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if moved := p.Moved(); !slices.Contains(moved, "staticcheck") {
		t.Errorf("Moved = %q after the fixture changed, want staticcheck", moved)
	}
}
