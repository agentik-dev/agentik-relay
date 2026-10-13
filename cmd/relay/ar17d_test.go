package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var upLiteral = regexp.MustCompile(`"(\.\./\.\.[^"]*)"`)

// realTreeReads scans every _test.go under root's cmd and internal for a
// `"../..` literal and returns each one that names a path in the repository
// (resolved from its file's directory), outside a testdata directory, on a
// line that is not a copyIn call: `file:line: path`, sorted by the walk.
func realTreeReads(root string) ([]string, error) {
	var bad []string
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			sc := bufio.NewScanner(f)
			for n := 1; sc.Scan(); n++ {
				line := sc.Text()
				for _, m := range upLiteral.FindAllStringSubmatch(line, -1) {
					path := filepath.Join(filepath.Dir(rel), m[1])
					if path == ".." || strings.HasPrefix(path, "../") || strings.Contains(line, "copyIn(") ||
						strings.Contains("/"+path+"/", "/testdata/") {
						continue
					}
					bad = append(bad, fmt.Sprintf("%s:%d: %s", rel, n, m[1]))
				}
			}
			return sc.Err()
		})
		if err != nil {
			return nil, err
		}
	}
	return bad, nil
}

// TestAR17D_4_NoTestReadsRealTree (ADV10-22): the copy rule. A planted tree
// holds one bare read of the real tree among a copyIn read, a fixture and a
// path outside the repository; the scan names the bare read alone. On a
// copy of this repository's cmd and internal it names nothing.
func TestAR17D_4_NoTestReadsRealTree(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "internal", "x"), 0o755)
	os.MkdirAll(filepath.Join(root, "cmd"), 0o755)
	os.WriteFile(filepath.Join(root, "internal", "x", "x_test.go"), []byte(strings.ReplaceAll(`package x
var a, _ = os.ReadFile("UP/UP/design/packets/p.md")
var b = copyIn(t, "UP/UP", "design/packets")
var c = "UP/UP/internal/x/testdata/f.md"
var d = "UP/UP/UP/UP/stream/testdata/real.jsonl"
var e = "UP/x.md"
`, "UP", "..")), 0o644)
	got, err := realTreeReads(root)
	if want := []string{"internal/x/x_test.go:2: ../../design/packets/p.md"}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("planted scan = %q, %v; want %q", got, err, want)
	}
	got, err = realTreeReads(copyIn(t, "../..", "cmd", "internal"))
	if err != nil || len(got) != 0 {
		t.Errorf("a test reads the real tree outside copyIn: %q, %v", got, err)
	}
}
