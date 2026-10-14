package effects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// The pinned env (AR22 act 1, ADV13-2, -3). A sitting writes as the
// owner, so every account-writable dir on the start PATH and every file in
// the owner's home is the child's to write: the gate's git, make and sh
// looked up by PATH, and a verify line reading ~/.config/go/env, judged
// with the child's programs and settings. Pin, once at start before any
// spawn, resolves git, make, sh and ps to absolute paths in dirs the
// account cannot write, makes a relay home, and builds the one env every
// gate line (acceptance, verify, baseline, probe) and every runner git and
// LsRemote run under: an allowlist the relay builds, never this process's
// env minus keys. The child sitting's own env (spawn.go) is not pinned.

// PinDirs are the dirs Pin takes git, make, sh and ps from, in order, and
// the pinned PATH after the relay home's bin/; a dir the account can write (it or a parent not
// root-owned, or group- or other-writable) is skipped. A variable so a test
// can point it at a dir it owns.
var PinDirs = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}

// PinTools are the account's tools the pinned PATH reaches first, through
// the relay home's bin/ (ahead of PinDirs, so no /usr/bin/python3 shadows
// the account's): each a symlink to the real file the start env resolves
// it to (resolveTool), its sha256 bound at start and read again before
// each gate (Moved), and bin/ holds nothing else. One the start env lacks
// is absent. pyenv is not one (AR24 act 1, ADV14-1): on the pinned PATH
// a Makefile's `pyenv which python3` would pick by the account's
// PYENV_ROOT/version or a committed .python-version, never the bound
// file; without it the line falls to bin/python3. staticcheck is one
// (AR27A act 1, ADV16-5), and one of GoBinTools: a Makefile that looks
// for it where `go install` puts it finds the bound file.
var PinTools = []string{"go", "gofmt", "python3", "claude", "staticcheck"}

// ErrUnpinnedTool is Pin's error when git, make, sh or ps is in no dir
// the account cannot write, or the temp root is refused (AR31 act 1):
// cmd/agentik-relay exits 2, unpinned-tool.
var ErrUnpinnedTool = errors.New("unpinned-tool")

// pycacheOff is the pinned env's PYTHONPYCACHEPREFIX (AR49 act 1,
// design/pin-pycache-r1.md §2): a path no process can make, since
// /dev/null is not a dir, so a python under the pinned env reads no
// bytecode and writes none, in its tree or anywhere. Never
// PYTHONDONTWRITEBYTECODE: a line's env is built from the pin's, and that
// name would leave each scratch's own prefix (scratchEnv) empty.
const pycacheOff = "/dev/null/pycache"

// The short temp dir (AR31 act 1; the owner's word, 2026-09-28). A line's
// TMPDIR in the scratch home was 40 bytes past the state root, too long
// for a unix socket a test binds in os.MkdirTemp("", ...): Go, like any
// caller that fills a fixed sockaddr_un, takes a path of at most 103
// bytes. So TMPDIR alone moves, to one dir per pin under TmpBase
// resolved (Pinned.Tmp, <root>/relay-<pid>-<n>, 0700), and each scratch
// makes its own dir in it; HOME, the XDG dirs and the caches stay in the
// scratch home. The root is refused unless its worst case, tmpWorst,
// leaves TmpRoom bytes after TMPDIR/ within SunPath.

// TmpBase is the lines' temp root, resolved at pin (AR31 act 1); a var so
// a test proves a refusal.
var TmpBase = "/tmp"

const (
	SunPath = 104                       // darwin's sun_path, its NUL counted
	TmpRoom = 40                        // what a line may add after $TMPDIR/ in a socket path
	TmpMax  = SunPath - 1 - 1 - TmpRoom // 62: a TMPDIR, a slash, TmpRoom bytes and the NUL
)

