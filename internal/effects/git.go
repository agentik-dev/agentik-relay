package effects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// OSGit runs the git binary (os/exec), one command per call.
type OSGit struct{}

// worktreesDir is where the relay keeps its worktrees inside the root; the
// root's Status leaves them out.
const worktreesDir = ".claude/worktrees/"

// gitCmd is the hermetic runner every git call of the gate and the merge
// goes through (AR20 act 0; ADV11-3, -5, -14): no system config, no hooks
// (core.hooksPath=/dev/null), no fsmonitor, and no prompt waiting on a
// terminal. No global config either (AR21 act 0, ADV12-1): a sitting can
// write ~/.gitconfig, and a smudge filter or an insteadOf there would judge
// the tree; nor the default global attributes and ignore files, so a global
// ignore never hides an untracked file from clean. The repo's own config is
// still read; GitStateDigest binds it. No replace refs (AR22 act 0,
// ADV13-1): refs/replace/ lives in the common dir a child writes, and a
// replaced blob would judge one tree and merge another.
func gitCmd(dir string, args ...string) *exec.Cmd { return runnerCmd(dir, runnerArgs, args...) }

// runnerCmd is git -C dir under the runner's env with pre, the runner's
// `-c` arguments (runnerArgs, or readArgs for the one read that must see
// a tree's own hooks path). It is the pinned git under the pinned env
// alone (AR22 act 1, ADV13-2, -3), which holds runnerEnv; before Pin the
// command fails to start.
func runnerCmd(dir string, pre []string, args ...string) *exec.Cmd {
	p, err := pinnedEnv()
	cmd := exec.Command(p.Git, append(append([]string{"-C", dir}, pre...), args...)...)
	cmd.Env = slices.Clone(p.Env)
	if err != nil {
		cmd.Err = err
	}
	return cmd
}

// runnerArgs and runnerEnv are the runner's: gitCmd's and LsRemote's;
// readArgs is runnerArgs without core.hooksPath=/dev/null.
var (
	readArgs = []string{"-c", "core.fsmonitor=false",
		"-c", "core.attributesFile=/dev/null", "-c", "core.excludesFile=/dev/null"}
	runnerArgs = append([]string{"-c", "core.hooksPath=/dev/null"}, readArgs...)
	runnerEnv  = []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0",
		"GIT_NO_REPLACE_OBJECTS=1"}
)

// identity is the committer identity Identity read, once.
var identity struct {
	once        sync.Once
	name, email string
}

// Identity is the committer the merge commits as (AR21 act 0, ADV12-1):
// read once per process, on the first call, by `git var
// GIT_COMMITTER_IDENT` outside any repository, one of the two reads that
// see the owner's global config (the other is the pin's of
// core.excludesFile, AR28B act 1). Pin calls it at start (AR22 act 1), before any
// spawn, so
// a ~/.gitconfig a child writes never names the merge's committer; the
// merge passes it by `-c user.name= -c user.email=` under the runner. Empty
// when git resolves none: the merge is then unrun, no-identity.
func Identity() (name, email string) {
	identity.once.Do(func() { identity.name, identity.email = readIdentity() })
	return identity.name, identity.email
}

// ParseIdent splits git's `Name <email> <time> <zone>` into its name and
// email; "", "" when it is not one.
func ParseIdent(s string) (name, email string) {
	i, j := strings.LastIndex(s, "<"), strings.LastIndex(s, ">")
	if i < 0 || j < i {
		return "", ""
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1 : j])
}

// identArgs are the `-c` pair that makes name and email the committer
// (and author) of a runner's merge or commit.
func identArgs(name, email string, args ...string) []string {
	return append([]string{"-c", "user.name=" + name, "-c", "user.email=" + email}, args...)
}

// git runs args under the hermetic runner; a failure's error keeps its
// stderr.
func git(dir string, args ...string) (string, error) {
	return output(gitCmd(dir, args...), args)
}

