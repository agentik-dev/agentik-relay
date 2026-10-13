package effects

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The toolchains' trees (AR24 act 0, ADV14-1, -2). A pinned tool's file
// is the least of what it runs: go compiles GOROOT's src and runs its
// pkg/tool, python3 imports from its prefixes and every sys.path dir (a
// .pth or sitecustomize.py there runs at every start). PinAt asks each
// tool for its roots under the pinned env, as a gate line runs it, and
// snapshots each root: every entry by Lstat (no symlink followed, a
// link's target recorded), each regular file's sha256 with its (dev, ino,
// size, mode, ctime), stat before and after the hash. Moved walks each
// root again and re-hashes only a file whose stat differs (the account
// cannot set ctime back), so a gate pays a walk, not a hash. It records
// each stat it read; the merge's MovedStrict hashes nothing and reads any
// stat changed since that record as moved (AR25 act 2, ADV15-2): a write
// restored between the gate and the merge is moved, though its bytes
// are equal.
//
// Python's own cache files are no entries (AR49 act 3,
// design/pin-pycache-r1.md §2), under a python3 that honors the pinned
// env's bytecode prefix: pyOwn, which the snapshot and both walks ask.

// treeEntry is one entry of a root: its mode, a symlink's target, a
// regular file's sha256 and the stat that sum was read under.
type treeEntry struct {
	mode fs.FileMode
	link string
	sum  string
	stat statKey
}

// treeSnap is one root's snapshot: the tool it is bound to, the paths the
// tool named that resolve to it (each resolved again in Moved), the real
// dir walked, and each entry by its path relative to it ("." the root);
// read is each entry's stat as the last moved read it (under mu); own is
// whether python's own cache entries are left out of it (pyOwn).
type treeSnap struct {
	tool    string
	named   []string
	real    string
	entries map[string]treeEntry
	own     bool
	mu      sync.Mutex
	read    map[string]statKey
}

// treeHashes counts the files hashed by snapshots and walks (a test reads
// it: an unchanged tree is walked, not hashed).
var treeHashes atomic.Int64

// pyRoots is the script PinAt hands the pinned python3: its prefixes and
// every sys.path entry, one a line.
const pyRoots = "import sys\nfor d in [sys.prefix, sys.base_prefix, sys.exec_prefix] + sys.path: print(d)\n"

// pyPrefix is the script PinAt hands the pinned python3 after pyRoots
// (AR49 act 2): the bytecode prefix it reads.
const pyPrefix = "import sys\nprint(sys.pycache_prefix)\n"

// askTimeout and askWait bound each ask of a pinned tool (toolRoots,
// pycacheHonored; AR50 act 1). The tool is killed at askTimeout; askWait
// after it has exited or been killed, its output is closed whoever still
// holds it (cmd.WaitDelay) and the ask is an error, so a child the tool
// left holding stdout cannot hold the pin. Variables, since a test
// shortens and restores them.
var (
	askTimeout = 30 * time.Second
	askWait    = 2 * time.Second
)

// pycacheHonored reports whether the python3 at link, asked once as
// toolRoots asks it (under env, the pinned one, in dir), names the pinned
// env's prefix (pycacheOff) as its sys.pycache_prefix: that answer alone,
// on one line. Any other answer (a python before 3.8, or one that drops
// the variable), an error, a timeout or an output still held askWait
// after the exit is not honored, and no pin error.
func pycacheHonored(link string, env []string, dir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), askTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, link, "-c", pyPrefix)
	cmd.Env, cmd.Dir = env, dir
	cmd.WaitDelay = askWait
	out, err := cmd.Output()
	return err == nil && strings.TrimSuffix(string(out), "\n") == pycacheOff
}

