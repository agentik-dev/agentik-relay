package effects

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestAR29_1_ToolchainFailsClosed is AR29 act 1 (ADV17-1): the pinned
// GOPROXY is file://<cache>,off, so a golang.org/toolchain module planted
// in the account's download cache (never in any go.sum) is refused: a
// line re-exporting GOTOOLCHAIN=go1.99.0 exits non-zero naming the
// disabled checksum database and never runs the planted bin/go. The same
// line with GOPROXY=file://<proxy> re-exported (the shape before AR29)
// runs it, so the fixture is live. Offline: the proxy is a temp dir.
// Skipped without a go the pin reaches.
func TestAR29_1_ToolchainFailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { PinTools = tools; Use(old) })
	PinTools = []string{"go", "gofmt"}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	account, mod := filepath.Join(dir, "account"), filepath.Join(dir, "a")
	proxy := filepath.Join(account, "go", "pkg", "mod", "cache", "download")
	version := "v0.0.1-go1.99.0." + runtime.GOOS + "-" + runtime.GOARCH
	at := filepath.Join(proxy, "golang.org", "toolchain", "@v")
	for f, body := range map[string]string{
		filepath.Join(at, version+".info"): `{"Version":"` + version + `","Time":"2026-01-01T00:00:00Z"}` + "\n",
		filepath.Join(at, version+".mod"):  "module golang.org/toolchain\n",
		filepath.Join(mod, "go.mod"):       "module example.com/a\n\ngo 1.21\n",
	} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	zf, err := os.Create(filepath.Join(at, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(zf)
	for _, name := range []string{"bin/go", "bin/gofmt", "pkg/tool/vet", "lib/wasm/go_js_wasm_exec"} {
		w, err := z.Create("golang.org/toolchain@" + version + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("#!/bin/sh\necho PLANTED \"$@\"\n"))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}
	p, err := PinAt(filepath.Join(dir, "state"), account)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()
	if want := "GOPROXY=file://" + proxy + ",off"; !slices.Contains(p.Env, want) {
		t.Errorf("the pinned env lacks %s", want)
	}
	if _, ok := p.Tools["go"]; !ok {
		t.Skip("no go the pin reaches")
	}
	out := filepath.Join(dir, "out")
	run := func(line string) (int, string) {
		t.Helper()
		sh, done, err := Scratched(OSShell{})
		if err != nil {
			t.Fatal(err)
		}
		scratch := sh.(OSShell).Scratch
		defer func() {
			// an extracted toolchain is read-only (the switch runs before
			// GOFLAGS' -modcacherw is read): make it removable
			filepath.Walk(scratch, func(path string, info os.FileInfo, err error) error {
				if err == nil {
					os.Chmod(path, info.Mode().Perm()|0o700)
				}
				return nil
			})
			done()
		}()
		exit, _, err := sh.Run(mod, line+" >"+out+" 2>&1", 2*time.Minute)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		b, _ := os.ReadFile(out)
		return exit, string(b)
	}
	exit, text := run("GOTOOLCHAIN=go1.99.0 go version")
	if exit == 0 || strings.Contains(text, "PLANTED") || !strings.Contains(text, "checksum database disabled by GOSUMDB=off") {
		t.Errorf("GOTOOLCHAIN=go1.99.0 go version under the pin: exit %d, want non-zero naming the disabled checksum database, never PLANTED\n%s", exit, text)
	}
	t.Logf("the toolchain probe with ,off: exit %d, %s", exit, strings.TrimSpace(text))
	exit, text = run("GOPROXY=file://" + proxy + " GOTOOLCHAIN=go1.99.0 go version")
	if !strings.Contains(text, "PLANTED") {
		t.Errorf("the old shape (GOPROXY=file://<proxy>) did not run the planted toolchain, so the fixture is dead: exit %d\n%s", exit, text)
	}
	t.Logf("the toolchain probe without ,off: exit %d, %s", exit, strings.TrimSpace(text))
}

// TestAR29_3_PacketsConfig is AR29 act 3's config half: `packets` is an
// absolute dir the gate reads the packets source from; relative is an
// error; unset is "".
func TestAR29_3_PacketsConfig(t *testing.T) {
	for body, want := range map[string]string{"tmux: auto\n": "", "packets: /srv/packets\n": "/srv/packets", "packets: rel/packets\n": "error"} {
		home, repo := t.TempDir(), t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".config", "agentik", "relay"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".config", "agentik", "relay", "config.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		env := func(k string) string {
			if k == "HOME" {
				return home
			}
			return ""
		}
		c, err := LoadConfig(OSFS{}, env, repo)
		if got := c.Packets; err != nil && want != "error" || err == nil && got != want {
			t.Errorf("%q: packets %q, %v; want %s", body, got, err, want)
		}
	}
}

// TestAR29_4_LsofPinned is AR29 act 4 (ADV17-4): ExeReader names what Exe
// reads: the proc dir when its self/exe reads as a link, else the pinned
// lsof, else "" (the breaker then counts a claude by its comm alone, and
// doctor warns).
func TestAR29_4_LsofPinned(t *testing.T) {
	dir := t.TempDir()
	lsof, noProc, proc := filepath.Join(dir, "lsof"), filepath.Join(dir, "noproc"), filepath.Join(dir, "proc")
	if err := os.MkdirAll(filepath.Join(proc, "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(noProc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(lsof, filepath.Join(proc, "self", "exe")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		p    Pinned
		proc string
		want string
	}{
		{"a pinned lsof, no proc", Pinned{Lsof: lsof}, noProc, lsof},
		{"a pinned lsof, proc unset", Pinned{Lsof: lsof}, "", lsof},
		{"neither", Pinned{}, noProc, ""},
		{"a proc", Pinned{Lsof: lsof}, proc, proc},
		{"a proc, no lsof", Pinned{}, proc, proc},
	} {
		if got := ExeReader(c.p, c.proc); got != c.want {
			t.Errorf("%s: ExeReader = %q, want %q", c.name, got, c.want)
		}
	}
}
