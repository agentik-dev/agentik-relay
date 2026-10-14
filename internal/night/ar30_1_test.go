package night

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// blockKeys are the Hermetic keys the plan reads the block by (AR30 act 1).
const (
	lsKey  = "/root ls-tree --full-tree --name-only " + mainAt + " -- " + LedgerFile + " CLAUDE.md"
	revKey = "/root rev-parse --verify --end-of-options refs/heads/main^{commit}"
	mainAt = "0123456789abcdef0123456789abcdef01234567"
	catKey = "/root cat-file blob " + mainAt + ":CLAUDE.md"
)

// TestAR30_1_PlanReadsBlock (AR30 act 1): the plan reads the block at
// main by Hermetic: no ledger, none and CLAUDE.md unread; a ledger with a
// block, the block verbatim and its source; a ledger with none, not
// rendered and valid; markers not one BEGIN before one END, a problem.
func TestAR30_1_PlanReadsBlock(t *testing.T) {
	block := prompt.BlockBegin + "\nsource\n" + prompt.BlockEnd + "\n"
	for _, c := range []struct {
		name, ls, md, source string
		block                []byte
		valid                bool
	}{
		{"no ledger", "CLAUDE.md", "", BlockNone, nil, true},
		{"a block", "CLAUDE.md\n" + LedgerFile, "# T\n" + block + "after", "CLAUDE.md at " + mainAt[:12], []byte(block), true},
		{"no block", "CLAUDE.md\n" + LedgerFile, "# T\nprose", BlockNotRendered, nil, true},
		{"bad markers", "CLAUDE.md\n" + LedgerFile, block + prompt.BlockEnd, BlockNotRendered, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo, q := fixture(t)
			fx, g, _, _ := fakes(repo, false)
			g.Hermetics = map[string]string{lsKey: c.ls, revKey: mainAt, catKey: c.md}
			p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a4-schemas"}})
			if err != nil {
				t.Fatal(err)
			}
			if p.BlockSource != c.source || !bytes.Equal(p.Block, c.block) || p.Valid != c.valid {
				t.Fatalf("block %q source %q valid %v; problems %v", p.Block, p.BlockSource, p.Valid, p.Problems)
			}
			read := false
			for _, call := range g.Calls() {
				read = read || strings.Contains(call, "CLAUDE.md") && strings.Contains(call, "cat-file")
			}
			if read != (c.ls != "CLAUDE.md") {
				t.Fatalf("CLAUDE.md read %v: %v", read, g.Calls())
			}
			var b bytes.Buffer
			Print(&b, p)
			if !strings.Contains(b.String(), "; block: "+c.source+"\n") {
				t.Fatalf("the template line does not name the block:\n%s", b.String())
			}
		})
	}
}

// movingMain is the machine's Git with a commit landing on main right
// after the plan's first read, as a merge landing during `agentik-relay plan`
// would; the commit removes CLAUDE.md.
type movingMain struct {
	effects.OSGit
	t     *testing.T
	moved bool
}

func (m *movingMain) Hermetic(dir string, args ...string) (string, error) {
	out, err := m.OSGit.Hermetic(dir, args...)
	if !m.moved {
		m.moved = true
		landedGit(m.t, dir, "rm", "-q", "CLAUDE.md")
		landedGit(m.t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "drop CLAUDE.md")
	}
	return out, err
}

// TestAR30_1_BlockReadsOneSha (AR30 act 1): the plan resolves main to
// one sha first and reads the ledger, CLAUDE.md and the block at that
// sha, so a commit landing on main between the reads is no plan problem.
func TestAR30_1_BlockReadsOneSha(t *testing.T) {
	d := t.TempDir()
	landedGit(t, d, "init", "-q", "-b", "main")
	block := prompt.BlockBegin + "\nsource\n" + prompt.BlockEnd + "\n"
	for name, body := range map[string]string{LedgerFile: "# D\n", "CLAUDE.md": "# C\n" + block} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	landedGit(t, d, "add", "-A")
	landedGit(t, d, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "pinned")
	pinned := landedGit(t, d, "rev-parse", "main")
	g := &movingMain{t: t}
	var p Plan
	if why := readBlock(g, d, &p); why != "" {
		t.Fatalf("main moved between the reads: %s", why)
	}
	if !g.moved || landedGit(t, d, "rev-parse", "main") == pinned {
		t.Fatal("main never moved")
	}
	if p.BlockSource != "CLAUDE.md at "+short(pinned) || string(p.Block) != block {
		t.Fatalf("block %q source %q; want the block at %s", p.Block, p.BlockSource, short(pinned))
	}
}