// toolRoots are the dirs tool (by its bin/ link, under env, in dir)
// names as its trees: go's `go env GOROOT`; python3's prefixes and each
// sys.path dir that exists. An error when the tool names none it can be
// asked for, when it outlives askTimeout, or when its output is still
// held askWait after its exit (its roots are then unread); either of the
// last two says it timed out and names its bound.
func toolRoots(name, link string, env []string, dir string) ([]string, error) {
	var args []string
	switch name {
	case "go":
		args = []string{"env", "GOROOT"}
	case "python3":
		args = []string{"-c", pyRoots}
	default:
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), askTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, link, args...)
	cmd.Env, cmd.Dir = env, dir
	cmd.WaitDelay = askWait
	out, err := cmd.Output()
	if err != nil {
		switch {
		case errors.Is(err, exec.ErrWaitDelay):
			err = fmt.Errorf("timed out, its output still held %s after its exit: %w", askWait, err)
		case ctx.Err() != nil:
			err = fmt.Errorf("timed out after %s: %w", askTimeout, err)
		}
		return nil, fmt.Errorf("%s %s: %v", name, strings.Join(args[:1], " "), err)
	}
	var roots []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); !filepath.IsAbs(l) {
			continue // python's "" (the cwd)
		}
		if fi, err := os.Stat(l); err == nil && fi.IsDir() {
			roots = append(roots, filepath.Clean(l))
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("%s names no root", name)
	}
	return roots, nil
}

// pyCacheDir is the dir python keeps a source's bytecode in, beside it.
const pyCacheDir = "__pycache__"

// pyOwn reports whether path, which a walk of s met as d, is python's own
// cache entry and so no entry of the snapshot (AR49 act 3): a real dir
// named __pycache__ whose parent is not named so, or a regular file
// directly in such a dir whose name carries a cache tag (cacheTagged).
// Only when own: a snapshot whose tool is python3 alone (one shared with
// go is walked whole), pinned by a python3 that honors the prefix. The
// root and a file directly in it are always entries. Everything else in a
// __pycache__ is an entry as anywhere (a .py, a .so, a .pyc of any other
// name, a subdir, a symlink: a __pycache__ is an importable namespace
// package), so a walk still descends into one. It reads d's type, never a
// stat.
func (s *treeSnap) pyOwn(path string, d fs.DirEntry) bool {
	if !s.own || path == s.real {
		return false
	}
	dir := filepath.Dir(path)
	name, parent := filepath.Base(path), filepath.Base(dir)
	switch {
	case d.IsDir():
		return name == pyCacheDir && parent != pyCacheDir
	case d.Type().IsRegular():
		return dir != s.real && parent == pyCacheDir && filepath.Base(filepath.Dir(dir)) != pyCacheDir && cacheTagged(name)
	}
	return false
}

// cacheTagged reports whether name is <name>.cpython-<digits>.pyc or
// <name>.cpython-<digits>.opt-<digits>.pyc, no part empty: the names
// python gives its own cache files, none importable by its name.
func cacheTagged(name string) bool {
	rest, ok := strings.CutSuffix(name, ".pyc")
	if !ok {
		return false
	}
	if i := strings.LastIndex(rest, ".opt-"); i >= 0 && digits(rest[i+len(".opt-"):]) {
		rest = rest[:i]
	}
	i := strings.LastIndex(rest, ".cpython-")
	return i > 0 && digits(rest[i+len(".cpython-"):])
}

// digits reports whether s is one or more ASCII digits.
func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// snapRoots snapshots roots (each named dir to its tool): each resolved
// to its real dir, a dir nested in another's dropped (its entries are the
// outer's), then each walked and hashed. With own (the pinned python3
// honors the prefix), a snapshot bound to python3 alone leaves python's
// own cache entries out (pyOwn). An error when a root does not resolve or
// a file changes under its hash.
func snapRoots(roots map[string]string, own bool) ([]*treeSnap, error) {
	byReal := map[string]*treeSnap{}
	for named, tool := range roots {
		real, err := filepath.EvalSymlinks(named)
		if err != nil {
			return nil, err
		}
		if s := byReal[real]; s != nil {
			s.named = append(s.named, named)
			if !strings.Contains(","+s.tool+",", ","+tool+",") {
				s.tool += "," + tool
			}
			continue
		}
		byReal[real] = &treeSnap{tool: tool, named: []string{named}, real: real}
	}
	reals := make([]string, 0, len(byReal))
	for r := range byReal {
		reals = append(reals, r)
	}
	slices.Sort(reals)
	var snaps []*treeSnap
	for _, r := range reals {
		s := byReal[r]
		i := slices.IndexFunc(snaps, func(o *treeSnap) bool { return within(o.real, r) })
		if i < 0 { // sorted: an outer root precedes its nested ones
			snaps = append(snaps, s)
			continue
		}
		snaps[i].named = append(snaps[i].named, s.named...)
		for _, t := range strings.Split(s.tool, ",") {
			if !strings.Contains(","+snaps[i].tool+",", ","+t+",") {
				snaps[i].tool += "," + t
			}
		}
	}
	for _, s := range snaps {
		s.own = own && s.tool == "python3"
		s.entries = map[string]treeEntry{}
		err := filepath.WalkDir(s.real, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if s.pyOwn(path, d) {
				return nil
			}
			e, err := readEntry(path, nil)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(s.real, path)
			s.entries[rel] = e
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("%s's tree %s: %w", s.tool, s.real, err)
		}
	}
	return snaps, nil
}

