package effects

import (
	"reflect"
	"testing"
)

// rv1Load is LoadConfig over a repo file holding text alone.
func rv1Load(t *testing.T, text string) Config {
	t.Helper()
	fs := &FakeFS{}
	fs.WriteFile("/r/"+RepoConfigFile, []byte(text), 0o600)
	c, err := LoadConfig(fs, func(k string) string { return map[string]string{"HOME": "/h"}[k] }, "/r")
	if err != nil {
		t.Fatalf("LoadConfig(%q): %v", text, err)
	}
	return c
}

// TestRV1_3_EnabledKnown (RV1 R4): the session tool's `enabled` in the
// relay section warns nothing, kept and read by no behaviour here.
func TestRV1_3_EnabledKnown(t *testing.T) {
	c := rv1Load(t, "relay: {enabled: auto, queue: q, merge: m}\n")
	if len(c.Unknown) != 0 || len(c.Warnings) != 0 || c.Enabled != "auto" || c.Queue != "q" || c.Merge != "m" {
		t.Fatalf("Unknown %q, Warnings %q, Enabled %q, Queue %q, Merge %q", c.Unknown, c.Warnings, c.Enabled, c.Queue, c.Merge)
	}
	for _, v := range []string{"on", "off", "true", "false"} {
		if c := rv1Load(t, "relay:\n  enabled: "+v+"\n"); len(c.Unknown) != 0 || c.Enabled != v {
			t.Errorf("enabled: %s: Unknown %q, Enabled %q", v, c.Unknown, c.Enabled)
		}
	}
	known := false
	for _, kl := range c.Lines {
		known = known || (kl.Known && kl.Key == "enabled" && kl.Line == 1)
	}
	if !known {
		t.Errorf("no known line for enabled: %+v", c.Lines)
	}
}

// TestRV1_3_EnabledBadStaysUnknown (RV1 R4): a value outside the choices,
// or a misspelt key, warns as at the pin, where both were unknown keys.
func TestRV1_3_EnabledBadStaysUnknown(t *testing.T) {
	for text, want := range map[string]string{
		"relay: {enabled: 7, queue: q}\n":    "enabled",
		"relay:\n  enabled:\n    x: 1\n":     "enabled",
		"relay: {enabeld: auto, queue: q}\n": "enabeld",
	} {
		c := rv1Load(t, text)
		if !reflect.DeepEqual(c.Unknown, []string{"/r/" + RepoConfigFile + ": " + want}) || c.Enabled != "" {
			t.Errorf("%q: Unknown %q, Enabled %q", text, c.Unknown, c.Enabled)
		}
	}
}
