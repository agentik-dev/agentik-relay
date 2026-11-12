package packet

import (
	"path/filepath"
	"testing"
)

// TestAR101_8_AgentEffort (AR101 ruling 3): the builder and the adversary
// pin effort high, the attendant low, each beside its model, which stays.
func TestAR101_8_AgentEffort(t *testing.T) {
	dir := filepath.Join(copyIn(t, "../..", "plugin/agents"), "plugin", "agents")
	for _, c := range []struct{ name, model, effort string }{
		{"builder", "claude-opus-5-5", "high"},
		{"adversary", "claude-opus-5-5", "high"},
		{"attendant", "sonnet", "low"},
	} {
		fm, _ := ar72Agent(t, dir, c.name)
		var model, effort string
		if v, ok := fm.Get("model"); ok {
			model, _ = v.String()
		}
		if v, ok := fm.Get("effort"); ok {
			effort, _ = v.String()
		}
		if model != c.model || effort != c.effort {
			t.Errorf("%s.md: model %q, effort %q; want %s at %s", c.name, model, effort, c.model, c.effort)
		}
	}
}
