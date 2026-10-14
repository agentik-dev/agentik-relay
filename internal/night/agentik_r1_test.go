package night_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/hooks"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAgentikR1_3_PidDirNewElseOld (agentik-r1 §6.3 c2; ruling 3): the
// pid dir under the linked home is agentik-relay when it exists, else
// agent-relay when it exists, else agentik-relay. The package vars are set
// in-process and restored: no binary is built or run.
func TestAgentikR1_3_PidDirNewElseOld(t *testing.T) {
	oldPort, oldHome := night.LockPortLink, night.PidHomeLink
	t.Cleanup(func() { night.LockPortLink, night.PidHomeLink = oldPort, oldHome })
	home := t.TempDir()
	night.LockPortLink, night.PidHomeLink = "29872", home
	state := filepath.Join(home, ".local", "state")
	nw, old := filepath.Join(state, "agentik-relay"), filepath.Join(state, "agent-relay")
	for _, step := range []struct{ mk, want string }{{"", nw}, {old, old}, {nw, nw}} {
		if step.mk != "" {
			if err := os.MkdirAll(step.mk, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := night.PidDir(); err != nil || got != step.want {
			t.Errorf("after %q: PidDir = %q, %v; want %q", step.mk, got, err, step.want)
		}
	}
}

// TestAgentikR1_3_HooksAndRelayAgree (ruling 3): the soft request the relay
// writes (hooks.SoftPath) lies under the state dir it reads
// (effects.StateDir) whichever spelling exists.
func TestAgentikR1_3_HooksAndRelayAgree(t *testing.T) {
	base := t.TempDir()
	env := func(k string) string {
		if k == "XDG_STATE_HOME" {
			return base
		}
		return ""
	}
	for _, mk := range []string{"", "agent-relay", "agentik-relay"} {
		if mk != "" {
			if err := os.Mkdir(filepath.Join(base, mk), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		soft, dir := hooks.SoftPath(env, "s1"), effects.StateDir(env)
		if !strings.HasPrefix(soft, dir+string(filepath.Separator)) {
			t.Errorf("after %q: SoftPath %q is not under StateDir %q", mk, soft, dir)
		}
	}
	if got := effects.StateDir(env); got != filepath.Join(base, "agentik-relay") {
		t.Errorf("both: StateDir = %q", got)
	}
}
