package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// withEnv layers vals over getenv for the test.
func withEnv(t *testing.T, vals map[string]string) {
	t.Helper()
	old := getenv
	t.Cleanup(func() { getenv = old })
	getenv = func(k string) string {
		if v, ok := vals[k]; ok {
			return v
		}
		return old(k)
	}
}

// TestAR32A_1_PlanRowsLockFreeInRow (orchestrator r1 §6): inside a row
// (RELAY_ROW, or a helper's CLAUDE_SESSION_HELPER) `relay plan --rows`
// takes no pid lock, measures nothing and writes no ledger line, even with
// the measurement guard cleared; outside one the same plan takes the lock.
func TestAR32A_1_PlanRowsLockFreeInRow(t *testing.T) {
	const dir = "../../internal/queue/testdata/"
	pid, err := night.PidDir()
	if err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(pid, night.PidFileName)
	os.MkdirAll(pid, 0o700)
	if _, err := os.Stat(blocked); err == nil {
		t.Fatalf("a pid file is already held at %s", blocked)
	}
	if err := os.MkdirAll(filepath.Join(blocked, "x"), 0o700); err != nil { // the lock cannot be taken
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(blocked) })
	for _, v := range []string{night.RowVar, night.HelperVar} {
		t.Run(v, func(t *testing.T) {
			sh, l := planFakes(t, "main")
			measuring(t)
			withEnv(t, map[string]string{v: "o"})
			var out, errb bytes.Buffer
			if code := run([]string{"plan", "--queue", dir + "valid.yaml", "--rows", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb); code != exitOK {
				t.Fatalf("plan in a row = %d: %s", code, errb.String())
			}
			var p night.Plan
			if err := json.Unmarshal(out.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if !p.Nested || len(sh.Calls()) != 0 || len(l.Written()) != 0 || len(p.Rows) != 1 || p.Rows[0].Baseline[0].MainLast != effects.NestedReason {
				t.Fatalf("plan in a row: nested %v, %d shell calls, %d ledger lines: %s", p.Nested, len(sh.Calls()), len(l.Written()), out.String())
			}
		})
	}
	planFakes(t, "main")
	measuring(t)
	var out, errb bytes.Buffer
	if code := run([]string{"plan", "--queue", dir + "valid.yaml", "--rows", "a3-machine", "--json"}, strings.NewReader(""), &out, &errb); code != exitUsage {
		t.Fatalf("a measuring plan outside a row = %d; want 2 at the held lock: %s", code, errb.String())
	}
}

// TestAR32A_5_OrchestrateRefusals is ar32a act 5 (orchestrator r1 §8):
// `relay run --orchestrate` exits 2 before the lock and the plan without
// --max-usd or --until, with --parallel above 1 or --rows, without ntfy's
// URL and topic, with no orchestrator row, or with a red or failed
// sitting of the row today; a red sitting yesterday does not refuse (ar32b
// act 4 drops the branch-only case: TestAR32B_4_OrchestrateAcceptsBranchOnly).
func TestAR32A_5_OrchestrateRefusals(t *testing.T) {
	_, l := planFakes(t, "main")
	measuring(t)
	top, _ := toplevel()
	home := getenv("HOME")
	const head = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: %s, device: never}\nwords: \"\"\nrows:\n"
	const orch = "  - id: o\n    kind: orchestrator\n    prompt: brief.md\n    sections: [How this runs]\n"
	const build = "  - {id: ok-minimal, packet: internal/packet/testdata/ok-minimal.md}\n"
	write := func(path, text string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(home, ".config", "agent-relay", "config.yaml")
	good := func() {
		write(cfg, "ntfy:\n  url: https://ntfy.invalid\n  topic: relay-test\n")
		write(filepath.Join(top, "relay", "queue.yaml"), fmt.Sprintf(head, "merge-on-green")+orch)
		l.Prior = l.Prior[:1]
	}
	for _, c := range []struct {
		name string
		args []string
		set  func()
		want string
	}{
		{"no bound", nil, nil, "want --max-usd or --until"},
		{"parallel", []string{"--until", "23:00", "--parallel", "2"}, nil, "--parallel 2"},
		{"rows", []string{"--max-usd", "50", "--rows", "o"}, nil, "--rows"},
		{"no ntfy", []string{"--max-usd", "50"}, func() { os.Remove(cfg) }, "no ntfy url and topic"},
		{"no row", []string{"--max-usd", "50"}, func() {
			write(filepath.Join(top, "relay", "queue.yaml"), fmt.Sprintf(head, "merge-on-green")+build)
		}, "no orchestrator row"},
		{"red today", []string{"--until", "23:30"}, func() {
			l.Prior = append(l.Prior, map[string]any{"event": "sitting", "row": "o.3", "state": "red", "ts": "2026-09-22T03:00:00Z"})
		}, "parked for today: o.3 was red"},
	} {
		good()
		if c.set != nil {
			c.set()
		}
		var out, errb bytes.Buffer
		code := run(append([]string{"run", "--orchestrate"}, c.args...), strings.NewReader(""), &out, &errb)
		if code != exitUsage || !strings.Contains(errb.String(), c.want) || len(l.Written()) != 0 {
			t.Errorf("%s: exit %d, %d ledger lines, stderr %q; want 2 and %q", c.name, code, len(l.Written()), errb.String(), c.want)
		}
	}
	good()
	l.Prior = append(l.Prior, map[string]any{"event": "sitting", "row": "o.2", "state": "failed", "ts": "2026-09-21T03:00:00Z"})
	if why := orchestrateRefusal(); why != "" {
		t.Fatalf("a good queue and config, a failed sitting yesterday: %q", why)
	}
}
