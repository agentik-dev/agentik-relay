package night_test

import (
	"path/filepath"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR36_2_LinkedPortNeedsLinkedHome (AR36 ruling 2): a binary linked
// with LockPortLink and without PidHomeLink refuses to resolve its pid
// dir, so holdPid exits 2 before it takes anything; linked with both, the
// pid dir is under the linked home. The package vars are set in-process
// and restored: no binary is built or run.
func TestAR36_2_LinkedPortNeedsLinkedHome(t *testing.T) {
	oldPort, oldHome := night.LockPortLink, night.PidHomeLink
	t.Cleanup(func() { night.LockPortLink, night.PidHomeLink = oldPort, oldHome })

	night.LockPortLink, night.PidHomeLink = "29871", ""
	dir, err := night.PidDir()
	const want = "a linked lock port (LockPortLink) needs a linked pid home (PidHomeLink): refusing the account's real pid dir"
	if err == nil || err.Error() != want {
		t.Fatalf("PidDir with a linked port and no linked home = %q, %v; want the refusal %q", dir, err, want)
	}
	if dir != "" {
		t.Errorf("the refusal names a dir: %q", dir)
	}

	home := t.TempDir()
	night.PidHomeLink = home
	dir, err = night.PidDir()
	if err != nil || dir != filepath.Join(home, ".local", "state", "agentik-relay") {
		t.Errorf("PidDir with both links = %q, %v; want under %s", dir, err, home)
	}
}