// Pinned is the env Pin built: the absolute git, make, sh and ps; the
// relay home (<state>/home/<pid>, 0700); the short temp dir each scratch's
// TMPDIR is made in; the dir the kept line logs live in; the allowlist
// env; Tools, each account tool's name to
// the real file its bin/ link names; and Roots, each toolchain tree's dir
// (as the tool named it) to its tool (AR24 act 0: GOROOT, python3's
// prefixes and sys.path dirs).
type Pinned struct {
	Git, Make, Sh, Ps, Home string
	Lsof                    string // optional (AR27C act 2): "" when no pinned dir has it
	Excludes                string // <Home>/excludes, the owner's ignore copied (AR28B act 1): RootStatus's; "" when git reads none
	Tmp                     string // <root>/relay-<pid>-<n> (0700): each scratch's dir is made in it (AR31 act 1)
	Logs                    string // <state>/logs: the kept line logs (AR31 act 2); made at the first log
	Env                     []string
	Tools                   map[string]string
	Roots                   map[string]string
	sums                    map[string]string // name to the real file's sha256 at start
	trees                   []*treeSnap       // each real root's snapshot at start
	stats                   *toolStats        // each real file's stat as Moved last read it
	pycache                 bool              // the pinned python3 honors the prefix (AR49 act 2): the snapshot leaves its own cache files out
}

// toolStats is each account tool's real file's stat, as Pin and then each
// Moved read it before its hash (AR25 act 2): MovedStrict's record.
type toolStats struct {
	mu sync.Mutex
	m  map[string]statKey
}

