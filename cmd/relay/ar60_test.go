package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR60_5_VerbsReadPlanning: a night whose plan is still running reads
// `planning` in status (its JSON with the relay's pid) and in report, and
// stop says the night is still measuring (AR34's line), not "after its
// next event".
func TestAR60_5_VerbsReadPlanning(t *testing.T) {
	home := t.TempDir()
	old := getenv
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = old })
	state := effects.StateDir(getenv)
	const id = "2026-10-10-1"
	night.MarkPlanning(effects.Effects{FS: effects.OSFS{}}, state, id, home)
	do := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		return code, out.String() + errb.String()
	}

	code, out := do("status", "--json", "--night", id)
	var st night.Status
	if err := json.Unmarshal([]byte(out), &st); code != exitOK || err != nil || st.State != "planning" || st.PID <= 0 {
		t.Fatalf("status --json = %d %v %q", code, err, out)
	}
	code, out = do("status", "--night", id)
	if first, _, _ := strings.Cut(out, "\n"); code != exitOK || !strings.HasSuffix(first, ": planning; 0 rows") {
		t.Fatalf("status = %d, first line %q", code, first)
	}
	code, out = do("report", "--json", "--night", id)
	var r night.Report
	if err := json.Unmarshal([]byte(out), &r); code != exitOK || err != nil || r.State != "planning" {
		t.Fatalf("report --json = %d %v %q", code, err, out)
	}
	code, out = do("report", "--night", id)
	if code != exitOK || !strings.HasPrefix(out, "# Night "+id+": planning\n") {
		t.Fatalf("report = %d %q", code, out)
	}
	const measuring = "the night is still measuring: it stops within a few seconds, before any row starts"
	if code, out = do("stop"); code != exitOK || !strings.Contains(out, measuring) || strings.Contains(out, "after its next event") {
		t.Fatalf("stop while planning = %d %q", code, out)
	}
	t.Log(strings.TrimSpace(out))
}

// ar60Run is TestRunVerb's fixture on the machine's file system: a temp
// repo whose queue holds a3-machine, the fakes' effects handing OSFS, and
// the state directory under the fixture's temp home.
func ar60Run(t *testing.T) (state, repo string) {
	t.Helper()
	t.Setenv("TMPDIR", "/tmp")
	planFakes(t, "feature")
	measuring(t)
	oldTmux := newTmux
	newTmux = func(string) effects.Tmux { return &effects.FakeTmux{} } // never a real session
	t.Cleanup(func() { newTmux = oldTmux })
	repo = t.TempDir()
	os.MkdirAll(filepath.Join(repo, "relay"), 0o755)
	os.MkdirAll(filepath.Join(repo, "design", "packets"), 0o755)
	src, err := os.ReadFile("../../internal/packet/testdata/a3-machine.md")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "design", "packets", "a3-machine.md"), src, 0o644)
	os.WriteFile(filepath.Join(repo, "relay", "queue.yaml"), []byte(`version: 1
defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,
           merge: branch-only, device: never, verify: auto}
words: ""
rows:
  - id: a3-machine
    packet: design/packets/a3-machine.md
`), 0o644)
	fakeTop(t, repo)
	faked := effectsFor
	effectsFor = func(state, night string) effects.Effects {
		fx := faked(state, night)
		g := fx.Git.(*effects.FakeGit)
		g.Roots[repo], g.Branches[repo] = repo, "feature"
		fx.FS = effects.OSFS{}
		return fx
	}
	return effects.StateDir(getenv), repo
}

// TestAR60_6_RunEndsPlanning: a run whose plan is not valid opens no night,
// and its mark ends `stopped:queue-invalid` under this process's pid; a dry
// run marks nothing. TestRunVerb's fixture, on the machine's file system.
func TestAR60_6_RunEndsPlanning(t *testing.T) {
	state, repo := ar60Run(t)

	var out, errb bytes.Buffer
	if code := run([]string{"run", "--rows", "nosuchrow"}, strings.NewReader(""), &out, &errb); code != exitUsage {
		t.Fatalf("run nosuchrow = %d, %q", code, errb.String())
	}
	nights := night.Nights(effects.OSFS{}, state)
	if len(nights) != 1 {
		t.Fatalf("nights after run = %v", nights)
	}
	st := night.ReadStatus(effects.OSFS{}, state, nights[0])
	if st.State != "stopped:queue-invalid" || st.Reason != "queue-invalid" || st.PID != os.Getpid() || st.Root != repo {
		t.Fatalf("status after a plan not valid = %+v", st)
	}
	t.Logf("night %s: %s, pid %d", nights[0], st.State, st.PID)

	if code := run([]string{"run", "--rows", "nosuchrow", "--dry"}, strings.NewReader(""), &out, &errb); code != exitUsage {
		t.Fatalf("run nosuchrow --dry = %d, %q", code, errb.String())
	}
	if after := night.Nights(effects.OSFS{}, state); strings.Join(after, " ") != strings.Join(nights, " ") {
		t.Fatalf("a dry run added a night: %v, was %v", after, nights)
	}
}

