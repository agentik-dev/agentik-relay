package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestAgentikR1_4_StopLineReadsBoth (agentik-r1 §6.3; ruling 5): a ledger's
// stop line parks its rows whichever spelling its resume lines hold.
func TestAgentikR1_4_StopLineReadsBoth(t *testing.T) {
	got := redSittings(map[string]any{"event": "stop", "resume": []any{"relay resume a", "agentik relay resume b", "no line"}})
	var ids []string
	for _, r := range got {
		ids = append(ids, r[0])
	}
	if !reflect.DeepEqual(ids, []string{"a", "b"}) {
		t.Errorf("redSittings read %q, want a and b", ids)
	}
}

// TestAgentikR1_4_OldNameNote (§4.1; ruling 6): under the old name, on a
// terminal, for any verb but hook, one note; else none.
func TestAgentikR1_4_OldNameNote(t *testing.T) {
	const note = "relay: renamed; use 'agentik relay …' (ends 2026-12-31)"
	for _, tc := range []struct {
		argv0, verb string
		tty         bool
		want        string
	}{
		{"relay", "status", true, note},
		{"/opt/go/bin/relay", "run", true, note},
		{"relay", "", true, note},
		{"relay", "hook", true, ""},
		{"relay", "status", false, ""},
		{"agentik-relay", "status", true, ""},
		{"/x/agentik-relay", "hook", false, ""},
	} {
		if got := aliasNote(tc.argv0, tc.verb, tc.tty); got != tc.want {
			t.Errorf("aliasNote(%q, %q, %v) = %q, want %q", tc.argv0, tc.verb, tc.tty, got, tc.want)
		}
	}
}

// TestAgentikR1_5_InstallTarget (agentik-r1 §4.1, D3): `make -n install
// BINDIR=D` builds agentik-relay into D, links relay to it, installs the
// agentik front door, then relinks GOPATH's relay only when it exists.
// make -n runs no recipe: nothing is built or installed.
func TestAgentikR1_5_InstallTarget(t *testing.T) {
	d := t.TempDir()
	cmd := exec.Command("make", "--no-print-directory", "-n", "install", "BINDIR="+d)
	cmd.Dir = filepath.Join("..", "..")
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MAKE") && !strings.HasPrefix(kv, "MFLAGS=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("make -n install: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{"mkdir -p " + d, "go build -o " + d + "/agentik-relay ./cmd/agentik-relay",
		"ln -sf agentik-relay " + d + "/relay", "install -m 0755 scripts/agentik " + d + "/agentik"}
	if len(lines) != 5 || !reflect.DeepEqual(lines[:4], want) {
		t.Fatalf("make -n install printed:\n%s\nwant first:\n%s", out, strings.Join(want, "\n"))
	}
	if l := lines[4]; !strings.Contains(l, "$(go env GOPATH)/bin/relay") || !strings.Contains(l, "ln -sf "+d+"/agentik-relay") ||
		!strings.Contains(l, "-e ") {
		t.Errorf("the GOPATH relink: %q", l)
	}
}
