package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ownLogs puts a copy of the package pin whose Logs is a dir of the
// test's own, restored at cleanup, and names it: the package pin's holds
// the red logs of earlier tests (AR31 act 2, D18).
func ownLogs(t *testing.T) string {
	t.Helper()
	old, _ := effects.Current()
	p := old
	p.Logs = filepath.Join(t.TempDir(), "logs")
	effects.Use(p)
	t.Cleanup(func() { effects.Use(old) })
	return p.Logs
}

// seqOut is `seq 1 n`'s output.
func seqOut(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%d\n", i)
	}
	return b.String()
}

// names are the names in dir, sorted.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// holds fails t unless the file at log holds want.
func holds(t *testing.T, what, log, want string) {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil || string(b) != want {
		t.Errorf("%s's log %q holds %d bytes (%v), want %d", what, log, len(b), err, len(want))
	}
}

// TestAR31_2_GateKeepsLog is AR31 act 2: a gate whose green line prints
// 100 KiB and whose second line and verify line are red keeps the two
// reds' logs, each holding its whole output, and names them in
// branch_log and on their `gate` ledger lines; the green line's is gone,
// the probe left none, the clone and the scratches are gone and the logs
// stay. A gate stopped after its lines ran (the red line moved the
// remote: ls-remote fails) writes no check line, and its line keeps its
// log, which Gate.Logs names.
func TestAR31_2_GateKeepsLog(t *testing.T) {
	w := hermeticWorld(t)
	logs := ownLogs(t)
	state := t.TempDir()
	w.fx.Getenv = func(k string) string { return map[string]string{"XDG_STATE_HOME": state}[k] }
	p, ok := effects.Current()
	if !ok {
		t.Fatal("no pin")
	}
	st := w.stage(t, "a.txt")
	green := packet.Line{Shell: `head -c 102400 /dev/zero | tr '\0' x; echo; echo ok`, Expect: "reads `ok`", Kind: packet.Reads, Value: "ok"}
	red := packet.Line{Shell: "seq 1 30000; exit 1", Expect: "exits 0", Kind: packet.Exits}
	fresh := filepath.Join(effects.StateDir(w.fx.Getenv), "gate")
	tmpBefore := names(t, p.Tmp)
	res, _, g, err := Run(w.fx, st, pk([]string{"a.txt"}, green, red), nil, seen, Verify{Name: "seq 1 30000; exit 2"})
	if err != nil || res != relay.RedResult || len(g.Lines) != 2 || g.Verify == nil {
		t.Fatalf("Run = %s %v; lines %+v, verify %+v", res, err, g.Lines, g.Verify)
	}
	if c := g.Lines[0]; c.Status != relay.CheckGreen || c.BranchLog != "" {
		t.Errorf("the green line: %s, log %q; want green and no log", c.Status, c.BranchLog)
	}
	for _, c := range []relay.Check{g.Lines[1], *g.Verify} {
		if c.Status != relay.CheckRed || c.BranchLog == "" {
			t.Errorf("%s: %s, log %q; want red with a log", c.Name, c.Status, c.BranchLog)
			continue
		}
		holds(t, c.Name, c.BranchLog, seqOut(30000))
	}
	want := []string{filepath.Base(g.Lines[1].BranchLog), filepath.Base(g.Verify.BranchLog)}
	slices.Sort(want)
	if got := names(t, logs); !slices.Equal(got, want) || filepath.Dir(g.Lines[1].BranchLog) != logs {
		t.Errorf("the test's logs dir holds %q, want the two reds' %q", got, want)
	}
	if got := g.Logs(); !slices.Equal(got, []string{g.Lines[1].BranchLog, g.Verify.BranchLog}) {
		t.Errorf("Gate.Logs = %q", got)
	}
	checks := map[string]map[string]any{}
	for _, l := range w.ledger.Written() {
		if l["event"] == "gate" && l["check"] != nil {
			checks[fmt.Sprint(l["check"], l["n"])] = l
		}
	}
	for key, want := range map[string]string{"acceptance1": "", "acceptance2": g.Lines[1].BranchLog, "verify<nil>": g.Verify.BranchLog} {
		l, ok := checks[key]
		if !ok {
			t.Errorf("no gate ledger line %s", key)
			continue
		}
		if got, _ := l["log"].(string); got != want {
			t.Errorf("gate ledger line %s: log %q, want %q", key, got, want)
		}
	}
	if left := names(t, fresh); len(left) > 0 {
		t.Errorf("the gate's clone outlived it: %q in %s", left, fresh)
	}
	if got := names(t, p.Tmp); !slices.Equal(got, tmpBefore) {
		t.Errorf("the scratches' temp dirs outlived the gate: %q, before %q", got, tmpBefore)
	}
	// The probe (`make -n check`, through the memo) keeps no log: an
	// `auto` verify, red on the branch's a.txt, adds its line's alone.
	before := names(t, logs)
	_, _, g, err = Run(w.fx, st, pk([]string{"a.txt"}, green), nil, seen, Verify{Name: "auto"})
	if v := g.Verify; err != nil || v == nil || v.Name != "make check" || v.Status != relay.CheckRed || v.BranchLog == "" {
		t.Fatalf("an auto verify: %v, %+v; want make check, red, with a log", err, v)
	}
	if got := names(t, logs); len(got) != len(before)+1 || !slices.Contains(got, filepath.Base(g.Verify.BranchLog)) {
		t.Errorf("an auto verify's gate left %q in the logs dir, before %q: want its line's log alone", got, before)
	}

	w = hermeticWorld(t)
	logs = ownLogs(t)
	st = w.stage(t, "a.txt")
	remote := filepath.Join(filepath.Dir(w.root), "remote.git")
	moved := packet.Line{Shell: fmt.Sprintf("mv %s %s.gone; seq 1 30000; exit 1", remote, remote), Expect: "exits 0", Kind: packet.Exits}
	res, _, g, err = Run(w.fx, st, pk([]string{"a.txt"}, moved), nil, seen, Verify{})
	if err == nil || !strings.HasPrefix(err.Error(), "ls-remote: ") || res != relay.Unrun || len(g.Lines) != 1 {
		t.Fatalf("a gate whose line moved the remote: %s %v; lines %+v", res, err, g.Lines)
	}
	for _, l := range w.ledger.Written() {
		if l["event"] == "gate" && l["check"] != nil {
			t.Errorf("a stopped gate wrote a check line: %v", l)
		}
	}
	if c := g.Lines[0]; c.BranchLog == "" || !slices.Equal(g.Logs(), []string{c.BranchLog}) {
		t.Fatalf("the stopped gate's line: log %q, Logs %q; want it kept and named", c.BranchLog, g.Logs())
	}
	holds(t, "the stopped gate's line", g.Lines[0].BranchLog, seqOut(30000))
	if got := names(t, logs); len(got) != 1 {
		t.Errorf("the second world's logs dir holds %q, want the one line's", got)
	}
}

