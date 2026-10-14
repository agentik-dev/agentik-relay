// Package effects puts every effect the relay has (git, the shell, the clock,
// files, the ledger) behind an interface with a fake (conventions §4), so the
// night's logic runs over maps in tests and over the machine in the binary.
package effects

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Git is git as the plan, the stage, the gate and the merge need it: the
// read side first, then the write side (A6's, folded in by AR4).
type Git interface {
	Toplevel(dir string) (root string, err error) // the working tree holding dir
	Root(dir string) (string, error)              // the main worktree
	Head(dir string) (string, error)              // HEAD's sha
	Branch(dir string) (string, error)            // the checked-out branch, "" when detached
	Tip(dir, branch string) (string, error)       // refs/heads/<branch>'s sha
	Status(dir string) ([]string, error)          // porcelain lines, worktrees excluded
	RootStatus(dir string) ([]string, error)      // Status under the pin's copy of the owner's ignore: the root's alone (AR28B)
	IsAncestor(dir, a, b string) (bool, error)    // git merge-base --is-ancestor a b
	// FirstParentMerged: some merge on main's first-parent line has tip
	// (a rev, resolved) as a later parent (§3's landed; never bare ancestry).
	FirstParentMerged(dir, tip, main string) (bool, error)
	BranchExists(dir, name string) (bool, error) // refs/heads/<name>
	RemoteRefs(dir string) (map[string]string, error)
	Remotes(dir string) ([]string, error) // `git remote`: every remote's name
	// RevParseCommit: ref (a full refname, `^{commit}`) as a sha; stderr is an error.
	RevParseCommit(dir, ref string) (string, error)
	LsRemote(dir, url string) (map[string]string, error) // the refs at url, read over its transport
	RemoteURLs(dir, remote string) ([]string, error)     // `git remote get-url --all`: remote's fetch URLs

	WorktreeAdd(dir, path, branch, base string) error // a new branch off base
	WorktreeAddDetached(dir, path, ref string) error
	// Clone: path a new repository holding refs fetched from dir's by full
	// refname, each at its bound sha (else ErrRefMoved), the first checked
	// out detached; the gate's and the merge's fresh tree (AR22 act 0).
	Clone(dir, path string, refs ...Ref) error
	WorktreeRemove(dir, path string) error  // never forced: a dirty worktree stays
	WorktreeDiscard(dir, path string) error // forced; only for the gate's own gate-<id>
	Worktrees(dir string) ([]Worktree, error)
	DiffNames(dir, base, branch string) ([]string, error) // three-dot, --no-renames; stderr is an error
	MergeNoCommit(dir, branch, name, email string) error  // --no-ff --no-commit, as name <email>
	MergeAbort(dir string) error
	Overlay(dir, sha string, paths ...string) error              // paths as at sha, over dir's tree and index
	Commit(dir, msg, name, email string) (sha string, err error) // as name <email>
	InstallPrePush(path string) error
	Exclude(dir, pattern string) error
	// Hermetic is one git command under the gate's runner (no system
	// config, no hooks, no fsmonitor): stdout, or an error with stderr.
	Hermetic(dir string, args ...string) (stdout string, err error)
}

// Shell runs one line with /bin/sh -c. Last is the last non-empty line of
// stdout (stderr's only when stdout has none), capped at LastCap bytes; exit is 124 on a
// timeout. Err is for a line that could not be started at all.
type Shell interface {
	Run(dir, line string, timeout time.Duration) (exit int, last string, err error)
}

// Clock tells the time, and waits on it (§15: every wait of the bounded
// night goes through After, so a test's StepClock moves it).
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// FS is the file system, as far as the relay reads and writes it.
type FS interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	Exists(path string) bool
	List(dir string) ([]string, error) // entry names, sorted
	Rename(from, to string) error      // atomic on one file system (the memo's temp file, §15)
	Remove(path string) error          // one file (AR28: the soft request)
	// WriteTemp writes data to a new file of its own in dir, named from
	// pattern as os.CreateTemp does, and returns its path: two writers never
	// share a temp name (ADV6-9).
	WriteTemp(dir, pattern string, data []byte) (string, error)
	// Lock takes an exclusive lock on path (created when missing) and
	// returns its release: flock on the machine, held across processes.
	Lock(path string) (unlock func(), err error)
}

// Ledger is the night's JSONL record (§5): Append writes one event line with
// ts and night filled in; Lines reads every line of every night on this
// machine, oldest first.
type Ledger interface {
	Append(event string, fields map[string]any) error
	Lines() ([]map[string]any, error)
}

