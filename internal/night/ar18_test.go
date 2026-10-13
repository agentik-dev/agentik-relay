package night

import (
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR18_2_BaselineStopUnrun (conventions §5): on main, a verify line or
// an acceptance line bounded stopped is unrun, carrying the stop line, never
// red.
func TestAR18_2_BaselineStopUnrun(t *testing.T) {
	var sh effects.OSShell
	root := t.TempDir()
	line := "echo 'bounded: stop: load' >&2; exit 2"
	if c := verifyBaseline(sh, root, line, ""); c == nil || c.Status != relay.CheckUnrun || c.MainLast != "bounded: stop: load" {
		t.Fatalf("verify baseline = %+v, want unrun: bounded: stop: load", c)
	}
	cs := baseline(sh, root, []packet.Line{{Shell: line, Expect: "exits 0", Kind: packet.Exits, Value: "0"}}, "")
	if len(cs) != 1 || cs[0].Status != relay.CheckUnrun || cs[0].MainLast != "bounded: stop: load" {
		t.Fatalf("acceptance baseline = %+v, want unrun", cs)
	}
}
