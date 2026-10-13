package effects

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// modZip writes a module zip for example.com/b@v1.0.0 holding go.mod and
// b.go (no std import: the build compiles only the fixture).
func modZip(t *testing.T, path, src string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range map[string]string{"go.mod": "module example.com/b\n\ngo 1.21\n", "b.go": src} {
		w, err := z.Create("example.com/b@v1.0.0/" + name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestAR27A_2_ModulesThroughSum is AR27A act 2 (ADV16-5): the pinned env
// takes modules from the account's download cache as a file proxy
// (GOPROXY=file://<account>/go/pkg/mod/cache/download,off, GOSUMDB=off,
// GOFLAGS=-modcacherw) into each scratch's own GOMODCACHE, each checked
// against the main module's go.sum: a broken tree planted in the
// account's extracted cache is never read; the zip altered, the build
// names a checksum mismatch; a toolchain switch finds no network; and each
// scratch home (its extracted modules writable) is gone after. Skipped
// without a go the pin reaches.
func TestAR27A_2_ModulesThroughSum(t *testing.T) {
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
	at := filepath.Join(proxy, "example.com", "b", "@v")
	planted := filepath.Join(account, "go", "pkg", "mod", "example.com", "b@v1.0.0")
	for f, body := range map[string]string{
		filepath.Join(at, "v1.0.0.info"): `{"Version":"v1.0.0","Time":"2026-01-01T00:00:00Z"}` + "\n",
		filepath.Join(at, "v1.0.0.mod"):  "module example.com/b\n\ngo 1.21\n",
		filepath.Join(planted, "go.mod"): "module example.com/b\n\ngo 1.21\n",
		filepath.Join(planted, "b.go"):   "package b\n\nthis is not go\n",
		filepath.Join(mod, "go.mod"):     "module example.com/a\n\ngo 1.21\n\nrequire example.com/b v1.0.0\n",
		filepath.Join(mod, "a.go"):       "package a\n\nimport \"example.com/b\"\n\n// A is b's value.\nconst A = b.B\n",
	} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	modZip(t, filepath.Join(at, "v1.0.0.zip"), "package b\n\n// B is the fixture's value.\nconst B = 1\n")
	p, err := PinAt(filepath.Join(dir, "state"), account)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()
	for _, want := range []string{"GOPROXY=file://" + proxy + ",off", "GOSUMDB=off", "GOFLAGS=-modcacherw"} {
		if !slices.Contains(p.Env, want) {
			t.Errorf("the pinned env lacks %s", want)
		}
	}
	for _, kv := range p.Env {
		if strings.HasPrefix(kv, "GOMODCACHE=") || kv == "GOPROXY=off" {
			t.Errorf("the pinned env holds %s: GOMODCACHE is each scratch's, GOPROXY the account's download cache", kv)
		}
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
		exit, _, err := sh.Run(mod, line+" >"+out+" 2>&1", 2*time.Minute)
		done()
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if _, err := os.Stat(scratch); !os.IsNotExist(err) {
			t.Errorf("%s: the scratch home %s outlived its func (its modules read-only?): %v", line, scratch, err)
		}
		b, _ := os.ReadFile(out)
		return exit, string(b)
	}
	if exit, text := run("go env GOMODCACHE"); exit != 0 || !strings.HasPrefix(strings.TrimSpace(text), filepath.Join(p.Home, "cache", "scratch-")) ||
		filepath.Base(strings.TrimSpace(text)) != "gomod" {
		t.Errorf("go env GOMODCACHE: %q (exit %d), want <scratch>/gomod", text, exit)
	}
	if exit, text := run("go mod tidy"); exit != 0 {
		t.Fatalf("go mod tidy over the fixture proxy: exit %d\n%s", exit, text)
	}
	if b, _ := os.ReadFile(filepath.Join(mod, "go.sum")); !strings.Contains(string(b), "example.com/b v1.0.0 h1:") {
		t.Fatalf("go.sum %q: want example.com/b v1.0.0's zip hash", b)
	}
	start := time.Now()
	if exit, text := run("go build ./..."); exit != 0 {
		t.Fatalf("go build ./... read the account's extracted tree, not the zip: exit %d\n%s", exit, text)
	}
	t.Logf("the fixture build: %.1f s", time.Since(start).Seconds())
	modZip(t, filepath.Join(at, "v1.0.0.zip"), "package b\n\n// B is the altered value.\nconst B = 2\n")
	if exit, text := run("go build ./..."); exit == 0 || !strings.Contains(text, "checksum mismatch") {
		t.Errorf("go build ./... over an altered zip: exit %d, want non-zero naming a checksum mismatch\n%s", exit, text)
	}
	start = time.Now()
	exit, text := run("GOTOOLCHAIN=go1.99.0 go version")
	if exit == 0 || strings.Contains(text, "proxy.golang.org") || time.Since(start) > time.Minute {
		t.Errorf("GOTOOLCHAIN=go1.99.0 go version: exit %d in %s, want non-zero with no network\n%s", exit, time.Since(start), text)
	}
	t.Logf("the toolchain probe: exit %d, %s", exit, strings.TrimSpace(text))
}
