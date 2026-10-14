package night

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// Staged is a row's worktree as Stage left it: the path, the branch off its
// base, HEAD before the sitting, the origin snapshot (§6's origin check
// compares against it) and the lock file held for the path. Detached is
// the hand gate's own worktree, checked out detached at the branch's tip:
// the gate then binds HEAD to the tip, not the checked-out branch (§6).
type Staged struct {
	Path, Branch, Base, HeadBefore string
	Origin                         map[string]string
	// BaseSha is refs/heads/<Base>^{commit} at stage: the gate diffs from it
	// and reds a base that moved since, main-moved (ADV10-1, -2).
	BaseSha string
	// MainSha is refs/heads/main^{commit} at stage, for every row whatever
	// its base: the gate reds a main that moved since but by the relay's
	// own merges in this process, main-moved (AR20 act 1, ADV11-1, -2).
	MainSha string
	// FromLedger is a Staged read back from a `stage` line (an earlier
	// process staged it): its shas, digest and remotes are not read from
	// the line; the gate binds them now (Afresh, AR21 act 5).
	FromLedger bool
	// BaseClaim is a ledger row's base_sha: a claim, never a binding. The
	// gate diffs from the base bound now (Afresh), and a base not main
	// that is not at its claim is main-moved: a forged one only refuses.
	BaseClaim string
	// OriginClaim is a ledger row's origin_refs: a claim, never a binding.
	// The gate reads ls-remote now (Afresh), and what a remote holds now
	// that is not its claim is origin-moved; the same, ls-remote is still
	// skipped, never proven from the ledger (AR22 act 4, ADV13-6).
	OriginClaim map[string]string
	// OriginRefs is `git ls-remote` per remote at stage, keyed
	// "<remote> <ref>"; a remote ls-remote could not read is the one key
	// "<remote>" with the value OriginUnknown (ADV10-3).
	OriginRefs map[string]string
	// RemoteURLs is each remote's `git remote get-url --all` at stage: the
	// literal URLs its OriginRefs were read from, and the gate's ls-remote
	// reads again, never the name the child can repoint (AR20 act 3,
	// ADV11-7). A remote whose URL could not be read has none.
	RemoteURLs map[string][]string
	// StateDigest is effects.GitStateDigest(root, Path) at stage, after the
	// hook is installed: the gate and the merge recompute it first, and a
	// change is git-state-moved (AR20 act 0, ADV11-3, -5).
	StateDigest string
	Remotes     []string // the root's remotes, each denied to the child (ADV6-6)
	Lock        string
	Detached    bool
	// lockFile is the open lock file whose flock is the stage's ownership
	// of Lock (AR17C, ADV10-13); nil for a Staged read from the ledger.
	lockFile *os.File
}

// Unlock releases the path's lock; a second call is a no-op. The file is
// removed while the flock is still held, then closed (AR17C, ADV10-13): a
// taker that opened it meanwhile flocks an unlinked file, sees so and
// opens the path again.
func (s *Staged) Unlock() error {
	if s.Lock == "" {
		return nil
	}
	err := os.Remove(s.Lock)
	s.Lock = ""
	if s.lockFile != nil {
		s.lockFile.Close()
		s.lockFile = nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// LockDir is where Stage's locks live; a variable so a test can move it.
var LockDir = func() string { return filepath.Join(effects.StateDir(os.Getenv), "locks") }

// LiveClaude reports whether a running claude has its cwd at or below path;
// a variable so a test can fake it. The machine's answer is lsof's
// (liveClaude).
var LiveClaude = liveClaude

// LiveClaudeTimeout bounds the probe (R6 §4: under load lsof takes
// minutes); past it the answer is unknown, a refusal.
var LiveClaudeTimeout = 30 * time.Second

// lsofArgv is the probe's command; a variable so a test can put a script
// in lsof's place.
var lsofArgv = []string{"lsof", "-a", "-c", "claude", "-d", "cwd", "-Fn"}

// liveClaude is lsof's answer under LiveClaudeTimeout: exit 1 with both
// streams empty is no claude, with stderr text an lsof error; a probe past
// the timeout is killed and unknown (an error). The probe runs in its own
// process group and the timeout kills the group (AR17C, ADV10-21): killing
// the process alone orphaned its children, a `sleep 20` per run of
// TestAR13_5_LiveClaudeTimesOut.
func liveClaude(path string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), LiveClaudeTimeout)
	defer cancel()
	return probeClaude(ctx, path)
}

