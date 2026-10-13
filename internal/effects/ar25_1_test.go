package effects

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestAR25_1_UserSiteOff (ADV15-3): the pinned env carries
// PYTHONNOUSERSITE=1, so the pinned python3 reads no user site: a .pth or
// usercustomize.py in the relay home runs at no gate. Skipped without a
// python3 the pin reaches.
func TestAR25_1_UserSiteOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { PinTools = tools; Use(old) })
	PinTools = []string{"python3"}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := PinAt(filepath.Join(dir, "state"), filepath.Join(dir, "account"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()
	if !slices.Contains(p.Env, "PYTHONNOUSERSITE=1") {
		t.Errorf("the pinned env has no PYTHONNOUSERSITE=1: %q", p.Env)
	}
	if _, ok := p.Tools["python3"]; !ok {
		t.Skip("no python3 the pin reaches")
	}
	exit, last, err := OSShell{}.Run(dir, `python3 -c 'import site; print(site.ENABLE_USER_SITE)'`, time.Minute)
	if err != nil || exit != 0 || last != "False" {
		t.Fatalf("the pinned python3's ENABLE_USER_SITE = %q (exit %d, %v), want False", last, exit, err)
	}
}
