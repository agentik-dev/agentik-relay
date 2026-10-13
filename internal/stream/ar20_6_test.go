package stream

import (
	"os"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR20_6_AccountCells (ADV11-12, and ADV11-5 of the sibling's review):
// every cell of the one table a re-sync copies, read as an is_error result's text,
// names its reason ("" for no stop). Digits count only in an `API Error:
// NNN` prefix, needles match whole words, and `limit reached` only after
// usage, 5-hour, weekly or session: a request id, "catalog in" and a
// context limit stop nothing; the sibling's three phrasings stop.
func TestAR20_6_AccountCells(t *testing.T) {
	b, err := os.ReadFile("testdata/account-cells.tsv")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if lines[0] != "text\treason" {
		t.Fatalf("header %q", lines[0])
	}
	for _, l := range lines[1:] {
		text, reason, ok := strings.Cut(l, "\t")
		if !ok {
			t.Fatalf("cell %q has no tab", l)
		}
		ev := map[string]any{"type": "result", "subtype": "success", "is_error": true, "result": text}
		if got := Account(ev); got != relay.StopReason(reason) {
			t.Errorf("Account(%q) = %q, want %q", text, got, reason)
		}
	}
}