// probeClaude is liveClaude under the caller's context: a done context
// kills the probe's group and the answer is unknown. A test cancels on a
// condition (its fake lsof has started a child), not on a clock that load
// can outrun (AR17C-deflake).
func probeClaude(ctx context.Context, path string) (bool, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, lsofArgv[0], lsofArgv[1:]...)
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second // a grandchild holding the pipes does not hold the probe
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return false, fmt.Errorf("lsof: unknown: no answer within %s", LiveClaudeTimeout)
	}
	msg := strings.TrimSpace(stderr.String())
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 && len(bytes.TrimSpace(out)) == 0 && msg == "" {
		return false, nil // no claude process at all
	}
	if err != nil {
		if msg != "" {
			return false, fmt.Errorf("lsof: %s", msg)
		}
		return false, fmt.Errorf("lsof: %w", err)
	}
	path = filepath.Clean(path)
	for _, l := range strings.Split(string(out), "\n") {
		if cwd, ok := strings.CutPrefix(l, "n"); ok {
			if cwd = filepath.Clean(cwd); cwd == path || strings.HasPrefix(cwd, path+"/") {
				return true, nil
			}
		}
	}
	return false, nil
}

// Locked reports whether a Stage lock is held for path (§8).
func Locked(path string) bool {
	_, err := os.Stat(lockPath(path))
	return err == nil
}

// lockPath is `<state>/locks/<sha1 of path>` (§8).
func lockPath(path string) string {
	return filepath.Join(LockDir(), fmt.Sprintf("%x", sha1.Sum([]byte(filepath.Clean(path)))))
}

// LockHolder is a stage lock's content: `pid<TAB>started<TAB>worktree`,
// the holder's pid, its start time as Procs.Started reads it, and the
// worktree (R6 §4). A lock of the older shape, `pid worktree`, has no
// Started.
type LockHolder struct {
	Pid      int
	Started  string
	Worktree string
}

// readLock parses the lock at path.
func readLock(path string) (LockHolder, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return LockHolder{}, err
	}
	var h LockHolder
	text := strings.TrimSpace(string(b))
	if f := strings.Split(text, "\t"); len(f) == 3 {
		h.Started, h.Worktree = f[1], f[2]
		_, err = fmt.Sscan(f[0], &h.Pid)
	} else {
		_, err = fmt.Sscan(text, &h.Pid, &h.Worktree)
	}
	if err != nil {
		return h, fmt.Errorf("the lock %s: %q: %w", path, text, err)
	}
	return h, nil
}

// StaleLock reads the stage lock at path and reports whether its holder
// is gone (R6 §4): the pid is not running, or runs with another start time
// (a reused pid). Unreadable, or with no Procs to ask, it is held.
func StaleLock(procs effects.Procs, path string) (LockHolder, bool) {
	h, err := readLock(path)
	if err != nil || procs == nil || h.Pid <= 0 {
		return h, false
	}
	started, err := procs.Started(h.Pid)
	if err != nil {
		return h, true
	}
	return h, h.Started != "" && started != h.Started
}

// LiveHolder is the holder of worktree's stage lock when the lock exists
// and StaleLock reads it held (AR66 ruling 4): `agentik-relay clean --merged`
// keeps such a tree, a paused row's at its cut, an ancestor of main
// (usage-limit-resume r1 §2). No lock, or a gone holder: false.
func LiveHolder(procs effects.Procs, worktree string) (LockHolder, bool) {
	path := lockPath(worktree)
	if _, err := os.Stat(path); err != nil {
		return LockHolder{}, false
	}
	h, stale := StaleLock(procs, path)
	return h, !stale
}

