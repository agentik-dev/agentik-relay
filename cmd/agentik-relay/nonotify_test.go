package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoNotify_OrchestrateNeedsNoTopic: the ledger's entry 7, quiet hours. A run
// started with --no-notify keeps every notice in its log, so --orchestrate does
// not refuse it for a missing ntfy topic; without the flag it still does.
func TestNoNotify_OrchestrateNeedsNoTopic(t *testing.T) {
	_, l := planFakes(t, "main")
	measuring(t)
	top, _ := toplevel()
	const queue = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nwords: \"\"\nrows:\n" +
		"  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
	os.MkdirAll(filepath.Join(top, "relay"), 0o755)
	if err := os.WriteFile(filepath.Join(top, "relay", "queue.yaml"), []byte(queue), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(getenv("HOME"), ".config", "agentik", "relay", "config.yaml"))
	l.Prior = l.Prior[:1]

	if why := orchestrateRefusalFor(false); !strings.Contains(why, "no ntfy url and topic") {
		t.Fatalf("no topic, no flag: %q; want the ntfy refusal", why)
	}
	if why := orchestrateRefusalFor(true); why != "" {
		t.Fatalf("no topic, --no-notify: %q; want none", why)
	}
	// The flag is a known one: with no bound the run still refuses for the bound, not the flag.
	var out, errb bytes.Buffer
	code := run([]string{"run", "--orchestrate", "--no-notify"}, strings.NewReader(""), &out, &errb)
	if code != exitUsage || !strings.Contains(errb.String(), "want --max-usd or --until") {
		t.Fatalf("--orchestrate --no-notify with no bound: exit %d, stderr %q", code, errb.String())
	}
	fmt.Fprint(&out, "")
}
