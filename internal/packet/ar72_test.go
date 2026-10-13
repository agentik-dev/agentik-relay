package packet

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/yaml"
)

// ar72Report is the set of plugin agents that owe their caller a report and
// so bound their probes in tool calls (AR72 rulings 1 and 3); ar72Exempt is
// the set that does not, each held to a cap of 20 turns or less (ruling 4).
// A new agent belongs in one of the two by a ruling. The attendant was exempt
// and moved to the report set under AR73 (its rulings 1 and 3), so the exempt
// set is empty and its cap waits for a future exempt agent.
var (
	ar72Report = []string{"adversary", "builder", "attendant"}
	ar72Exempt = []string{}
	ar72Bound  = regexp.MustCompile(`\b(\d+) tool calls\b`)
)

// ar72Keep is a stop an agent keeps beside its tool-call bound (AR72 fix
// round one, F1): every packet states its budget in turns and context, none
// in tool calls, so the builder's 80% checkpoint stays and the bound only
// adds a second stop, whichever comes first.
var ar72Keep = map[string]string{"builder": "Past 80% of the packet's budget"}

// ar72Margin is the turns an agent keeps past its tool-call bound to write
// its report (AR72 ruling 2): N + ar72Margin <= maxTurns.
const ar72Margin = 20

// ar72ExemptCap is the most turns an exempt agent may carry (AR72 ruling 4).
const ar72ExemptCap = 20

// ar72Agent reads plugin/agents/<name>.md from dir: its frontmatter between
// the first two `---` lines, parsed, and its body as bullets, each joined on
// whitespace (a bullet opens on a line starting "- " and runs to the next).
func ar72Agent(t *testing.T, dir, name string) (yaml.Node, []string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		t.Fatalf("%s.md: no frontmatter", name)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		t.Fatalf("%s.md: frontmatter not closed", name)
	}
	fm, err := yaml.Parse([]byte(strings.Join(lines[1:end], "\n") + "\n"))
	if err != nil {
		t.Fatalf("%s.md: frontmatter: %v", name, err)
	}
	var bullets []string
	cur := -1
	for _, line := range lines[end+1:] {
		switch {
		case strings.HasPrefix(line, "- "):
			bullets = append(bullets, line)
			cur = len(bullets) - 1
		case strings.TrimSpace(line) == "":
			cur = -1
		case cur >= 0:
			bullets[cur] += " " + line
		}
	}
	for i, s := range bullets {
		bullets[i] = strings.Join(strings.Fields(s), " ")
	}
	return fm, bullets
}

// ar72MaxTurns reads the frontmatter's maxTurns as an int.
func ar72MaxTurns(t *testing.T, name string, fm yaml.Node) int {
	t.Helper()
	v, ok := fm.Get("maxTurns")
	if !ok {
		t.Fatalf("%s.md: no maxTurns in the frontmatter", name)
	}
	n, ok := v.Int()
	if !ok {
		t.Fatalf("%s.md: maxTurns %q is not an int", name, v.Str)
	}
	return n
}

// TestAR72_1_ReportBound (AR72 rulings 1 to 3, 6): each agent that owes a
// report carries, in its body, a bound of "<N> tool calls" (digits, the
// first such N) with N + 20 <= its maxTurns, and the bullet that carries it
// says the report is "never skipped": the agent can count its own calls but
// cannot see its turns. An agent named in ar72Keep also keeps that stop in
// a bullet (fix round one, F1). It reads a copy of plugin/agents.
func TestAR72_1_ReportBound(t *testing.T) {
	dir := filepath.Join(copyIn(t, "../..", "plugin/agents"), "plugin", "agents")
	for _, name := range ar72Report {
		fm, bullets := ar72Agent(t, dir, name)
		turns := ar72MaxTurns(t, name, fm)
		found := false
		for _, b := range bullets {
			m := ar72Bound.FindStringSubmatch(b)
			if m == nil {
				continue
			}
			found = true
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("%s.md: bound %q: %v", name, m[1], err)
			}
			if n+ar72Margin > turns {
				t.Errorf("%s.md: bound %d tool calls + %d > maxTurns %d", name, n, ar72Margin, turns)
			}
			if !strings.Contains(b, "never skipped") {
				t.Errorf("%s.md: the bullet with %q does not say the report is \"never skipped\"", name, m[0])
			}
			break
		}
		if !found {
			t.Errorf("%s.md: no \"<N> tool calls\" bound in the body", name)
		}
		if keep, ok := ar72Keep[name]; ok {
			kept := false
			for _, b := range bullets {
				kept = kept || strings.Contains(b, keep)
			}
			if !kept {
				t.Errorf("%s.md: no bullet keeps %q (fix round one, F1)", name, keep)
			}
		}
	}
}

// TestAR72_2_AgentsClassified (AR72 rulings 4, 6): the agents under
// plugin/agents are exactly the report set and the exempt set, so a new
// agent fails here until a ruling places it; each exempt agent's maxTurns is
// at most 20, so raising one needs a ruling too. It reads a copy of
// plugin/agents.
func TestAR72_2_AgentsClassified(t *testing.T) {
	dir := filepath.Join(copyIn(t, "../..", "plugin/agents"), "plugin", "agents")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			got = append(got, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	want := append(append([]string{}, ar72Report...), ar72Exempt...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("plugin/agents holds %v, want the report set %v plus the exempt set %v", got, ar72Report, ar72Exempt)
	}
	for _, name := range ar72Exempt {
		fm, _ := ar72Agent(t, dir, name)
		if turns := ar72MaxTurns(t, name, fm); turns > ar72ExemptCap {
			t.Errorf("%s.md: exempt with maxTurns %d, want at most %d", name, turns, ar72ExemptCap)
		}
	}
}