// get is name's recorded stat; ok false with none.
func (t *toolStats) get(name string) (statKey, bool) {
	if t == nil {
		return statKey{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k, ok := t.m[name]
	return k, ok
}

// set records name's stat.
func (t *toolStats) set(name string, k statKey) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[name] = k
}

// pinned is the process's pin: the runner and OSShell read it. Nil (no
// Pin yet) is an error at every runner git and gate line, never a PATH
// lookup or os.Environ().
var pinned atomic.Pointer[Pinned]

// errNotPinned is a runner git's or a gate line's error before Pin.
var errNotPinned = errors.New("the env is not pinned (effects.Pin)")

// Current is the process's pin, if any.
func Current() (Pinned, bool) {
	if p := pinned.Load(); p != nil {
		return *p, true
	}
	return Pinned{}, false
}

// Use makes p the process's pin (Pin and PinAt call it; a test restores
// its package's pin with it).
func Use(p Pinned) { pinned.Store(&p) }

// Pin is the pin at start (cmd/agentik-relay, before any spawn): PinAt under the
// state root and the account's home, then the merge's committer read
// once (Identity), while ~/.gitconfig is still the owner's.
func Pin() (Pinned, error) {
	state := StateDir(os.Getenv)
	if !filepath.IsAbs(state) {
		return Pinned{}, fmt.Errorf("the state root %q is not absolute (set HOME or XDG_STATE_HOME)", state)
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return Pinned{}, fmt.Errorf("%w: no account home: %v", ErrUnpinnedTool, err)
	}
	p, err := PinAt(state, home)
	if err != nil {
		return p, err
	}
	Identity()
	return p, nil
}

// PinAt is Pin under state (the relay home is state/home/<pid>, a dead
// pid's swept first) and account (GOPROXY is its download cache), made
// the process's pin. Tests call it under temp dirs (RunPinned).
//
// The temp root (AR31 act 1) is judged after the tools and before
// anything is made, so a refused root makes no home; a dead pid's temp
// dir in it is swept with the homes, and the pin's own (Pinned.Tmp) is
// its last make, so a PinAt that fails makes none. One per PinAt: a
// second pin in one process never shares or removes the first's. The
// line logs (AR31 act 2) are <state>/logs, shared by every pin under the
// state root and made at the first log; each PinAt removes those older
// than LogKeep.
//
// Modules through go.sum (AR27A act 2, ADV16-5): GOPROXY is the account's
// download cache read as a file proxy (no copy, no network, GOSUMDB off),
// and GOMODCACHE is each scratch's own (scratchEnv), so every module is
// extracted fresh from its zip and checked against the main module's
// go.sum: a tree planted in the account's extracted cache is never read.
// The proxy fails closed (AR29 act 1, ADV17-1): GOPROXY is
// file://<cache>,off, never the file proxy alone, because go waives the
// checksum database for golang.org/toolchain only under a lone file://
// proxy; with ,off a toolchain (never in a go.sum) needs the database
// GOSUMDB=off disables, so a zip planted in the account's cache is refused
// ("checksum database disabled by GOSUMDB=off") even on a line that
// re-exports GOTOOLCHAIN; a module go.sum names is served as before.
// -modcacherw lets the scratch's removal take the extracted modules;
// -mod=readonly is go's default (set, it would override a vendored
// module's vendor/).
func PinAt(state, account string) (Pinned, error) {
	var p Pinned
	dirs := lockedDirs(PinDirs)
	for _, t := range []struct {
		name string
		to   *string
	}{{"git", &p.Git}, {"make", &p.Make}, {"sh", &p.Sh}, {"ps", &p.Ps}} {
		for _, d := range dirs {
			if f := filepath.Join(d, t.name); executable(f) && locked(f) {
				*t.to = f
				break
			}
		}
		if *t.to == "" {
			return Pinned{}, fmt.Errorf("%w: no %s in a dir the account cannot write (%s)", ErrUnpinnedTool, t.name, strings.Join(PinDirs, ", "))
		}
	}
	for _, d := range dirs {
		if f := filepath.Join(d, "lsof"); executable(f) && locked(f) {
			p.Lsof = f
			break
		}
	}
	root, err := tmpRoot(TmpBase)
	if err != nil {
		return Pinned{}, err
	}
	homes := filepath.Join(state, "home")
	if err := sweepHomes(homes); err != nil {
		return Pinned{}, err
	}
	sweepTmp(root)
	sweepLogs(filepath.Join(state, "logs"), time.Now())
	p.Home = filepath.Join(homes, strconv.Itoa(os.Getpid()))
	if err := os.RemoveAll(p.Home); err != nil {
		return Pinned{}, err
	}
	for _, d := range []string{"bin", "tmp", "config", "state", "data", "cache/go-build"} {
		if err := os.MkdirAll(filepath.Join(p.Home, d), 0o700); err != nil {
			return Pinned{}, err
		}
	}
	if err := os.Chmod(p.Home, 0o700); err != nil {
		return Pinned{}, err
	}
	excludes, err := pinExcludes(p.Git, p.Home)
	if err != nil {
		return Pinned{}, fmt.Errorf("%w: the owner's global ignore: %v", ErrUnpinnedTool, err)
	}
	p.Excludes = excludes
	p.Tools, p.sums = map[string]string{}, map[string]string{}
	p.stats = &toolStats{m: map[string]statKey{}}
	for _, name := range PinTools {
		real, ok := resolveTool(name)
		if !ok {
			continue // absent: the pinned PATH does not reach it
		}
		fi, err := os.Lstat(real)
		if err != nil {
			continue
		}
		sum, err := fileSum(real)
		if err != nil {
			continue
		}
		p.stats.set(name, statOf(fi))
		if err := os.Symlink(real, filepath.Join(p.Home, "bin", name)); err != nil {
			return Pinned{}, err
		}
		p.Tools[name], p.sums[name] = real, sum
	}
	user, logname := os.Getenv("USER"), os.Getenv("LOGNAME")
	if user == "" {
		user = logname
	}
	if logname == "" {
		logname = user
	}
	p.Env = []string{
		"PATH=" + strings.Join(append([]string{filepath.Join(p.Home, "bin")}, dirs...), string(os.PathListSeparator)),
		"HOME=" + p.Home,
		"XDG_CONFIG_HOME=" + filepath.Join(p.Home, "config"),
		"XDG_STATE_HOME=" + filepath.Join(p.Home, "state"),
		"XDG_DATA_HOME=" + filepath.Join(p.Home, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(p.Home, "cache"),
		"TMPDIR=" + filepath.Join(p.Home, "tmp"), // the runner's and toolRoots'; a line's is in Tmp (scratchEnv)
		"GOCACHE=" + filepath.Join(p.Home, "cache", "go-build"),
		"GOENV=off", "GOFLAGS=-modcacherw", "GOTOOLCHAIN=local", "GOWORK=off",
		"GOPROXY=file://" + filepath.Join(account, "go", "pkg", "mod", "cache", "download") + ",off", "GOSUMDB=off",
		"PYTHONNOUSERSITE=1",                // AR25 act 1 (ADV15-3): no user site's .pth runs at a gate
		"PYTHONPYCACHEPREFIX=" + pycacheOff, // AR49 act 1: the pin's asks write no bytecode; a line's is its scratch's
	}
	if user != "" {
		p.Env = append(p.Env, "USER="+user, "LOGNAME="+logname)
	}
	if lang := os.Getenv("LANG"); lang != "" {
		p.Env = append(p.Env, "LANG="+lang)
	}
	p.Env = append(p.Env, runnerEnv...)
	// The toolchains' trees (AR24 act 0), each asked of its tool under
	// the pinned env; GOROOT fixed to what the pinned go names.
	p.Roots = map[string]string{}
	for _, name := range []string{"go", "python3"} {
		if _, ok := p.Tools[name]; !ok {
			continue
		}
		roots, err := toolRoots(name, filepath.Join(p.Home, "bin", name), p.Env, filepath.Join(p.Home, "tmp"))
		if err != nil {
			return Pinned{}, fmt.Errorf("%w: %v", ErrUnpinnedTool, err)
		}
		if name == "go" {
			roots = roots[:1]
			p.Env = append(p.Env, "GOROOT="+roots[0])
		}
		for _, r := range roots {
			if _, ok := p.Roots[r]; !ok {
				p.Roots[r] = name
			}
		}
	}
	// The bytecode prefix (AR49 act 2): the pinned python3 is asked once,
	// as toolRoots asks it, whether it honors the pinned env's prefix; a
	// python that does not is no pin error, and its snapshot keeps every
	// __pycache__ in (PycacheNote, doctor's row).
	if _, ok := p.Tools["python3"]; ok {
		p.pycache = pycacheHonored(filepath.Join(p.Home, "bin", "python3"), p.Env, filepath.Join(p.Home, "tmp"))
	}
	trees, err := snapRoots(p.Roots, p.pycache)
	if err != nil {
		return Pinned{}, fmt.Errorf("%w: %v", ErrUnpinnedTool, err)
	}
	p.trees = trees
	p.Logs = filepath.Join(state, "logs") // made at the first log (AR31 act 2)
	// The last make (AR31 act 1): the pin's temp dir, set 0700 whatever
	// the umask left.
	p.Tmp, err = os.MkdirTemp(root, "relay-"+strconv.Itoa(os.Getpid())+"-")
	if err == nil {
		if err = os.Chmod(p.Tmp, 0o700); err != nil {
			os.Remove(p.Tmp)
		}
	}
	if err != nil {
		return Pinned{}, fmt.Errorf("%w: the temp root %s: %v", ErrUnpinnedTool, root, err)
	}
	Use(p)
	return p, nil
}

// RunPinned is the tests' shared pin (each package's TestMain): PinAt
// under a temp dir of its own, reaching only go and gofmt of the
// account's tools (never the owner's claude), then m.Run, then the pin
// released (its Tmp is outside the temp dir) and the temp dir removed.
func RunPinned(m interface{ Run() int }) int {
	dir, err := os.MkdirTemp("", "relay-pin-")
	if err == nil {
		dir, err = filepath.EvalSymlinks(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "RunPinned:", err)
		return 2
	}
	defer os.RemoveAll(dir)
	tools := PinTools
	PinTools = []string{"go", "gofmt"}
	p, err := PinAt(filepath.Join(dir, "state"), filepath.Join(dir, "account"))
	PinTools = tools
	if err != nil {
		fmt.Fprintln(os.Stderr, "RunPinned:", err)
		return 2
	}
	defer p.Release()
	return m.Run()
}

// Release removes the relay home, then the pin's temp dir, each when set
// (at exit; a killed relay's are swept at the next start).
func (p Pinned) Release() error {
	var errs []error
	for _, d := range []string{p.Home, p.Tmp} {
		if d != "" {
			errs = append(errs, os.RemoveAll(d))
		}
	}
	return errors.Join(errs...)
}

// PycacheNote is one line for doctor (AR49 act 2) when a python3 is pinned
// and does not honor the pinned env's PYTHONPYCACHEPREFIX, so the snapshot
// keeps every __pycache__ in; "" when it honors it or none is pinned.
func PycacheNote(p Pinned) string {
	if _, ok := p.Tools["python3"]; !ok || p.pycache {
		return ""
	}
	return "the pinned python3 does not honor PYTHONPYCACHEPREFIX: the cache files in its __pycache__ dirs stay counted, and one a python writes there is tool-moved"
}

// Moved names the account tools that are not as Pin bound them: a bin/
// link retargeted or removed, a real file whose sha256 changed, an entry
// in bin/ Pin never made, or a tool whose tree (Roots) moved: an entry
// added, removed or changed. The gate reads it before each gate: any is
// unrun, tool-moved. It records each stat it read (MovedStrict's).
func (p Pinned) Moved() []string { return p.moved(false) }

// MovedStrict is the merge's Moved (AR25 act 2, ADV15-2): it hashes
// nothing, and a file or tree entry whose stat (ctime included) differs
// from the one the gate's Moved last read (else Pin's) is moved, so a
// write restored between the gate and the merge is moved.
func (p Pinned) MovedStrict() []string { return p.moved(true) }

// moved is Moved, or MovedStrict when strict.
func (p Pinned) moved(strict bool) []string {
	var moved []string
	for _, s := range p.trees {
		if strict && s.movedStrict() || !strict && s.moved() {
			for _, t := range strings.Split(s.tool, ",") {
				if !slices.Contains(moved, t) {
					moved = append(moved, t)
				}
			}
		}
	}
	bin := filepath.Join(p.Home, "bin")
	for name, real := range p.Tools {
		if slices.Contains(moved, name) {
			continue
		}
		if to, err := os.Readlink(filepath.Join(bin, name)); err != nil || to != real {
			moved = append(moved, name)
			continue
		}
		fi, err := os.Lstat(real)
		if err != nil {
			moved = append(moved, name)
			continue
		}
		if strict {
			if want, ok := p.stats.get(name); !ok || statOf(fi) != want {
				moved = append(moved, name)
			}
			continue
		}
		if sum, err := fileSum(real); err != nil || sum != p.sums[name] {
			moved = append(moved, name)
		} else {
			p.stats.set(name, statOf(fi))
		}
	}
	if p.Home != "" {
		entries, err := os.ReadDir(bin)
		if err != nil {
			moved = append(moved, "bin/")
		}
		for _, e := range entries {
			if _, ok := p.Tools[e.Name()]; !ok && !slices.Contains(moved, e.Name()) {
				moved = append(moved, e.Name())
			}
		}
	}
	slices.Sort(moved)
	return moved
}

// resolveTool is the real file name resolves to under the start env,
// symlinks resolved: python3 is the interpreter `pyenv which python3`
// names when pyenv is on the start PATH (never pyenv's shim, which would
// loop through `pyenv exec` under the system version), else python3 on
// the start PATH; every other tool is the start PATH's. False when it
// resolves to none.
func resolveTool(name string) (string, bool) {
	found := ""
	if pyenv, err := exec.LookPath("pyenv"); name == "python3" && err == nil && filepath.IsAbs(pyenv) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, pyenv, "which", "python3").Output()
		if found = strings.TrimSpace(string(out)); err != nil || strings.Contains(found, "\n") {
			return "", false
		}
	} else if found, err = exec.LookPath(name); err != nil {
		return "", false
	}
	if !filepath.IsAbs(found) {
		return "", false
	}
	real, err := filepath.EvalSymlinks(found)
	return real, err == nil && executable(real)
}

