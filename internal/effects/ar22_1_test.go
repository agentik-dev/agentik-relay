package effects

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pinnedKeys are the only variables a gate line or a runner git sees
// (AR22 act 1, ADV13-2, -3): the allowlist Pin builds, the recursion
// guard and the push deny's GIT_CONFIG_* entries; shellKeys are the ones
// sh sets itself.
var (
	pinnedKeys = map[string]bool{"PATH": true, "HOME": true, "XDG_CONFIG_HOME": true, "XDG_STATE_HOME": true,
		"XDG_DATA_HOME": true, "XDG_CACHE_HOME": true, "TMPDIR": true, "GOCACHE": true, "USER": true,
		"LOGNAME": true, "LANG": true, "GOENV": true, "GOFLAGS": true, "GOTOOLCHAIN": true, "GOPROXY": true, "GOBIN": true, "GOSUMDB": true,
		"GOMODCACHE": true, "GOROOT": true, "GOWORK": true, "PYTHONNOUSERSITE": true, "PYTHONPYCACHEPREFIX": true, "GIT_CONFIG_NOSYSTEM": true, "GIT_CONFIG_GLOBAL": true,
		"GIT_TERMINAL_PROMPT": true, "GIT_NO_REPLACE_OBJECTS": true, "GIT_SSH_COMMAND": true, MeasureDepthVar: true}
	shellKeys = map[string]bool{"PWD": true, "OLDPWD": true, "SHLVL": true, "_": true}
)

// allowed reports whether kv's key is one a pinned line may see.
func allowed(kv string) bool {
	k, _, _ := strings.Cut(kv, "=")
	return pinnedKeys[k] || shellKeys[k] || strings.HasPrefix(k, "GIT_CONFIG_KEY_") ||
		strings.HasPrefix(k, "GIT_CONFIG_VALUE_") || k == "GIT_CONFIG_COUNT"
}

// TestAR22_1_EnvIsAllowlist is AR22 act 1 (ADV13-2, -3): a marker in this
// process's env, a GIT_TRACE file and a user.name set by GIT_CONFIG_*
// never reach a gate line, a runner git or LsRemote: each sees only the
// allowlist the relay built at start, never os.Environ() minus keys.
func TestAR22_1_EnvIsAllowlist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, bare := t.TempDir(), filepath.Join(t.TempDir(), "r.git")
	for _, args := range [][]string{{"init", "-q", dir}, {"init", "-q", "--bare", bare}} {
		cmd := exec.Command("git", args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	trace := filepath.Join(t.TempDir(), "trace")
	for k, v := range map[string]string{"AR22_MARKER": "leaked", "GIT_TRACE": trace, "GIT_CONFIG_COUNT": "1",
		"GIT_CONFIG_KEY_0": "user.name", "GIT_CONFIG_VALUE_0": "leaked"} {
		t.Setenv(k, v)
	}

	for _, sh := range []OSShell{{}, {DenyPush: []string{"origin"}}} {
		exit, _, err := sh.Run(dir, "env > env.out", 10*time.Second)
		if err != nil || exit != 0 {
			t.Fatalf("env: %d %v", exit, err)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "env.out"))
		for _, kv := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if strings.Contains(kv, "leaked") || !allowed(kv) {
				k, _, _ := strings.Cut(kv, "=")
				t.Errorf("a gate line saw %s: not in the pinned allowlist", k)
			}
		}
	}
	for _, kv := range runnerCmd(dir, runnerArgs, "status").Env {
		if !allowed(kv) || strings.Contains(kv, "leaked") {
			k, _, _ := strings.Cut(kv, "=")
			t.Errorf("a runner git sees %s: not in the pinned allowlist", k)
		}
	}
	if got, _ := git(dir, "config", "--get", "user.name"); got == "leaked" {
		t.Errorf("a runner git read user.name %q from this process's GIT_CONFIG_*", got)
	}
	if _, err := (OSGit{}).LsRemote(dir, bare); err != nil {
		t.Fatalf("LsRemote: %v", err)
	}
	if _, err := os.Stat(trace); err == nil {
		t.Errorf("a runner git or LsRemote wrote this process's GIT_TRACE %s", trace)
	}
}

