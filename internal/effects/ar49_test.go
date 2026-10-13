package effects

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestAR49_1_ScratchPrefix is AR49 act 1 (design/pin-pycache-r1.md §2),
// with the real python3 and a fake root (a temp dir that holds victim.py,
// the only module a python here imports besides sys): the pinned env
// names a bytecode prefix no process can make and never
// PYTHONDONTWRITEBYTECODE; a line's python that imports victim leaves no
// __pycache__ in the root; under Scratched a second line finds the first
// line's victim.cpython-*.pyc under its $PYTHONPYCACHEPREFIX, in the
// scratch home, gone with it; and a python run as toolRoots runs one (the
// bin/ link, the pinned env) writes no file under the root. Skipped
// without a python3 the pin reaches.
func TestAR49_1_ScratchPrefix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old, _ := Current()
	tools := PinTools
	t.Cleanup(func() { PinTools = tools; Use(old) })
	PinTools = []string{"python3"}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := PinAt(filepath.Join(dir, "state"), filepath.Join(dir, "account"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()
	if !slices.Contains(p.Env, "PYTHONPYCACHEPREFIX=/dev/null/pycache") {
		t.Errorf("the pinned env has no PYTHONPYCACHEPREFIX=/dev/null/pycache: %q", p.Env)
	}
	for _, kv := range p.Env {
		if strings.HasPrefix(kv, "PYTHONDONTWRITEBYTECODE=") {
			t.Errorf("the pinned env holds %s: it would leave each scratch's prefix empty", kv)
		}
	}
	if _, ok := p.Tools["python3"]; !ok {
		t.Skip("no python3 the pin reaches")
	}
	if note := PycacheNote(p); note != "" { // act 2: the real python3 (3.8 or later) honors the prefix
		t.Errorf("the real python3 does not answer the pin's ask with the prefix: %s", note)
	}
	root := func() string { return tree(t, map[string]string{"victim.py": "x = 1\n"}) }
	alone := func(what, root string) {
		t.Helper()
		var names []string
		filepath.WalkDir(root, func(path string, _ os.DirEntry, _ error) error {
			if path != root {
				names = append(names, strings.TrimPrefix(path, root+"/"))
			}
			return nil
		})
		if !slices.Equal(names, []string{"victim.py"}) {
			t.Errorf("%s left %q under the root, want victim.py alone", what, names)
		}
	}
	const imp = `python3 -c 'import sys; sys.path.insert(0, sys.argv[1]); import victim; print(victim.x)' `

	r := root()
	if exit, last, err := (OSShell{}).Run(dir, imp+r, time.Minute); err != nil || exit != 0 || last != "1" {
		t.Fatalf("a line's import of victim: %q (exit %d, %v)", last, exit, err)
	}
	alone("a line's python", r)

	scratchedPrefix(t, dir)

	r = root()
	cmd := exec.Command(filepath.Join(p.Home, "bin", "python3"), "-c", "import sys\nsys.path.insert(0, sys.argv[1])\nimport victim\nprint(victim.x)\n", r)
	cmd.Env, cmd.Dir = p.Env, filepath.Join(p.Home, "tmp")
	if out, err := cmd.CombinedOutput(); err != nil || string(out) != "1\n" {
		t.Fatalf("the pin's own python: %q, %v", out, err)
	}
	alone("the pin's own python", r)
}

// scratchedPrefix is the scratched part of TestAR49_1_ScratchPrefix (AR50
// act 2), under a pin of the real python3 and from dir: under Scratched,
// a line's python imports victim from a fresh root and leaves it alone
// there, and a second line finds the first's victim.cpython-*.pyc under
// its $PYTHONPYCACHEPREFIX, in the scratch home's pycache dir, and it and
// that dir are gone with the scratch.
func scratchedPrefix(t *testing.T, dir string) {
	t.Helper()
	r := tree(t, map[string]string{"victim.py": "x = 1\n"})
	sh, remove, err := Scratched(OSShell{})
	if err != nil {
		t.Fatal(err)
	}
	defer remove()
	want := filepath.Join(sh.(OSShell).Scratch, "pycache")
	if exit, last, err := sh.Run(dir, `python3 -c 'import sys; sys.path.insert(0, sys.argv[1]); import victim; print(victim.x)' `+r, time.Minute); err != nil || exit != 0 || last != "1" {
		t.Fatalf("a scratched line's import of victim: %q (exit %d, %v)", last, exit, err)
	}
	// Python mirrors the root's absolute path under the prefix, so the
	// file's path there holds the temp dir twice over and its path
	// relative to the prefix holds it once: under a long TMPDIR either
	// outruns a last line's cap (LastCap). The line cuts the prefix and
	// then the root from what it found, so what it prints is short
	// whatever the temp dir, and the path is joined again here.
	exit, name, err := sh.Run(dir, `f=$(find "$PYTHONPYCACHEPREFIX" -type f -name 'victim.cpython-*.pyc') && f=${f#"$PYTHONPYCACHEPREFIX"} && f=${f#'`+r+`'/} && echo "$f"`, time.Minute)
	if ok, _ := filepath.Match("victim.cpython-*.pyc", name); err != nil || exit != 0 || !ok {
		t.Fatalf("the second line found %q under its $PYTHONPYCACHEPREFIX, less the prefix and the root %s (exit %d, %v), want victim.cpython-*.pyc alone", name, r, exit, err)
	}
	rel := filepath.Join(r, name)[1:]
	pyc := filepath.Join(want, rel)
	t.Logf("the file's path relative to the prefix is %d bytes, and whole %d (LastCap %d)", len(rel), len(pyc), LastCap)
	if b, err := os.ReadFile(pyc); err != nil || len(b) == 0 {
		t.Fatalf("the first line's bytecode at %s, in the scratch's pycache dir: %d bytes, %v", pyc, len(b), err)
	}
	if es, err := os.ReadDir(r); err != nil || len(es) != 1 || es[0].Name() != "victim.py" {
		t.Errorf("a scratched line's python left %v under the root (%v), want victim.py alone", es, err)
	}
	remove()
	for _, f := range []string{pyc, want} {
		if _, err := os.Lstat(f); !os.IsNotExist(err) {
			t.Errorf("%s outlived its scratch: %v", f, err)
		}
	}
}

// honoring is a fake python3's body (fakeTool) that names roots as its
// trees and answers the pin's ask with its $PYTHONPYCACHEPREFIX, as a
// python that honors the prefix does.
func honoring(roots ...string) string {
	return `case "$2" in *pycache_prefix*) echo "$PYTHONPYCACHEPREFIX";; *) for d in ` + strings.Join(roots, " ") + `; do echo "$d"; done;; esac`
}

// TestAR49_2_AskAtPin is AR49 act 2: the pin asks its python3 once, under
// the pinned env, for sys.pycache_prefix, and honors the prefix only on
// the exact answer. A fake that answers with its $PYTHONPYCACHEPREFIX
// gives an empty PycacheNote; AR24's fake (every call answered with its
// roots), one that answers None and one that exits 1 on the ask each give
// a note that names python3 and __pycache__ and says the cache files stay
// counted, and no pin error; a pin with no python3 gives an empty note.
func TestAR49_2_AskAtPin(t *testing.T) {
	root := tree(t, map[string]string{"lib/python3.12/os.py": "# os\n"})
	for _, c := range []struct {
		name, tool, body string
		note             bool
	}{
		{"a python3 that answers with its prefix", "python3", honoring(root), false},
		{"AR24's fake: every call answered with its roots", "python3", "for d in " + root + " " + root + "/lib/python3.12; do echo \"$d\"; done", true},
		{"a python3 that drops the variable", "python3", `case "$2" in *pycache_prefix*) echo None;; *) echo ` + root + `;; esac`, true},
		{"a python3 that answers more than the prefix", "python3", `case "$2" in *pycache_prefix*) echo "$PYTHONPYCACHEPREFIX"; echo x;; *) echo ` + root + `;; esac`, true},
		{"a python3 that exits 1 on the ask", "python3", `case "$2" in *pycache_prefix*) echo "$PYTHONPYCACHEPREFIX"; exit 1;; *) echo ` + root + `;; esac`, true},
		{"no python3 pinned", "go", `[ "$1 $2" = "env GOROOT" ] && echo ` + root, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeTool(t, c.tool, c.body)
			p, err := PinAt(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatalf("PinAt = %v, want a pin whatever the ask's answer", err)
			}
			t.Cleanup(func() { p.Release() })
			if p.Roots[root] != c.tool {
				t.Fatalf("Roots = %v, want %s bound to %s", p.Roots, root, c.tool)
			}
			note := PycacheNote(p)
			if !c.note {
				if note != "" {
					t.Fatalf("PycacheNote = %q, want none", note)
				}
				return
			}
			for _, w := range []string{"python3", "__pycache__", "stay counted"} {
				if !strings.Contains(note, w) {
					t.Errorf("PycacheNote = %q, want %q in it", note, w)
				}
			}
			if note == "" || strings.Contains(note, "\n") {
				t.Errorf("PycacheNote = %q, want one line", note)
			}
		})
	}
}

