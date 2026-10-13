package effects

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestAR21_0_IdentityOnceRunnerNoGlobal is AR21 act 0 (ADV12-1) at the
// runner: Identity reads the global config once, and a later write to it
// (a child's) changes nothing; the runner itself never reads it.
func TestAR21_0_IdentityOnceRunnerNoGlobal(t *testing.T) {
	for in, want := range map[string][2]string{
		"Owner Name <o@x> 1790000000 -0700": {"Owner Name", "o@x"},
		"<o@x> 1 +0000":                     {"", "o@x"},
		"no ident":                          {"", ""},
	} {
		if n, e := ParseIdent(in); n != want[0] || e != want[1] {
			t.Errorf("ParseIdent(%q) = %q %q, want %q", in, n, e, want)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, k := range []string{"GIT_CONFIG_GLOBAL", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	gc := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gc, []byte("[user]\n\tname = Owner\n\temail = owner@x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity.once = sync.Once{}
	t.Cleanup(func() { identity.once = sync.Once{} })
	if n, e := Identity(); n != "Owner" || e != "owner@x" {
		t.Fatalf("Identity = %q %q, want the global config's", n, e)
	}
	os.WriteFile(gc, []byte("[user]\n\tname = Child\n\temail = child@x\n"), 0o644)
	if n, e := Identity(); n != "Owner" || e != "owner@x" {
		t.Fatalf("Identity read again: %q %q", n, e)
	}
	if got, err := git(home, "config", "--get", "user.name"); err == nil || got != "" {
		t.Fatalf("the runner read the global config: %q, %v", got, err)
	}
}
