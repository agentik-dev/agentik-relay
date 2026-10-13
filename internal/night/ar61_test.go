package night_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ar61Adv is AR38's advShell for this external package (advShell lives in
// package night, nightWorld here): each line takes step on a StepClock,
// and cut lines return the shell's cut exit; it counts every line run.
type ar61Adv struct {
	sh    effects.Shell
	clock *effects.StepClock
	step  time.Duration
	cut   string
	mu    sync.Mutex
	ran   map[string]int
}

func (a *ar61Adv) Run(dir, line string, timeout time.Duration) (int, string, error) {
	a.mu.Lock()
	if a.ran == nil {
		a.ran = map[string]int{}
	}
	a.ran[line]++
	a.mu.Unlock()
	defer a.clock.Advance(a.step)
	if line == a.cut {
		return effects.CutExit, "cut", nil
	}
	return a.sh.Run(dir, line, timeout)
}

// ar61World is a world over an ar61Adv at step a line, its verify line
// verify, and the plan options with the world's state and an hour's
// measure; edit, when not nil, changes the committed tree first (its
// change committed and the queue read again).
func ar61World(t *testing.T, verify string, step time.Duration, edit func(root string), specs ...spec) (*world, *queue.Queue, *ar61Adv, night.Options, func() *night.Plan) {
	t.Helper()
	w, q, by := nightWorld(t, specs...)
	if edit != nil {
		edit(w.root)
		gitIn(t, w.root, "commit", "-qam", "edit")
		var err error
		if q, err = queue.Load(w.root, "queue.yaml"); err != nil {
			t.Fatal(err)
		}
	}
	q.Defaults.Verify = verify
	clock := &effects.StepClock{T: time.Date(2026, 10, 10, 22, 0, 0, 0, time.UTC)}
	adv := &ar61Adv{sh: w.fx.Shell, clock: clock, step: step}
	w.fx.Clock, w.fx.Shell = clock, adv
	o := w.opts(t, by)
	po := o.PlanOptions
	po.State, po.MeasureSeconds = o.State, 3600
	build := func() *night.Plan {
		t.Helper()
		p, err := night.BuildPlan(w.fx, q, po)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	return w, q, adv, po, build
}

// TestAR61_3_VerifySeconds (AR61 act 1, ruling 1): the plan times its
// verify line by the bound shell's clock, rounded up: 250 s a line reads
// 250; a memo hit returns the entry's secs, 250, without a run; an entry
// with no secs, or a cut, reads 0; VerifyName reads `auto` as make check;
// RenderPrompt's stdin carries the CONTEXT clause.
func TestAR61_3_VerifySeconds(t *testing.T) {
	w, qu, adv, po, build := ar61World(t, "echo v", 250*time.Second, nil, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	p := build()
	if p.Verify == nil || p.Verify.Status != relay.CheckGreen || p.VerifySeconds != 250 || night.VerifyName(p) != "echo v" {
		t.Fatalf("verify %+v, %d s, name %q; want green at 250 s, echo v", p.Verify, p.VerifySeconds, night.VerifyName(p))
	}
	if p2 := build(); p2.VerifySeconds != 250 || adv.ran["echo v"] != 1 {
		t.Fatalf("a hit reads %d s after %d runs; want 250 after 1", p2.VerifySeconds, adv.ran["echo v"])
	}
	rp := p.Rows[0]
	stdin, _, _, err := night.RenderPrompt(w.fx, p, qu.Rows[0], &rp, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "the row's device: never; the verify line: `echo v`, 250 s on main, once before your last commit; the owner's words"; !strings.Contains(string(stdin), want) {
		t.Fatalf("stdin lacks %q:\n%s", want, stdin)
	}
	path := filepath.Join(po.State, night.MemoFile) // an entry with no secs: unmeasured
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), `"secs"`) {
		t.Fatalf("the memo holds no secs (%v):\n%s", err, b)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var drop func(v any)
	drop = func(v any) {
		if m, ok := v.(map[string]any); ok {
			if _, entry := m["status"]; entry {
				delete(m, "secs")
			}
			for _, e := range m {
				drop(e)
			}
		}
	}
	drop(doc)
	b, _ = json.Marshal(doc)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if p3 := build(); p3.VerifySeconds != 0 || adv.ran["echo v"] != 1 {
		t.Fatalf("an entry with no secs reads %d s after %d runs; want 0 after 1", p3.VerifySeconds, adv.ran["echo v"])
	}
	_, _, cut, _, buildCut := ar61World(t, "echo v", 250*time.Second, nil, spec{id: "r1", echo: "ok", fixture: "ok.jsonl"})
	cut.cut = "echo v"
	if pc := buildCut(); pc.Verify == nil || pc.Verify.MainExit != effects.CutExit || pc.VerifySeconds != 0 {
		t.Fatalf("a cut verify %+v reads %d s; want exit 124 at 0", pc.Verify, pc.VerifySeconds)
	}
	if night.VerifyName(&night.Plan{}) != "" || night.VerifyName(&night.Plan{Verify: &relay.Check{Name: "auto"}}) != "make check" {
		t.Fatal("VerifyName: nil is not empty, or auto is not make check")
	}
}

// ar61PerAct puts the analysis's example, with `make -k check`, in §2 of
// each named row's packet (AR61 §1, ruling 4).
func ar61PerAct(ids ...string) func(root string) {
	return func(root string) {
		for _, id := range ids {
			path := filepath.Join(root, "design", "packets", id+".md")
			b, _ := os.ReadFile(path)
			b = []byte(strings.Replace(string(b), "## 2. Rulings\nNone.\n",
				"## 2. Rulings\nEach act: [...] its package tests, `make lint`, then the gate `make -k check` in the foreground.\n", 1))
			os.WriteFile(path, b, 0o644)
		}
	}
}

// TestAR61_7_PlanWarns (AR61 act 3, ruling 4): r1 ready and r2 pending
// (on r1), each with §1's example in §2, take one per-act-verify warning
// at §2, RULES run it once at 250 s; r3 none, `[]`; the plan stays valid
// with no problem and exit 0; Print writes the warning under its row and
// the JSON holds `"warnings":[]`; a verify line of 601 s is the gate's
// alone.
func TestAR61_7_PlanWarns(t *testing.T) {
	specs := []spec{{id: "r1", echo: "ok", fixture: "ok.jsonl"}, {id: "r2", deps: "r1", echo: "ok", fixture: "ok.jsonl"},
		{id: "r3", echo: "ok", fixture: "ok.jsonl"}}
	_, _, _, _, build := ar61World(t, "make check", 250*time.Second, ar61PerAct("r1", "r2"), specs...)
	p := build()
	once := night.PerActVerify + ": §2 orders the verify line per act; RULES run it once"
	want := map[string]string{"r1": night.StateReady + " " + once, "r2": night.StatePending + " " + once, "r3": night.StateReady + " "}
	for _, rp := range p.Rows {
		if got := rp.State + " " + strings.Join(rp.Warnings, "|"); got != want[rp.ID] || rp.Warnings == nil || len(rp.Problems) != 0 {
			t.Errorf("row %s: %q, problems %v; want %q", rp.ID, got, rp.Problems, want[rp.ID])
		}
	}
	if p.Verify == nil || p.Verify.Status != relay.CheckRed || p.VerifySeconds != 250 {
		t.Fatalf("verify %+v at %d s; want make check red at 250 s (no Makefile)", p.Verify, p.VerifySeconds)
	}
	if !p.Valid || len(p.Problems) != 0 || p.RunStop != "" || p.Exit() != 0 {
		t.Fatalf("valid %v, problems %v, run stop %q, exit %d: a warning moved the plan", p.Valid, p.Problems, p.RunStop, p.Exit())
	}
	var out strings.Builder
	night.Print(&out, p)
	text := out.String()
	r1, r2 := strings.Index(text, "row r1:"), strings.Index(text, "row r2:")
	if w := strings.Index(text, "\n  warning: "+night.PerActVerify+": §2 "); r1 < 0 || w < r1 || w > r2 {
		t.Fatalf("Print holds no warning under row r1:\n%s", text)
	}
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "  warning: ") && len([]rune(l)) >= 100 {
			t.Fatalf("%d columns: %s", len([]rune(l)), l)
		}
	}
	b, err := json.Marshal(p)
	if err != nil || !strings.Contains(string(b), `"warnings":[]`) {
		t.Fatalf("the JSON holds no empty warnings (%v):\n%s", err, b)
	}
	_, _, _, _, slow := ar61World(t, "make check", 601*time.Second, ar61PerAct("r1", "r2"), specs...)
	ps := slow()
	if ps.VerifySeconds != 601 || len(ps.Rows[0].Warnings) != 1 || !strings.HasSuffix(ps.Rows[0].Warnings[0], "; it is the gate's alone") {
		t.Fatalf("at %d s r1 warns %q; want it is the gate's alone at 601 s", ps.VerifySeconds, ps.Rows[0].Warnings)
	}
}
