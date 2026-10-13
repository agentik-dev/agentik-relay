package night_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// ar55Night is the test night's id.
const ar55Night = "2026-10-03-1"

// ar55Lines is a night's ledger: its `night-open` line and one `sitting`
// line, plus extra (a `stop` line, say).
func ar55Lines(extra ...map[string]any) []map[string]any {
	lines := []map[string]any{
		{"event": "night-open", "night": ar55Night},
		{"event": "sitting", "night": ar55Night, "row": "r1", "state": "green"},
	}
	return append(lines, extra...)
}

// ar55State reads the report's state in its three places: the field, the
// JSON's `state` and the markdown's title.
func ar55State(t *testing.T, r night.Report, want string) {
	t.Helper()
	if r.State != want {
		t.Fatalf("Report.State = %q, want %q", r.State, want)
	}
	b, err := json.Marshal(r)
	if err != nil || !strings.Contains(string(b), `"state":"`+want+`"`) {
		t.Fatalf("report json (%v): %s", err, b)
	}
	md := string(night.Markdown(r, "", ""))
	if title := "# Night " + ar55Night + ": " + want + "\n"; !strings.HasPrefix(md, title) {
		t.Fatalf("markdown title, want %q:\n%s", title, md)
	}
}

// TestAR55_1 (AR55 ruling 1): a night with no `stop` line reads
// "not stopped" in the report's field, its JSON and its markdown title.
func TestAR55_1(t *testing.T) {
	if night.NotStopped != "not stopped" {
		t.Fatalf("NotStopped = %q", night.NotStopped)
	}
	ar55State(t, night.BuildReport(ar55Lines(), ar55Night), night.NotStopped)
}

// TestAR55_2 (AR55 ruling 1): a night with a `stop` line reads that line's
// state in all three places.
func TestAR55_2(t *testing.T) {
	stop := map[string]any{"event": "stop", "night": ar55Night, "state": "stopped:count-reached", "reason": "count-reached"}
	ar55State(t, night.BuildReport(ar55Lines(stop), ar55Night), "stopped:count-reached")
}
