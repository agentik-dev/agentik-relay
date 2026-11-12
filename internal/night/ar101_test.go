package night

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// ar101Queue is fixture's repo and three rows with the queue rewritten:
// defaults gains defaultsEffort when it is not "", and each row its value
// in rowEffort.
func ar101Queue(t *testing.T, defaultsEffort string, rowEffort map[string]string) (string, *queue.Queue) {
	t.Helper()
	repo, _ := fixture(t)
	d := ""
	if defaultsEffort != "" {
		d = ", effort: " + defaultsEffort
	}
	src := "version: 1\ndefaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,\n" +
		"           merge: branch-only, device: never, verify: auto" + d + "}\nrows:\n"
	for _, r := range []struct{ id, rest string }{
		{"a3-machine", "    depends_on: [a2-relay-design]\n"},
		{"a4-schemas", "    depends_on: [ar1-relay-hooks]\n"},
		{"a5-effects-plan", "    depends_on: [a4-schemas]\n    base: a4-schemas\n"},
	} {
		src += "  - id: " + r.id + "\n    packet: design/packets/" + r.id + ".md\n" + r.rest
		if e := rowEffort[r.id]; e != "" {
			src += "    effort: " + e + "\n"
		}
	}
	os.WriteFile(filepath.Join(repo, "queue.yaml"), []byte(src), 0o644)
	q, err := queue.Load(repo, "queue.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return repo, q
}

// ar101Plan is the plan over q with fixture's fakes, a4-schemas' dependency
// a2-relay-design landed in git as TestPlanOverFakes has it.
func ar101Plan(t *testing.T, repo string, q *queue.Queue) (*Plan, effects.Effects) {
	t.Helper()
	fx, g, _, _ := fakes(repo, false)
	g.Existing["a2-relay-design"], g.Merged["a2-relay-design..main"] = true, true
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", UUID: counter()})
	if err != nil {
		t.Fatal(err)
	}
	return p, fx
}

// ar101Effort is the --effort a spawn line names, and whether it sits
// right after --model's value.
func ar101Effort(argv []string) (string, bool) {
	i := slices.Index(argv, "--effort")
	m := slices.Index(argv, "--model")
	if i < 0 || i+1 >= len(argv) {
		return "", false
	}
	return argv[i+1], m >= 0 && i == m+2
}

// TestAR101_2_NoEffortSpawnsHigh (AR101 ruling 2): a queue that names no
// effort spawns every row with `--effort high`, right after --model; a zero
// row does too, and a continuation keeps the level.
func TestAR101_2_NoEffortSpawnsHigh(t *testing.T) {
	repo, q := ar101Queue(t, "", nil)
	p, _ := ar101Plan(t, repo, q)
	if len(p.Rows) != 3 {
		t.Fatalf("%d rows", len(p.Rows))
	}
	for _, rp := range p.Rows {
		if e, after := ar101Effort(rp.Spawn); e != "high" || !after {
			t.Errorf("row %s: spawn %q; want --effort high after --model", rp.ID, rp.Spawn)
		}
	}
	argv := SpawnArgv(queue.Row{Model: "m", Turns: 1, USD: 1}, "s", "Bash", false)
	if e, after := ar101Effort(argv); e != "high" || !after {
		t.Fatalf("a zero row's spawn %q", argv)
	}
	if e, _ := ar101Effort(continueArgv(argv, "s2", 10, 5)); e != "high" {
		t.Fatalf("the continuation drops the level")
	}
}

// TestAR101_3_DefaultsThenRow (ruling 2): the row's value, else the
// defaults', else high.
func TestAR101_3_DefaultsThenRow(t *testing.T) {
	for _, c := range []struct {
		defaults string
		rows     map[string]string
		want     []string
	}{
		{"medium", nil, []string{"medium", "medium", "medium"}},
		{"medium", map[string]string{"a5-effects-plan": "xhigh"}, []string{"medium", "medium", "xhigh"}},
		{"", map[string]string{"a4-schemas": "low", "a5-effects-plan": "max"}, []string{"high", "low", "max"}},
	} {
		repo, q := ar101Queue(t, c.defaults, c.rows)
		p, _ := ar101Plan(t, repo, q)
		var got []string
		for _, rp := range p.Rows {
			e, _ := ar101Effort(rp.Spawn)
			got = append(got, e)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("defaults %q rows %v: %v; want %v", c.defaults, c.rows, got, c.want)
		}
	}

	// The review's F1: a row whose effort is empty, quoted or bare, spawns at
	// the defaults' level. ar101Queue skips an empty value, so the queue is
	// written by hand.
	for _, c := range []struct{ name, val string }{{"empty-quoted", ` ""`}, {"empty-bare", ""}} {
		t.Run(c.name, func(t *testing.T) {
			repo, _ := fixture(t)
			os.WriteFile(filepath.Join(repo, "empty.yaml"), []byte("version: 1\n"+
				"defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,\n"+
				"           merge: branch-only, device: never, verify: auto, effort: medium}\n"+
				"rows:\n  - id: a3-machine\n    packet: design/packets/a3-machine.md\n"+
				"    depends_on: [a2-relay-design]\n    effort:"+c.val+"\n"), 0o644)
			q, err := queue.Load(repo, "empty.yaml")
			if err != nil {
				t.Fatal(err)
			}
			p, _ := ar101Plan(t, repo, q)
			if len(p.Rows) != 1 {
				t.Fatalf("%d rows", len(p.Rows))
			}
			if e, after := ar101Effort(p.Rows[0].Spawn); e != "medium" || !after {
				t.Errorf("row effort%s over medium: spawn %q; want --effort medium after --model", c.val, p.Rows[0].Spawn)
			}
		})
	}
}

// TestAR101_4_BadValueSpawnsNothing (ruling 1): a value outside the five is
// a row problem, the plan is invalid, and the night spawns nothing.
func TestAR101_4_BadValueSpawnsNothing(t *testing.T) {
	repo, q := ar101Queue(t, "", map[string]string{"a4-schemas": "huge"})
	p, fx := ar101Plan(t, repo, q)
	const why = `effort "huge"; want low, medium, high, xhigh or max`
	if p.Valid || p.Rows[1].State != StateInvalid || !slices.Contains(p.Rows[1].Problems, why) {
		t.Fatalf("plan valid %v, a4 %s %v", p.Valid, p.Rows[1].State, p.Rows[1].Problems)
	}
	sp := &effects.FakeSpawner{}
	_, err := Run(fx, sp, q, NightOptions{PlanOptions: Options{Repo: repo, UUID: counter()}, State: "/state"})
	if err == nil || !strings.Contains(err.Error(), "a4-schemas "+why) || len(sp.Spawns()) != 0 {
		t.Fatalf("run: err %v, %d spawns", err, len(sp.Spawns()))
	}
}

// TestAR101_5_DryRunShowsLevel (ruling 2): the plan's printed spawn lines
// and the wrapped line `run --dry` prints each name the level.
func TestAR101_5_DryRunShowsLevel(t *testing.T) {
	repo, q := ar101Queue(t, "low", map[string]string{"a3-machine": "max"})
	p, _ := ar101Plan(t, repo, q)
	var out strings.Builder
	Print(&out, p)
	if s := out.String(); strings.Count(s, "--effort max") != 1 || strings.Count(s, "--effort low") != 2 {
		t.Fatalf("Print:\n%s", s)
	}
	if s := strings.Join(WrapSpawn(p.Rows[0].Worktree, p.Rows[0].Spawn, "/home"), "\n"); !strings.Contains(s, "--effort max") {
		t.Fatalf("WrapSpawn:\n%s", s)
	}
}

// TestAR101_6_OrchestratorCarries (ruling 2): an orchestrator row's plan
// and its sitting spawn by the same rule.
func TestAR101_6_OrchestratorCarries(t *testing.T) {
	for _, c := range []struct{ defaults, row, want string }{
		{"", "", "high"},
		{"low", "", "low"},
		{"low", "xhigh", "xhigh"},
	} {
		repo, _ := fixture(t)
		d, o := "", ""
		if c.defaults != "" {
			d = ", effort: " + c.defaults
		}
		if c.row != "" {
			o = "    effort: " + c.row + "\n"
		}
		os.WriteFile(filepath.Join(repo, "orch.yaml"), []byte("version: 1\n"+
			"defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,\n"+
			"           merge: merge-on-green, device: never, verify: auto"+d+"}\n"+
			"rows:\n  - id: o\n    kind: orchestrator\n    prompt: history/brief.md\n    sections: [How this runs]\n"+o), 0o644)
		q, err := queue.Load(repo, "orch.yaml")
		if err != nil {
			t.Fatal(err)
		}
		fx, g, _, _ := fakes(repo, false)
		g.Hermetics = map[string]string{revKey: mainAt, promptKey: "history/brief.md"}
		p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Orchestrate: true, UUID: counter()})
		if err != nil {
			t.Fatal(err)
		}
		r, _ := q.Orchestrator()
		s := SittingPlan(p, r, nil)
		e1, after := ar101Effort(s.Spawn)
		e0, _ := ar101Effort(p.Rows[0].Spawn)
		if e1 != c.want || !after || e0 != c.want {
			t.Errorf("defaults %q row %q: sitting %q, plan row %q; want %s", c.defaults, c.row, e1, e0, c.want)
		}
	}
}