func output(cmd *exec.Cmd, args []string) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// gitStrict is git for the gate's reads (ADV10-2): a warning on stderr (an
// ambiguous refname, an unreadable directory) is an error, never dropped,
// so the fact it would have bent is unrun. It runs under the hermetic
// runner too.
func gitStrict(dir string, args ...string) (string, error) {
	return strict(gitCmd(dir, args...), args)
}

// strict is cmd's stdout, a warning on its stderr an error (gitStrict's).
func strict(cmd *exec.Cmd, args []string) (string, error) {
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if msg := strings.TrimSpace(stderr.String()); err != nil || msg != "" {
		if msg == "" {
			msg = fmt.Sprint(err)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Hermetic is one git command under the hermetic runner: its stdout, or
// an error holding its stderr.
func (OSGit) Hermetic(dir string, args ...string) (string, error) { return git(dir, args...) }

// RevParseCommit is ref as a commit's sha, read strictly: the gate binds
// `refs/heads/<base>^{commit}`, never a bare name a tag can shadow.
func (OSGit) RevParseCommit(dir, ref string) (string, error) {
	return gitStrict(dir, "rev-parse", "--verify", "--end-of-options", ref)
}

// LsRemoteTimeout bounds one ls-remote (ADV11-16): a stalled transport is
// an error after it, never a gate that waits; a variable so a test can
// shorten it.
var LsRemoteTimeout = 30 * time.Second

// ErrLsRemoteTimeout is wrapped by LsRemote's error at LsRemoteTimeout, so
// the gate retries a stalled read and names it remote-timeout (AR88); the
// error's text stays `git ls-remote <url>: timed out after <d>`.
var ErrLsRemoteTimeout = errors.New("timed out")

// lsRemoteSSH is LsRemote's GIT_SSH_COMMAND, the one ssh it runs (never
// this process's GIT_SSH_COMMAND: AR22 act 1); a variable so a test can
// stall the transport.
var lsRemoteSSH = "ssh -o BatchMode=yes"

// LsRemote is `git ls-remote <url>`, ref name to sha: what the remote
// holds, which a push by URL changes though no tracking ref moves (ADV10-3).
// url is the literal URL stage resolved (AR20 act 3, ADV11-7): it runs
// outside any repository (no repo config, so no insteadOf or remote the
// child can repoint), under the runner's no-system and no-global config
// (no global insteadOf either: ADV12-1), with
// LsRemoteTimeout, in its own process group killed whole on the timeout,
// and with `ssh -o BatchMode=yes` (ADV11-16). It runs the pinned git under
// the pinned env alone (AR22 act 1): an ssh remote it cannot read there is
// an error, which stage skips.
// A relative local path is dir's.
func (OSGit) LsRemote(dir, url string) (map[string]string, error) {
	if localPath(url) && !filepath.IsAbs(url) {
		url = filepath.Join(dir, url)
	}
	ctx, cancel := context.WithTimeout(context.Background(), LsRemoteTimeout)
	defer cancel()
	args := []string{"ls-remote", "--end-of-options", url}
	p, perr := pinnedEnv()
	cmd := exec.CommandContext(ctx, p.Git, append(slices.Clone(runnerArgs), args...)...)
	cmd.Dir = "/"
	cmd.Env = append(slices.Clone(p.Env), "GIT_SSH_COMMAND="+lsRemoteSSH)
	if perr != nil {
		cmd.Err = perr
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // whatever the group still holds
		return nil, fmt.Errorf("git ls-remote %s: %w after %s", url, ErrLsRemoteTimeout, LsRemoteTimeout)
	}
	if msg := strings.TrimSpace(stderr.String()); err != nil || msg != "" {
		if msg == "" {
			msg = fmt.Sprint(err)
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	refs := map[string]string{}
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if sha, name, ok := strings.Cut(l, "\t"); ok {
			refs[name] = sha
		}
	}
	return refs, nil
}

// noRepoEnv is environ without the variables that name a repository, so a
// git run outside one stays outside.
func noRepoEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_NAMESPACE", "GIT_CEILING_DIRECTORIES":
			continue
		}
		out = append(out, kv)
	}
	return out
}

// localPath reports whether url is a local path: no scheme, no `::`
// transport, and not scp's `host:path`.
func localPath(url string) bool {
	if strings.Contains(url, "://") || strings.Contains(url, "::") {
		return false
	}
	colon := strings.IndexByte(url, ':')
	slash := strings.IndexByte(url, '/')
	return colon < 0 || (slash >= 0 && slash < colon)
}

// RemoteURLs is `git remote get-url --all <remote>`: the URLs a fetch of
// remote reads, a relative local path made dir's (AR20 act 3, ADV11-7).
func (OSGit) RemoteURLs(dir, remote string) ([]string, error) {
	out, err := git(dir, "remote", "get-url", "--all", "--end-of-options", remote)
	if err != nil {
		return nil, err
	}
	var urls []string
	for _, u := range strings.Split(out, "\n") {
		if u == "" {
			continue
		}
		if localPath(u) && !filepath.IsAbs(u) {
			u = filepath.Join(dir, u)
		}
		urls = append(urls, u)
	}
	return urls, nil
}

func (OSGit) Toplevel(dir string) (string, error) { return git(dir, "rev-parse", "--show-toplevel") }

// Root is the main worktree: the first entry of `git worktree list`.
func (OSGit) Root(dir string) (string, error) {
	out, err := git(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(out, "\n")
	root, ok := strings.CutPrefix(first, "worktree ")
	if !ok {
		return "", fmt.Errorf("git worktree list: no main worktree")
	}
	return filepath.Clean(root), nil
}

func (OSGit) Head(dir string) (string, error) { return git(dir, "rev-parse", "HEAD") }

func (OSGit) Branch(dir string) (string, error) { return git(dir, "branch", "--show-current") }

func (OSGit) Tip(dir, branch string) (string, error) {
	return git(dir, "rev-parse", "--verify", "-q", "refs/heads/"+branch+"^{commit}")
}

// Status lists every file, untracked ones one by one (so an untracked
// `.claude/` is never a single opaque line), and skips only paths under
// `.claude/worktrees/`: any other `.claude/` content is dirt.
func (OSGit) Status(dir string) ([]string, error) { return status(gitCmd(dir, statusArgs...)) }

// RootStatus is Status under the owner's global ignore as the pin copied
// it (AR28B act 1, run 15's decision 4): `-c core.excludesFile=<Excludes>`
// after runnerArgs, git's last `-c` winning, so a path the owner ignores
// (run 14's `.claude/settings.local.json`) is not dirt in the root. For the
// root's readers alone (the baseline's, doctor's root row, the merge's
// clean check, the memo's tree); a worktree's stays Status, strict. The
// copy is the pin's, so an ignore edited later hides nothing; Status when
// the pin copied none.
func (g OSGit) RootStatus(dir string) ([]string, error) {
	p, err := pinnedEnv()
	if err != nil {
		return nil, err
	}
	if p.Excludes == "" {
		return g.Status(dir)
	}
	pre := slices.Concat(runnerArgs, []string{"-c", "core.excludesFile=" + p.Excludes})
	return status(runnerCmd(dir, pre, statusArgs...))
}

// statusArgs are Status's and RootStatus's.
var statusArgs = []string{"status", "--porcelain", "--untracked-files=all"}

// status is cmd's porcelain, read strictly, less `.claude/worktrees/`.
func status(cmd *exec.Cmd) ([]string, error) {
	out, err := strict(cmd, statusArgs)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l == "" || (len(l) > 3 && strings.HasPrefix(l[3:], worktreesDir)) {
			continue
		}
		lines = append(lines, l)
	}
	return lines, nil
}

func (OSGit) IsAncestor(dir, a, b string) (bool, error) {
	err := gitCmd(dir, "merge-base", "--is-ancestor", a, b).Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return false, nil
	default:
		return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %v", a, b, err)
	}
}

