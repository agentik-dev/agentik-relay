package effects

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestAR90_1_BareBashKept (AR90 ruling 1): the default keeps its bare
// `Bash`, one word of it; LoadConfig with no file gives the default, and a
// user file's `allowed_tools` passes as written. The default's comment names
// the 2026-10-16 measurement, the CLI version it was read on and the reason
// the entry stays: a session whose auto mode is unavailable falls back to
// Manual, where the entry pre-allows a headless child's Bash calls.
func TestAR90_1_BareBashKept(t *testing.T) {
	bare := 0
	for _, w := range strings.Fields(DefaultAllowedTools) {
		if w == "Bash" {
			bare++
		}
	}
	if bare != 1 {
		t.Fatalf("DefaultAllowedTools %q: %d bare Bash words; want 1", DefaultAllowedTools, bare)
	}
	env := map[string]string{"HOME": "/h"}
	get := func(k string) string { return env[k] }
	fs := &FakeFS{}
	if c, err := LoadConfig(fs, get, "/r"); err != nil || c.AllowedTools != DefaultAllowedTools {
		t.Fatalf("no file: allowed tools %q, %v; want the default %q", c.AllowedTools, err, DefaultAllowedTools)
	}
	fs.WriteFile("/h/"+userConfig, []byte("allowed_tools: \"Bash(go test:*) Edit\"\n"), 0o600)
	if c, err := LoadConfig(fs, get, "/r"); err != nil || c.AllowedTools != "Bash(go test:*) Edit" {
		t.Fatalf("a user file: allowed tools %q, %v; want it as written", c.AllowedTools, err)
	}

	f, err := parser.ParseFile(token.NewFileSet(), "config.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	doc := ""
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.ValueSpec); ok && len(s.Names) == 1 && s.Names[0].Name == "DefaultAllowedTools" {
			doc = s.Doc.Text()
		}
		return true
	})
	for _, w := range []string{"2026-10-16", "2.1.302", "permission_denials", "Manual"} {
		if !strings.Contains(doc, w) {
			t.Fatalf("DefaultAllowedTools' comment does not name %q:\n%s", w, doc)
		}
	}
}
