package night_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// surfaces is every printed surface of a two-row night over the fake
// claude, r1 red and r2 green: the plan, the night's own lines, each row's
// live log, the dashboard (`status`, and `run`'s tail) and the report. The
// temp root reads `$REPO` and the state directory `$STATE`. With
// UX1_CAPTURE=<dir> each is written to <dir>/<name>.txt (UX1's captures).
func surfaces(t *testing.T) map[string]string {
	t.Helper()
	w, q, by := nightWorld(t, spec{id: "r1", echo: "no", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	// AR31 act 2: r1's kept log lives in the state root's logs/, as the
	// relay's own pin puts it (Pinned.Logs), so its line reads $STATE.
	old, _ := effects.Current()
	p := old
	p.Logs = filepath.Join(o.State, "logs")
	effects.Use(p)
	t.Cleanup(func() { effects.Use(old) })
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, w, q, o)
	got := map[string]string{"run": out.String()}
	var b bytes.Buffer
	night.Print(&b, n.Plan)
	got["plan"] = b.String()
	b.Reset()
	night.PrintStatus(&b, night.ReadStatus(effects.OSFS{}, o.State, n.ID))
	got["status"] = b.String()
	got["run"] += b.String() // run ends with the dashboard
	lines := w.ledger.Written()
	got["report"] = string(night.Markdown(night.BuildReport(lines, n.ID), w.root, ""))
	for _, id := range []string{"r1", "r2"} {
		log, _ := os.ReadFile(night.RowLog(o.State, n.ID, id))
		got["live-"+id] = string(log)
	}
	dir := os.Getenv("UX1_CAPTURE")
	for k, v := range got {
		v = strings.ReplaceAll(strings.ReplaceAll(v, w.root, "$REPO"), o.State, "$STATE")
		got[k] = v
		if dir != "" {
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, k+".txt"), []byte(v), 0o644)
		}
	}
	return got
}

func TestCaptureSurfaces(t *testing.T) {
	for k, v := range surfaces(t) {
		t.Logf("%s:\n%s", k, v)
	}
}

// TestResumeLineOnItsOwn (UX1): the red row's `relay resume r1` is a whole
// line in `status`, in the report, and the last line of `run`'s tail.
func TestResumeLineOnItsOwn(t *testing.T) {
	got := surfaces(t)
	for _, k := range []string{"status", "report", "run"} {
		if !strings.Contains("\n"+got[k], "\nrelay resume r1\n") {
			t.Errorf("%s has no line `relay resume r1`:\n%s", k, got[k])
		}
	}
	for _, k := range []string{"status", "run"} {
		lines := strings.Split(strings.TrimRight(got[k], "\n"), "\n")
		if lines[len(lines)-1] != "relay resume r1" {
			t.Errorf("%s does not end with the resume line:\n%s", k, got[k])
		}
	}
}

// TestPrintedLinesUnder100 (UX1): every human line of every surface of the
// two-row night is under 100 columns, paths written `$REPO` and `$STATE`.
// The spawn line wraps with ` \` continuations (AR5), so no line is exempt
// but a fenced block in the report (the resume lines).
func TestPrintedLinesUnder100(t *testing.T) {
	for k, v := range surfaces(t) {
		fenced := false
		for _, l := range strings.Split(v, "\n") {
			if strings.HasPrefix(l, "```") {
				fenced = !fenced
				continue
			}
			if fenced {
				continue
			}
			if n := len([]rune(l)); n >= 100 {
				t.Errorf("%s: %d columns: %s", k, n, l)
			}
		}
	}
}

// TestAR4_StatusRowWorktree: every planned row's status carries its
// worktree, in status.json and so in `status --json`.
func TestAR4_StatusRowWorktree(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "no", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	n := runNight(t, w, q, o)
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	if len(st.Rows) != 2 {
		t.Fatalf("status rows %+v", st.Rows)
	}
	for _, r := range st.Rows {
		if want := filepath.Join(w.root, ".claude", "worktrees", r.ID); r.Worktree != want {
			t.Errorf("row %s worktree %q, want %q", r.ID, r.Worktree, want)
		}
	}
	b, err := os.ReadFile(filepath.Join(night.NightDir(o.State, n.ID), "status.json"))
	if err != nil || !strings.Contains(string(b), `"worktree": "`) {
		t.Fatalf("status.json lacks worktree: %v\n%s", err, b)
	}
}
