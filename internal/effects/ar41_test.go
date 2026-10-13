package effects

import (
	"strings"
	"testing"
)

// TestAR41_1_ChildEnvCarriesProfile (relay-r1 §7; AR41 act 1): after the
// CLAUDE* drop, ChildEnv adds CLAUDE_SESSION_PROFILE=<profile> once, so a
// parent's profile never passes; "" adds none.
func TestAR41_1_ChildEnvCarriesProfile(t *testing.T) {
	if SessionProfileVar != "CLAUDE_SESSION_PROFILE" {
		t.Fatalf("the name is %s", SessionProfileVar)
	}
	in := []string{"HOME=/h", SessionProfileVar + "=orchestrator", SessionDepthVar + "=3"}
	profiles := func(env []string) []string {
		var out []string
		for _, kv := range env {
			if strings.HasPrefix(kv, SessionProfileVar+"=") {
				out = append(out, kv)
			}
		}
		return out
	}
	got := ChildEnv(in, "r1", "builder", "", 80, []string{"origin"}, "/abs/relay")
	if p := profiles(got); len(p) != 1 || p[0] != SessionProfileVar+"=builder" {
		t.Fatalf("ChildEnv(builder) profiles %q in\n%q", p, got)
	}
	if p := profiles(ChildEnv(in, "r1", "", "", 80, nil, "")); len(p) != 0 {
		t.Fatalf("ChildEnv(\"\") profiles %q; want none", p)
	}
}