// Effects is the bundle a verb runs over. Getenv is the environment the
// recursion guard reads (Nested); nil, as in a fake bundle, is never nested.
// Load is what the breaker reads (§15); nil never opens it. Idle is its
// second key, the idle share of the CPU; nil leaves the load alone to
// decide. Procs is what
// the watchdog samples and kills with (§15); nil runs no watchdog. Groups
// is the live process groups the night tracks (ADV6-2), shared with the
// OSShell that runs its lines; nil, the night keeps its own.
type Effects struct {
	Git
	Shell
	Clock
	FS
	Ledger
	Getenv func(string) string
	Load   Load
	Idle   Idle
	Procs  Procs
	Groups *Groups
	// Identity is the committer a merge commits as; nil is the package's
	// Identity, read once at start (AR21 act 0, ADV12-1).
	Identity func() (name, email string)
}

// MeasureDepthVar is the recursion guard (§15): set in every line OSShell
// runs and in every spawned child, so a plan, a baseline or a gate started
// by a measurement or a test measures nothing.
const MeasureDepthVar = "RELAY_MEASURE_DEPTH"

// NestedReason is the last line of every check the guard left unrun.
const NestedReason = "nested"

// Nested reports whether getenv holds MeasureDepthVar: this process runs
// inside a measurement (or a spawned row) and must not measure again.
func Nested(getenv func(string) string) bool {
	return getenv != nil && getenv(MeasureDepthVar) != ""
}

// depthEnv is environ without MeasureDepthVar, then MeasureDepthVar=1.
func depthEnv(environ []string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if !strings.HasPrefix(kv, MeasureDepthVar+"=") {
			out = append(out, kv)
		}
	}
	return append(out, MeasureDepthVar+"=1")
}

// Caps on the free text the ledger and the shell carry (§5: no message
// content, ever; the only free text is capped).
const (
	LastCap = 200 // a check's last line
	TextCap = 400 // any string field of a ledger line
)

// Timeout is an acceptance line's limit (§6), and the verify line's when
// the repo sets no verify_seconds (AR59).
const Timeout = 900 * time.Second

// MaxVerifySeconds bounds verify_seconds (AR59 act 1): a green row runs
// the verify line twice (gate, then merge), and the merge holds the root's
// lock through it, so 3,600 s holds a row's close to two hours and the
// root to one. CutExit is the exit the shell returns at its deadline.
const (
	MaxVerifySeconds = 3600
	CutExit          = 124
)

// Cut reports whether a line that exited exit after took (real time,
// time.Since: the deadline is real time, never the Clock's) was cut at
// limit (AR59 act 3): exit CutExit at or past the limit. A line that exits
// 124 sooner is a plain red.
func Cut(exit int, took, limit time.Duration) bool {
	return exit == CutExit && limit > 0 && took >= limit
}

// SuggestVerifySeconds is `doctor --verify`'s suggested verify_seconds for
// a line that took took and exited 0 (AR59 act 4): twice took, rounded up
// to a whole minute, at least 900 and at most MaxVerifySeconds.
func SuggestVerifySeconds(took time.Duration) int {
	d := 2 * took
	s := int((d+time.Minute-1)/time.Minute) * 60
	return min(max(s, int(Timeout/time.Second)), MaxVerifySeconds)
}

// StateDir is StateIn(${XDG_STATE_HOME:-~/.local/state}) (D9's shape).
func StateDir(env func(string) string) string {
	base := env("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(env("HOME"), ".local", "state")
	}
	return StateIn(base)
}

// StateIn is the relay's state dir under base: base/agentik-relay, else
// the old base/agentik-relay when only it exists (agentik-r1 §6.3 c2).
func StateIn(base string) string {
	return NewElseOld(osExists, filepath.Join(base, stateName), filepath.Join(base, oldStateName))
}

// NewElseOld is agentik-r1 §6.3's rule for a renamed path: the new one when
// it exists, else the old one when it exists, else the new. Folders move
// only in the cold window; until then readers take the new, else the old.
func NewElseOld(exists func(string) bool, nw, old string) string {
	if !exists(nw) && exists(old) {
		return old
	}
	return nw
}

// osExists reports a path on the real file system, a symlink followed.
func osExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// LedgerDir is where the ledger's day files live, under the state directory
// only (D18).
func LedgerDir(state string) string { return filepath.Join(state, "ledger") }

// capText trims s to n bytes on a rune boundary.
func capText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] >= 0xC0 {
		s = s[:len(s)-1]
	}
	return s
}
