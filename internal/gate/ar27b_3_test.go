package gate

import (
	"reflect"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR27B_3_GateRedsSettings (ADV16-2, ADV14-7): a branch's .claude/ is
// inside its touch set only when a span names the path or .claude/, never
// through `*`, `.*` or a glob; a settings*.json under a .claude element is
// outside whatever the spans say (SettingsPath), so the gate reds it.
func TestAR27B_3_GateRedsSettings(t *testing.T) {
	const cfg = ".claude/tool/config.yaml"
	for _, c := range []struct {
		touch, may, outside []string
	}{
		{[]string{".claude/settings.json"}, []string{".claude/"}, []string{".claude/settings.json"}},
		{[]string{".claude/settings.local.json"}, []string{".claude/settings.local.json", "*", ".claude"}, []string{".claude/settings.local.json"}},
		{[]string{"sub/.claude/settings.json"}, []string{"sub/", "sub/.claude/settings.json"}, []string{"sub/.claude/settings.json"}},
		{[]string{cfg}, []string{"*", ".*", ".claude/tool/*"}, []string{cfg}},
		{[]string{cfg}, []string{cfg}, []string{}},
		{[]string{cfg}, []string{".claude/"}, []string{}},
		{[]string{cfg}, []string{".claude"}, []string{}},
	} {
		w := newWorld(t)
		st := w.stage(t, c.touch...)
		res, _, g, _ := Run(w.fx, st, pk(c.may), nil, seen, Verify{})
		if !reflect.DeepEqual(g.Outside, c.outside) {
			t.Errorf("touch %q, may touch %q: outside %q, want %q", c.touch, c.may, g.Outside, c.outside)
		}
		if len(c.outside) > 0 && res != relay.RedResult {
			t.Errorf("touch %q, may touch %q: %s, want red", c.touch, c.may, res)
		}
	}
	for p, want := range map[string]bool{
		".claude/settings.json": true, ".claude/settings.local.json": true, "a/.claude/x/settings-2.json": true,
		".Claude/Settings.JSON": true, ".claude/tool/config.yaml": false, "settings.json": false,
		"claude/settings.json": false, ".claude/settings.json.bak": false,
	} {
		if SettingsPath(p) != want {
			t.Errorf("SettingsPath(%q) = %v, want %v", p, !want, want)
		}
	}
}
