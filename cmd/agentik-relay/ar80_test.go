package main

import (
	"os"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR80_12_E2EBandOff (AR80 fix round, finding 2): the e2e fixture's
// relay config turns the band off, so the built binary under make e2e
// never asks the account's agentik-session.
func TestAR80_12_E2EBandOff(t *testing.T) {
	fs := &effects.FakeFS{}
	fs.WriteFile("/h/.config/agentik/relay/config.yaml", []byte(e2eConfig), 0o600)
	c, err := effects.LoadConfig(fs, func(k string) string { return map[string]string{"HOME": "/h"}[k] }, "/r")
	if err != nil || c.Band != effects.BandOff || c.Caffeinate {
		t.Fatalf("the e2e config %q reads band %q, caffeinate %v, %v; want off, false", e2eConfig, c.Band, c.Caffeinate, err)
	}
	src, err := os.ReadFile("e2e_test.go")
	if err != nil || !strings.Contains(string(src), `"config.yaml"), []byte(e2eConfig), 0o644)`) {
		t.Fatalf("e2e_test.go does not write e2eConfig as the relay's config (%v)", err)
	}
}