// takeLock takes the stage lock at path for worktree and returns the open
// file whose flock is the ownership, held until Unlock closes it (AR17C,
// ADV10-13: the old read, remove, create let two takers of a stale lock
// both hold it; six at once left more than one holder in 40 of 40 runs).
// The kernel decides the one holder: LOCK_EX|LOCK_NB on the path's file,
// then the file must still be the path's (an Unlock or a reap may have
// removed it between open and flock; then open again, once). A holder
// line already there without a flock is a crashed holder or a lock of the
// pre-flock shape: StaleLock rules it, a live one refuses, a stale one is
// reclaimed with a `lock-reclaimed` line. The file then holds this
// process's pid and start time, for humans and for Locked.
func takeLock(fx effects.Effects, row, path, worktree string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	var f *os.File
	for try := 0; ; try++ {
		var err error
		if f, err = flockPath(path); err != nil {
			return nil, err
		}
		if f != nil {
			break
		}
		if try == 1 {
			return nil, fmt.Errorf("the lock %s is contended, not taken", path)
		}
	}
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		h, stale := StaleLock(fx.Procs, path)
		if !stale {
			f.Close()
			return nil, fmt.Errorf("the lock %s is held by pid %d", path, h.Pid)
		}
		if fx.Ledger != nil {
			fx.Ledger.Append("lock-reclaimed", map[string]any{"row": row, "lock": path,
				"pid": h.Pid, "started": h.Started, "worktree": h.Worktree})
		}
	}
	started := ""
	if fx.Procs != nil {
		started, _ = fx.Procs.Started(os.Getpid())
	}
	err := f.Truncate(0)
	if err == nil {
		_, err = f.WriteAt([]byte(fmt.Sprintf("%d\t%s\t%s\n", os.Getpid(), started, worktree)), 0)
	}
	if err != nil {
		os.Remove(path) // this process's own, half written, removed under the flock
		f.Close()
		return nil, err
	}
	return f, nil
}

// flockPath opens path (creating it) and flocks it LOCK_EX|LOCK_NB (AR17C,
// ADV10-13): the flocked file when it is still the one at path, nil when
// it was removed or replaced between the open and the flock (try again),
// an error naming the holder when another holds the flock.
func flockPath(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			h, _ := readLock(path)
			return nil, fmt.Errorf("the lock %s is held by pid %d", path, h.Pid)
		}
		return nil, fmt.Errorf("the lock %s: flock: %w", path, err)
	}
	held, err1 := f.Stat()
	now, err2 := os.Stat(path)
	if err1 != nil || err2 != nil || !os.SameFile(held, now) {
		f.Close()
		return nil, nil
	}
	return f, nil
}

// ReapStale removes every stage lock under LockDir whose holder is gone
// (StaleLock), each with a `lock-reclaimed` line, and returns their
// holders: `agentik-relay clean --stale` (§9). The roots' flock files are never
// stale (the kernel drops a dead holder's) and stay.
func ReapStale(fx effects.Effects) ([]LockHolder, error) {
	dir := LockDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var gone []LockHolder
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), "root-") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		// Only under the lock's own flock (AR17C, ADV10-13): a held lock is
		// never removed from under its holder, nor one a taker is reclaiming.
		f, err := flockPath(p)
		if err != nil || f == nil {
			continue
		}
		h, stale := StaleLock(fx.Procs, p)
		if !stale {
			f.Close()
			continue
		}
		err = os.Remove(p)
		f.Close()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return gone, err
		}
		if fx.Ledger != nil {
			fx.Ledger.Append("lock-reclaimed", map[string]any{"row": filepath.Base(h.Worktree), "lock": p,
				"pid": h.Pid, "started": h.Started, "worktree": h.Worktree})
		}
		gone = append(gone, h)
	}
	return gone, nil
}

