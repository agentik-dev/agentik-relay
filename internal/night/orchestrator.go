package night

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// CharterDir is where a sitting's charter lives, at main (orchestrator r1
// §3): `<date>-<n>-<id>.md`.
const CharterDir = "history/sittings"

// AgentTool is the tool an orchestrator sitting's helpers run by (§3).
const AgentTool = "Agent"

// WithAgent is the allowed tools with Agent added once (§3).
func WithAgent(tools string) string {
	for _, t := range strings.Fields(tools) {
		if t == AgentTool {
			return tools
		}
	}
	return strings.TrimSpace(tools + " " + AgentTool)
}

// mainSha is main's commit in root, read by the hermetic runner.
func mainSha(g effects.Git, root string) (string, error) {
	return g.Hermetic(root, "rev-parse", "--verify", "--end-of-options", "refs/heads/main^{commit}")
}

// promptProblem is why the orchestrator row's brief is absent at main
// (§2), empty when it is there. sha is main's, read again when empty.
func promptProblem(g effects.Git, root, sha string, r queue.Row) string {
	var err error
	if sha == "" {
		if sha, err = mainSha(g, root); err != nil {
			return fmt.Sprintf("prompt %s: %v", r.Prompt, err)
		}
	}
	ls, err := g.Hermetic(root, "ls-tree", "--full-tree", "--name-only", sha, "--", r.Prompt)
	if err != nil {
		return fmt.Sprintf("prompt %s: %v", r.Prompt, err)
	}
	if !contains(strings.Split(ls, "\n"), filepath.ToSlash(filepath.Clean(r.Prompt))) {
		return fmt.Sprintf("prompt %s: absent at main", r.Prompt)
	}
	return ""
}

// NextAct is the newest charter of row at main and the first non-empty
// paragraph under its `## Next act` (§3): the highest date, then the
// highest n as a number, among `history/sittings/<date>-<n>-<row>.md`.
// With none, both are empty and err nil.
func NextAct(g effects.Git, root, row string) (charter, act string, err error) {
	sha, err := mainSha(g, root)
	if err != nil {
		return "", "", fmt.Errorf("main: %w", err)
	}
	if charter, err = NewestCharter(g, root, sha, row); err != nil || charter == "" {
		return "", "", err
	}
	md, err := g.Hermetic(root, "cat-file", "blob", sha+":"+charter)
	if err != nil {
		return charter, "", fmt.Errorf("%s at main: %w", charter, err)
	}
	return charter, NextActOf(md), nil
}

// NewestCharter is the newest charter of row in the tree of sha (§3): the
// highest date, then the highest n as a number, among
// `history/sittings/<date>-<n>-<row>.md`; "" with none.
func NewestCharter(g effects.Git, dir, sha, row string) (string, error) {
	ls, err := g.Hermetic(dir, "ls-tree", "--full-tree", "--name-only", sha, "--", CharterDir+"/")
	if err != nil {
		return "", fmt.Errorf("the charters at %s: %w", sha, err)
	}
	shape := regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2})-([0-9]+)-` + regexp.QuoteMeta(row) + `\.md$`)
	var date, charter string
	n := -1
	for _, p := range strings.Split(ls, "\n") {
		m := shape.FindStringSubmatch(strings.TrimPrefix(p, CharterDir+"/"))
		if m == nil || filepath.Dir(p) != CharterDir {
			continue
		}
		k, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if m[1] > date || m[1] == date && k > n {
			date, n, charter = m[1], k, p
		}
	}
	return charter, nil
}

// NextActOf is the first non-empty paragraph under `## Next act` in md:
// its lines up to a blank line or the next heading, joined by newlines.
func NextActOf(md string) string {
	var para []string
	in := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#"):
			if len(para) > 0 {
				return strings.Join(para, "\n")
			}
			in = strings.TrimSpace(strings.TrimLeft(line, "#")) == "Next act" && strings.HasPrefix(line, "## ")
		case !in:
		case trimmed == "":
			if len(para) > 0 {
				return strings.Join(para, "\n")
			}
		default:
			para = append(para, strings.TrimRight(line, " \t\r"))
		}
	}
	return strings.Join(para, "\n")
}

// RespawnPrefix is the checkpoint's mark an act may carry (§4).
const RespawnPrefix = "RESPAWN: "

// ActKey is the no-progress key of a Next act (§4): a leading
// `RESPAWN: ` stripped and whitespace folded.
func ActKey(act string) string {
	return strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(act), RespawnPrefix)), " ")
}

// SittingPlan is the orchestrator row r's next sitting (§4): `<id>.<n>`,
// n one past the highest the ledger's lines hold for the row across
// nights, with a fresh session, branch and worktree, Agent among the
// allowed tools, and Row set to r's id for RELAY_ROW.
func SittingPlan(p *Plan, r queue.Row, lines []map[string]any) RowPlan {
	n := 0
	for _, l := range lines {
		id, _ := l["row"].(string)
		rest, ok := strings.CutPrefix(id, r.ID+".")
		if !ok {
			continue
		}
		if k, err := strconv.Atoi(rest); err == nil && k > n && strconv.Itoa(k) == rest {
			n = k
		}
	}
	id := fmt.Sprintf("%s.%d", r.ID, n+1)
	uuid, tools := p.opts.UUID, p.opts.AllowedTools
	if uuid == nil {
		uuid = NewUUID
	}
	if tools == "" {
		tools = effects.DefaultAllowedTools
	}
	rp := RowPlan{ID: id, State: StateReady, Pending: []string{}, Satisfied: []string{}, Baseline: []relay.Check{},
		Problems: []string{}, Branch: id, Base: "main", Row: r.ID,
		Worktree: filepath.Join(p.Root, ".claude", "worktrees", id), SessionID: uuid()}
	rp.Spawn = SpawnArgv(r, rp.SessionID, WithAgent(tools), p.opts.Caffeinate)
	return rp
}