// FirstParentMerged reads `git log --first-parent --merges --format=%P main`:
// true when a merge commit on main's first-parent line has tip's sha as its
// second (or later) parent. A branch cut off main and never moved is on the
// first-parent line itself, never a merged parent, so it is not landed.
func (OSGit) FirstParentMerged(dir, tip, main string) (bool, error) {
	sha, err := git(dir, "rev-parse", "--verify", "--quiet", tip+"^{commit}")
	if err != nil {
		return false, err
	}
	out, err := git(dir, "log", "--first-parent", "--merges", "--format=%P", main)
	if err != nil {
		return false, err
	}
	for _, l := range strings.Split(out, "\n") {
		if ps := strings.Fields(l); len(ps) > 1 && slices.Contains(ps[1:], sha) {
			return true, nil
		}
	}
	return false, nil
}

func (OSGit) BranchExists(dir, name string) (bool, error) {
	err := gitCmd(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+name).Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return false, nil
	default:
		return false, err
	}
}

// RemoteRefs is `git for-each-ref refs/remotes`, ref name to sha (§6 origin).
func (OSGit) RemoteRefs(dir string) (map[string]string, error) {
	out, err := git(dir, "for-each-ref", "--format=%(refname) %(objectname)", "refs/remotes")
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if name, sha, ok := strings.Cut(l, " "); ok {
			refs[name] = sha
		}
	}
	return refs, nil
}