// ToolsMoved is the process's pin's Moved; nil with no pin.
func ToolsMoved() []string {
	if p, ok := Current(); ok {
		return p.Moved()
	}
	return nil
}

// ToolsMovedStrict is the process's pin's MovedStrict (the merge's); nil
// with no pin.
func ToolsMovedStrict() []string {
	if p, ok := Current(); ok {
		return p.MovedStrict()
	}
	return nil
}

// pinnedEnv is the pin's env and git, or errNotPinned.
func pinnedEnv() (Pinned, error) {
	p, ok := Current()
	if !ok {
		return Pinned{}, errNotPinned
	}
	return p, nil
}

// lockedDirs are those of dirs the account cannot write.
func lockedDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if filepath.IsAbs(d) && locked(d) && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// locked reports whether path (symlinks resolved) and every parent is
// root-owned with no group or other write.
func locked(path string) bool {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	for p := real; ; p = filepath.Dir(p) {
		fi, err := os.Stat(p)
		if err != nil {
			return false
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || fi.Mode().Perm()&0o022 != 0 {
			return false
		}
		if p == filepath.Dir(p) {
			return true
		}
	}
}

// executable reports whether f is a regular file with an execute bit.
func executable(f string) bool {
	fi, err := os.Stat(f)
	return err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0
}

// fileSum is f's sha256, hex.
func fileSum(f string) (string, error) {
	fh, err := os.Open(f)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sweepHomes removes each relay home under homes whose pid is dead: a
// killed relay's, left behind at its exit. homes a symlink or not a dir
// is unpinned-tool, nothing swept (AR24 act 0, ADV14-10: RemoveAll through
// a link is a delete wherever it points).
func sweepHomes(homes string) error {
	if fi, err := os.Lstat(homes); err == nil && !fi.IsDir() {
		return fmt.Errorf("%w: %s is not a dir (%s)", ErrUnpinnedTool, homes, fi.Mode().Type())
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrUnpinnedTool, err)
	}
	entries, err := os.ReadDir(homes)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 || pid == os.Getpid() {
			continue
		}
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			os.RemoveAll(filepath.Join(homes, e.Name()))
		}
	}
	return nil
}

