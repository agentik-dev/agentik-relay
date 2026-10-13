package night

import (
	"reflect"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

func TestReadControl(t *testing.T) {
	fs := &effects.FakeFS{}
	p := ControlPath("/s", "2026-09-22-1")
	if stop, kills := ReadControl(fs, p); stop || kills != nil {
		t.Fatalf("no file: %v %v", stop, kills)
	}
	for _, l := range []string{"kill a8", "junk line here", "stop", "kill"} {
		if err := WriteControl(fs, p, l); err != nil {
			t.Fatal(err)
		}
	}
	stop, kills := ReadControl(fs, p)
	if !stop || !reflect.DeepEqual(kills, []string{"a8", "*"}) {
		t.Fatalf("got %v %v", stop, kills)
	}
}
