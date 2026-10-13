package night_test

import (
	"os"
	"slices"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// ar39State makes a temp state root holding the named night dirs.
func ar39State(t *testing.T, names ...string) string {
	t.Helper()
	state := t.TempDir()
	for _, n := range names {
		if err := os.MkdirAll(night.NightDir(state, n), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

// TestAR39_1_LatestSkipsHand (AR39 act 1): the gate's `hand` dir sorts
// after every dated name, yet the latest night is the newest dated one
// (2026-09-29-11 after 2026-09-29-9), never `hand`.
func TestAR39_1_LatestSkipsHand(t *testing.T) {
	state := ar39State(t, "2026-09-29-9", "2026-09-29-11", "hand")
	if got := night.Latest(effects.OSFS{}, state); got != "2026-09-29-11" {
		t.Fatalf("Latest = %q; want 2026-09-29-11", got)
	}
}

// TestAR39_1_NightsDatedOnly (AR39 act 1): Nights lists only the
// <YYYY-MM-DD>-<n> dirs (not hand, notes or 2026-09-30-x), and with only
// hand there is no latest night.
func TestAR39_1_NightsDatedOnly(t *testing.T) {
	state := ar39State(t, "hand", "notes", "2026-09-30-x", "2026-09-30-2")
	if got := night.Nights(effects.OSFS{}, state); !slices.Equal(got, []string{"2026-09-30-2"}) {
		t.Fatalf("Nights = %q; want [2026-09-30-2]", got)
	}
	state = ar39State(t, "hand")
	if got := night.Latest(effects.OSFS{}, state); got != "" {
		t.Fatalf("Latest with only hand = %q; want \"\"", got)
	}
}
