package effects

import (
	"strings"
	"testing"
)

// TestAR32A_5_QuestionNoticeCarriesNoText is ar32a act 5 (orchestrator r1
// §7): a question line is a priority 4 notice whose body is
// `open=<n> file=OPEN-QUESTIONS.md at=<sha12>`; whatever else the line
// holds, a question's text never leaves.
func TestAR32A_5_QuestionNoticeCarriesNoText(t *testing.T) {
	n, ok := Notices("question", map[string]any{"night": "2026-09-29-1", "open": 3, "file": "OPEN-QUESTIONS.md",
		"at": "0123456789ab", "text": "Should the secret plan ship?", "question": "the secret plan"})
	if !ok || n.Priority != 4 || n.Body != "open=3 file=OPEN-QUESTIONS.md at=0123456789ab" {
		t.Fatalf("notice %+v, %v", n, ok)
	}
	if strings.Contains(n.Body+n.Title, "secret") || n.Title != "relay 2026-09-29-1 question" {
		t.Fatalf("the notice carries text: %+v", n)
	}
}