// within reports whether path is dir or under it.
func within(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// readEntry is path's entry by Lstat: a symlink's target, a regular
// file's sha256 read between two equal stats. With was (the snapshot's
// entry) a regular file whose stat is unchanged keeps was's sum unread.
func readEntry(path string, was *treeEntry) (treeEntry, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return treeEntry{}, err
	}
	e := treeEntry{mode: fi.Mode(), stat: statOf(fi)}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		e.link, err = os.Readlink(path)
	case fi.Mode().IsRegular():
		if was != nil && was.mode == e.mode && was.stat == e.stat {
			e.sum = was.sum
			return e, nil
		}
		treeHashes.Add(1)
		if e.sum, err = fileSum(path); err != nil {
			return e, err
		}
		after, err := os.Lstat(path)
		if err != nil {
			return e, err
		}
		if statOf(after) != e.stat || after.Mode() != e.mode {
			return e, fmt.Errorf("%s changed under its hash", path)
		}
	}
	return e, err
}

// moved reports whether the snapshot's tree is not as bound: a named path
// resolving elsewhere, or an entry added, removed or changed (a regular
// file's bytes by its sha256, read again only when its stat differs).
func (s *treeSnap) moved() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.namedMoved() {
		return true
	}
	if s.read == nil {
		s.read = map[string]statKey{}
	}
	seen := 0
	err := filepath.WalkDir(s.real, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if s.pyOwn(path, d) {
			return nil // python's own cache entry: none of the snapshot's
		}
		rel, _ := filepath.Rel(s.real, path)
		was, ok := s.entries[rel]
		if !ok {
			return fs.ErrExist // added
		}
		seen++
		e, err := readEntry(path, &was)
		if err != nil {
			return err
		}
		if e.mode != was.mode || e.link != was.link || e.sum != was.sum {
			return fs.ErrInvalid // changed
		}
		s.read[rel] = e.stat
		return nil
	})
	return err != nil || seen != len(s.entries)
}

// namedMoved reports whether a path the tool named resolves out of the
// tree.
func (s *treeSnap) namedMoved() bool {
	for _, n := range s.named {
		if real, err := filepath.EvalSymlinks(n); err != nil || !within(s.real, real) {
			return true
		}
	}
	return false
}

// movedStrict is the merge's moved (AR25 act 2, ADV15-2): no hash; an
// entry added or removed, or whose mode, link or stat (ctime included)
// differs from the stat the last moved read (else the snapshot's), is
// moved.
func (s *treeSnap) movedStrict() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.namedMoved() {
		return true
	}
	seen := 0
	err := filepath.WalkDir(s.real, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if s.pyOwn(path, d) {
			return nil // python's own cache entry: none of the snapshot's
		}
		rel, _ := filepath.Rel(s.real, path)
		was, ok := s.entries[rel]
		if !ok {
			return fs.ErrExist // added
		}
		seen++
		fi, err := os.Lstat(path)
		if err != nil {
			return err
		}
		want, ok := s.read[rel]
		if !ok {
			want = was.stat
		}
		if fi.Mode() != was.mode || statOf(fi) != want {
			return fs.ErrInvalid // changed since the gate read it
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			if link, err := os.Readlink(path); err != nil || link != was.link {
				return fs.ErrInvalid
			}
		}
		return nil
	})
	return err != nil || seen != len(s.entries)
}
