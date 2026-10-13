package effects

import (
	"strings"
	"testing"
)

// TestAR74_1_ChildEnvCarriesModel (relay-r1 §7; AR74 act 1): after the
// CLAUDE* drop, ChildEnv adds CLAUDE_SESSION_MODEL=<model> once, the spawn
// line's --model as the queue row writes it, its window marker intact; ""
// adds none, and a parent's never passes.
func TestAR74_1_ChildEnvCarriesModel(t *testing.T) {
	if SessionModelVar != "CLAUDE_SESSION_MODEL" {
		t.Fatalf("the name is %s", SessionModelVar)
	}
	in := []string{"HOME=/h", SessionModelVar + "=x", SessionDepthVar + "=3"}
	models := func(env []string) []string {
		var out []string
		for _, kv := range env {
			if strings.HasPrefix(kv, SessionModelVar+"=") {
				out = append(out, kv)
			}
		}
		return out
	}
	got := ChildEnv(in, "r1", "builder", "claude-opus-5-5[1m]", 80, []string{"origin"}, "/abs/relay")
	if m := models(got); len(m) != 1 || m[0] != SessionModelVar+"=claude-opus-5-5[1m]" {
		t.Fatalf("ChildEnv(claude-opus-5-5[1m]) models %q in\n%q", m, got)
	}
	if m := models(ChildEnv(in, "r1", "builder", "", 80, nil, "")); len(m) != 0 {
		t.Fatalf("ChildEnv(\"\") models %q; want none", m)
	}
}