// pyTree is a fake python3's root for acts 3 to 5 (a stdlib dir with one
// source and its cache file) and its __pycache__.
func pyTree(t *testing.T) (root, cache string) {
	t.Helper()
	root = tree(t, map[string]string{"lib/python3.12/os.py": "# os\n", "lib/python3.12/__pycache__/os.cpython-312.pyc": "os 1"})
	return root, filepath.Join(root, "lib/python3.12/__pycache__")
}

// pinFake is a fresh pin whose only account tool is the fake tool (body:
// fakeTool), released at cleanup.
func pinFake(t *testing.T, tool, body string) Pinned {
	t.Helper()
	fakeTool(t, tool, body)
	p, err := PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })
	return p
}

// write writes each named file (its bytes the name's own) and fails the
// test on an error.
func write(t *testing.T, files ...string) {
	t.Helper()
	for _, f := range files {
		if err := os.WriteFile(f, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAR49_3_CacheFilesNotMoved is AR49 act 3: under a python3 that honors
// the prefix, python's own cache files are no entries of the snapshot. Two
// cache-tagged files added to a pinned __pycache__ and one there at the
// pin rewritten, or that one removed, and Moved and then MovedStrict name
// nothing; nor does a MovedStrict with no Moved before it.
func TestAR49_3_CacheFilesNotMoved(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(t *testing.T, cache string)
		strict bool // MovedStrict alone, no Moved before it
	}{
		{"two added, one rewritten", func(t *testing.T, cache string) {
			write(t, filepath.Join(cache, "pty.cpython-312.pyc"), filepath.Join(cache, "pty.cpython-312.opt-2.pyc"), filepath.Join(cache, "os.cpython-312.pyc"))
		}, false},
		{"one removed", func(t *testing.T, cache string) {
			if err := os.Remove(filepath.Join(cache, "os.cpython-312.pyc")); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"two added, one rewritten: the merge alone", func(t *testing.T, cache string) {
			write(t, filepath.Join(cache, "pty.cpython-312.pyc"), filepath.Join(cache, "pty.cpython-312.opt-2.pyc"), filepath.Join(cache, "os.cpython-312.pyc"))
		}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			root, cache := pyTree(t)
			p := pinFake(t, "python3", honoring(root, root+"/lib/python3.12"))
			if note := PycacheNote(p); note != "" || p.Roots[root] != "python3" {
				t.Fatalf("PycacheNote %q, Roots %v: want the prefix honored and %s bound to python3", note, p.Roots, root)
			}
			time.Sleep(10 * time.Millisecond)
			c.change(t, cache)
			if !c.strict {
				if moved := p.Moved(); len(moved) != 0 {
					t.Fatalf("Moved = %v, want none", moved)
				}
			}
			if moved := p.MovedStrict(); len(moved) != 0 {
				t.Fatalf("MovedStrict = %v, want none", moved)
			}
		})
	}
}

// TestAR49_4_StillMoved is AR49 act 4 (design/pin-pycache-r1.md §7, F1):
// only python's own cache files left the snapshot. Under the honoring
// fake, each case is added after a fresh pin and named python3 by Moved,
// and after a second fresh pin by MovedStrict: a __pycache__ is an
// importable namespace package, so everything in one but a cache-tagged
// regular file is still an entry, and so is a cache-tagged name anywhere
// else. A tree bound to go, alone or with python3, is walked whole. The
// control: a cache-tagged file added to the pinned __pycache__ is not
// moved, so the table's roots are ones the predicate holds in.
func TestAR49_4_StillMoved(t *testing.T) {
	const lib = "lib/python3.12"
	const cache = lib + "/__pycache__"
	file := func(rel string) func(*testing.T, string) {
		return func(t *testing.T, root string) { write(t, filepath.Join(root, rel)) }
	}
	dir := func(rel string) func(*testing.T, string) {
		return func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, rel), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	link := func(rel, to string) func(*testing.T, string) {
		return func(t *testing.T, root string) {
			if err := os.Symlink(filepath.Join(root, to), filepath.Join(root, rel)); err != nil {
				t.Fatal(err)
			}
		}
	}
	fifo := func(rel string) func(*testing.T, string) {
		return func(t *testing.T, root string) {
			if err := syscall.Mkfifo(filepath.Join(root, rel), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// fresh is a fresh root (pyTree, and a __pycache__ nested in its
	// __pycache__, there at the pin) under a fresh pin of the honoring
	// fake.
	fresh := func(t *testing.T) (Pinned, string) {
		root, c := pyTree(t)
		if err := os.Mkdir(filepath.Join(c, "__pycache__"), 0o755); err != nil {
			t.Fatal(err)
		}
		p := pinFake(t, "python3", honoring(root, root+"/"+lib))
		if note := PycacheNote(p); note != "" || len(p.Moved()) != 0 {
			t.Fatalf("PycacheNote %q, Moved %v: want the prefix honored and nothing moved as pinned", note, p.Moved())
		}
		return p, root
	}
	t.Run("the control: a cache-tagged file in the __pycache__", func(t *testing.T) {
		p, root := fresh(t)
		write(t, filepath.Join(root, cache, "x.cpython-312.pyc"))
		if moved := p.Moved(); len(moved) != 0 {
			t.Fatalf("Moved = %v, want none", moved)
		}
	})
	for _, c := range []struct {
		name string
		add  func(*testing.T, string)
	}{
		{"a .pth", file(lib + "/evil.pth")},
		{"a sitecustomize.py", file(lib + "/sitecustomize.py")},
		{"x.cpython-312.pyc outside any __pycache__", file(lib + "/x.cpython-312.pyc")},
		{"a regular file named __pycache__", file("lib/__pycache__")},
		{"a symlink named __pycache__ to a directory", link("lib/__pycache__", cache)},
		{"in the __pycache__: evil.py", file(cache + "/evil.py")},
		{"in the __pycache__: evil.so", file(cache + "/evil.so")},
		{"in the __pycache__: evil.pyc", file(cache + "/evil.pyc")},
		{"in the __pycache__: .cpython-312.pyc", file(cache + "/.cpython-312.pyc")},
		{"in the __pycache__: x.cpython-.pyc", file(cache + "/x.cpython-.pyc")},
		{"in the __pycache__: x.cpython-312.opt-.pyc", file(cache + "/x.cpython-312.opt-.pyc")},
		{"in the __pycache__: a subdirectory", dir(cache + "/sub")},
		{"in the __pycache__: a directory named x.cpython-312.pyc", dir(cache + "/x.cpython-312.pyc")},
		{"in the __pycache__: a symlink named x.cpython-312.pyc", link(cache+"/x.cpython-312.pyc", cache+"/os.cpython-312.pyc")},
		{"in the __pycache__: a FIFO named x.cpython-312.pyc", fifo(cache + "/x.cpython-312.pyc")},
		{"in the __pycache__: a cache-tagged file in the __pycache__ nested in it", file(cache + "/__pycache__/x.cpython-312.pyc")},
		{"in the __pycache__: a __pycache__ made in the one nested in it", dir(cache + "/__pycache__/__pycache__")},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, strict := range []bool{false, true} {
				p, root := fresh(t)
				c.add(t, root)
				moved := p.Moved
				if strict {
					moved = p.MovedStrict
				}
				if got := moved(); !slices.Equal(got, []string{"python3"}) {
					t.Errorf("strict %v: moved = %v, want [python3]", strict, got)
				}
			}
		})
	}
	goRoot := `[ "$1 $2" = "env GOROOT" ] && echo `
	t.Run("a cache-tagged file in a go root's __pycache__", func(t *testing.T) {
		for _, strict := range []bool{false, true} {
			root, c := pyTree(t)
			p := pinFake(t, "go", goRoot+root)
			write(t, filepath.Join(c, "x.cpython-312.pyc"))
			moved := p.Moved
			if strict {
				moved = p.MovedStrict
			}
			if got := moved(); !slices.Equal(got, []string{"go"}) {
				t.Errorf("strict %v: moved = %v, want [go]", strict, got)
			}
		}
	})
	t.Run("a cache-tagged file in a root python3 shares with go", func(t *testing.T) {
		for _, strict := range []bool{false, true} {
			root, c := pyTree(t)
			fakeTool(t, "python3", honoring(root+"/"+lib)) // nested in go's root: one tree, both tools
			bin, _, _ := strings.Cut(os.Getenv("PATH"), ":")
			if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\n"+goRoot+root+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			PinTools = []string{"go", "python3"}
			p, err := PinAt(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { p.Release() })
			if note := PycacheNote(p); note != "" || len(p.trees) != 1 || p.trees[0].tool != "go,python3" {
				t.Fatalf("PycacheNote %q, %d trees: want the prefix honored and one tree bound to go and python3", note, len(p.trees))
			}
			write(t, filepath.Join(c, "x.cpython-312.pyc"))
			moved := p.Moved
			if strict {
				moved = p.MovedStrict
			}
			if got := moved(); !slices.Equal(got, []string{"go", "python3"}) {
				t.Errorf("strict %v: moved = %v, want [go python3]", strict, got)
			}
		}
	})
}

// TestAR49_5_NewDirAndUnhonored is AR49 act 5. Under the honoring fake, a
// __pycache__ made after the gate's Moved, in a dir that had none, holds
// no entry of its own, but its parent's stat changed after the gate read
// it: the merge's MovedStrict names python3 and fails closed
// (design/pin-pycache-r1.md §6); the next Moved names nothing, and the
// MovedStrict after it nothing. Under AR24's fake, which does not honor
// the prefix, a cache-tagged file added to a pinned __pycache__ is moved.
func TestAR49_5_NewDirAndUnhonored(t *testing.T) {
	t.Run("a new __pycache__", func(t *testing.T) {
		root, _ := pyTree(t)
		p := pinFake(t, "python3", honoring(root, root+"/lib/python3.12"))
		if note := PycacheNote(p); note != "" {
			t.Fatalf("PycacheNote = %q, want the prefix honored", note)
		}
		if moved := p.Moved(); len(moved) != 0 {
			t.Fatalf("Moved as pinned = %v", moved)
		}
		time.Sleep(10 * time.Millisecond)
		if err := os.Mkdir(filepath.Join(root, "lib/__pycache__"), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(root, "lib/__pycache__/site.cpython-312.pyc"))
		if moved := p.MovedStrict(); !slices.Equal(moved, []string{"python3"}) {
			t.Fatalf("MovedStrict after a new __pycache__ = %v, want [python3]: its parent's stat changed after the gate read it", moved)
		}
		if moved := p.Moved(); len(moved) != 0 {
			t.Fatalf("Moved after a new __pycache__ = %v, want none", moved)
		}
		if moved := p.MovedStrict(); len(moved) != 0 {
			t.Fatalf("MovedStrict after that Moved = %v, want none", moved)
		}
	})
	t.Run("a python3 that does not honor the prefix", func(t *testing.T) {
		root, cache := pyTree(t)
		p := pinFake(t, "python3", "for d in "+root+" "+root+"/lib/python3.12; do echo \"$d\"; done")
		if note := PycacheNote(p); note == "" {
			t.Fatalf("PycacheNote is empty, want a note under AR24's fake")
		}
		if moved := p.Moved(); len(moved) != 0 {
			t.Fatalf("Moved as pinned = %v", moved)
		}
		write(t, filepath.Join(cache, "pty.cpython-312.pyc"))
		if moved := p.Moved(); !slices.Equal(moved, []string{"python3"}) {
			t.Fatalf("Moved after a cache-tagged file, the prefix not honored = %v, want [python3]", moved)
		}
	})
}
