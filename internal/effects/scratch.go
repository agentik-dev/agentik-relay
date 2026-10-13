package effects

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The scratch home (AR25 act 1; ADV15-3). A line's writable names (HOME,
// the XDG dirs, TMPDIR and GOCACHE) never point into the relay home's
// own dirs, which live the whole night: each gate, each merge verify and
// each baseline runs its lines under one fresh dir of its own
// (os.MkdirTemp in <home>/cache, removed at its end), and any other line
// under one of its own. Fresh, never a snapshot: no walk of an unbounded
// cache. So nothing a line writes (a go-build entry, go's telemetry under
// Library, a user site, a test's state) reaches a later gate's line; the
// cold gate pays for it (157 s for this repo's make check, measured).
// Only the relay home's bin/ outlives a line, and Moved reads it.
//
// GOBIN is the scratch's own bin/ (AR27A act 1, ADV16-5), holding a link
// to each of GoBinTools the pin bound (<home>/bin/<name>): a line's `go
// install` lands there and goes with the scratch, never moving the relay
// home's bin/ (which would stop the night tool-moved). GOMODCACHE is the
// scratch's own gomod/ (AR27A act 2): modules extracted fresh from the
// account's download cache, checked against go.sum (Pin).
//
// TMPDIR is outside the scratch home (AR31 act 1): each scratch makes a
// fresh dir of its own in the pin's short temp dir (Pinned.Tmp), and its
// t/ is the line's TMPDIR, so a socket a line binds in it fits (TmpMax);
// the macOS idiom "${TMPDIR}name", which assumes a trailing slash, lands
// in that dir and goes with it. The pin's temp dir is read by Lstat at
// each scratch (a dir, never a link, the account's, 0700), since the
// shared root is not the relay's alone; a scratch that cannot have its
// temp dir fails, and no line gets the long path back. Removed with the
// scratch home, the temp dir first.

// GoBinTools are the bound tools (PinTools) a line finds in GOBIN, where
// `go install` would put them.
var GoBinTools = []string{"staticcheck"}

// scratchDirs are the dirs a scratch home holds, each named by scratchEnv.
var scratchDirs = []string{"config", "state", "data", "cache", "go-build", "bin", "gomod", "pycache"}

// tmpLeaf is the line's TMPDIR in its scratch's temp dir: <tmp>/t.
const tmpLeaf = "t"

// newScratch makes a fresh temp dir in the pin's Tmp, then a fresh scratch
// home under the pin's <home>/cache. Any error removes what it made.
func newScratch() (home, tmp string, err error) {
	p, err := pinnedEnv()
	if err != nil {
		return "", "", err
	}
	if p.Tmp == "" {
		return "", "", errors.New("the pin has no temp dir (effects.PinAt)")
	}
	if why := tmpOurs(p.Tmp); why != "" {
		return "", "", fmt.Errorf("the pin's temp dir %s: %s", p.Tmp, why)
	}
	defer func() {
		if err != nil {
			removeScratch(home, tmp)
			home, tmp = "", ""
		}
	}()
	if tmp, err = os.MkdirTemp(p.Tmp, ""); err != nil {
		return
	}
	leaf := filepath.Join(tmp, tmpLeaf)
	if err = os.Mkdir(leaf, 0o700); err != nil {
		return
	}
	if len(leaf) > TmpMax {
		err = fmt.Errorf("the pin's temp dir %s: a TMPDIR of %d bytes, over %d", p.Tmp, len(leaf), TmpMax)
		return
	}
	if home, err = os.MkdirTemp(filepath.Join(p.Home, "cache"), "scratch-"); err != nil {
		return
	}
	for _, d := range scratchDirs {
		if err = os.Mkdir(filepath.Join(home, d), 0o700); err != nil {
			return
		}
	}
	for _, name := range GoBinTools {
		if _, ok := p.Tools[name]; !ok {
			continue
		}
		if err = os.Symlink(filepath.Join(p.Home, "bin", name), filepath.Join(home, "bin", name)); err != nil {
			return
		}
	}
	return home, tmp, nil
}

// tmpOurs is why dir is not a pin's temp dir as the pin made it (a dir,
// never a link, the account's, mode 0700), or "".
func tmpOurs(dir string) string {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err.Error()
	}
	if !fi.IsDir() {
		return "not a dir"
	}
	if uid, ok := ownerOf(fi); !ok || int(uid) != os.Getuid() {
		return fmt.Sprintf("not the account's (uid %d)", uid)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		return fmt.Sprintf("mode %#o, not 0700", perm)
	}
	return ""
}

// removeScratch removes a scratch's temp dir, then its home, each when set.
func removeScratch(home, tmp string) {
	if tmp != "" {
		os.RemoveAll(tmp)
	}
	if home != "" {
		os.RemoveAll(home)
	}
}

// scratchEnv is environ with its writable names pointed into home, and
// TMPDIR at tmp's t/. PYTHONPYCACHEPREFIX is the scratch's own pycache
// (AR49 act 1): a line's python reads and writes its bytecode there, in a
// mirror tree, and never beside the source in the tool's tree; the one
// environ carries (the pin's, which no process can make) is dropped.
func scratchEnv(environ []string, home, tmp string) []string {
	set := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"TMPDIR=" + filepath.Join(tmp, tmpLeaf),
		"GOCACHE=" + filepath.Join(home, "go-build"),
		"GOBIN=" + filepath.Join(home, "bin"),
		"GOMODCACHE=" + filepath.Join(home, "gomod"),
		"PYTHONPYCACHEPREFIX=" + filepath.Join(home, "pycache"),
	}
	out := make([]string, 0, len(environ)+len(set))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "TMPDIR", "GOCACHE", "GOBIN", "GOMODCACHE", "PYTHONPYCACHEPREFIX":
			continue
		}
		out = append(out, kv)
	}
	return append(out, set...)
}

// Scratched is sh with every line it runs under one fresh scratch home and
// temp dir (Scratch and Tmp), and the func that removes them, the temp dir
// first (a gate's, a merge verify's, a baseline's). Any other Shell (a
// test's fake), or an OSShell already scratched, is returned as it is,
// with a no-op.
func Scratched(sh Shell) (Shell, func(), error) {
	o, ok := sh.(OSShell)
	if !ok || o.Scratch != "" {
		return sh, func() {}, nil
	}
	home, tmp, err := newScratch()
	if err != nil {
		return sh, func() {}, err
	}
	o.Scratch, o.Tmp = home, tmp
	return o, func() { removeScratch(home, tmp) }, nil
}