// TestAR60Fix_3_FailedRunKeepsLatest (the review's block 2, probes
// TestADV3_FailedRunShadows and TestADVA1; the observer's ruling): a run
// refused at its plan leaves its night stopped:queue-invalid, read by
// --night, but the verbs given no night still read the last night that
// opened: status with its resume line, resume and report.
func TestAR60Fix_3_FailedRunKeepsLatest(t *testing.T) {
	state, _ := ar60Run(t)
	const prev = "2026-09-22-1"
	lg := effects.NewLedger(state, effects.FakeClock{T: time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)}, prev)
	lg.Append("night-open", map[string]any{"root": "", "rows": []string{"a3-machine"}})
	lg.Append("sitting", map[string]any{"row": "a3-machine", "state": "red", "reason": "acceptance"})
	lg.Append("stop", map[string]any{"state": "stopped:queue-empty", "reason": "queue-empty"})
	os.MkdirAll(night.NightDir(state, prev), 0o700)
	b, _ := json.Marshal(night.Status{Night: prev, State: "stopped:queue-empty", Reason: "queue-empty",
		Rows: []night.RowStatus{{ID: "a3-machine", State: "red", Resume: "relay resume a3-machine"}}, Resume: []string{"relay resume a3-machine"}})
	os.WriteFile(filepath.Join(night.NightDir(state, prev), "status.json"), b, 0o600)
	do := func(args ...string) (int, string) {
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		return code, out.String() + errb.String()
	}

	if code, out := do("run", "--rows", "nosuchrow"); code != exitUsage {
		t.Fatalf("run nosuchrow = %d %q", code, out)
	}
	nights := night.Nights(effects.OSFS{}, state)
	if len(nights) != 2 || nights[0] != prev {
		t.Fatalf("nights = %v", nights)
	}
	refused := nights[1]
	var st night.Status
	if code, out := do("status", "--json"); code != exitOK || json.Unmarshal([]byte(out), &st) != nil || st.Night != prev || len(st.Resume) != 1 {
		t.Fatalf("status --json after a refused run = %d %q; want night %s and its resume line", code, out, prev)
	}
	if code, out := do("resume"); code != exitOK || !strings.Contains(out, "night "+prev+": 1 red or failed row(s) to resume") {
		t.Fatalf("resume after a refused run = %d %q", code, out)
	}
	var r night.Report
	if code, out := do("report", "--json"); code != exitOK || json.Unmarshal([]byte(out), &r) != nil || r.Night != prev || r.State != "stopped:queue-empty" {
		t.Fatalf("report --json after a refused run = %d %q", code, out)
	}
	if code, out := do("status", "--json", "--night", refused); code != exitOK || json.Unmarshal([]byte(out), &st) != nil || st.State != "stopped:queue-invalid" {
		t.Fatalf("status --night %s = %d %q", refused, code, out)
	}
	t.Logf("latest %s; refused %s reads %s by name", prev, refused, st.State)
}

// TestAR60Fix_4_SignalEndsOwnerKill (the review's N2): a signal the run's
// trap caught during the plan ends the mark stopped:owner-kill, never
// queue-invalid, and the run exits 1 naming it, ahead of the plan's own
// refusal. The trap is handed in already signalled, so no signal is sent;
// the row does not exist, so no path reaches night.Run.
func TestAR60Fix_4_SignalEndsOwnerKill(t *testing.T) {
	state, _ := ar60Run(t)
	old := runTrap
	t.Cleanup(func() { runTrap = old })
	runTrap = func(ctx context.Context, reg *effects.Groups) (context.Context, func()) {
		c, cancel := context.WithCancelCause(ctx)
		cancel(errors.New("signal:SIGTERM"))
		return c, func() {}
	}
	var out, errb bytes.Buffer
	if code := run([]string{"run", "--rows", "nosuchrow"}, strings.NewReader(""), &out, &errb); code != exitRed || !strings.Contains(errb.String(), "relay run: stopped by signal:SIGTERM") {
		t.Fatalf("run under a signal = %d %q %q", code, out.String(), errb.String())
	}
	nights := night.Nights(effects.OSFS{}, state)
	if len(nights) != 1 {
		t.Fatalf("nights = %v", nights)
	}
	st := night.ReadStatus(effects.OSFS{}, state, nights[0])
	if st.State != "stopped:owner-kill" || st.Reason != "owner-kill" || st.PID != os.Getpid() {
		t.Fatalf("status after a signal in the plan = %+v", st)
	}
	t.Logf("night %s: %s", nights[0], st.State)
}
