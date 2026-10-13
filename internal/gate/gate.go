// Package gate gathers §6's facts for one row through effects and judges
// them with relay.Decide, so `relay gate <row>` by hand and the night's own
// gate are the same code; and it merges only on green (D22).
package gate

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// ExitFacts are the sitting's end as the stream showed it; nil for a hand
// gate, whose exit check is then skipped.
type ExitFacts struct{ Seen, IsError bool }

// Verify is the repo's verify line for the gate: Name is the queue's
// `verify` (night.VerifyLine's input; empty is none), Baseline the plan's
// measure on main (nil for a hand gate: green only), Harness the row's
// queue `harness` (queue.HarnessOf resolves it; nil is the default).
// Source is the packets source (effects.Config.Packets, AR29 act 3): set,
// the gate compares design/packets/<id>.md at the head with the row's file
// there (PacketSource); "" compares nothing. CopyInSkipped is a row that
// declared its copy-in skipped (the settled entry 1, 2026-09-25): nothing is read and
// the check is recorded skipped.
//
// Orchestrator set is an orchestrator sitting's gate (AR32A act 3,
// orchestrator r1 §5): the charter, queue and merge checks, and its own
// touch bounds; the packet then holds no acceptance line. QueuePath is the
// queue's repo-relative path in an orchestrated night: the orchestrator's
// queue check reads it (effects.DefaultQueue when empty), and it is
// outside every build row's bounds.
//
// Limit is the verify line's limit (the repo's verify_seconds through
// effects.Config.VerifyLimit; AR59 act 2): 0 is effects.Timeout. The
// gate's verify and the merge's (Gate.VerifySeconds) run under it; the
// acceptance lines keep effects.Timeout.
type Verify struct {
	Name          string
	Baseline      *relay.Check
	Harness       []string
	Source        string
	CopyInSkipped bool
	Orchestrator  *Orchestrated
	QueuePath     string
	Limit         time.Duration
}

// limit is v's verify limit: Limit, else effects.Timeout.
func (v Verify) limit() time.Duration {
	if v.Limit <= 0 {
		return effects.Timeout
	}
	return v.Limit
}

// Orchestrated is what an orchestrator sitting is judged against: Row, the
// standing row as the night began; Night, the queue as the night began,
// whose defaults, words and standing row the head must keep; Began, the
// queue at main when the sitting began, whose rows the head keeps as they
// were, in order, before the rows it appends.
type Orchestrated struct {
	Row          queue.Row
	Night, Began *queue.Queue
}

// CharterMax is a charter's size bound: under 4 KB (§5).
const CharterMax = 4096

// PacketDir is where act 0 commits a row's packet, <id>.md (AR26).
const PacketDir = "design/packets"

