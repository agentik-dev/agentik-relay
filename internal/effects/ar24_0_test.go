package effects

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeTool writes name as a script in a fresh dir, puts that dir and the
// root-owned dirs alone on PATH (no pyenv ahead), makes it PinTools'
// only entry, and restores the pin and PinTools at cleanup.
func fakeTool(t *testing.T, name, body string) {
	t.Helper()
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { Use(old); PinTools = tools })
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	PinTools = []string{name}
}

// tree writes files (a path to its bytes) under a fresh dir, returned.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for f, b := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755)
		if err := os.WriteFile(filepath.Join(root, f), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestAR24_0_PythonRoots is ADV14-1's sitecustomize.py and a .pth: the
// pinned python3's roots are its prefixes and each sys.path dir it
// prints under the pinned env (the pinned HOME, `-c`), one outside the
// prefix among them (Homebrew's site-packages); a .pth added there, or a
// sitecustomize.py in the prefix, and Moved names python3.
func TestAR24_0_PythonRoots(t *testing.T) {
	prefix := tree(t, map[string]string{"lib/python3.12/os.py": "# os\n", "bin/x": "x"})
	site := tree(t, map[string]string{"six.py": "# six\n"})
	seen := filepath.Join(t.TempDir(), "seen")
	for _, c := range []struct{ name, add string }{
		{"a .pth outside the prefix", filepath.Join(site, "evil.pth")},
		{"sitecustomize.py in the prefix", filepath.Join(prefix, "lib/python3.12/sitecustomize.py")},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeTool(t, "python3", "echo \"$HOME $1\" > "+seen+"\nfor d in "+prefix+" "+prefix+" "+prefix+
				" '' "+prefix+"/lib/python3.12.zip "+prefix+"/lib/python3.12 "+site+"; do echo \"$d\"; done")
			p, err := PinAt(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { p.Release() })
			if b, _ := os.ReadFile(seen); string(b) != p.Home+" -c\n" {
				t.Fatalf("python3 was asked %q, want the pinned HOME and -c", b)
			}
			for _, r := range []string{prefix, prefix + "/lib/python3.12", site} {
				if p.Roots[r] != "python3" {
					t.Fatalf("Roots = %v, want %s bound to python3", p.Roots, r)
				}
			}
			if len(p.Roots) != 3 || len(p.trees) != 2 {
				t.Fatalf("Roots %v, %d trees: want 3 named, the nested one deduped to 2", p.Roots, len(p.trees))
			}
			if moved := p.Moved(); len(moved) != 0 {
				t.Fatalf("Moved as pinned = %v", moved)
			}
			if err := os.WriteFile(c.add, []byte("import os\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Remove(c.add) })
			if moved := p.Moved(); !slices.Equal(moved, []string{"python3"}) {
				t.Fatalf("Moved after %s = %v, want [python3]", c.add, moved)
			}
		})
	}
}

// TestAR24_0_StatFastPath is AR24 act 0's walk: an unchanged tree is
// walked, not hashed; a chmod and back changes the ctime, so the file is
// hashed again and not moved; new bytes of the same size with the mtime
// set back still change the ctime: moved.
func TestAR24_0_StatFastPath(t *testing.T) {
	root := tree(t, map[string]string{"src/testing/testing.go": "package testing // Fail marks\n", "pkg/tool/compile": "c"})
	fakeTool(t, "go", "[ \"$1 $2\" = \"env GOROOT\" ] && echo "+root)
	p, err := PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })
	if !slices.Contains(p.Env, "GOROOT="+root) || p.Roots[root] != "go" {
		t.Fatalf("Env %v, Roots %v: want GOROOT=%s, bound to go", p.Env, p.Roots, root)
	}
	f := filepath.Join(root, "src/testing/testing.go")
	before := treeHashes.Load()
	if moved := p.Moved(); len(moved) != 0 || treeHashes.Load() != before {
		t.Fatalf("Moved as pinned = %v, %d files hashed, want none", moved, treeHashes.Load()-before)
	}
	time.Sleep(10 * time.Millisecond)
	os.Chmod(f, 0o600)
	os.Chmod(f, 0o644)
	if moved := p.Moved(); len(moved) != 0 || treeHashes.Load() != before+1 {
		t.Fatalf("Moved after a chmod and back = %v, %d files hashed, want none moved and 1 hashed", moved, treeHashes.Load()-before)
	}
	fi, _ := os.Stat(f)
	if err := os.WriteFile(f, []byte("package testing // Fail skips\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(f, fi.ModTime(), fi.ModTime())
	if after, _ := os.Stat(f); after.Size() != fi.Size() || !after.ModTime().Equal(fi.ModTime()) {
		t.Fatalf("the rewrite changed the size or mtime")
	}
	if moved := p.Moved(); !slices.Equal(moved, []string{"go"}) {
		t.Fatalf("Moved after new bytes, mtime restored = %v, want [go]", moved)
	}
}

// TestAR24_0_HomeNotSymlink is ADV14-10's sweep: <state>/home a symlink
// (to a dir holding a dead pid's home) or a file is unpinned-tool, and
// nothing is swept through it.
func TestAR24_0_HomeNotSymlink(t *testing.T) {
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { Use(old); PinTools = tools })
	PinTools = nil
	victim := tree(t, map[string]string{"99999999/keep": "k"})
	link := t.TempDir()
	os.Symlink(victim, filepath.Join(link, "home"))
	file := t.TempDir()
	os.WriteFile(filepath.Join(file, "home"), nil, 0o644)
	for name, state := range map[string]string{"symlink": link, "file": file} {
		if _, err := PinAt(state, t.TempDir()); !errors.Is(err, ErrUnpinnedTool) || !strings.Contains(err.Error(), "not a dir") {
			t.Fatalf("PinAt with <state>/home a %s = %v, want unpinned-tool", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(victim, "99999999/keep")); err != nil {
		t.Fatalf("the sweep went through the link: %v", err)
	}
}