// tmpWorst is the longest TMPDIR a line can get under root: root,
// "/relay-", a pid of ten digits (the longest any pid has, so the answer
// never turns on this process's), "-", MkdirTemp's ten digits, "/", ten
// more, "/t": the root plus 41.
func tmpWorst(root string) int {
	return len(root) + len("/relay-") + 10 + len("-") + 10 + len("/") + 10 + len("/"+tmpLeaf)
}

// tmpRoot is base resolved, the root each pin's temp dir is made in (AR31
// act 1), or unpinned-tool naming why it is refused: it must be a
// root-owned dir, sticky when group or others can write it (so no other
// account renames or removes what the pin makes), whose parent is locked,
// whose worst case is at most TmpMax, and which the account can write and
// enter.
func tmpRoot(base string) (string, error) {
	refuse := func(t, why string) (string, error) {
		return "", fmt.Errorf("%w: the temp root %s: %s", ErrUnpinnedTool, t, why)
	}
	root, err := filepath.EvalSymlinks(base)
	if err != nil {
		return refuse(base, err.Error())
	}
	if !filepath.IsAbs(root) {
		return refuse(root, "not absolute")
	}
	fi, err := os.Lstat(root)
	if err != nil {
		return refuse(root, err.Error())
	}
	if !fi.IsDir() {
		return refuse(root, "not a dir")
	}
	if uid, ok := ownerOf(fi); !ok || uid != 0 {
		return refuse(root, fmt.Sprintf("not root-owned (uid %d)", uid))
	}
	if fi.Mode().Perm()&0o022 != 0 && fi.Mode()&fs.ModeSticky == 0 {
		return refuse(root, "others can write it and it is not sticky")
	}
	if parent := filepath.Dir(root); !locked(parent) {
		return refuse(root, fmt.Sprintf("its parent %s is not root-owned or others can write it", parent))
	}
	if n := tmpWorst(root); n > TmpMax {
		return refuse(root, fmt.Sprintf("a line's TMPDIR could reach %d bytes, over %d", n, TmpMax))
	}
	// W_OK|X_OK: syscall names neither on darwin
	if err := syscall.Access(root, 0x2|0x1); err != nil {
		return refuse(root, "the account cannot write or enter it: "+err.Error())
	}
	return root, nil
}

