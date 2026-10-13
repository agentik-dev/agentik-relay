package night

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// The wake (usage-limit-resume r1 §2, AR67): a paused row's continuation
// runs in its own tree under what its parts tonight left of its budget.

// The continuation's floor: never under 10 turns or $5 (r1 §2).
const (
	continueTurns = 10
	continueUSD   = 5
)

// remainder is a continuation's --max-turns and --max-budget-usd (AR67
// ruling 2): the queue row's turns and usd less its parts' tonight, each
// `sitting` line's turns and its cost_usd, or, when cost_read is false,
// its `spawn` line's usd (the last before it); at least 10 turns and $5,
// or the row's own turns and usd when less (AR69 ruling 3), the dollars
// rounded down. parts is the row's lines in ledger order.
func remainder(turns, usd int, parts []map[string]any) (int, int) {
	var spentTurns, spentUSD, spawnUSD float64
	for _, l := range parts {
		switch l["event"] {
		case "spawn":
			spawnUSD = number(l["usd"])
		case "sitting":
			spentTurns += number(l["turns"])
			if l["cost_read"] == false {
				spentUSD += spawnUSD
			} else {
				spentUSD += number(l["cost_usd"])
			}
		}
	}
	t := max(turns-int(spentTurns), min(continueTurns, turns))
	d := max(int(math.Floor(float64(usd)-spentUSD)), min(continueUSD, usd))
	return t, d
}

// continueArgv is the plan's argv with a new --session-id, --max-turns
// and --max-budget-usd (AR67 ruling 2): a copy, the rest the plan's.
func continueArgv(argv []string, session string, turns, usd int) []string {
	out := slices.Clone(argv)
	set := map[string]string{"--session-id": session, "--max-turns": strconv.Itoa(turns), "--max-budget-usd": strconv.Itoa(usd)}
	for i := 0; i+1 < len(out); i++ {
		if v, ok := set[out[i]]; ok {
			out[i+1] = v
			i++
		}
	}
	return out
}

// rowKit is what a row's start rendered, kept for its continuations
// (AR67 ruling 5): the prompt, the options before their Ctx, hold and
// Log, the last part's number and its session (AR68's RESUME block).
type rowKit struct {
	stdin   []byte
	ro      RunOptions
	part    int
	session string
}

// resumeOf is a continuation's RESUME block (AR68 ruling 1): part, the
// last part's session and the part's ceilings; the commits since the
// stage's head_before by git log in the tree, newest first, and its
// uncommitted paths by Status. A read that fails is listed as one line.
func resumeOf(g effects.Git, st *Staged, part int, session string, turns, usd int) string {
	r := prompt.Resume{Part: part, Turns: turns, USD: usd, Session: session}
	if out, err := g.Hermetic(st.Path, "log", "--no-show-signature", "--format=%h %s", st.HeadBefore+"..HEAD"); err != nil {
		r.Commits = []string{"(not read: " + oneLine(err.Error()) + ")"}
	} else {
		for _, l := range strings.Split(out, "\n") {
			if l != "" {
				r.Commits = append(r.Commits, l)
			}
		}
	}
	if lines, err := g.Status(st.Path); err != nil {
		r.Paths = []string{"(not read: " + oneLine(err.Error()) + ")"}
	} else {
		for _, l := range lines {
			if len(l) > 3 { // porcelain: two status letters and a space, then the path
				l = l[3:]
			}
			r.Paths = append(r.Paths, l)
		}
	}
	return prompt.ResumeBlock(r)
}

// resumeStdin is a continuation's stdin (AR68 ruling 1): the row's
// prompt, a blank line whether or not the prompt ends in a newline (the
// fix round's F2), then the RESUME block.
func resumeStdin(row []byte, block string) []byte {
	out := append([]byte(nil), row...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(append(out, '\n'), block...)
}

// oneLine is s's first line.
func oneLine(s string) string { return strings.TrimSpace(strings.SplitN(s, "\n", 2)[0]) }

// parts is row id's `spawn` and `sitting` lines tonight, in ledger order:
// remainder's input.
func (t *tap) parts(id string) []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []map[string]any
	for _, l := range t.lines {
		if l["row"] == id && (l["event"] == "spawn" || l["event"] == "sitting") {
			out = append(out, l)
		}
	}
	return out
}

// treeHeld is the tree check at the wake (AR67 ruling 4): the paused
// row's worktree is on the stage's branch and its HEAD descends from the
// stage's head_before. A read that fails is a moved tree.
func treeHeld(g effects.Git, st *Staged) bool {
	if b, err := g.Branch(st.Path); err != nil || b != st.Branch {
		return false
	}
	head, err := g.Head(st.Path)
	if err != nil {
		return false
	}
	ok, err := g.IsAncestor(st.Path, st.HeadBefore, head)
	return err == nil && ok
}