// TestAR31_2_MergeNamesLog is AR31 act 2: a merge whose verify line is red
// on the merged tree is refused naming its kept log, which holds the whole
// output, with no `(<nil>)` tail; its `merge` ledger line carries the same
// `log`. A green merge verify keeps none.
func TestAR31_2_MergeNamesLog(t *testing.T) {
	w := newWorld(t)
	logs := ownLogs(t)
	st := w.stage(t, "a.txt")
	green := Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	_, err := MergeIfGreen(w.fx, w.root, green, "a1", "seq 1 30000; exit 1", nil)
	l := w.ledger.Written()
	last := l[len(l)-1]
	log, _ := last["log"].(string)
	if last["event"] != "merge" || last["result"] != "aborted" || log == "" {
		t.Fatalf("the last ledger line: %v; want merge, aborted, with its log", last)
	}
	if want := fmt.Sprintf("merge a1: verify exit 1, last %q; log: %s", "30000", log); err == nil || err.Error() != want {
		t.Errorf("MergeIfGreen = %v, want %s", err, want)
	}
	if err != nil && strings.Contains(err.Error(), "(<nil>)") {
		t.Errorf("the refusal ends in a nil error: %v", err)
	}
	if filepath.Dir(log) != logs {
		t.Errorf("the merge's log %s is not in the test's %s", log, logs)
	}
	holds(t, "the merge's verify", log, seqOut(30000))

	w = newWorld(t)
	logs = ownLogs(t)
	st = w.stage(t, "a.txt")
	green = Gather(w.fx, st, pk([]string{"a.txt"}), nil, seen)
	if _, err := MergeIfGreen(w.fx, w.root, green, "a1", "seq 1 10", nil); err != nil || !w.merged() {
		t.Fatalf("a green merge verify: %v", err)
	}
	l = w.ledger.Written()
	if last := l[len(l)-1]; last["event"] != "merge" || last["result"] != "merged" || last["log"] != nil {
		t.Errorf("the green merge's line: %v; want merged with no log", last)
	}
	if got := names(t, logs); len(got) > 0 {
		t.Errorf("a green merge verify kept a log: %q", got)
	}
}