// ownerOf is fi's owner's uid; false when fi carries no Stat_t.
func ownerOf(fi fs.FileInfo) (uint32, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}

// tmpName is a pin's temp dir's name in the root: relay-<pid>-<n>.
var tmpName = regexp.MustCompile(`^relay-([1-9][0-9]{0,9})-[0-9]{1,10}$`)

// sweepTmp removes each pin's temp dir in root whose pid is dead: a killed
// relay's (AR31 act 1), as sweepHomes does the homes. Only a name of
// tmpName's shape whose pid is not this process's, read by Lstat as a dir
// (a link is never followed or removed), the account's and mode 0700; the
// root is shared with every account, so each is checked. A pid reused by a
// live process keeps its dir until that process dies. Errors are ignored.
func sweepTmp(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		m := tmpName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		pid, err := strconv.Atoi(m[1])
		if err != nil || pid > math.MaxInt32 || pid == os.Getpid() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		fi, err := os.Lstat(dir)
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			continue
		}
		if uid, ok := ownerOf(fi); !ok || int(uid) != os.Getuid() {
			continue
		}
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			os.RemoveAll(dir)
		}
	}
}

// pinExcludes copies the excludes file git would read to home/excludes
// (0600) and names the copy (AR28B act 1, run 15's decision 4): "" when git
// would read none, or the file it names is absent. The copy is the pin's,
// so an ignore edited after the start hides nothing from the root.
func pinExcludes(gitPath, home string) (string, error) {
	src, err := excludesFile(gitPath)
	if err != nil || src == "" {
		return "", err
	}
	b, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dst := filepath.Join(home, "excludes")
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		return "", err
	}
	return dst, nil
}

