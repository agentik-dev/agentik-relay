package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR11_4_NestedRunRefuses: `agentik-relay run` under RELAY_MEASURE_DEPTH (every
// test's fakeEnv) or RELAY_ROW (a sitting) exits 2 before any stage, naming
// the guard, with zero spawns and no shell line (relay-r1 §15, AR11; the
// verb's other tests clear the depth with measuring).
func TestAR11_4_NestedRunRefuses(t *testing.T) {
	for _, name := range []string{"depth", "row"} {
		sh, l := planFakes(t, "main")
		if name == "row" {
			measuring(t)
			inner := getenv
			getenv = func(k string) string {
				if k == night.RowVar {
					return "ar11-tla-night"
				}
				return inner(k)
			}
		}
		sp := &effects.FakeSpawner{}
		old := spawner
		spawner = func(effects.Config) effects.Spawner { return sp }
		t.Cleanup(func() { spawner = old })
		var out, errb bytes.Buffer
		code := run([]string{"run", "--rows", "a3-machine"}, strings.NewReader(""), &out, &errb)
		if code != exitUsage || !strings.Contains(errb.String(), "relay run: nested: never under a measurement or a sitting") {
			t.Fatalf("%s: exit %d, stderr %q", name, code, errb.String())
		}
		if len(sp.Spawns()) != 0 || len(sh.Calls()) != 0 || len(l.Written()) != 0 {
			t.Fatalf("%s: spawns %d, shell %v, ledger %v", name, len(sp.Spawns()), sh.Calls(), l.Written())
		}
	}
}
