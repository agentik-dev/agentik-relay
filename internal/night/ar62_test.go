package night_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR62_3_SittingCause: the `sitting` line's gate object carries the
// gate's cause (AR62 act 2), the one its end line carries; `""` when green.
// TestAR45_2's row names clean, touch and acceptance, in that order.
func TestAR62_3_SittingCause(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	g := ar45Row(t, w, "a1", ar45{outside: true})
	var end map[string]any
	for _, l := range w.ledger.Written() {
		if _, ok := l["result"]; ok && l["event"] == "gate" && l["row"] == "a1" {
			end = l
		}
	}
	if want := "touch: b.txt outside may touch"; g["cause"] != want || end == nil || end["cause"] != want {
		t.Fatalf("outside: the sitting gate %v, the end line %v; want cause %q in both", g, end, want)
	}

	if g := ar45Row(t, newWorld(t), "a1", ar45{}); g["result"] != "green" || g["cause"] != "" {
		t.Fatalf("green: gate %v, want cause \"\" (present)", g)
	}

	g = ar45Row(t, newWorld(t), "a1", ar45{outside: true, dirty: true, unrun: true})
	cause, _ := g["cause"].(string)
	c, tc, a := strings.Index(cause, "clean: 1 path"), strings.Index(cause, "touch: b.txt"), strings.Index(cause, "acceptance 2: unrun")
	if c != 0 || tc <= c || a <= tc {
		t.Fatalf("three reds: cause %q, want clean, touch, acceptance in order", cause)
	}
}

// TestAR62_4_StatusAndReport: TestStatusResumeLines's night, r1 red on its
// acceptance line: status.json's row, the dashboard, the live lines, the
// report and its markdown name r1's cause; r2, green, has none.
func TestAR62_4_StatusAndReport(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "no", fixture: "ok.jsonl"}, spec{id: "r2", echo: "ok", fixture: "ok.jsonl"})
	o := w.opts(t, by)
	var out bytes.Buffer
	o.Out = &out
	n := runNight(t, w, q, o)
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	if len(st.Rows) != 2 || !strings.HasPrefix(st.Rows[0].Cause, `acceptance 1: last "no"`) || st.Rows[1].Cause != "" {
		t.Fatalf("status rows %+v", st.Rows)
	}
	var b strings.Builder
	night.PrintStatus(&b, st)
	if !strings.Contains(b.String(), "\ncause of r1: acceptance 1:") || strings.Contains(b.String(), "cause of r2") {
		t.Fatalf("dashboard:\n%s", b.String())
	}
	for _, l := range strings.Split(b.String()+out.String(), "\n") { // a temp root's lines are long
		if strings.Contains(l, "cause") && len([]rune(l)) >= 100 {
			t.Errorf("%d columns: %s", len([]rune(l)), l)
		}
	}
	if !strings.Contains(out.String(), "agentik relay resume r1\nrelay r1: cause: acceptance 1:") {
		t.Fatalf("out:\n%s", out.String())
	}
	r := night.BuildReport(w.ledger.Written(), n.ID)
	var cause string
	for _, s := range r.Stopped.Rows {
		if s.Row == "r1" {
			cause = s.Cause
		}
	}
	if cause != st.Rows[0].Cause {
		t.Fatalf("report stopped %+v, want r1's cause %q", r.Stopped.Rows, st.Rows[0].Cause)
	}
	md := string(night.Markdown(r, night.RootOf(w.ledger.Written(), n.ID), ""))
	if !strings.Contains(md, "- r1: red (acceptance 1:") {
		t.Fatalf("markdown:\n%s", md)
	}
}

// TestAR62Fix_1_SkipReasonWhole: a stopped row with no cause is written
// whole, as before AR62 (round one's F1): a long skip reason keeps its end.
func TestAR62Fix_1_SkipReasonWhole(t *testing.T) {
	why := "stage es2: git-state-moved: config, hooks, info or alternates changed since the root's first stage"
	r := night.Report{Night: "2026-10-01-1", State: "stopped:queue-empty",
		Stopped: night.Stopped{Reason: "queue-empty", Rows: []night.StoppedRow{{Row: "es2", State: "skipped", Reason: why}}}}
	if md := string(night.Markdown(r, "", "")); !strings.Contains(md, "\n- es2: skipped, "+why+"\n") {
		t.Fatalf("the skip reason is not whole:\n%s", md)
	}
}

// TestAR62Fix_2_RootBeforeClip: the root and home are written `.` and `~`
// before any clip (round one's F1, relay-r1 §14 D18): a skipped row's
// reason naming its worktree, and a red row's long cause naming a file
// under the root at the clip, leave no fragment of either.
func TestAR62Fix_2_RootBeforeClip(t *testing.T) {
	root, home := "/home/u/src/agent-relay", "/home/u"
	id := "ar62-red-names-its-cause"
	why := "stage " + id + ": the worktree " + root + "/.claude/worktrees/" + id + " exists"
	cause := `acceptance 1: last "FAIL x_test.go:3 ` + strings.Repeat("y", 60) + ` open ` + root + `/a.txt", want reads 1`
	r := night.Report{Night: "2026-10-01-1", State: "stopped:queue-empty",
		Stopped: night.Stopped{Reason: "queue-empty", Rows: []night.StoppedRow{
			{Row: id, State: "skipped", Reason: why}, {Row: "r1", State: "red", Cause: cause}}}}
	md := string(night.Markdown(r, root, home))
	if strings.Contains(md, "/home/u") || !strings.Contains(md, "the worktree ./.claude/worktrees/"+id+" exists\n") {
		t.Fatalf("a root or home fragment, or the reason cut:\n%s", md)
	}
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(l, "- r1: red (") && (len([]rune(l)) >= 100 || !strings.HasSuffix(l, ")")) {
			t.Errorf("the cause line is not clipped under 100 columns with its `)`: %s", l)
		}
	}
}