// excludesFile is the excludes file git would read, as git resolves it:
// core.excludesFile from the owner's global config, read as readIdentity
// reads (by the pinned git outside any repository, no system config),
// with --path; unset, $XDG_CONFIG_HOME/git/ignore, or with that unset
// $HOME/.config/git/ignore. A relative core.excludesFile names none (git
// would read it against each tree; the root's reader has one pin).
func excludesFile(gitPath string) (string, error) {
	cmd := exec.Command(gitPath, "config", "--path", "--get", "core.excludesFile")
	cmd.Dir = "/"
	cmd.Env = append(noRepoEnv(os.Environ()), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	var ee *exec.ExitError
	switch {
	case err == nil:
		if f := strings.TrimRight(string(out), "\n"); filepath.IsAbs(f) {
			return f, nil
		}
		return "", nil
	case errors.As(err, &ee) && ee.ExitCode() == 1: // unset
	default:
		return "", fmt.Errorf("git config core.excludesFile: %v", err)
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "git", "ignore"), nil
	}
	if h := os.Getenv("HOME"); h != "" {
		return filepath.Join(h, ".config", "git", "ignore"), nil
	}
	return "", nil
}

// readIdentity is `git var GIT_COMMITTER_IDENT` by the pinned git outside
// any repository, the one read that sees the owner's global config and
// env (Pin runs it at start, before any spawn).
func readIdentity() (name, email string) {
	p, err := pinnedEnv()
	if err != nil {
		return "", ""
	}
	cmd := exec.Command(p.Git, "var", "GIT_COMMITTER_IDENT")
	cmd.Dir = "/"
	cmd.Env = append(noRepoEnv(os.Environ()), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return "", ""
	}
	return ParseIdent(string(out))
}