// SittingStart is what an orchestrator sitting began from (§3, §5): the
// queue at main, main's sha, main's newest charter of the row and its Next
// act, the red and parked rows, the build rows green on their branches
// with no MergeSHA (ar32b act 2), and the open questions at main. The loop
// sets it on the sitting's RowPlan; a caller's Row reads it for the
// prompt and the gate.
type SittingStart struct {
	Began              *queue.Queue
	Main, Charter, Act string
	Red, Green         []string
	Open               int
}

// RenderSitting is an orchestrator sitting's stdin (§3): the plan's block,
// the orchestrator's RULES, CONTEXT from rp.Sitting, and the body line.
func RenderSitting(p *Plan, r queue.Row, rp *RowPlan, words string) ([]byte, string, string) {
	s := rp.Sitting
	if s == nil {
		s = &SittingStart{}
	}
	c := prompt.OrchestratorContext{Row: r.ID, Sitting: rp.ID, Tree: rp.Worktree, Branch: rp.Branch, Main: s.Main,
		Brief: r.Prompt, CharterDir: CharterDir, Charter: s.Charter, NextAct: s.Act, Red: s.Red, Green: s.Green, OpenQuestions: s.Open,
		Words: words, Ceiling: r.Context, Soft: int(SoftLine * float64(r.Context)), Kill: int(HardCap * float64(r.Context)), Turns: r.Turns}
	return prompt.RenderOrchestrator(c, p.Block, r.Sections)
}

// OpenQuestionsFile is where the owner's batched questions live (§7).
const OpenQuestionsFile = "OPEN-QUESTIONS.md"

// OpenQuestions counts md's open questions (§7): the `## <n>.` headings
// above the first `## Decided without asking` or `## Answered`.
func OpenQuestions(md []byte) int {
	n := 0
	for _, line := range strings.Split(string(md), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimRight(line, " \t\r"), "## ")
		if !ok {
			continue
		}
		if strings.HasPrefix(rest, "Decided without asking") || strings.HasPrefix(rest, "Answered") {
			break
		}
		if num, _, ok := strings.Cut(rest, "."); ok && num != "" {
			if _, err := strconv.Atoi(num); err == nil {
				n++
			}
		}
	}
	return n
}

// openAt is the open questions in OPEN-QUESTIONS.md at sha; 0 without it.
func openAt(g effects.Git, root, sha string) int {
	md, err := g.Hermetic(root, "cat-file", "blob", sha+":"+OpenQuestionsFile)
	if err != nil {
		return 0
	}
	return OpenQuestions([]byte(md))
}

// actWord is a Next act key's word (§3): "DONE" or "WAITING", read past a
// list mark; "" for a concrete step.
func actWord(key string) string {
	k := strings.TrimLeft(key, "-* ")
	for _, w := range []string{"DONE", "WAITING"} {
		if strings.HasPrefix(k, w+":") {
			return w
		}
	}
	return ""
}

// orchestra is an orchestrated night's standing row (§4): its sittings,
// the rows they added, the start of the one running, and its hold, red
// and stuck state.
type orchestra struct {
	row      queue.Row
	qpath    string
	sittings map[string]bool
	added    map[string]bool
	current  string
	start    SittingStart
	startKey string
	hold     bool // WAITING, or DONE with its rows pending: until a build row ends
	red      bool // a red sitting: parked for the day, the ready rows run, then the stop
	stuck    int
	open     int // open questions at main as last counted
}

func (o *orchestra) is(id string) bool { return o != nil && o.sittings[id] }

// pending is the first row the orchestrator added that is ready, running,
// or green and unmerged (§4); "" with none.
func (o *orchestra) pending(ids []string, s map[string]*relay.Sitting) string {
	for _, id := range ids {
		st := s[id]
		if !o.added[id] || st == nil {
			continue
		}
		switch st.State {
		case relay.Ready, relay.Waiting, relay.Running:
			return id
		case relay.Green:
			if st.MergeSHA == "" {
				return id
			}
		}
	}
	return ""
}

// stuckAfter reports a stuck sitting (§4): the key at main after its merge
// equals the key it began from, and the merge changed no path outside
// CharterDir.
func stuckAfter(g effects.Git, root, before, after, key0, key1 string) (bool, error) {
	if key0 != key1 {
		return false, nil
	}
	out, err := g.Hermetic(root, "diff", "--name-only", "--no-renames", before, after, "--")
	if err != nil {
		return false, err
	}
	for _, p := range strings.Split(out, "\n") {
		if p = strings.TrimSpace(p); p != "" && !strings.HasPrefix(p, CharterDir+"/") {
			return false, nil
		}
	}
	return true, nil
}