// PacketSource is row id's packet in the packets source dir (AR29 act 3,
// ADV17-2): <dir>/<id>.md, else the one <dir>/<YYYY-MM-DD>-<id>.md (a
// helper's batch names its files by date); none is "", several an error,
// and so is a relative dir or an id that is not one path element.
func PacketSource(dir, id string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("packets %q: want an absolute dir", dir)
	}
	if id == "" || id != filepath.Base(id) || !filepath.IsLocal(id) {
		return "", fmt.Errorf("packets: row id %q is not one path element", id)
	}
	exact := filepath.Join(dir, id+".md")
	if _, err := os.Stat(exact); err == nil {
		return exact, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("packets: %w", err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("packets: %w", err)
	}
	var found []string
	for _, e := range ents {
		n := e.Name()
		if len(n) != len(id)+14 || !strings.HasSuffix(n, "-"+id+".md") {
			continue
		}
		if _, err := time.Parse("2006-01-02", n[:10]); err == nil {
			found = append(found, n)
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return filepath.Join(dir, found[0]), nil
	}
	return "", fmt.Errorf("packets %s: %d sources for %s (%s)", dir, len(found), id, strings.Join(found, ", "))
}

// packetDiffers compares the packet committed at head, <PacketDir>/<id>.md,
// with the source file src by blob id (git's own hash of the source's
// bytes, no filters), so every byte counts, a trailing newline included:
// the hermetic runner trims its output. Absent at head, or not a blob, it
// differs.
func packetDiffers(git effects.Git, dir, head, id, src string) (bool, error) {
	entry, err := git.Hermetic(dir, "ls-tree", "--full-tree", head, "--", path.Join(PacketDir, id+".md"))
	if err != nil {
		return false, err
	}
	f := strings.Fields(entry)
	if len(f) < 3 || f[1] != "blob" {
		return true, nil
	}
	want, err := git.Hermetic(dir, "hash-object", "--no-filters", "--", src)
	if err != nil {
		return false, err
	}
	return f[2] != want, nil
}

// RulesPaths are the paths the RULES require every sitting to write (its
// charter and the NEXT file), so they belong to every touch set.
var RulesPaths = []packet.Glob{glob("history/sittings/*.md"), glob("history/sessions/NEXT")}

// GuardedPaths are inside a touch set only when a may-touch span names one
// exactly (its Raw): never through `*`, a directory span or RulesPaths, so a
// sitting widens the hygiene allowlist or re-pins the README only when its
// packet says so (the session's S10 handover; §6). An entry ending `/`
// guards everything under it; a span naming the entry or the path admits.
// The harness (AR17A act 2, ADV10-10) is guarded too: these are the
// defaults'; a row's gate guards pinnedPaths and its own harness.
var GuardedPaths = []string{".hygiene-allow", "design/readme/rendered.sha256", "GNUmakefile", "makefile", "Makefile", "scripts/"}

// pinnedPaths are the GuardedPaths every row guards, whatever its harness,
// and .claude/ (AR27B act 3; ADV16-2, ADV14-7): inside only when a span
// names the path or .claude/.
var pinnedPaths = append(append([]string(nil), GuardedPaths[:2]...), ".claude/")

// SettingsPath reports a path under a .claude element (any case) whose
// base matches settings*.json (AR27B act 3; ADV16-2): Claude Code's
// settings, which can turn a later row's hooks off, are outside every
// touch set whatever the spans say.
func SettingsPath(p string) bool {
	els := strings.Split(strings.ToLower(p), "/")
	for _, e := range els[:len(els)-1] {
		if e == ".claude" {
			ok, _ := path.Match("settings*.json", els[len(els)-1])
			return ok
		}
	}
	return false
}

// HarnessPaths are the verify line's default harness: the gate's and the
// merge's verify run with them as they are on main, over the branch's tree,
// so a branch cannot neuter its own verify (ADV10-10). Every make spelling
// is in it (ADV11-4), and a row's queue `harness` replaces the rest
// (queue.HarnessOf; AR20 act 2): the overlay deletes a branch's copy that
// main lacks.
var HarnessPaths = queue.HarnessOf(nil)

// HarnessChanged is the note a merge line carries when the branch changed
// the harness it was judged without.
const HarnessChanged = "harness changed"

func glob(raw string) packet.Glob {
	return packet.Glob{Raw: raw, Dir: strings.HasSuffix(raw, "/"),
		Segs: strings.Split(strings.TrimSuffix(raw, "/"), "/")}
}

// facts are what Gather learned beyond the Gate: the checks it skipped and
// why it stopped short, when it did.
type facts struct {
	skipped  map[string]bool
	err      error
	queueWhy string // why an orchestrator sitting's queue is invalid
}

// Gather collects §6's facts in order in st.Path, bound to one sha first:
// the tree must have the row's branch checked out (the hand gate's own
// detached tree, the branch's tip), else the gate is red; HEAD is then
// g.Head, and every later fact reads it: exit (from the stream, or
// skipped), commit (g.Head moved from HeadBefore; with no HeadBefore, it
// has commits beyond its merge-base with the base), clean, origin (equal
// to the stage snapshot; skipped with none), touch (base...g.Head, no
// rename detection), every acceptance line re-run against its baseline,
// and the verify line (none here: skipped). A fact that cannot be read
// leaves Gathered false: unrun, never a pass.
func Gather(fx effects.Effects, st *night.Staged, p *packet.Packet, baseline []relay.Check, exit *ExitFacts) relay.Gate {
	g, _ := gather(fx, st, p, baseline, exit, Verify{})
	return g
}

func gather(fx effects.Effects, st *night.Staged, p *packet.Packet, baseline []relay.Check, exit *ExitFacts, v Verify) (relay.Gate, facts) {
	var g relay.Gate
	g.VerifySeconds = int(v.limit() / time.Second) // the merge's verify runs under it too (AR59 act 2)
	f := facts{skipped: map[string]bool{}}
	stop := func(what string, err error) (relay.Gate, facts) {
		f.err = fmt.Errorf("%s: %w", what, err)
		return g, f
	}
	w := fx.Git
	nested := effects.Nested(fx.Getenv) // §15: a gate inside a measurement runs no line
	if exit == nil {
		f.skipped["exit"] = true
		g.ResultSeen, g.ExitOK = true, true
	} else {
		g.ResultSeen, g.ExitOK = exit.Seen, !exit.IsError
	}
	// AR22 act 1 (ADV13-2): every account tool the pinned PATH reaches is
	// the file Pin bound at start, read again now; else unrun, tool-moved.
	if moved := effects.ToolsMoved(); len(moved) > 0 {
		g.ToolMoved = true
		return stop("pinned tools", fmt.Errorf("%s: %s", relay.ToolMoved, strings.Join(moved, ", ")))
	}
	// AR20 act 0, first: the git state the stage bound (the common dir's
	// config, hooks and info, the tree's own config.worktree) is as it
	// was, else unrun, git-state-moved; a hand gate with none binds it now.
	root, err := fx.Git.Root(st.Path)
	if err != nil {
		return stop("root", err)
	}
	g.Worktree = st.Path
	if g.StateDigest, err = effects.GitStateDigest(root, st.Path); err != nil {
		return stop("git state", err)
	}
	if st.StateDigest != "" && g.StateDigest != st.StateDigest {
		g.StateMoved = true
		what := effects.GitStateMovedEntry(st.StateDigest, g.StateDigest) // AR53 act 2
		if what == "" {
			what = "config, hooks or info"
		}
		return stop("git state", fmt.Errorf("%s: %s changed since stage", relay.GitStateMoved, what))
	}
	// AR21 act 1 (ADV12-2): no program-valued key in the root's or any
	// worktree's config, and the root's state is the one this process
	// bound at its first stage (a hand gate binds it now).
	if why, err := night.GitStateRefused(root, st.Path, true); why != "" {
		g.StateMoved, g.ConfigProgram = why == relay.GitStateMoved, why == relay.GitConfigProgram
		return stop("git state", fmt.Errorf("%s: %w", why, err))
	}
	// AR21 act 5 (ADV12-7): a row read from the ledger is a claim, never
	// a binding: main, the base, the remotes, their URLs and what each
	// holds are bound from git now, after the program keys are refused.
	if st.FromLedger {
		if st, err = night.Afresh(fx.Git, root, st); err != nil {
			return stop("afresh", err)
		}
	}
	// ADV6-3: the tree's HEAD is the row's branch, and its sha is the one
	// judged and merged; a tree on another branch keeps nothing.
	on, err := fx.Git.Branch(st.Path)
	if err != nil {
		return stop("branch", err)
	}
	head, err := fx.Git.Head(st.Path)
	if err != nil {
		return stop("branch", err)
	}
	tip, err := fx.Git.Tip(st.Path, st.Branch)
	if err != nil {
		return stop("branch", err)
	}
	g.Head = head
	g.OffBranch = head != tip || (on != st.Branch && !(on == "" && st.Detached))
	if st.HeadBefore != "" {
		g.Commit = head != st.HeadBefore
	} else {
		merged, err := fx.Git.IsAncestor(st.Path, head, "refs/heads/"+st.Base)
		if err != nil {
			return stop("commit", err)
		}
		g.Commit = !merged
	}
	status, err := fx.Git.Status(st.Path)
	if err != nil {
		return stop("clean", err)
	}
	g.Clean, g.Dirty = len(status) == 0, len(status)
	if st.Origin == nil {
		f.skipped["origin"] = true
		g.OriginSame = true
	} else {
		refs, err := fx.Git.RemoteRefs(st.Path)
		if err != nil {
			return stop("origin", err)
		}
		g.OriginSame = sameRefs(refs, st.Origin)
	}
	// ADV10-3: what each remote holds now, against stage's ls-remote; a
	// remote stage could not read is skipped, one the gate cannot read unrun.
	// AR20 act 3 (ADV11-7): read at the literal URL stage resolved, never
	// by a name the child can repoint; a stage line with none resolves it
	// now. It is read again after the lines and the verify line (ADV11-6).
	g.Remotes, g.RemoteRefs, g.RemoteURLs = st.Remotes, st.OriginRefs, st.RemoteURLs
	if g.Remotes == nil {
		if g.Remotes, err = fx.Git.Remotes(root); err != nil {
			return stop("remotes", err)
		}
	}
	if g.RemoteURLs == nil && len(st.OriginRefs) > 0 {
		g.RemoteURLs = night.RemoteURLsOf(fx.Git, root, remotesOf(st.OriginRefs))
	}
	same, skipped, err := remotesSame(fx.Git, root, g.RemoteRefs, g.RemoteURLs)
	if err != nil {
		return stop("ls-remote", err)
	}
	f.skipped["ls-remote"] = skipped
	g.OriginSame = g.OriginSame && same
	// AR22 act 4 (ADV13-6): a row read from the ledger bound its refs by
	// ls-remote now (Afresh), so the read above compares now with now and
	// proves nothing: ls-remote is skipped, never proven from the ledger.
	// The line's origin_refs is a claim that can only red: what each remote
	// holds now, at the URLs bound now, differing from it is origin-moved.
	if st.FromLedger {
		f.skipped["ls-remote"] = true
		if len(st.OriginClaim) > 0 {
			claimed, _, err := remotesSame(fx.Git, root, st.OriginClaim, g.RemoteURLs)
			if err != nil {
				return stop("ls-remote", err)
			}
			g.OriginSame = g.OriginSame && claimed
		}
	}
	// ADV10-1, -2: the base is bound by its sha at stage, read by full
	// refname, and the touch set is read from the sha, never a name.
	// AR20 act 1 (ADV11-1, -2): main is bound for every row, whatever its
	// base; main anywhere but MainSha advanced by this process's own
	// merges, or a base not main anywhere but BaseSha, is main-moved. The
	// ledger is never read for it: the child can write the ledger.
	if g.Main, err = fx.Git.RevParseCommit(st.Path, night.BaseRef("main")); err != nil {
		return stop("base", err)
	}
	// baseNow is the base's ref as read here: the fresh clone fetches it
	// beside the branch, bound to it, for the harness at base (AR22 act 0).
	base, baseNow := st.BaseSha, g.Main
	if base == "" {
		f.skipped["base"] = true // a hand gate with no stage line: bound now
		if base, err = fx.Git.RevParseCommit(st.Path, night.BaseRef(st.Base)); err != nil {
			return stop("base", err)
		}
		baseNow = base
	} else {
		want := expectedMain(root, st)
		g.MainMoved = want == "" || g.Main != want
		if st.Base != "main" {
			now, err := fx.Git.RevParseCommit(st.Path, night.BaseRef(st.Base))
			if err != nil {
				return stop("base", err)
			}
			g.MainMoved = g.MainMoved || now != st.BaseSha || (st.BaseClaim != "" && now != st.BaseClaim)
			baseNow = now
		}
	}
	harness := queue.HarnessOf(v.Harness)
	g.Harness = harness
	g.Outside = []string{}
	if v.Orchestrator == nil { // an orchestrator's touch reads its own commits, below
		names, err := w.DiffNames(st.Path, base, head)
		if err != nil {
			return stop("touch", err)
		}
		for _, n := range names {
			if SettingsPath(n) || n == v.QueuePath || !inside(n, p.MayTouch, harness) && !inside(n, RulesPaths, harness) {
				g.Outside = append(g.Outside, n)
			}
		}
	}
	// AR29 act 3 (ADV17-2): with a packets source, the row's committed
	// packet is the source's bytes, else red (packet-differs); a row with
	// no file there is skipped, one that declared its copy-in skipped reads
	// nothing; a source that cannot be resolved or read is unrun.
	if v.Source != "" {
		if v.CopyInSkipped {
			f.skipped["packet-source"] = true
		} else if src, err := PacketSource(v.Source, st.Branch); err != nil {
			return stop("packet source", err)
		} else if src == "" {
			f.skipped["packet-source"] = true
		} else if g.PacketDiffers, err = packetDiffers(fx.Git, st.Path, head, st.Branch, src); err != nil {
			return stop("packet source", err)
		}
	}
	// ADV10-7: the lines judge the commit, not the child's tree: they run
	// in a fresh detached tree at g.Head, so an index flag or an exclude
	// in the child's tree changes nothing they read. The tree is read
	// only for clean, above. AR22 act 0 (ADV13-1): it is a new repository
	// holding the branch and its base, fetched from the root by full
	// refname at the shas read above, so no replace ref, config, hook or
	// info/ of the root's or the child's judges it.
	at := st.Path
	denied := effects.DenyingPush(fx.Shell, g.Remotes) // the branch's code runs: no push (ADV11-6)
	// AR25 act 1 (ADV15-3): the gate's acceptance lines share one fresh
	// scratch home (GOCACHE, GOMODCACHE, HOME, the XDG dirs, TMPDIR),
	// removed when the gate ends; AR27A act 2 (ADV16-7): the verify line
	// runs under one of its own, so no acceptance line's write reaches it.
	sh, unscratch, err := effects.Scratched(denied)
	if err != nil {
		return stop("scratch home", err)
	}
	defer unscratch()
	vsh, unvscratch, err := effects.Scratched(denied)
	if err != nil {
		return stop("scratch home", err)
	}
	defer unvscratch()
	if !nested {
		refs := []effects.Ref{{Name: "refs/heads/" + st.Branch, Sha: head}}
		if b := "refs/heads/" + st.Base; st.Base != "" && b != refs[0].Name {
			refs = append(refs, effects.Ref{Name: b, Sha: baseNow})
		}
		dir, done, err := Fresh(fx, root, refs...)
		if err != nil {
			g.RefMoved = errors.Is(err, effects.ErrRefMoved)
			return stop("fresh", err)
		}
		defer done()
		// ADV10-10: main's harness at the base's sha, over the branch's tree.
		if err := fx.Git.Overlay(dir, base, harness...); err != nil {
			return stop("harness", err)
		}
		at = dir
	}
	if o := v.Orchestrator; o != nil {
		if g.NoCharter, err = noCharter(w, st.Path, base, head, o.Row.ID); err != nil {
			return stop("charter", err)
		}
		qp := v.QueuePath
		if qp == "" {
			qp = effects.DefaultQueue
		}
		f.queueWhy = queueProblem(at, qp, o)
		g.QueueInvalid = f.queueWhy != ""
		// ar32b act 1: the merges are read, and merge-tree writes its
		// trees, in the gate's fresh repository, never the root's.
		proven, unproven, err := provenMerges(w, root, at, base, head)
		if err != nil {
			return stop("merges", err)
		}
		g.Unproven = unproven
		names, err := ownNames(w, at, base, head, proven)
		if err != nil {
			return stop("touch", err)
		}
		for _, n := range names {
			if SettingsPath(n) || !orchestratorInside(n, o.Row, harness) {
				g.Outside = append(g.Outside, n)
			}
		}
	}
	for i, l := range p.Acceptance {
		var b *relay.Check
		if i < len(baseline) {
			b = &baseline[i]
		}
		if l.Kind == packet.AsMain {
			b = anchor(fx, st.Path, b, l.Shell)
		}
		g.Lines = append(g.Lines, rerun(sh, at, l, b, nested, fmt.Sprintf("%s-line%d", st.Branch, i+1)))
	}
	g.Verify = verify(fx, vsh, st.Path, at, v, nested, st.Branch+"-verify")
	// ADV11-6: a line that pushed by URL moved a remote; read them again.
	if same, _, err := remotesSame(fx.Git, root, g.RemoteRefs, g.RemoteURLs); err != nil {
		return stop("ls-remote", err)
	} else if !same {
		g.OriginSame = false
	}
	g.Skipped = []string{}
	for _, name := range []string{"exit", "origin", "ls-remote", "base", "packet-source"} {
		if f.skipped[name] {
			g.Skipped = append(g.Skipped, name)
		}
	}
	g.Gathered = true
	return g, f
}

// remotesOf is the remote names OriginRefs' keys hold, sorted.
func remotesOf(refs map[string]string) []string {
	var out []string
	for k := range refs {
		r, _, _ := strings.Cut(k, " ")
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out
}

// remotesSame reads what each remote in refs (stage's OriginRefs) holds
// now at its first URL in urls and compares; skipped is refs empty or a
// remote stage could not read, or one with no URL; an error is a remote
// the gate cannot read (unrun). The read runs outside any repository, with
// a timeout (effects.LsRemoteTimeout).
func remotesSame(git effects.Git, dir string, refs map[string]string, urls map[string][]string) (same, skipped bool, err error) {
	same, skipped = true, len(refs) == 0
	for _, r := range remotesOf(refs) {
		if refs[r] == night.OriginUnknown || len(urls[r]) == 0 {
			skipped = true
			continue
		}
		got, err := git.LsRemote(dir, urls[r][0])
		if err != nil {
			return false, skipped, err
		}
		now := map[string]string{}
		for k, v := range got {
			now[r+" "+k] = v
		}
		if !sameRefs(now, only(refs, r)) {
			same = false
		}
	}
	return same, skipped, nil
}

// only is refs' entries for remote r.
func only(refs map[string]string, r string) map[string]string {
	out := map[string]string{}
	for k, v := range refs {
		if name, _, _ := strings.Cut(k, " "); name == r {
			out[k] = v
		}
	}
	return out
}

// merges is main's moves by this process's own merges, per root: from
// the main a merge read to the sha it committed. It is held in memory and
// never read back from the ledger (ADV11-1).
var merges = struct {
	sync.Mutex
	m map[string]map[string]string
}{m: map[string]map[string]string{}}

// rootKey is root as merges keys it: its symlinks resolved when it has any.
func rootKey(root string) string {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		return r
	}
	return filepath.Clean(root)
}

// ownMerge records that this process's merge in root moved main from to.
func ownMerge(root, from, to string) {
	merges.Lock()
	defer merges.Unlock()
	k := rootKey(root)
	if merges.m[k] == nil {
		merges.m[k] = map[string]string{}
	}
	merges.m[k][from] = to
}

// expectedMain is the sha main should be at now for st: its MainSha (for a
// stage line before AR20, BaseSha when its base is main), advanced along
// this process's own merges in root when this process staged it; a row
// read back from the ledger takes none (the gate bound its MainSha now).
// "" is a main never bound: main-moved.
func expectedMain(root string, st *night.Staged) string {
	want := st.MainSha
	if want == "" && st.Base == "main" {
		want = st.BaseSha
	}
	if want == "" || st.FromLedger {
		return want
	}
	merges.Lock()
	defer merges.Unlock()
	chain := merges.m[rootKey(root)]
	for range len(chain) { // merges are serialised: a chain, never a cycle
		next, ok := chain[want]
		if !ok {
			break
		}
		want = next
	}
	return want
}

func sameRefs(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// orchestratorInside reports whether p lies in an orchestrator row's
// bounds (§5): every unguarded path, and the guarded ones its may_touch
// names as inside does; the brief is guarded unless may_touch names it.
func orchestratorInside(p string, r queue.Row, harness []string) bool {
	var named []packet.Glob
	for _, m := range r.MayTouch {
		named = append(named, glob(m))
	}
	if brief := path.Clean(r.Prompt); r.Prompt != "" && p == brief {
		for _, m := range r.MayTouch {
			if path.Clean(m) == brief {
				return true
			}
		}
		return false
	}
	return guardOf(p, harness) == "" || inside(p, named, harness)
}

// provenMerges reads the merges on base..head's first-parent line in dir
// (orchestrator r1 §5, ar32b act 1). A merge is proven when it has two
// parents, the second a head a green build-row gate of this process noted
// in root (night.GreenHead), and its tree is `git merge-tree --write-tree`
// of the two, exiting 0. unproven is any other merge: a helper's branch,
// a red row's head, main, an octopus, an edit inside the merge or a
// conflict resolved by hand.
func provenMerges(git effects.Git, root, dir, base, head string) (proven map[string]bool, unproven bool, err error) {
	out, err := git.Hermetic(dir, "log", "--first-parent", "--merges", "--no-show-signature", "--format=%H %T %P", base+".."+head)
	if err != nil {
		return nil, false, err
	}
	proven = map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		if len(f) != 4 || night.GreenHead(root, f[3]) == "" {
			unproven = true
			continue
		}
		auto, err := git.Hermetic(dir, "merge-tree", "--write-tree", f[2], f[3])
		if tree, _, _ := strings.Cut(auto, "\n"); err != nil || strings.TrimSpace(tree) != f[1] {
			unproven = true // a conflict exits 1: resolved by hand, never automatic
			continue
		}
		proven[f[0]] = true
	}
	return proven, unproven, nil
}

// ownNames is the paths the sitting's own commits change (§5, ar32b act
// 1): each commit on base..head's first-parent line read against its first
// parent, the proven merges exempt, sorted and each once.
func ownNames(git effects.Git, dir, base, head string, proven map[string]bool) ([]string, error) {
	out, err := git.Hermetic(dir, "rev-list", "--first-parent", base+".."+head)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, c := range strings.Fields(out) {
		if proven[c] {
			continue
		}
		ns, err := git.DiffNames(dir, c+"^1", c)
		if err != nil {
			return nil, err
		}
		for _, n := range ns {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	}
	slices.Sort(names)
	return names, nil
}

// noCharter reports an orchestrator sitting with no charter of row (§5):
// the newest at head is absent, was already at base, is 4 KB or more, or
// has no Next act.
func noCharter(git effects.Git, dir, base, head, row string) (bool, error) {
	c, err := night.NewestCharter(git, dir, head, row)
	if err != nil || c == "" {
		return c == "", err
	}
	at, err := git.Hermetic(dir, "ls-tree", "--full-tree", "--name-only", base, "--", c)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(at) != "" {
		return true, nil
	}
	size, err := git.Hermetic(dir, "cat-file", "-s", head+":"+c)
	if err != nil {
		return false, err
	}
	if n, err := strconv.Atoi(strings.TrimSpace(size)); err != nil || n >= CharterMax {
		return true, err
	}
	md, err := git.Hermetic(dir, "cat-file", "blob", head+":"+c)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(night.NextActOf(md)) == "", nil
}

// queueProblem is why the queue at qp in the tree at dir fails an
// orchestrator sitting (§5), "" when it holds: it loads and validates;
// Began's rows lead it, each unchanged; its defaults, words and standing
// row are Night's; and each row it appends is a build row whose model,
// merge, device and harness are the defaults' and whose budget is within
// theirs.
func queueProblem(dir, qp string, o *Orchestrated) string {
	q, err := queue.Load(dir, qp)
	if err != nil {
		return err.Error()
	}
	if ps := q.Validate(); len(ps) > 0 {
		return fmt.Sprintf("%s: %s", ps[0].Row, ps[0].Msg)
	}
	d := o.Night.Defaults
	switch r, _ := q.Orchestrator(); {
	case !queue.SameRow(q.Defaults, d):
		return "the defaults are not the night's"
	case q.Words != o.Night.Words:
		return "the words are not the night's"
	case !queue.SameRow(r, o.Row):
		return "the orchestrator row is not the night's"
	case len(q.Rows) < len(o.Began.Rows):
		return "a row was dropped"
	}
	for i, r := range o.Began.Rows {
		if !queue.SameRow(q.Rows[i], r) {
			return fmt.Sprintf("row %s was changed, dropped, moved or unparked", r.ID)
		}
	}
	for _, r := range q.Rows[len(o.Began.Rows):] {
		switch {
		case r.IsOrchestrator():
			return fmt.Sprintf("row %s: an added orchestrator row", r.ID)
		case r.Model != d.Model || r.Merge != d.Merge || r.Device != d.Device || !slices.Equal(r.Harness, d.Harness):
			return fmt.Sprintf("row %s: its model, merge, device or harness is not the defaults'", r.ID)
		case r.Turns > d.Turns || r.Context > d.Context || r.USD > d.USD || r.Wall > d.Wall:
			return fmt.Sprintf("row %s: its budget is above the defaults'", r.ID)
		}
	}
	return ""
}

func inside(p string, globs []packet.Glob, harness []string) bool {
	guard := guardOf(p, harness)
	for _, g := range globs {
		if guard != "" && (strings.TrimSuffix(g.Raw, "/") == strings.TrimSuffix(guard, "/") || g.Raw == p) || guard == "" && g.Match(p) {
			return true
		}
	}
	return false
}

// guardOf is the guarded entry p falls under, or "": a pinnedPaths entry,
// or a harness path (the path itself or anything under it).
func guardOf(p string, harness []string) string {
	for _, e := range pinnedPaths {
		if p == e || strings.HasSuffix(e, "/") && strings.HasPrefix(p, e) {
			return e
		}
	}
	for _, h := range harness {
		if p == h || strings.HasPrefix(p, h+"/") {
			return h
		}
	}
	return ""
}

// harnessIn reports whether names touch the harness.
func harnessIn(names, harness []string) bool {
	for _, n := range names {
		for _, h := range harness {
			if n == h || strings.HasPrefix(n, h+"/") {
				return true
			}
		}
	}
	return false
}

// anchor is b, a baseline of line on main, placed at main's head now
// (night.Anchor, R6 §4) in the root dir's repo has; a root that cannot be
// read places nothing: unrun, stale-baseline.
func anchor(fx effects.Effects, dir string, b *relay.Check, line string) *relay.Check {
	if b == nil || b.Status == relay.CheckUnrun {
		return b
	}
	root, err := fx.Git.Root(dir)
	if err != nil {
		return &relay.Check{Name: b.Name, Expect: b.Expect, MainExit: -1, MainLast: night.StaleReason, Status: relay.CheckUnrun}
	}
	return night.Anchor(fx, root, b, line)
}

// rerun runs one acceptance line in dir with §6's timeout and judges it:
// its expectation holds, green; an `as main` line equal to its baseline
// (exit and last line), green when it exits 0 and same-as-main when it
// fails; anything else red. A line that cannot start, or an `as main` line
// with no measured baseline (none, or one not at main's head: stale), is
// unrun; so is every line when nested (§15), never run. Its log (AR31 act
// 2, label names it) is kept when it is not green, else removed.
func rerun(sh effects.Shell, dir string, l packet.Line, b *relay.Check, nested bool, label string) relay.Check {
	c := relay.Check{Name: l.Shell, Expect: l.Expect, MainExit: -1}
	if b != nil {
		c.MainExit, c.MainLast = b.MainExit, b.MainLast
	}
	if nested {
		c.BranchExit, c.BranchLast, c.Status = -1, effects.NestedReason, relay.CheckUnrun
		return c
	}
	exit, last, log, err := effects.RunLogged(sh, dir, l.Shell, effects.Timeout, label)
	c.BranchExit, c.BranchLast = exit, last
	switch {
	case err != nil:
		c.BranchExit, c.BranchLast, c.Status = -1, effects.LastLine(err.Error()), relay.CheckUnrun
	case l.Kind == packet.AsMain && (b == nil || b.Status == relay.CheckUnrun):
		c.Status = relay.CheckUnrun
	case l.Kind == packet.AsMain && exit == b.MainExit && last == b.MainLast:
		c.Status = relay.CheckSameAsMain
		if exit == 0 {
			c.Status = relay.CheckGreen
		}
	case l.Kind == packet.AsMain:
		c.Status = relay.CheckRed
	case night.Holds(l, exit, last):
		c.Status = relay.CheckGreen
	default:
		c.Status = relay.CheckRed
	}
	c.BranchLog = effects.KeepLog(log, c.Status != relay.CheckGreen)
	return c
}

// Fresh is a new repository made under the state root's gate/ (the
// system's temp directory when Effects has no Getenv), holding refs
// fetched from dir's repository by full refname, each at its bound sha
// (else an error wrapping effects.ErrRefMoved), the first checked out
// detached; and done, which removes it (ADV10-7). The gate's lines run
// there, never in the child's tree. It is no worktree of dir and shares
// none of its config, refs, alternates, hooks, info/ or shallow; git runs
// under the hermetic runner, so no hook fires and no fsmonitor starts
// (ADV11-3; AR22 act 0, ADV13-1, -14).
func Fresh(fx effects.Effects, dir string, refs ...effects.Ref) (string, func(), error) {
	base := os.TempDir()
	if fx.Getenv != nil {
		base = filepath.Join(effects.StateDir(fx.Getenv), "gate")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", nil, err
	}
	tmp, err := os.MkdirTemp(base, "fresh-")
	if err != nil {
		return "", nil, err
	}
	if err := fx.Git.Clone(dir, tmp, refs...); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	return tmp, func() { os.RemoveAll(tmp) }, nil
}

// verify runs the verify line with sh (push denied) in at (the fresh tree) after the acceptance lines,
// under v's limit (AR59 act 2): green on
// exit 0; red with Cut the limit in seconds when the line was cut there
// (effects.Cut, AR59 act 3); same-as-main when its exit and last line equal the baseline's
// (a measured one, at main's head now or the memo's there, R6 §4); unrun:
// stale-baseline when neither answers; red otherwise; unrun when it cannot start; unrun:
// nested under the guard (§15), never run. The line is resolved once on
// main (the root dir's repo has), never in the row's tree (ADV6-10): a
// root with no `check:` is skipped, a probe that could not start unrun.
// The line's log (AR31 act 2, label names it) is kept when it is not
// green, else removed; the probe (through the memo) keeps none.
func verify(fx effects.Effects, sh effects.Shell, dir, at string, v Verify, nested bool, label string) *relay.Check {
	if nested && v.Name != "" {
		return &relay.Check{Name: v.Name, Expect: "exit 0", MainExit: -1, BranchExit: -1,
			BranchLast: effects.NestedReason, Status: relay.CheckUnrun}
	}
	line, root, err := "", "", error(nil)
	if v.Name != "" {
		if root, err = fx.Git.Root(dir); err == nil {
			line, err = night.ResolveVerify(night.MemoShell(fx, root), root, v.Name)
		}
	}
	if err != nil {
		return &relay.Check{Name: v.Name, Expect: "exit 0", MainExit: -1, BranchExit: -1,
			BranchLast: effects.LastLine("the probe on main: " + err.Error()), Status: relay.CheckUnrun}
	}
	if line == "" {
		return &relay.Check{Name: v.Name, MainExit: -1, BranchExit: -1, Status: relay.CheckSkipped}
	}
	c := &relay.Check{Name: line, Expect: "exit 0", MainExit: -1}
	b := v.Baseline
	if b != nil && b.Status != relay.CheckUnrun {
		b = night.Anchor(fx, root, b, line, v.limit())
	}
	if b != nil {
		c.MainExit, c.MainLast = b.MainExit, b.MainLast
	}
	limit := v.limit()
	start := time.Now() // real time: the deadline is (AR59 act 3)
	exit, last, log, err := effects.RunLogged(sh, at, line, limit, label)
	took := time.Since(start)
	c.BranchExit, c.BranchLast = exit, last
	switch {
	case err != nil:
		c.BranchExit, c.BranchLast, c.Status = -1, effects.LastLine(err.Error()), relay.CheckUnrun
	case exit == 0:
		c.Status = relay.CheckGreen
	case effects.Cut(exit, took, limit): // cut at its limit: red, whatever main did
		c.Status, c.Cut = relay.CheckRed, int(limit/time.Second)
	case night.Stale(b):
		c.Status = relay.CheckUnrun
	case b != nil && b.Status != relay.CheckUnrun && exit == b.MainExit && last == b.MainLast:
		c.Status = relay.CheckSameAsMain
	default:
		c.Status = relay.CheckRed
	}
	c.BranchLog = effects.KeepLog(log, c.Status != relay.CheckGreen)
	return c
}

// mergedVerify runs line with sh (push denied) under the root's lock on main merged with head,
// in a fresh tree at the root's HEAD with main's harness, paths, over
// it (the merged copies of paths are never the ones run, and a copy main
// lacks is deleted); harness is whether
// the branch changed them, and tree the merged tree (before the harness
// is laid over it): the one the root's merge commit must hold (ADV11-5).
// The fresh tree is a clone holding main and branch, each fetched by full
// refname at the sha bound (main the root's HEAD, branch head; AR22 act
// 0), and is removed before it returns. log is the line's (AR31 act 2,
// labelled <branch>-merge), for the caller to keep or remove; "" when the
// line never ran. The line runs under limit (the gate's, AR59 act 2), and
// cut is a line cut there (effects.Cut).
func mergedVerify(fx effects.Effects, sh effects.Shell, root, branch, head, line string, paths []string, name, email string, limit time.Duration) (exit int, last, log string, harness, cut bool, tree string, err error) {
	main, err := fx.Git.Head(root)
	if err != nil {
		return -1, "", "", false, false, "", err
	}
	names, err := fx.Git.DiffNames(root, main, head)
	if err != nil {
		return -1, "", "", false, false, "", err
	}
	refs := []effects.Ref{{Name: "refs/heads/main", Sha: main}}
	if b := "refs/heads/" + branch; b != refs[0].Name {
		refs = append(refs, effects.Ref{Name: b, Sha: head})
	}
	dir, done, err := Fresh(fx, root, refs...)
	if err != nil {
		return -1, "", "", false, false, "", err
	}
	defer done()
	if err := fx.Git.MergeNoCommit(dir, head, name, email); err != nil {
		return -1, "", "", false, false, "", err
	}
	if tree, err = fx.Git.Hermetic(dir, "write-tree"); err != nil {
		return -1, "", "", false, false, "", err
	}
	if err := fx.Git.Overlay(dir, main, paths...); err != nil {
		return -1, "", "", false, false, "", err
	}
	start := time.Now() // real time: the deadline is (AR59 act 3)
	exit, last, log, err = effects.RunLogged(sh, dir, line, limit, branch+"-merge")
	cut = err == nil && effects.Cut(exit, time.Since(start), limit)
	return exit, last, log, harnessIn(names, paths), cut, tree, err
}

// Run is Gather (with v, the verify line) then relay.Decide, with one `gate` ledger line per check
// (its status only; never the shell text; a kept log's path, AR31 act 2),
// a `touch` line naming the paths
// outside may-touch, and the `gate` result line §3's satisfaction reads.
// The error is why the gate is unrun: the fact the gather could not read,
// or the ledger line it could not write.
//
// The invariants (§15) are read after Decide over a gathered gate: HEAD and
// git status read afresh against its commit and clean, the dependencies
// (Hand's; none from the night's gate, whose sitting line reads them), the
// live children, the verify line and the checks against the result. One
// broken: the result is failed with reason invariant, and the result line
// carries `stop: invariant` and `invariant: <name>` (the first broken).
// A verify line cut at its limit (AR59 act 3): its `gate` line carries
// `cut` (the limit in seconds), and the result line reads reason
// `verify-cut`, detail `verify cut at <n> s`.
func Run(fx effects.Effects, st *night.Staged, p *packet.Packet, baseline []relay.Check, exit *ExitFacts, v Verify) (relay.Result, relay.StopReason, relay.Gate, error) {
	return run(fx, st, p, baseline, exit, v, nil)
}

// Mark is one check a gathered gate marks: the `check` and the `status`
// its `gate` event carries, and N an acceptance line's number, from 1.
type Mark struct {
	Check, Status string
	N             int
}

// Marks is the checks of a gathered gate in the order the gate marks them,
// each with the status its `gate` event carries (AR45): branch (only when
// the tree is off it), exit, commit, clean, origin, touch, the
// orchestrator's charter, queue and merges, packet-source, each acceptance
// line and verify. A check the gather skipped (g.Skipped) reads skipped.
// The ledger carries the orchestrator's three only for an orchestrator
// sitting and packet-source only with a packets source, and none of the
// four is red without one. None for a gate that did not gather.
func Marks(g relay.Gate) []Mark {
	if !g.Gathered {
		return nil
	}
	var out []Mark
	mark := func(name string, ok bool) {
		s := relay.CheckGreen
		switch {
		case slices.Contains(g.Skipped, name):
			s = relay.CheckSkipped
		case !ok:
			s = relay.CheckRed
		}
		out = append(out, Mark{Check: name, Status: s})
	}
	if g.OffBranch {
		out = append(out, Mark{Check: "branch", Status: relay.CheckRed})
	}
	mark("exit", g.ResultSeen && g.ExitOK)
	mark("commit", g.Commit)
	mark("clean", g.Clean)
	mark("origin", g.OriginSame)
	mark("touch", len(g.Outside) == 0)
	mark("charter", !g.NoCharter)
	mark("queue", !g.QueueInvalid)
	mark("merges", !g.Unproven)
	mark("packet-source", !g.PacketDiffers)
	for i, c := range g.Lines {
		out = append(out, Mark{Check: "acceptance", Status: c.Status, N: i + 1})
	}
	if c := g.Verify; c != nil {
		out = append(out, Mark{Check: "verify", Status: c.Status})
	}
	return out
}

// Red is the names of what relay.Decide does not pass in a gathered g
// (AR45). First every check of Marks whose status is none of green,
// same-as-main and skipped (a red one, and an unrun line), in the order the
// gate marks them, each the `check` its own `gate` event carries (two such
// acceptance lines are `acceptance` twice); then the two with no `gate`
// event behind them: `main-moved` when the base moved, `orphans` when the
// sweep killed any. Empty, never nil, when there is none or the gate did
// not gather. The `sitting` line's `gate.red` is this, through
// night.RedChecks.
func Red(g relay.Gate) []string {
	red := []string{}
	for _, m := range Marks(g) {
		if !passes(m.Status) {
			red = append(red, m.Check)
		}
	}
	if g.Gathered && g.MainMoved {
		red = append(red, string(relay.MainMoved))
	}
	if g.Gathered && g.Orphans > 0 {
		red = append(red, string(relay.Orphans))
	}
	return red
}

// Cause is the first fact behind each check Red names in g (AR62 act 1):
// each check once, in Red's order, joined by "; ", a path or a last line
// clipped to 60 runes, after its root is written `.` and HOME `~`, and the
// whole to 200. An acceptance line's first
// failing one speaks for it, `(+<n> more)` counting the others; its shell
// is never in it. "" when Red names none. The gate's end line carries it
// as `cause` and the `sitting` line's gate object through night.RedCause;
// the reason stays the stop reason.
func Cause(g relay.Gate) string {
	var parts []string
	named := map[string]bool{}
	for _, check := range Red(g) {
		if !named[check] {
			named[check] = true
			parts = append(parts, causeOf(g, check))
		}
	}
	return clip(strings.Join(parts, "; "), 200)
}

// causeOf is the first fact of one check Red named in g.
func causeOf(g relay.Gate, check string) string {
	switch check {
	case "branch":
		return "branch: off the row's branch"
	case "exit":
		if !g.ResultSeen {
			return "exit: no result event"
		}
		return "exit: is_error"
	case "commit":
		return "commit: none"
	case "clean":
		if g.Dirty == 1 {
			return "clean: 1 path"
		}
		return fmt.Sprintf("clean: %d paths", g.Dirty)
	case "origin":
		return "origin: a remote moved"
	case "touch":
		return "touch: " + clip(shown(g, g.Outside[0]), 60) + " outside may touch" + more(len(g.Outside)-1)
	case "charter":
		return "charter: none"
	case "queue":
		return "queue: invalid at the head"
	case "merges":
		return "merges: unproven"
	case "packet-source":
		return "packet-source: differs at the head"
	case "acceptance":
		first, n := -1, 0
		for i, c := range g.Lines {
			if !passes(c.Status) {
				if first < 0 {
					first = i
				}
				n++
			}
		}
		c := g.Lines[first]
		if c.Status == relay.CheckUnrun {
			return fmt.Sprintf("acceptance %d: unrun", first+1) + more(n-1)
		}
		return fmt.Sprintf("acceptance %d: last %q, want %s", first+1, clip(shown(g, c.BranchLast), 60), c.Expect) + more(n-1)
	case "verify":
		switch c := g.Verify; {
		case c.Cut > 0:
			return fmt.Sprintf("verify: exit 124, cut at %d s", c.Cut)
		case c.Status == relay.CheckUnrun:
			return "verify: unrun"
		default:
			return fmt.Sprintf("verify: exit %d, last %q", c.BranchExit, clip(shown(g, c.BranchLast), 60))
		}
	case string(relay.MainMoved):
		main := g.Main
		if len(main) > 12 {
			main = main[:12]
		}
		return "main-moved: main at " + main
	case string(relay.Orphans):
		return fmt.Sprintf("orphans: %d swept", g.Orphans)
	}
	return check
}

// shown is s with the row's root (the tree's, above .claude/worktrees)
// written `.` and then HOME `~`, as report's Markdown writes them, before
// any clip, so no clip leaves a fragment of either (AR62 fix round, F1;
// relay-r1 §14 D18).
func shown(g relay.Gate, s string) string {
	wts := filepath.Dir(filepath.Clean(g.Worktree))
	if root := filepath.Dir(filepath.Dir(wts)); g.Worktree != "" && filepath.Base(wts) == "worktrees" &&
		filepath.Base(filepath.Dir(wts)) == ".claude" && root != "/" {
		s = strings.ReplaceAll(s, root, ".")
	}
	if home := filepath.Clean(os.Getenv("HOME")); home != "." && home != "/" {
		s = strings.ReplaceAll(s, home, "~")
	}
	return s
}

// more is ` (+<n> more)`, or "" when n is none.
func more(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(" (+%d more)", n)
}

// clip is s cut to n runes, `...` marking the cut: a copy of night's
// unexported clip (AR62 act 1).
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

// passes is relay.Decide's pass statuses (its own list is unexported).
func passes(status string) bool {
	return status == relay.CheckGreen || status == relay.CheckSameAsMain || status == relay.CheckSkipped
}

// The night cannot import the gate (the gate imports the night), so the
// gate hands it Red and Cause (AR62 act 2) wherever the gate is linked.
func init() { night.RedChecks, night.RedCause = Red, Cause }

// invariants reads §15's invariants for a gathered gate g judged res.
func invariants(fx effects.Effects, st *night.Staged, g relay.Gate, res relay.Result, deps map[string]bool) []string {
	f := relay.Facts{HeadBefore: st.HeadBefore, Gathered: g.Gathered, Commit: g.Commit, Clean: g.Clean,
		Deps: deps, Live: night.LiveChildren(), Pass: res == relay.GreenResult}
	if st.HeadBefore != "" {
		f.HeadAfter, _ = fx.Git.Head(st.Path)
	}
	if status, err := fx.Git.Status(st.Path); err == nil {
		clean := len(status) == 0
		f.Fresh = &clean
	}
	if g.Verify != nil {
		f.Verify = g.Verify.Status
	}
	for _, c := range g.Lines {
		f.Checks = append(f.Checks, c.Status)
	}
	return relay.Invariants(f)
}

func run(fx effects.Effects, st *night.Staged, p *packet.Packet, baseline []relay.Check, exit *ExitFacts, v Verify, deps map[string]bool) (relay.Result, relay.StopReason, relay.Gate, error) {
	g, f := gather(fx, st, p, baseline, exit, v)
	res, why := relay.Decide(g)
	var broken []string
	if g.Gathered {
		if broken = invariants(fx, st, g, res, deps); len(broken) > 0 {
			res, why = relay.FailedResult, relay.Invariant
		}
	}
	row := st.Branch
	var lost error
	put := func(event string, m map[string]any) {
		m["row"] = row
		if err := fx.Ledger.Append(event, m); err != nil && lost == nil {
			lost = err
		}
	}
	if g.Gathered {
		// Every check's `gate` line is one of Marks' (AR45 act 2), so the
		// names the `sitting` line reads through Red are these lines' own.
		// A kept log is named by its path, never its bytes (AR31 act 2).
		line := func(m map[string]any, c relay.Check) {
			m["exit"], m["last"], m["main_exit"], m["main_last"] = c.BranchExit, c.BranchLast, c.MainExit, c.MainLast
			if c.BranchLog != "" {
				m["log"] = c.BranchLog
			}
			if c.Cut > 0 { // cut at its limit (AR59 act 3)
				m["cut"] = c.Cut
			}
		}
		paths := len(g.Outside) > 0 // the `touch` line goes ahead of the first line's check
		for _, k := range Marks(g) {
			m := map[string]any{"check": k.Check, "status": k.Status}
			switch k.Check {
			case "branch":
				m["head"] = g.Head
			case "charter", "queue", "merges":
				if v.Orchestrator == nil {
					continue
				}
				if k.Check == "queue" && f.queueWhy != "" {
					m["detail"] = f.queueWhy
				}
			case "packet-source":
				if v.Source == "" {
					continue
				}
			case "acceptance", "verify":
				if paths {
					put("touch", map[string]any{"paths": g.Outside})
					paths = false
				}
				if k.Check == "acceptance" {
					m["n"] = k.N
					line(m, g.Lines[k.N-1])
				} else if k.Status != relay.CheckSkipped {
					line(m, *g.Verify)
				}
			}
			put("gate", m)
		}
		if paths {
			put("touch", map[string]any{"paths": g.Outside})
		}
	}
	if lost != nil {
		res, why = relay.Unrun, ""
	}
	end := map[string]any{"result": string(res), "reason": string(why)}
	if c := Cause(g); c != "" { // AR62 act 1: the first fact of each red check
		end["cause"] = c
	}
	if g.Head != "" {
		end["head"] = g.Head
	}
	if len(broken) > 0 {
		end["stop"], end["invariant"] = string(relay.Invariant), broken[0]
	}
	if f.err != nil {
		end["detail"] = f.err.Error()
	} else if why == relay.VerifyCut && g.Verify != nil { // AR59 act 3: the cut says so
		end["detail"] = fmt.Sprintf("verify cut at %d s", g.Verify.Cut)
	}
	put("gate", end)
	if res == relay.GreenResult && lost == nil {
		// AR21 act 5: this process's green verdict is memory, beside the
		// ledger line a child could have written (night.DepSatisfied).
		// ar32b act 1: a build row's gate notes the head it judged, which
		// an orchestrator's merge may take in; an orchestrator's notes none.
		head := g.Head
		if v.Orchestrator != nil {
			head = ""
		}
		if root, err := fx.Git.Root(st.Path); err == nil {
			night.NoteGreen(root, row, head)
		}
	}
	if lost != nil {
		// A gate the ledger does not hold is unrun: §3 reads its result line.
		return relay.Unrun, "", g, fmt.Errorf("the ledger: %w", lost)
	}
	return res, why, g, f.err
}

// Hand is `relay gate <row>` by hand (§9, the packet's rulings): the stage
// facts come from the row's last `stage` ledger line when there is one; the
// acceptance rerun happens in the row's worktree when it exists, else in a
// temporary detached worktree `.claude/worktrees/gate-<id>` at the branch's
// head, removed before Hand returns, even on error. The baseline on main is
// measured only when a line is `as main`; the verify line (the row's
// `verify`) runs with no baseline: green only.
// q, when given, is the queue r is a row of: a dependency's cut is read
// against its own row's base (night.DepSatisfied). v gives the packets
// source (v.Source; "" compares nothing; AR29 act 3) and the verify limit
// (v.Limit; 0 is effects.Timeout; AR59 act 2), so the verb passes the
// config's as the night's gate does; the rest of v is the row's.
func Hand(fx effects.Effects, root string, v Verify, r queue.Row, q ...*queue.Queue) (relay.Result, relay.StopReason, relay.Gate, error) {
	var none relay.Gate
	if r.Packet == nil {
		return relay.Unrun, "", none, fmt.Errorf("gate %s: no packet", r.ID)
	}
	w := fx.Git
	if ok, err := fx.Git.BranchExists(root, r.ID); err != nil || !ok {
		return relay.Unrun, "", none, fmt.Errorf("gate %s: no branch %s (%v)", r.ID, r.ID, err)
	}
	// AR22 act 3 (ADV13-5): the base is the queue row's, refs/heads/<base>
	// (else main), and the cut its merge-base with the head: a stage line's
	// base and head_before are claims, never the base or the cut. A line
	// naming another base refuses, unrun ledger-base-differs.
	base := r.Base
	if base == "" {
		base = "main"
	}
	st := &night.Staged{Branch: r.ID, Base: base}
	if lines, err := fx.Ledger.Lines(); err != nil {
		return relay.Unrun, "", none, fmt.Errorf("gate %s: the ledger: %w", r.ID, err)
	} else if staged := night.StagedFromLedger(lines, r.ID); staged != nil {
		if staged.Base != "" && staged.Base != base {
			return relay.Unrun, relay.LedgerBaseDiffers, none, fmt.Errorf("gate %s: %s: the ledger's base %q is not the queue's %q",
				r.ID, relay.LedgerBaseDiffers, staged.Base, base)
		}
		st = staged
		st.Base, st.HeadBefore = base, ""
	}
	wts, err := w.Worktrees(root)
	if err != nil {
		return relay.Unrun, "", none, fmt.Errorf("gate %s: %w", r.ID, err)
	}
	staged := st.Path
	st.Path = ""
	for _, wt := range wts {
		if wt.Branch == r.ID {
			st.Path = wt.Path
		}
	}
	if st.Path != "" {
		if night.Locked(st.Path) {
			return relay.Unrun, "", none, fmt.Errorf("gate %s: a sitting holds %s's lock (§8)", r.ID, st.Path)
		}
		if live, err := night.LiveClaude(st.Path); err != nil || live {
			return relay.Unrun, "", none, fmt.Errorf("gate %s: a live claude in %s (%v, %v)", r.ID, st.Path, live, err)
		}
	}
	if st.Path == "" {
		tmp := filepath.Join(root, ".claude", "worktrees", "gate-"+r.ID)
		for _, wt := range wts {
			if wt.Path == filepath.Clean(tmp) && wt.Branch != "" {
				return relay.Unrun, "", none, fmt.Errorf("gate %s: %s is row %s's worktree, not a leftover", r.ID, tmp, wt.Branch)
			}
		}
		if night.Locked(tmp) {
			return relay.Unrun, "", none, fmt.Errorf("gate %s: %s is locked by a sitting", r.ID, tmp)
		}
		// AR21 act 1: nothing is checked out under a program-valued key.
		if why, err := night.GitStateRefused(root, "", false); why != "" {
			return relay.Unrun, why, none, fmt.Errorf("gate %s: %s: %w", r.ID, why, err)
		}
		if fx.FS.Exists(tmp) {
			w.WorktreeDiscard(root, tmp) // a crashed hand gate's leftover
		}
		if err := w.Exclude(root, ".claude/worktrees/"); err != nil {
			return relay.Unrun, "", none, fmt.Errorf("gate %s: exclude: %w", r.ID, err)
		}
		if err := w.WorktreeAddDetached(root, tmp, r.ID); err != nil {
			return relay.Unrun, "", none, fmt.Errorf("gate %s: a temporary worktree: %w", r.ID, err)
		}
		defer w.WorktreeDiscard(root, tmp)
		st.Path, st.Detached = tmp, true
	}
	if st.Path != staged {
		st.StateDigest = "" // not the staged tree: the git state is bound now
	}
	var baseline []relay.Check
	for _, l := range r.Packet.Acceptance {
		if l.Kind == packet.AsMain {
			baseline = night.BaselineOnMain(fx, root, r.Packet.Acceptance)
			break
		}
	}
	// §3 over the row's edges (its dependencies and a base not main): the
	// invariants read them, so a hand gate over a row whose dependency is
	// not satisfied is failed invariant.
	var queued *queue.Queue
	if len(q) > 0 {
		queued = q[0]
	}
	deps := map[string]bool{}
	for _, d := range append(append([]string{}, r.DependsOn...), r.Base) {
		if d != "" && d != "main" {
			deps[d] = night.DepSatisfied(fx.Git, root, queued, d)
		}
	}
	return run(fx, st, r.Packet, baseline, nil, Verify{Name: r.Verify, Harness: r.Harness, Source: v.Source, Limit: v.Limit}, deps)
}

// MergeIfGreen merges branch into the root per D22: it refuses unless
// relay.Decide(g) is green and the root is clean; then `git merge --no-ff
// --no-commit`, the verify line, and a commit; on any failure after the
// merge started, `git merge --abort`, so the root returns to its head and
// the branch is untouched. Verify is the queue's `verify` (`auto` resolves
// by night.VerifyLine). The verify passes green (exit 0) only (ADV6-5):
// a red main never merges. The anchor is baseline, the plan's measure on
// main, placed at the root's head (night.Anchor: at another head the memo
// answers there, else it is stale); with none, or an unrun one, the memo's
// measure at the root's HEAD (§15). An anchor red there refuses the merge
// before it starts: a `merge` line `branch-only` noting `main red`, the row
// branch-only. A root whose HEAD is not g.Main, the main the gate read, is
// refused with a `merge` line `main-moved` (ADV10-1); the row
// branch-only. baseline is variadic so AR2's five-argument callers stay as
// they are. It merges g.Head,
// the sha the gate measured, never the branch by name: a branch whose tip
// moved after the gate is refused with a `merge` line `moved` (ADV6-3). It
// writes a `merge` ledger line either way, carrying the root (ADV11-17)
// and a red verify line's kept log (AR31 act 2); a merged one moves the main this process's next gates expect (ADV11-1).
// The verify line runs under the gate's limit (g.VerifySeconds; 0:
// effects.Timeout; AR59 act 2); one cut there refuses with `merge <b>:
// verify cut at <n> s, last "<last>"; log: <path>`, a MergeRefused whose
// `merge` line reads `verify-cut`, so the row is red (AR59 act 3 and its
// fix round).
// It merges and commits as the identity read at start (fx.Identity, else
// effects.Identity); with none, nothing runs: a `merge` line `unrun`,
// reason `no-identity` (AR21 act 0, ADV12-1).
func MergeIfGreen(fx effects.Effects, root string, g relay.Gate, branch, verify string, baseline ...*relay.Check) (sha string, err error) {
	if res, _ := relay.Decide(g); res != relay.GreenResult {
		return "", fmt.Errorf("merge %s: the gate is %s, not green", branch, res)
	}
	if effects.Nested(fx.Getenv) {
		return "", fmt.Errorf("merge %s: nested (%s set): the verify line is never measured", branch, effects.MeasureDepthVar)
	}
	w := fx.Git
	var mlog string // the verify line's kept log (AR31 act 2): its path, never its bytes
	record := func(res, detail string) {
		m := map[string]any{"row": branch, "root": root, "result": res, "sha": sha, "detail": detail}
		if mlog != "" {
			m["log"] = mlog
		}
		fx.Ledger.Append("merge", m)
	}
	// AR21 act 0 (ADV12-1): the committer is the identity read at start,
	// never a global config's; none, and the merge is unrun, no-identity.
	identity := fx.Identity
	if identity == nil {
		identity = effects.Identity
	}
	name, email := identity()
	if name == "" || email == "" {
		err := fmt.Errorf("merge %s: %s: no committer identity (user.name, user.email) was read at start", branch, relay.NoIdentity)
		fx.Ledger.Append("merge", map[string]any{"row": branch, "root": root, "result": string(relay.Unrun),
			"reason": string(relay.NoIdentity), "sha": "", "detail": err.Error()})
		return "", err
	}
	// AR22 act 1 (ADV13-2): the pinned PATH's account tools are as bound
	// at start before the merge's verify line; else unrun, tool-moved.
	// Strict (AR25 act 2, ADV15-2): any stat changed since the gate's
	// read is moved, a write restored to equal bytes too.
	if moved := effects.ToolsMovedStrict(); len(moved) > 0 {
		err := fmt.Errorf("merge %s: %s: %s", branch, relay.ToolMoved, strings.Join(moved, ", "))
		fx.Ledger.Append("merge", map[string]any{"row": branch, "root": root, "result": string(relay.Unrun),
			"reason": string(relay.ToolMoved), "sha": "", "detail": err.Error()})
		return "", err
	}
	// AR20 act 0, first: the git state is the one the gate read (hooks,
	// config, info); a change since is git-state-moved, and nothing merges.
	if g.StateDigest != "" {
		if now, err := effects.GitStateDigest(root, g.Worktree); err != nil || now != g.StateDigest {
			err = fmt.Errorf("merge %s: %s: config, hooks or info changed since the gate (%v)", branch, relay.GitStateMoved, err)
			record(string(relay.GitStateMoved), err.Error())
			return "", &relay.MergeRefused{Reason: relay.GitStateMoved, Err: err}
		}
	}
	// AR21 act 1 (ADV12-2): the root's state is the one bound at its first
	// stage, with no program-valued key; checked again after the verify
	// line and after the commit.
	stateHeld := func(when string) error {
		if g.StateDigest != "" {
			if now, err := effects.GitStateDigest(root, g.Worktree); err != nil || now != g.StateDigest {
				err = fmt.Errorf("merge %s: %s %s: config, hooks or info changed since the gate (%v)", branch, relay.GitStateMoved, when, err)
				record(string(relay.GitStateMoved), err.Error())
				return &relay.MergeRefused{Reason: relay.GitStateMoved, Err: err}
			}
		}
		if why, err := night.GitStateRefused(root, g.Worktree, true); why != "" {
			err = fmt.Errorf("merge %s: %s %s: %w", branch, why, when, err)
			record(string(why), err.Error())
			return &relay.MergeRefused{Reason: why, Err: err}
		}
		return nil
	}
	if err := stateHeld("before the merge"); err != nil {
		return "", err
	}
	if on, err := fx.Git.Branch(root); err != nil || on != "main" {
		return "", fmt.Errorf("merge %s: the root is on %q, not main (%v)", branch, on, err)
	}
	if status, err := fx.Git.RootStatus(root); err != nil || len(status) > 0 {
		return "", fmt.Errorf("merge %s: the root is not clean (%d paths, %v)", branch, len(status), err)
	}
	// On main, before the merge: the probe and the anchor read the memo
	// (§15); the merged tree's verify line is never memoized.
	line := night.VerifyLine(night.MemoShell(fx, root), root, verify)
	abort := func(err error) (string, error) {
		if aerr := w.MergeAbort(root); aerr != nil {
			err = fmt.Errorf("%w; merge --abort: %v", err, aerr)
		}
		record("aborted", err.Error())
		return "", err
	}
	limit := time.Duration(g.VerifySeconds) * time.Second // the gate's (AR59 act 2)
	if limit <= 0 {
		limit = effects.Timeout
	}
	var base *relay.Check
	if len(baseline) > 0 {
		base = night.Anchor(fx, root, baseline[0], line, limit)
	}
	if base == nil || base.Status == relay.CheckUnrun {
		base = night.MemoCheck(fx, root, line, limit) // measured on main at this head by a plan, under this limit
	}
	if line != "" && base != nil && base.Status == relay.CheckRed {
		err := fmt.Errorf("merge %s: main red: %s exits %d, last %q at %s; the row lands branch-only",
			branch, line, base.MainExit, base.MainLast, base.Head)
		record("branch-only", err.Error())
		return "", err
	}
	// The root's lock (R6 §4), held from the tip's check through the
	// commit: no measurement in this root, from any process, sees the
	// merge half done.
	unlock, err := night.RootLock(root)
	if err != nil {
		return "", fmt.Errorf("merge %s: %w", branch, err)
	}
	defer unlock()
	if tip, err := w.Tip(root, branch); err != nil || tip != g.Head {
		err = fmt.Errorf("merge %s: the gate measured %q, the branch is at %q (%v)", branch, g.Head, tip, err)
		record("moved", err.Error())
		return "", err
	}
	// ADV10-1: main is where the gate read it; a move since (a child's
	// update-ref, anyone's commit) refuses the merge with `main-moved`.
	before, err := w.Head(root)
	if err != nil || g.Main != "" && before != g.Main {
		err = fmt.Errorf("merge %s: main-moved: the gate read main at %q, it is at %q (%v)", branch, g.Main, before, err)
		record(string(relay.MainMoved), err.Error())
		return "", err
	}
	verdict, tree := "", ""
	if line != "" {
		// ADV10-10: the verify line runs on the merged tree in a fresh
		// worktree at main, with main's harness laid over it; the root is
		// merged only after it is green.
		// The harness is the one the gate judged with (the row's queue
		// `harness`); a gate built by hand carries none: the defaults.
		hp := g.Harness
		if hp == nil {
			hp = HarnessPaths
		}
		remotes := g.Remotes
		if remotes == nil { // a gate built by hand: the root's
			remotes, _ = w.Remotes(root)
		}
		exit, last, log, harness, cut, merged, err := mergedVerify(fx, effects.DenyingPush(fx.Shell, remotes), root, branch, g.Head, line, hp, name, email, limit)
		tree = merged
		// AR31 act 2: a line that erred or exited non-zero keeps its log,
		// named in the refusal and on the `merge` line; a green one's goes.
		mlog = effects.KeepLog(log, err != nil || exit != 0)
		sfx := ""
		if mlog != "" {
			sfx = "; log: " + mlog
		}
		if errors.Is(err, effects.ErrRefMoved) { // AR22 act 0: the clone fetched another main or branch
			err = fmt.Errorf("merge %s: %s: %w", branch, relay.RefMoved, err)
			record(string(relay.RefMoved), err.Error())
			return "", err
		}
		if err != nil { // bounded's stop among them: unrun, never red (conventions §5)
			err = fmt.Errorf("merge %s: verify exit %d, last %q (%w)%s", branch, exit, last, err, sfx)
			record("aborted", err.Error())
			return "", err
		}
		if cut { // AR59 act 3: cut at the gate's limit; the row is red, verify-cut (fix round, block 1)
			err = fmt.Errorf("merge %s: verify cut at %d s, last %q%s", branch, int(limit/time.Second), last, sfx)
			record(string(relay.VerifyCut), err.Error())
			return "", &relay.MergeRefused{Reason: relay.VerifyCut, Err: err}
		}
		if exit != 0 { // green only: never same-as-main (ADV6-5)
			err = fmt.Errorf("merge %s: verify exit %d, last %q%s", branch, exit, last, sfx)
			record("aborted", err.Error())
			return "", err
		}
		verdict = relay.CheckGreen
		if harness {
			verdict += "; " + HarnessChanged
		}
	}
	// AR21 act 1 (ADV12-2): the verify line ran the branch's code in a
	// linked worktree of the root; the git state is read again before the
	// root merges.
	if err := stateHeld("after the verify line"); err != nil {
		return "", err
	}
	// ADV11-6: the verify line ran the branch's code on the merged tree;
	// the remotes the gate read are read again, and one moved refuses the
	// merge, origin-moved (the row red).
	if same, _, err := remotesSame(w, root, g.RemoteRefs, g.RemoteURLs); err != nil {
		err = fmt.Errorf("merge %s: ls-remote: %w", branch, err)
		record("aborted", err.Error())
		return "", err
	} else if !same {
		err = fmt.Errorf("merge %s: %s: a remote moved since stage", branch, relay.OriginMoved)
		record(string(relay.OriginMoved), err.Error())
		return "", &relay.MergeRefused{Reason: relay.OriginMoved, Err: err}
	}
	if err := w.MergeNoCommit(root, g.Head, name, email); err != nil {
		return abort(fmt.Errorf("merge %s: %w", branch, err))
	}
	msg := fmt.Sprintf("relay: merge %s (gate green", branch)
	if verdict != "" {
		msg += "; verify " + verdict
	}
	if sha, err = w.Commit(root, msg+")", name, email); err != nil {
		return abort(fmt.Errorf("merge %s: commit: %w", branch, err))
	}
	// ADV11-5: the commit holds the tree the verify judged, else it is
	// undone to the recorded main and the row is red.
	if tree != "" {
		if got, err := w.Hermetic(root, "rev-parse", "--verify", "--end-of-options", sha+"^{tree}"); err != nil || got != tree {
			err = fmt.Errorf("merge %s: %s: the commit holds %q, the verify judged %q (%v)", branch, relay.MergeTreeDiffers, got, tree, err)
			if _, rerr := w.Hermetic(root, "reset", "-q", "--hard", before); rerr != nil {
				err = fmt.Errorf("%w; undo to %s: %v", err, before, rerr)
			}
			record(string(relay.MergeTreeDiffers), err.Error())
			return "", &relay.MergeRefused{Reason: relay.MergeTreeDiffers, Err: err}
		}
	}
	// AR21 act 1: a state changed while the root merged (an escaped
	// daemon) undoes the merge to the recorded main.
	if err := stateHeld("after the commit"); err != nil {
		if _, rerr := w.Hermetic(root, "reset", "-q", "--hard", before); rerr != nil {
			err = fmt.Errorf("%w; undo to %s: %v", err, before, rerr)
		}
		return "", err
	}
	ownMerge(root, before, sha) // main's move the next gate in this process expects
	record("merged", strings.TrimSpace(line+" "+verdict))
	return sha, nil
}
