package night

import (
	"errors"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR11_4_NightRunNestedRefuses: night.Run under RELAY_MEASURE_DEPTH or
// RELAY_ROW returns ErrNested for any caller, before the plan: no shell
// line, no ledger line, no spawn (relay-r1 §15, AR11; R6 §3 C4, ADV6-1).
func TestAR11_4_NightRunNestedRefuses(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"depth": {effects.MeasureDepthVar: "1"},
		"row":   {RowVar: "ar11-tla-night"},
	} {
		repo, q := fixture(t)
		fx, _, sh, l := fakes(repo, false)
		fx.Getenv = func(k string) string { return env[k] }
		sp := &effects.FakeSpawner{}
		n, err := Run(fx, sp, q, NightOptions{PlanOptions: Options{Repo: repo}, State: "/state"})
		if !errors.Is(err, ErrNested) || n != nil {
			t.Fatalf("%s: Run = %v, %v; want ErrNested", name, n, err)
		}
		if len(sh.Calls()) != 0 || len(l.Written()) != 0 || len(sp.Spawns()) != 0 {
			t.Fatalf("%s: shell %v, ledger %v, spawns %d", name, sh.Calls(), l.Written(), len(sp.Spawns()))
		}
	}
	if Nested(nil) || Nested(func(string) string { return "" }) {
		t.Fatal("nil or an empty environment read as nested")
	}
}