// rootOf is the root a row's worktree `<root>/.claude/worktrees/<id>` lives in.
func rootOf(worktree string) (string, error) {
	wts := filepath.Dir(filepath.Clean(worktree))
	if filepath.Base(wts) != "worktrees" || filepath.Base(filepath.Dir(wts)) != ".claude" {
		return "", fmt.Errorf("stage: %s is not under <root>/.claude/worktrees/", worktree)
	}
	return filepath.Dir(filepath.Dir(wts)), nil
}

// Stage makes a row's worktree (§8): it refuses a worktree or branch that
// already exists, a lock held for the path, or a live claude whose cwd is
// the path; then excludes the worktrees from the root's status, adds the
// worktree on a new branch off the base, installs the pre-push hook,
// snapshots origin and HEAD, and writes the `stage` ledger line. The lock
// is held until Unlock.
func Stage(fx effects.Effects, rp *RowPlan) (*Staged, error) {
	root, err := rootOf(rp.Worktree)
	if err != nil {
		return nil, err
	}
	w := fx.Git
	if fx.FS.Exists(rp.Worktree) {
		return nil, fmt.Errorf("stage %s: the worktree %s exists", rp.ID, rp.Worktree)
	}
	if exists, err := fx.Git.BranchExists(root, rp.Branch); err != nil || exists {
		return nil, fmt.Errorf("stage %s: the branch %s exists (%v)", rp.ID, rp.Branch, err)
	}
	st := &Staged{Path: rp.Worktree, Branch: rp.Branch, Base: rp.Base, Lock: lockPath(rp.Worktree)}
	if st.lockFile, err = takeLock(fx, rp.ID, st.Lock, rp.Worktree); err != nil {
		return nil, fmt.Errorf("stage %s: %w", rp.ID, err)
	}
	fail := func(format string, a ...any) (*Staged, error) {
		st.Unlock()
		return nil, fmt.Errorf("stage %s: "+format, append([]any{rp.ID}, a...)...)
	}
	if live, err := LiveClaude(rp.Worktree); err != nil {
		return fail("a live claude in %s is unknown: %v", rp.Worktree, err)
	} else if live {
		return fail("a live claude has its cwd in %s", rp.Worktree)
	}
	if err := w.Exclude(root, ".claude/worktrees/"); err != nil {
		return fail("exclude: %v", err)
	}
	// AR21 act 1 (ADV12-2), before the checkout: no program-valued key in
	// the root's config, and its git state is the one this process bound
	// at the root's first stage; else nothing is checked out or spawned.
	if why, err := GitStateRefused(root, "", false); why != "" {
		return fail("%s: %v", why, err)
	}
	if err := w.WorktreeAdd(root, rp.Worktree, rp.Branch, rp.Base); err != nil {
		return fail("worktree: %v", err)
	}
	if err := w.InstallPrePush(rp.Worktree); err != nil {
		// No relay worktree lives without its hook; the branch stays.
		w.WorktreeRemove(root, rp.Worktree)
		return fail("the hook: %v", err)
	}
	if st.Origin, err = fx.Git.RemoteRefs(root); err != nil {
		return fail("origin: %v", err)
	}
	if st.Remotes, err = fx.Git.Remotes(root); err != nil {
		return fail("remotes: %v", err)
	}
	if st.HeadBefore, err = fx.Git.Head(rp.Worktree); err != nil {
		return fail("head: %v", err)
	}
	if st.MainSha, err = fx.Git.RevParseCommit(root, BaseRef("main")); err != nil {
		return fail("main: %v", err)
	}
	st.BaseSha = st.MainSha
	if st.Base != "main" {
		if st.BaseSha, err = fx.Git.RevParseCommit(root, BaseRef(st.Base)); err != nil {
			return fail("base: %v", err)
		}
	}
	st.RemoteURLs = RemoteURLsOf(fx.Git, root, st.Remotes)
	st.OriginRefs = LsRemotes(fx.Git, root, st.Remotes, st.RemoteURLs)
	// The stage's own writes are done: the root's state is bound now, at
	// its first stage in this process, else compared again.
	if why, err := GitStateRefused(root, rp.Worktree, true); why != "" {
		return fail("%s: %v", why, err)
	}
	if st.StateDigest, err = effects.GitStateDigest(root, rp.Worktree); err != nil {
		return fail("git state: %v", err)
	}
	if err := fx.Ledger.Append("stage", map[string]any{"row": rp.ID, "worktree": st.Path,
		"branch": st.Branch, "base": st.Base, "head_before": st.HeadBefore, "origin": st.Origin,
		"base_sha": st.BaseSha, "main_sha": st.MainSha, "origin_refs": st.OriginRefs, "remote_urls": st.RemoteURLs, "state_digest": st.StateDigest}); err != nil {
		return fail("the ledger: %v", err)
	}
	return st, nil
}

