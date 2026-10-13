package night_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR33_1_StatusWindowNamesItsNight (AR33 act 1): the session's first
// window is `<relay> status --night <night> --watch 5 --hold`, no shell, so
// each night's window watches its own night, not the latest.
func TestAR33_1_StatusWindowNamesItsNight(t *testing.T) {
	const id = "2026-09-29-3"
	ft := &effects.FakeTmux{IsPresent: true}
	if _, err := night.Open(ft, "auto", id, "/s"); err != nil {
		t.Fatal(err)
	}
	var session string
	for _, c := range ft.Recorded() {
		if strings.HasPrefix(c, "NewSession ") {
			session = c
		}
	}
	argv := night.StatusArgv(id)
	if session != "NewSession relay-"+id+" /s "+strings.Join(argv, " ") {
		t.Fatalf("calls = %q", ft.Recorded())
	}
	if want := []string{"status", "--night", id, "--watch", "5", "--hold"}; len(argv) != 7 || !slices.Equal(argv[1:], want) {
		t.Fatalf("StatusArgv = %q", argv)
	}
	for _, a := range argv {
		if a == "sh" || a == "-c" || strings.HasSuffix(a, "/sh") || strings.Contains(a, "clear") {
			t.Fatalf("StatusArgv runs a shell: %q", argv)
		}
	}
	if argv[0] == "" {
		t.Fatal("StatusArgv names no relay")
	}
	t.Logf("status window: %s", strings.Join(argv, " "))
}
