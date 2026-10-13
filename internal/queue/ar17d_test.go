package queue

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR17D_3_RowsFilesRepoRelative (run 10 ruling 8, ADV10-8): every
// tracked rows file, read from a copy under t.TempDir(), names its repo by
// a bare name and its prompt repo-relative; no string in a row is an
// absolute or `~` path.
func TestAR17D_3_RowsFilesRepoRelative(t *testing.T) {
	src, err := filepath.Glob(filepath.Join(copyIn(t, "../..", "design/packets"), "design", "packets", "rows-*.json"))
	if err != nil || len(src) == 0 {
		t.Fatalf("no rows files: %v", err)
	}
	dir := t.TempDir()
	for _, p := range src {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		cp := filepath.Join(dir, filepath.Base(p))
		if err := os.WriteFile(cp, b, 0o600); err != nil {
			t.Fatal(err)
		}
		b, _ = os.ReadFile(cp)
		var rows []map[string]any
		if err := json.Unmarshal(b, &rows); err != nil {
			t.Fatalf("%s: %v", filepath.Base(p), err)
		}
		for _, r := range rows {
			if name, _ := r["repo"].(string); name == "" || strings.ContainsAny(name, "/~") {
				t.Errorf("%s %v: repo %q, want a bare name", filepath.Base(p), r["id"], name)
			}
			for k, v := range r {
				if s, ok := v.(string); ok && (strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || strings.Contains(s, "/Users/")) {
					t.Errorf("%s %v: %s %q is an absolute or ~ path", filepath.Base(p), r["id"], k, s)
				}
			}
		}
	}
}

// TestAR17D_4_RefShapedIds (ADV10-19): an id names a branch, so it passes
// `git check-ref-format --branch` (no `..`, no trailing `.`, no `.lock`
// suffix, never HEAD) and is never a base: `main`, or its own row's base.
// HEAD, main and .lock are case-folded (this Mac's filesystem). With git on
// the path, each refused id git refuses too is cross-checked, and every
// accepted id passes git.
func TestAR17D_4_RefShapedIds(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "q.yaml"), []byte(`version: 1
defaults: {model: claude-opus-5-5, turns: 100, context: 130000, usd: 15, wall: 4h,
           merge: branch-only, device: never}
words: ""
rows:
  - {id: a..b, packet: p.md}
  - {id: x., packet: p.md}
  - {id: x.lock, packet: p.md}
  - {id: y.LOCK, packet: p.md}
  - {id: HEAD, packet: p.md}
  - {id: head, packet: p.md}
  - {id: main, packet: p.md}
  - {id: dev, packet: p.md, base: dev}
  - {id: a.b, packet: p.md}
  - {id: ar17d-rulings-r10, packet: p.md}
  - {id: x.locked, packet: p.md}
`), 0o644)
	q, err := Load(root, "q.yaml")
	if err != nil {
		t.Fatal(err)
	}
	refused := map[string]bool{}
	for _, p := range q.Validate() {
		if strings.HasPrefix(p.Msg, "id ") {
			refused[p.Row] = true
		}
	}
	gitRefuses := []string{"a..b", "x.", "x.lock", "HEAD"}
	for _, id := range append(gitRefuses, "y.LOCK", "head", "main", "dev") {
		if !refused[id] {
			t.Errorf("id %q is valid; want it refused", id)
		}
	}
	accepted := []string{"a.b", "ar17d-rulings-r10", "x.locked"}
	for _, id := range accepted {
		if refused[id] {
			t.Errorf("id %q is refused; want it valid", id)
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	for _, id := range gitRefuses {
		if exec.Command("git", "check-ref-format", "--branch", id).Run() == nil {
			t.Errorf("git check-ref-format --branch %s passes; the table is wrong", id)
		}
	}
	for _, id := range accepted {
		if out, err := exec.Command("git", "check-ref-format", "--branch", id).CombinedOutput(); err != nil {
			t.Errorf("git check-ref-format --branch %s: %v %s", id, err, out)
		}
	}
}