// stateBase is each root's git state, effects.GitStateDigest(root, ""), as
// this process bound it at the root's first stage (a hand gate's, when it
// staged none): held in memory beside the gate's own merges, never read
// back from the ledger (AR21 act 1, ADV12-2).
var stateBase = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

// GitStateRefused is why root's git state refuses a stage, a gate or a
// merge, else "" (AR21 act 1, ADV12-2): a program-valued key in root's
// local config or any worktree config (effects.ProgramKeys, worktree's
// own among them), or a config that does not parse, is
// git-config-program; a state that is not the one this process bound for
// root is git-state-moved. bind binds it when none is bound yet (the
// stage's own writes done, or a hand gate); without it, a root never
// bound compares nothing. err says what was found.
func GitStateRefused(root, worktree string, bind bool) (relay.StopReason, error) {
	keys, err := effects.ProgramKeys(root, worktree)
	if err != nil {
		return relay.GitConfigProgram, fmt.Errorf("the git config: %w", err)
	}
	if len(keys) > 0 {
		return relay.GitConfigProgram, fmt.Errorf("program-valued git config: %s", strings.Join(keys, ", "))
	}
	now, err := effects.GitStateDigest(root, "")
	if err != nil {
		return relay.GitStateMoved, fmt.Errorf("the git state: %w", err)
	}
	k := root
	if r, err := filepath.EvalSymlinks(root); err == nil {
		k = r
	}
	k = filepath.Clean(k)
	stateBase.Lock()
	defer stateBase.Unlock()
	base, ok := stateBase.m[k]
	if !ok {
		if bind {
			stateBase.m[k] = now
		}
		return "", nil
	}
	if base != now {
		// AR53 act 2: the entry that moved, named from the per-entry sums
		// effects keeps beside each digest; all of them when it has none.
		what := effects.GitStateMovedEntry(base, now)
		if what == "" {
			what = "config, hooks, info or alternates"
		}
		return relay.GitStateMoved, fmt.Errorf("%s changed since the root's first stage", what)
	}
	return "", nil
}

// OriginUnknown is a remote's OriginRefs value when ls-remote failed at
// stage: the gate cannot compare it, and says so (skipped `ls-remote`).
const OriginUnknown = "unknown"

// BaseRef is the full refname the base is bound by: a tag or a remote
// branch named like it never answers (ADV10-2).
func BaseRef(base string) string { return "refs/heads/" + base + "^{commit}" }

// RemoteURLsOf is each of remotes' URLs as `git remote get-url --all`
// reads them in dir; a remote whose URLs cannot be read is left out.
func RemoteURLsOf(g effects.Git, dir string, remotes []string) map[string][]string {
	out := map[string][]string{}
	for _, r := range remotes {
		if urls, err := g.RemoteURLs(dir, r); err == nil && len(urls) > 0 {
			out[r] = urls
		}
	}
	return out
}

// LsRemotes is OriginRefs' shape for remotes: "<remote> <ref>" to sha,
// read at the remote's first URL in urls (the one a fetch reads), or
// "<remote>" to OriginUnknown for one with no URL or one ls-remote could
// not read.
func LsRemotes(g effects.Git, dir string, remotes []string, urls map[string][]string) map[string]string {
	out := map[string]string{}
	for _, r := range remotes {
		if len(urls[r]) == 0 {
			out[r] = OriginUnknown
			continue
		}
		refs, err := g.LsRemote(dir, urls[r][0])
		if err != nil {
			out[r] = OriginUnknown
			continue
		}
		for k, v := range refs {
			out[r+" "+k] = v
		}
	}
	return out
}