// Remotes is `git remote`: the names ChildEnv denies a push to.
func (OSGit) Remotes(dir string) ([]string, error) {
	out, err := git(dir, "remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// Worktree is one entry of `git worktree list --porcelain`; Branch is ""
// when detached.
type Worktree struct {
	Path, Head, Branch string
}

// WorktreeAdd runs under the runner too (AR22 act 0, ADV13-10): a global
// core.hooksPath a sitting wrote never runs a post-checkout as the relay.
func (OSGit) WorktreeAdd(dir, path, branch, base string) error {
	_, err := git(dir, "worktree", "add", "-b", branch, path, base)
	return err
}

func (OSGit) WorktreeAddDetached(dir, path, ref string) error {
	_, err := git(dir, "worktree", "add", "--detach", path, ref)
	return err
}

// Ref is a full refname and the sha it is bound to.
type Ref struct{ Name, Sha string }

// ErrRefMoved is Clone's error when a ref it fetched is not at its bound
// sha (relay's ref-moved).
var ErrRefMoved = errors.New("ref-moved")

// Clone makes path, an empty directory, a new repository (`git init
// --template=`: no hooks, no info/) and fetches refs into it from dir's
// repository, each by its full refname into the same name (no pattern, no
// tag, no submodule); each fetched sha must be its bound one, else an
// error wrapping ErrRefMoved; then it checks out the first ref's sha,
// detached. The clone shares no config, refs, alternates, hooks, info/ or
// shallow with dir's repository, and is none of its worktrees; all of it
// runs under the runner (AR22 act 0, ADV13-1, -14).
func (OSGit) Clone(dir, path string, refs ...Ref) error {
	if len(refs) == 0 {
		return errors.New("clone: no ref to fetch")
	}
	src, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	fetch := []string{"fetch", "-q", "--no-tags", "--no-recurse-submodules", "--update-head-ok", src}
	for _, r := range refs {
		if !strings.HasPrefix(r.Name, "refs/") || strings.ContainsAny(r.Name, "*:^~ ") || r.Sha == "" {
			return fmt.Errorf("clone: %q is not a full refname bound to a sha", r.Name)
		}
		fetch = append(fetch, r.Name+":"+r.Name)
	}
	if _, err := git(path, "init", "-q", "--template="); err != nil {
		return err
	}
	if _, err := git(path, fetch...); err != nil {
		return err
	}
	for _, r := range refs {
		got, err := gitStrict(path, "rev-parse", "--verify", "--end-of-options", r.Name+"^{commit}")
		if err != nil {
			return err
		}
		if got != r.Sha {
			return fmt.Errorf("clone: %w: %s is at %s, bound at %s", ErrRefMoved, r.Name, got, r.Sha)
		}
	}
	_, err = git(path, "checkout", "-q", "-f", "--detach", refs[0].Sha)
	return err
}

func (OSGit) WorktreeRemove(dir, path string) error {
	_, err := git(dir, "worktree", "remove", path)
	return err
}

func (OSGit) WorktreeDiscard(dir, path string) error {
	_, err := git(dir, "worktree", "remove", "--force", path)
	if err != nil {
		return err
	}
	_, err = git(dir, "worktree", "prune")
	return err
}

func (OSGit) Worktrees(dir string) ([]Worktree, error) {
	out, err := git(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var wts []Worktree
	for _, block := range strings.Split(out, "\n\n") {
		var w Worktree
		for _, l := range strings.Split(block, "\n") {
			k, v, _ := strings.Cut(l, " ")
			switch k {
			case "worktree":
				w.Path = filepath.Clean(v)
			case "HEAD":
				w.Head = v
			case "branch":
				w.Branch = strings.TrimPrefix(v, "refs/heads/")
			}
		}
		if w.Path != "" {
			wts = append(wts, w)
		}
	}
	return wts, nil
}

func (OSGit) DiffNames(dir, base, branch string) ([]string, error) {
	// --no-renames: a rename lists its source too, so moving a path out
	// from under may-touch never hides it (ADV6-4).
	out, err := gitStrict(dir, "diff", "--name-only", "--no-renames", base+"..."+branch, "--")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

// Overlay lays paths as they are at sha over dir's tree and index (`git
// restore --source`): a path sha lacks is removed, one neither side knows
// is left alone (AR17A act 2: main's harness over the gate's fresh tree).
func (OSGit) Overlay(dir, sha string, paths ...string) error {
	var known []string
	for _, p := range paths {
		inSha, err := git(dir, "ls-tree", "--name-only", sha, "--", p)
		if err != nil {
			return err
		}
		inIndex, err := git(dir, "ls-files", "--", p)
		if err != nil {
			return err
		}
		if inSha != "" || inIndex != "" {
			known = append(known, p)
		}
	}
	if len(known) == 0 {
		return nil
	}
	_, err := git(dir, append([]string{"restore", "--source=" + sha, "--staged", "--worktree", "--"}, known...)...)
	return err
}

// MergeNoCommit is `merge --no-ff --no-commit` as name <email> (Identity):
// the runner reads no global config, and git wants a committer here.
func (OSGit) MergeNoCommit(dir, branch, name, email string) error {
	_, err := git(dir, identArgs(name, email, "merge", "--no-ff", "--no-commit", branch)...)
	return err
}

func (OSGit) MergeAbort(dir string) error {
	_, err := git(dir, "merge", "--abort")
	return err
}

func (OSGit) Commit(dir, msg, name, email string) (string, error) {
	// ADV11-5: --no-verify under the runner; the merge then checks the
	// committed tree against the one it verified. ADV12-1: as name <email>,
	// the identity read at start, never a global config's.
	// AR22 act 0 (ADV13-9): never signed, so no gpg or ssh key program a
	// repo config names runs.
	if _, err := git(dir, identArgs(name, email, "-c", "commit.gpgSign=false", "commit", "-q", "--no-verify", "-m", msg)...); err != nil {
		return "", err
	}
	return git(dir, "rev-parse", "HEAD")
}

// PrePushHook is the hook InstallPrePush writes (R25-91 (4)): every push
// from the worktree is refused with one line naming the relay.
const PrePushHook = "#!/bin/sh\necho \"relay: push refused\" >&2; exit 1\n"

// InstallPrePush writes the pre-push hook for the linked worktree at path
// only, under the runner (AR22 act 0). Hooks live in the common git
// directory, shared with the root, so the worktree gets its own hooks
// directory (core.hooksPath in its per-worktree config) and the owner's
// pushes from the root stay untouched; the repo's other hooks are linked
// into it (linkHooks). The main worktree is refused.
func (OSGit) InstallPrePush(path string) error {
	gitDir, err := git(path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	common, err := git(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	if filepath.Clean(gitDir) == filepath.Clean(common) {
		return fmt.Errorf("pre-push hook: %s is the main worktree, not a relay worktree", path)
	}
	hooks := filepath.Join(gitDir, "hooks")
	// The repo's own hooks (ADV10-11): the common config's core.hooksPath
	// (relative to the tree, as git reads it), else the common hooks
	// directory; never the worktree's own, so a second install is the same.
	repoHooks := filepath.Join(common, "hooks")
	if p, err := git(path, "config", "--local", "--get", "core.hooksPath"); err == nil && p != "" {
		if repoHooks = p; !filepath.IsAbs(p) {
			repoHooks = filepath.Join(path, p)
		}
	}
	if _, err := git(path, "config", "extensions.worktreeConfig", "true"); err != nil {
		return err
	}
	if _, err := git(path, "config", "--worktree", "core.hooksPath", hooks); err != nil {
		return err
	}
	// The runner's own core.hooksPath=/dev/null would answer here: this
	// read, which runs no hook, sees the tree's.
	args := []string{"rev-parse", "--path-format=absolute", "--git-path", "hooks"}
	got, err := output(runnerCmd(path, readArgs, args...), args)
	if err != nil {
		return err
	}
	if filepath.Clean(got) != hooks {
		return fmt.Errorf("pre-push hook: git resolves hooks to %s, not %s", got, hooks)
	}
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		return err
	}
	if err := linkHooks(repoHooks, hooks); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(hooks, "pre-push"), []byte(PrePushHook), 0o755)
}

// linkHooks links every hook in from into to beside the relay's pre-push,
// so a repo's pre-commit or commit-msg still runs in a relay worktree
// (ADV10-11). Samples and the pre-push hook are skipped; from missing is
// no hooks.
func linkHooks(from, to string) error {
	es, err := os.ReadDir(from)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range es {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, ".sample") || name == "pre-push" {
			continue
		}
		dst := filepath.Join(to, name)
		if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.Symlink(filepath.Join(from, name), dst); err != nil {
			return err
		}
	}
	return nil
}

// Exclude appends pattern to the repository's info/exclude unless a line
// already equals it.
func (OSGit) Exclude(dir, pattern string) error {
	p, err := git(dir, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == pattern {
			return nil
		}
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, append(b, pattern+"\n"...), 0o644)
}

// GitStateDigest is sha256 over the git state the child could write and
// the gate's git would read (AR20 act 0): the common dir's config,
// config.worktree, hooks/, info/ and objects/info/alternates (AR21 act 1),
// and worktree's own config.worktree
// when worktree is a linked one; info/refs, which `git update-server-info`
// writes, is not part of it (AR53 act 1). Stage records it; the gate and the merge
// recompute it first, and a change is git-state-moved. It reads files
// only, never git; a path missing is hashed as missing, so a tree that
// is not a repository has a digest too.
func GitStateDigest(root, worktree string) (string, error) {
	common, err := gitDirOf(root)
	if err != nil {
		return "", err
	}
	if common, err = commonOf(common); err != nil {
		return "", err
	}
	h := sha256.New()
	var paths [][2]string // label, path
	if common != "" {
		// objects/info/alternates (AR21 act 1, ADV12-10): a borrowed object
		// store is state the child could write, too.
		for _, n := range []string{"config", "config.worktree", "hooks", "info", "objects/info/alternates"} {
			paths = append(paths, [2]string{n, filepath.Join(common, n)})
		}
	}
	if worktree != "" {
		wt, err := gitDirOf(worktree)
		if err != nil {
			return "", err
		}
		if wt != "" && filepath.Clean(wt) != filepath.Clean(common) {
			paths = append(paths, [2]string{"worktree/config.worktree", filepath.Join(wt, "config.worktree")})
		}
	}
	entries := make([]stateEntry, 0, len(paths))
	for _, p := range paths {
		e := sha256.New()
		if err := digestTree(io.MultiWriter(h, e), p[0], p[1]); err != nil {
			return "", err
		}
		entries = append(entries, stateEntry{p[0], string(e.Sum(nil))})
	}
	sum := hex.EncodeToString(h.Sum(nil))
	stateSeen.Lock()
	if _, ok := stateSeen.m[sum]; !ok && len(stateSeen.m) < stateSeenCap {
		stateSeen.m[sum] = entries
	}
	stateSeen.Unlock()
	return sum, nil
}

// stateEntry is one entry of the git state, by its label in
// GitStateDigest's stream, and the sha256 of its own part of that stream.
type stateEntry struct{ label, sum string }

// stateSeen is each digest GitStateDigest returned in this process, with
// its entries' own sums beside it (AR53 act 2): the combined digest stays
// the comparison and the ledger's state_digest; these only name the entry
// that moved. Past stateSeenCap digests a new one is not kept, and a move
// to it is named as before AR53.
var stateSeen = struct {
	sync.Mutex
	m map[string][]stateEntry
}{m: map[string][]stateEntry{}}

const stateSeenCap = 4096

// GitStateMovedEntry is the first entry whose own part differs between
// was and now, two digests GitStateDigest returned in this process, in its
// order: config, config.worktree, hooks, info, objects/info/alternates,
// then the tree's config.worktree (AR53 act 2). It is "" when either was
// not returned here or none differs: the caller then names them all.
func GitStateMovedEntry(was, now string) string {
	stateSeen.Lock()
	a, okA := stateSeen.m[was]
	b, okB := stateSeen.m[now]
	stateSeen.Unlock()
	if !okA || !okB {
		return ""
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		var moved string
		switch {
		case i >= len(a):
			moved = b[i].label
		case i >= len(b) || a[i] != b[i]:
			moved = a[i].label
		default:
			continue
		}
		if moved == "worktree/config.worktree" {
			return "the tree's config.worktree"
		}
		return moved
	}
	return ""
}

// ProgramKeys is each program-valued key in root's local config and in
// every worktree config (the common config.worktree, each linked
// worktree's, and worktree's own) (AR21 act 1, ADV12-2): a key git would
// run as a program, or one that reroutes a remote or pulls in another
// file. Each raw file is read by `git config --file <f> --no-includes`
// under the runner, so git's own syntax and case folding apply (section
// and variable folded, subsection kept) and an include.path is seen, never
// followed. The names are as git prints them, in file order; none is
// nil. A file that does not parse is an error: the caller refuses.
func ProgramKeys(root, worktree string) ([]string, error) {
	gd, err := gitDirOf(root)
	if err != nil {
		return nil, err
	}
	common, err := commonOf(gd)
	if err != nil {
		return nil, err
	}
	var files []string
	if common != "" {
		files = append(files, filepath.Join(common, "config"), filepath.Join(common, "config.worktree"))
		linked, err := filepath.Glob(filepath.Join(common, "worktrees", "*", "config.worktree"))
		if err != nil {
			return nil, err
		}
		files = append(files, linked...)
	}
	if worktree != "" {
		wt, err := gitDirOf(worktree)
		if err != nil {
			return nil, err
		}
		if wt != "" {
			files = append(files, filepath.Join(wt, "config.worktree"))
		}
	}
	var keys []string
	read, seen := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		if f = filepath.Clean(f); read[f] {
			continue
		}
		read[f] = true
		if _, err := os.Lstat(f); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		out, err := gitStrict(filepath.Dir(f), "config", "--file", f, "--no-includes", "--name-only", "--list")
		if err != nil {
			return nil, err
		}
		for _, k := range strings.Split(out, "\n") {
			if k != "" && ProgramKey(k) && !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return keys, nil
}

// ProgramKey reports whether name matches one of ruling 1's patterns:
// filter.*, merge.*.driver, diff.*.textconv, diff.*.command (and the
// bare diff.external, which git diff and git show run), core.sshCommand, core.gitProxy, core.askPass, credential.*,
// gpg.*.program (and the bare gpg.program it replaced), include.*,
// includeIf.*, url.*.insteadOf, url.*.pushInsteadOf,
// remote.*.uploadpack, remote.*.receivepack. Section and variable match
// case-insensitively, as git reads them.
func ProgramKey(name string) bool {
	sec, rest, ok := strings.Cut(name, ".")
	if !ok {
		return false
	}
	sub, v := false, rest
	if i := strings.LastIndex(rest, "."); i >= 0 {
		sub, v = true, rest[i+1:]
	}
	sec, v = strings.ToLower(sec), strings.ToLower(v)
	switch sec {
	case "filter", "credential", "include", "includeif":
		return true
	case "merge":
		return sub && v == "driver"
	case "diff":
		return sub && (v == "textconv" || v == "command") || !sub && v == "external"
	case "core":
		return !sub && (v == "sshcommand" || v == "gitproxy" || v == "askpass")
	case "gpg":
		return v == "program"
	case "url": // insteadOf and its push twin: both end so
		return sub && strings.HasSuffix(v, "insteadof")
	case "remote":
		return sub && (v == "uploadpack" || v == "receivepack")
	}
	return false
}

// digestTree writes label's entry for path into h, and each entry under it
// when it is a directory, in lexical order: a file by mode and content, a
// symlink by its target (never followed), a missing path as missing.
func digestTree(h io.Writer, label, path string) error {
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(path, p)
		if notState(label, rel, d) {
			return nil
		}
		name := filepath.ToSlash(filepath.Join(label, rel))
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			fmt.Fprintf(h, "d %s\x00", name)
		case info.Mode()&fs.ModeSymlink != 0:
			to, err := os.Readlink(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "l %s %s\x00", name, to)
		default:
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "f %s %o %d\x00", name, info.Mode().Perm(), len(b))
			h.Write(b)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(h, "- %s\x00", label)
		return nil
	}
	return err
}

// notState reports whether the entry rel under label is left out of the
// git state (AR53 act 1): a regular file `refs` directly inside the common
// dir's info/, the listing `git update-server-info` writes for dumb-HTTP
// serving (and `git gc --auto` with it), which no local git command reads;
// and (AR53 act 4) a regular file there named `refs_` plus exactly six
// characters, the mkstemp temp file it writes and renames onto info/refs.
// A directory or a symlink named so, `refs_` of any other length, and
// every other entry, stays in.
func notState(label, rel string, d fs.DirEntry) bool {
	if label != "info" || !d.Type().IsRegular() {
		return false
	}
	if rel == "refs" {
		return true
	}
	tmp, ok := strings.CutPrefix(rel, "refs_")
	return ok && !strings.ContainsRune(tmp, filepath.Separator) && utf8.RuneCountInString(tmp) == 6
}

// gitDirOf is tree's git directory: tree/.git itself, or the directory a
// `.git` file names (a linked worktree); "" when tree has no .git.
func gitDirOf(tree string) (string, error) {
	p := filepath.Join(tree, ".git")
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil || fi.IsDir() {
		return p, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	dir, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
	if !ok {
		return "", fmt.Errorf("%s: not a gitdir file", p)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(tree, dir)
	}
	return filepath.Clean(dir), nil
}

// commonOf is the common dir a git directory's `commondir` file names,
// else the git directory itself.
func commonOf(gitDir string) (string, error) {
	if gitDir == "" {
		return "", nil
	}
	b, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if errors.Is(err, fs.ErrNotExist) {
		return gitDir, nil
	}
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(string(b))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(gitDir, dir)
	}
	return filepath.Clean(dir), nil
}
