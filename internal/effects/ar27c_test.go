package effects

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAR27C_2_ExeFromLsof (ADV16-3): this process's Exe, read by /proc
// where it exists or by the pinned lsof, resolves to os.Executable()'s
// real file; a pid that is no process has none. Skipped without lsof.
func TestAR27C_2_ExeFromLsof(t *testing.T) {
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, t.TempDir())
	}
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { Use(old); PinTools = tools })
	PinTools = nil
	p, err := PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })
	if _, err := os.Stat("/proc/self/exe"); err != nil && p.Lsof == "" {
		t.Skip("no /proc and no lsof in a pinned dir")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(self)
	exe, err := OSProcs{}.Exe(os.Getpid())
	if err != nil {
		t.Fatalf("Exe(self): %v", err)
	}
	if got, _ := filepath.EvalSymlinks(exe); got != want {
		t.Fatalf("Exe(self) = %q (%q); want %q", exe, got, want)
	}
	if exe, err := (OSProcs{}).Exe(1 << 30); err == nil {
		t.Fatalf("a pid that is no process has an executable %q", exe)
	}
}