// AccountKept is the account stop the row's last `exit` line carries, ""
// when it carries none (run 10 ruling 1): `agentik-relay clean --stale` keeps that
// worktree, its uncommitted files the next attempt's.
func AccountKept(lines []map[string]any, row string) relay.StopReason {
	var r relay.StopReason
	for _, l := range lines {
		if l["event"] == "exit" && l["row"] == row {
			s, _ := l["reason"].(string)
			r = relay.StopReason(s)
		}
	}
	if relay.AccountStop(r) {
		return r
	}
	return ""
}

// StagedFromLedger is the row's last `stage` line as a Staged (no lock), or
// nil when the ledger has none: a hand gate then has no exit facts and no
// origin snapshot. The ledger is a claim, never a binding (AR21 act 5,
// ADV12-7): the child can append a line, so its main_sha, state_digest
// and remote_urls are never read, its base_sha only as BaseClaim and its
// origin_refs only as OriginClaim (AR22 act 4); the gate binds them from
// git now (Afresh).
func StagedFromLedger(lines []map[string]any, row string) *Staged {
	var st *Staged
	for _, l := range lines {
		if l["event"] != "stage" || l["row"] != row {
			continue
		}
		str := func(k string) string { s, _ := l[k].(string); return s }
		st = &Staged{Path: str("worktree"), Branch: str("branch"), Base: str("base"),
			HeadBefore: str("head_before"), Origin: map[string]string{}, FromLedger: true,
			BaseClaim: str("base_sha")}
		if o, ok := l["origin"].(map[string]any); ok {
			for k, v := range o {
				st.Origin[k], _ = v.(string)
			}
		}
		if o, ok := l["origin_refs"].(map[string]any); ok {
			st.OriginClaim = map[string]string{}
			for k, v := range o {
				st.OriginClaim[k], _ = v.(string)
			}
		}
	}
	return st
}

// Afresh is st, a Staged read from the ledger, bound from git in root now
// (AR21 act 5, ADV12-7): main and the base by refname, the root's remotes,
// each one's URLs, and what each holds by ls-remote at them. Its git state
// is act 1's, bound in this process's memory at the root's first stage or
// gate (GitStateRefused), so StateDigest is left empty. st is unchanged.
func Afresh(g effects.Git, root string, st *Staged) (*Staged, error) {
	b := *st
	b.StateDigest = ""
	var err error
	if b.MainSha, err = g.RevParseCommit(root, BaseRef("main")); err != nil {
		return nil, fmt.Errorf("main: %w", err)
	}
	b.BaseSha = b.MainSha
	if b.Base != "" && b.Base != "main" {
		if b.BaseSha, err = g.RevParseCommit(root, BaseRef(b.Base)); err != nil {
			return nil, fmt.Errorf("base: %w", err)
		}
	}
	if b.Remotes, err = g.Remotes(root); err != nil {
		return nil, fmt.Errorf("remotes: %w", err)
	}
	b.RemoteURLs = RemoteURLsOf(g, root, b.Remotes)
	b.OriginRefs = LsRemotes(g, root, b.Remotes, b.RemoteURLs)
	return &b, nil
}

// BaselineOnMain is §6's baseline for lines, measured in the root when it
// is clean and on main, else each line unrun with the reason; under the
// recursion guard (§15) every line is unrun: nested, with no shell call.
// A line measured at the root's HEAD is read from the memo under
// XDG's state directory (none when Getenv is nil).
func BaselineOnMain(fx effects.Effects, root string, lines []packet.Line) []relay.Check {
	if effects.Nested(fx.Getenv) {
		return baseline(fx.Shell, root, lines, effects.NestedReason)
	}
	if why := baselineBlocked(fx.Git, root); why != "" {
		return baseline(fx.Shell, root, lines, why)
	}
	return baseline(newBound(fx, memoState(fx), root), root, lines, "") // through the memo (§15)
}