// TestAR22_1_UnpinnedTool is AR22 act 1 (ADV13-2): git, make, sh and ps
// come only from dirs the account cannot write. A dir the account owns,
// or a root-owned one others can write (/private/tmp), is skipped: Pin
// fails, unpinned-tool, and the process's pin is not replaced.
func TestAR22_1_UnpinnedTool(t *testing.T) {
	old, _ := Current()
	dirs := PinDirs
	t.Cleanup(func() { PinDirs = dirs; Use(old) })
	own := t.TempDir()
	for _, name := range []string{"git", "make", "sh", "ps"} {
		if err := os.WriteFile(filepath.Join(own, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range [][]string{{own}, {"/private/tmp"}, {"relative/bin"}} {
		PinDirs = d
		if _, err := PinAt(t.TempDir(), t.TempDir()); !errors.Is(err, ErrUnpinnedTool) {
			t.Errorf("PinAt from %v: %v, want unpinned-tool", d, err)
		}
		if p, _ := Current(); p.Home != old.Home {
			t.Errorf("a failed PinAt replaced the pin: %q", p.Home)
		}
	}
	PinDirs = dirs
	p, err := PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("PinAt from %v: %v", dirs, err)
	}
	defer p.Release()
	for _, f := range []string{p.Git, p.Make, p.Sh, p.Ps} {
		if !filepath.IsAbs(f) || !locked(f) {
			t.Errorf("pinned %q: not absolute in a dir the account cannot write", f)
		}
	}
}

// TestAR22_1_RelayHome is AR22 act 1: the relay home is <state>/home/<pid>
// (0700), made at start and removed by Release; a dead pid's is swept at
// the next start, a live one's kept.
func TestAR22_1_RelayHome(t *testing.T) {
	old, _ := Current()
	t.Cleanup(func() { Use(old) })
	state := t.TempDir()
	dead := exec.Command("/usr/bin/true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(state, "home", strconv.Itoa(dead.Process.Pid))
	live := filepath.Join(state, "home", strconv.Itoa(os.Getppid()))
	for _, d := range []string{gone, live} {
		os.MkdirAll(filepath.Join(d, "tmp"), 0o700)
	}
	p, err := PinAt(state, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p.Home); err != nil || fi.Mode().Perm() != 0o700 || p.Home != filepath.Join(state, "home", strconv.Itoa(os.Getpid())) {
		t.Fatalf("relay home %q: %v %v", p.Home, fi, err)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Errorf("a dead pid's relay home was not swept: %s", gone)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("a live pid's relay home was swept: %v", err)
	}
	for _, want := range []string{"HOME=" + p.Home, "TMPDIR=" + filepath.Join(p.Home, "tmp"), "GOENV=off", "GOFLAGS=-modcacherw", "GOTOOLCHAIN=local", "GOSUMDB=off"} {
		if !slices.Contains(p.Env, want) {
			t.Errorf("the pinned env lacks %s", want)
		}
	}
	if !slices.Contains(p.Env, "PATH="+strings.Join(append([]string{filepath.Join(p.Home, "bin")}, lockedDirs(PinDirs)...), ":")) {
		t.Errorf("the pinned PATH is not the relay home's bin/, then the root-owned dirs: %v", p.Env)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	// python3 links the interpreter `pyenv which python3` names under the
	// start env, never the shim ahead on PATH (nor /usr/bin/python3).
	tools := PinTools
	t.Cleanup(func() { PinTools = tools })
	bin, interp := t.TempDir(), filepath.Join(t.TempDir(), "python3.99")
	os.WriteFile(interp, []byte("#!/bin/sh\necho \"${0%/*}\"\n"), 0o755) // its roots: its own dir (AR24 act 0)
	os.WriteFile(filepath.Join(bin, "pyenv"), []byte("#!/bin/sh\n[ \"$1 $2\" = \"which python3\" ] && echo "+interp+"\n"), 0o755)
	os.WriteFile(filepath.Join(bin, "python3"), []byte("#!/bin/sh\nexec pyenv exec python3 \"$@\"\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	PinTools = []string{"python3"}
	q, err := PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer q.Release()
	want, _ := filepath.EvalSymlinks(interp)
	if to, err := os.Readlink(filepath.Join(q.Home, "bin", "python3")); err != nil || to != want || q.Tools["python3"] != want {
		t.Errorf("bin/python3 -> %q (%v), Tools %v; want the interpreter pyenv names, %q", to, err, q.Tools, want)
	}
	if _, err := os.Stat(p.Home); err == nil {
		t.Errorf("Release left the relay home %s", p.Home)
	}
}
