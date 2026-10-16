package hooks

import "testing"

// ar89Evasions are the AR40 review's F3 helper evasions (design
// guard-parser-r1, "Why a parser at all") and their twins under the relay's
// other names (agentik relay, agentik-relay): a relay verb that writes,
// reached by a command word the guard cannot read, or a night's process
// signalled by its name. Each is denied for a helper (AR89 ruling 1).
var ar89Evasions = []string{
	// F3's four, as recorded
	"$(which relay) stop",
	"`which relay` stop",
	"r=relay; $r stop",
	"pkill -f 'relay run'",
	// the front door's twins
	"$(which agentik) relay stop",
	"`which agentik` relay stop",
	"a=agentik; $a relay stop",
	"r='agentik relay'; $r stop",
	"pkill -f 'agentik relay run'",
	// the binary's own name
	"$(which agentik-relay) stop",
	"`which agentik-relay` kill 2026-10-16-1",
	"pkill -f 'agentik-relay run'",
	// the same shapes with the other writing verbs and spellings
	"$(which relay) --json clean",
	"$(command -v relay) run --orchestrate",
	"\"$(which relay)\" gate 2026-10-16-1",
	"r=relay; \"$r\" resume 2026-10-16-1",
	"${RELAY_BIN} plan --rows a",
	"agentik $(echo relay) stop",
	"pkill relay",
	"pkill -9 -f relay",
	"killall relay",
	"killall agentik-relay",
	"cd /tmp && pkill -f 'relay run'",
}

// ar89Passes are a helper's ordinary lines that hold the word relay, and the
// read verbs under a hidden name: none is denied (AR89 rulings 2 and 3).
var ar89Passes = []string{
	"relay status",
	"relay report",
	"relay report --night 2026-10-16-1",
	"agentik relay status",
	"agentik relay report",
	"agentik-relay status",
	"$(which relay) status",
	"`which agentik` relay report",
	"r=relay; $r status",
	"which relay",
	"command -v relay",
	"echo $(which relay)",
	"ls -l $(which relay)",
	"grep -rn 'relay stop' design/",
	"grep -n 'pkill -f' internal/hooks/guards.go",
	"rg -n 'relay run' $(git rev-parse --show-toplevel)/design",
	"cat cmd/agentik-relay/main.go",
	"git log --oneline -- cmd/agentik-relay",
	"pgrep -f 'relay run'",
	"ps -ax -o pid,command | grep -v grep | grep 'relay run'",
	"pkill -f 'go test ./internal/hooks'",
	"go test ./internal/hooks -run TestAR89_ -count=1",
	"\"$(go env GOPATH)/bin/staticcheck\" ./...",
	"$EDITOR design/relay-r1.md",
	"$(go env GOROOT)/bin/go vet ./internal/hooks",
}

// TestAR89_1_EvasionsDenied: out of a row, each evasion from a helper is
// denied by helper-relay.
func TestAR89_1_EvasionsDenied(t *testing.T) {
	for _, cmd := range ar89Evasions {
		if got := helperSays(t, cmd, "a1b2c3d4", "agent-relay:adversary", false); got != "helper-relay" {
			t.Errorf("helper, %q = %q, want helper-relay", cmd, got)
		}
	}
}

// TestAR89_2_EvasionsDeniedInRow: a helper in a row is denied each evasion
// too, by whichever guard answers first (a row's literal guard reads the
// hidden command word before helper-relay does).
func TestAR89_2_EvasionsDeniedInRow(t *testing.T) {
	for _, cmd := range ar89Evasions {
		if got := helperSays(t, cmd, "a1b2c3d4", "", true); got == "" {
			t.Errorf("helper in a row, %q passed, want a deny", cmd)
		}
	}
}

// TestAR89_3_OrdinaryLinesPass: a helper's read verbs and its ordinary
// lines that hold the word relay pass out of a row: the hook prints nothing.
func TestAR89_3_OrdinaryLinesPass(t *testing.T) {
	for _, cmd := range ar89Passes {
		if got := helperSays(t, cmd, "a1b2c3d4", "agent-relay:adversary", false); got != "" {
			t.Errorf("helper, %q denied by %s", cmd, got)
		}
	}
}

// TestAR89_4_MainThreadUnchanged: the main thread out of a row (no agent_id,
// no agent_type) meets no guard for any evasion: the owner's session is as
// it was.
func TestAR89_4_MainThreadUnchanged(t *testing.T) {
	for _, cmd := range ar89Evasions {
		if got := helperSays(t, cmd, "", "", false); got != "" {
			t.Errorf("main thread, %q denied by %s", cmd, got)
		}
	}
}
