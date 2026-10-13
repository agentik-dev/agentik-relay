package hooks

import "testing"

// TestPublicR1_7_NewModulePathBuildDenied (packet public-r1, the observer's
// addendum): the module path moved to github.com/agentik-dev/agentik-relay,
// and the hand-build guard still denies a go build or go install of its
// cmd/relay, by import path as by directory, in a row and from a helper.
func TestPublicR1_7_NewModulePathBuildDenied(t *testing.T) {
	cmds := []string{
		"go build github.com/agentik-dev/agentik-relay/cmd/relay",
		"go build -o x github.com/agentik-dev/agentik-relay/cmd/relay",
		"go install github.com/agentik-dev/agentik-relay/cmd/relay@latest",
		"go install github.com/agentik-dev/agentik-relay/cmd/relay",
		"cd /tmp && go build -o bin/relay github.com/agentik-dev/agentik-relay/cmd/relay",
	}
	for _, cmd := range cmds {
		if got := hookSays(t, cmd, true); got != "nested-run" {
			t.Errorf("in a row, %q = %q, want nested-run", cmd, got)
		}
		if got := helperSays(t, cmd, "a1b2c3d4", "agent-relay:adversary", false); got != "helper-relay" {
			t.Errorf("helper, %q = %q, want helper-relay", cmd, got)
		}
	}
	for _, cmd := range []string{
		"go test github.com/agentik-dev/agentik-relay/internal/hooks -run TestPublicR1_ -count=1",
		"go vet github.com/agentik-dev/agentik-relay/...",
	} {
		if got := hookSays(t, cmd, true); got != "" {
			t.Errorf("in a row, %q denied by %s", cmd, got)
		}
	}
}
