package gate

import (
	"reflect"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR45_Marks: Marks reads a gate's checks in the order the gate marks
// them, and Red the names of those relay.Decide does not pass. A gate that
// did not gather has none and Red is `[]`, never nil, whatever else it
// holds; a skipped check and a same-as-main line are never named; every
// check failing at once reads each name once, an acceptance line once per
// red or unrun line, an unrun verify by its name; `main-moved` and
// `orphans`, which no `gate` event carries, come last in that order.
func TestAR45_Marks(t *testing.T) {
	none := relay.Gate{MainMoved: true, Orphans: 2}
	if m, red := Marks(none), Red(none); m != nil || red == nil || len(red) != 0 {
		t.Fatalf("an ungathered gate: marks %v, red %#v", m, red)
	}
	green := relay.Gate{Gathered: true, Commit: true, Clean: true, OriginSame: true, Skipped: []string{"exit", "origin", "packet-source"},
		Lines: []relay.Check{{Status: relay.CheckGreen}, {Status: relay.CheckSameAsMain}}, Verify: &relay.Check{Status: relay.CheckSkipped}}
	var names []string
	for _, m := range Marks(green) {
		names = append(names, m.Check+":"+m.Status)
	}
	want := []string{"exit:skipped", "commit:green", "clean:green", "origin:skipped", "touch:green", "charter:green",
		"queue:green", "merges:green", "packet-source:skipped", "acceptance:green", "acceptance:same-as-main", "verify:skipped"}
	if red := Red(green); !reflect.DeepEqual(names, want) || red == nil || len(red) != 0 {
		t.Fatalf("a green gate: marks %v, red %#v", names, red)
	}
	all := relay.Gate{Gathered: true, OffBranch: true, OriginSame: true, Outside: []string{"b.txt"}, NoCharter: true,
		QueueInvalid: true, Unproven: true, PacketDiffers: true, MainMoved: true, Orphans: 1,
		Lines: []relay.Check{{Status: relay.CheckGreen}, {Status: relay.CheckRed}, {Status: relay.CheckUnrun},
			{Status: relay.CheckSameAsMain}, {Status: relay.CheckRed}},
		Verify: &relay.Check{Status: relay.CheckUnrun}}
	if got, want := Red(all), []string{"branch", "exit", "commit", "clean", "touch", "charter", "queue", "merges",
		"packet-source", "acceptance", "acceptance", "acceptance", "verify", "main-moved", "orphans"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("red = %v, want %v", got, want)
	}
	if m := Marks(all); m[len(m)-2] != (Mark{Check: "acceptance", Status: relay.CheckRed, N: 5}) {
		t.Fatalf("the last acceptance mark = %+v", m[len(m)-2])
	}
	green.Lines[1].Status, green.MainMoved = relay.CheckUnrun, true
	if got := Red(green); !reflect.DeepEqual(got, []string{"acceptance", "main-moved"}) {
		t.Fatalf("an unrun line under a moved main: red = %v", got)
	}
}
