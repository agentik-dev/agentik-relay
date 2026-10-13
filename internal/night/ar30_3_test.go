package night_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR30_3_ReadActs (AR30 act 3): the acts are the branch's commits from
// the staged base, by git log over `<base sha>..refs/heads/<branch>`, in
// commit order; a tab in a subject is kept; empty output is `[]`; a read
// error is an error.
func TestAR30_3_ReadActs(t *testing.T) {
	const key = "/root log --reverse --format=%H%x09%s 0123456789abcdef0123456789abcdef01234567..refs/heads/r1"
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	g := &effects.FakeGit{Hermetics: map[string]string{key: a + "\tact 1: the block\n" + b + "\tact 2:\ta tab kept"}}
	acts, err := night.ReadActs(g, "/root", "0123456789abcdef0123456789abcdef01234567", "r1")
	want := []night.Act{{SHA: a, Subject: "act 1: the block"}, {SHA: b, Subject: "act 2:\ta tab kept"}}
	if err != nil || fmt.Sprint(acts) != fmt.Sprint(want) {
		t.Fatalf("acts %q, %v; want %q", acts, err, want)
	}
	if calls := g.Calls(); len(calls) != 1 || calls[0] != "Hermetic "+key {
		t.Fatalf("calls %v; want the one log over the range from the base sha", calls)
	}
	acts, err = night.ReadActs(&effects.FakeGit{}, "/root", "0123456789abcdef0123456789abcdef01234567", "r1")
	if j, _ := json.Marshal(acts); err != nil || string(j) != "[]" {
		t.Fatalf("no commits: %s, %v; want []", j, err)
	}
	g = &effects.FakeGit{Err: map[string]error{"Hermetic": errors.New("git log: bad revision")}}
	if acts, err = night.ReadActs(g, "/root", "0123456789abcdef0123456789abcdef01234567", "r1"); err == nil || acts != nil {
		t.Fatalf("a read error: %v, %v; want an error", acts, err)
	}
}

// TestAR30_3_StatusShowsActs (AR30 act 3): status.json's running row reads
// its acts as the fake's commits move the branch tip, keeps them after the
// exit, from the staged base so a merge into main loses none; the printed
// status counts them before merge and lists each under 100 columns.
func TestAR30_3_StatusShowsActs(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "r1", echo: "ok", fixture: "ok.jsonl",
		env: []string{"FAKECLAUDE_COMMIT_AT=2", "FAKECLAUDE_DELAY_MS=250"}})
	base := gitIn(t, w.root, "rev-parse", "main")
	o := w.opts(t, by)
	mid := make(chan []night.Act, 1)
	go func() {
		for i := 0; i < 1000; i++ {
			st := night.ReadStatus(effects.OSFS{}, o.State, "2026-09-22-1")
			if len(st.Rows) == 1 && st.Rows[0].State == "running" && len(st.Rows[0].Acts) > 0 {
				mid <- st.Rows[0].Acts
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		mid <- nil
	}()
	n := runNight(t, w, q, o)
	if got := <-mid; len(got) != 1 || got[0].Subject != "fake mid act" || len(got[0].SHA) != 40 {
		t.Fatalf("the running row's acts: %q; want the mid-run commit", got)
	}
	st := night.ReadStatus(effects.OSFS{}, o.State, n.ID)
	log := gitIn(t, w.root, "log", "--reverse", "--format=%H %s", base+"..r1")
	var shas []string
	for _, a := range st.Rows[0].Acts {
		shas = append(shas, a.SHA+" "+a.Subject)
	}
	if strings.Join(shas, "\n") != log || len(shas) != 2 || !strings.HasSuffix(log, " fake act") {
		t.Fatalf("after the exit: %q; want the branch's log %q", shas, log)
	}
	gitIn(t, w.root, "merge", "-q", "--ff-only", "r1")
	if acts, err := night.ReadActs(effects.OSGit{}, w.root, base, "r1"); err != nil || len(acts) != 2 {
		t.Fatalf("after a merge into main: %v, %v; want both acts", acts, err)
	}
	var b bytes.Buffer
	night.PrintStatus(&b, st)
	out := b.String()
	a0 := st.Rows[0].Acts[0]
	if !strings.Contains(out, "acts of r1:\n  "+a0.SHA[:12]+"  fake mid act\n") {
		t.Fatalf("the printed status does not list the acts:\n%s", out)
	}
	header := strings.Split(out, "\n")[1]
	if i, j := strings.Index(header, "acts"), strings.Index(header, "merge"); i < 0 || j < i {
		t.Fatalf("no acts count before merge: %q", header)
	}
	// AR37 act 1: the first line names the root, written `$REPO` as UX1's
	// surfaces write it (TestPrintedLinesUnder100).
	for _, l := range strings.Split(strings.ReplaceAll(out, w.root, "$REPO"), "\n") {
		if len([]rune(l)) >= 100 {
			t.Fatalf("a line reaches 100 columns: %q", l)
		}
	}
	long := st
	long.Rows = append([]night.RowStatus(nil), st.Rows...)
	long.Rows[0].Acts = []night.Act{{SHA: a0.SHA, Subject: strings.Repeat("x", 200)}}
	b.Reset()
	night.PrintStatus(&b, long)
	for _, l := range strings.Split(strings.ReplaceAll(b.String(), w.root, "$REPO"), "\n") {
		if len([]rune(l)) >= 100 {
			t.Fatalf("a long subject reaches 100 columns: %q", l)
		}
	}
}
