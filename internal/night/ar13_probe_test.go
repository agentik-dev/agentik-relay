package night

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAR13_5_LiveClaudeTimesOut (R6 §4): the live-claude probe runs under
// LiveClaudeTimeout; a probe that outlives it is unknown (an error, a
// refusal at Stage and the gate), returned within the bound. The probe is
// a script that sleeps, never lsof or claude.
func TestAR13_5_LiveClaudeTimesOut(t *testing.T) {
	if LiveClaudeTimeout != 30*time.Second {
		t.Fatalf("LiveClaudeTimeout %s, want 30s", LiveClaudeTimeout)
	}
	script := filepath.Join(t.TempDir(), "lsof")
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 20\n"), 0o755)
	oldArgv, oldTimeout := lsofArgv, LiveClaudeTimeout
	t.Cleanup(func() { lsofArgv, LiveClaudeTimeout = oldArgv, oldTimeout })
	lsofArgv, LiveClaudeTimeout = []string{script}, 200*time.Millisecond
	start := time.Now()
	live, err := liveClaude(t.TempDir())
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("the probe took %s past a %s bound", took, LiveClaudeTimeout)
	}
	if err == nil || live || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("a timed-out probe = %v, %v; want unknown", live, err)
	}
}
