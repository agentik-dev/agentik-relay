package prompt_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestRender(t *testing.T) {
	c := prompt.Context{Row: "a5-effects-plan", Tree: "/r/.claude/worktrees/a5-effects-plan",
		Branch: "a5-effects-plan", Base: "a4-schemas", Unmerged: []string{"a4-schemas at a80b84b"},
		Device: "never", Words: "the owner's words"}
	packet := []byte("# Packet A5: the body\n")
	stdin, rulesHash, promptHash := prompt.Render(c, nil, packet)
	parts := strings.SplitN(string(stdin), "\n\n", 3)
	if len(parts) != 3 || parts[0]+"\n" != string(prompt.Rules()) || parts[2] != string(packet) {
		t.Fatalf("stdin is not RULES, CONTEXT, packet:\n%s", stdin)
	}
	want := "CONTEXT: relay row a5-effects-plan; your working tree is /r/.claude/worktrees/a5-effects-plan on branch a5-effects-plan (off a4-schemas); "
	if !strings.HasPrefix(parts[1], want) || !strings.Contains(parts[1], "a4-schemas at a80b84b") ||
		!strings.Contains(parts[1], "the owner's words") || strings.Contains(parts[1], "\n") {
		t.Fatalf("CONTEXT = %q", parts[1])
	}
	if rulesHash != prompt.TemplateHash() {
		t.Fatalf("rulesHash %s is not the template hash %s", rulesHash, prompt.TemplateHash())
	}
	sum := sha256.Sum256(stdin)
	if promptHash != hex.EncodeToString(sum[:]) {
		t.Fatal("promptHash is not sha256 of stdin")
	}
	if l := prompt.Line(prompt.Context{Row: "r", Tree: "t", Branch: "b"}); strings.Count(l, "none") != 5 {
		t.Fatalf("empty fields do not read none: %q", l)
	}
	if bytes.Contains(prompt.Rules(), []byte("NEXT")) {
		t.Fatal("RULES still names NEXT (the queue replaced it)")
	}
}

// planOverFakes builds a one-row plan over the fakes from the packet corpus
// and returns the ledger it wrote to and the packet's bytes.
func planOverFakes(t *testing.T) (*effects.FakeLedger, []byte) {
	t.Helper()
	repo := t.TempDir()
	src, err := os.ReadFile(filepath.Join("..", "packet", "testdata", "a3-machine.md"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "a3-machine.md"), src, 0o644)
	os.WriteFile(filepath.Join(repo, "q.yaml"), []byte("version: 1\ndefaults: {model: m, turns: 1, context: 1, usd: 1, wall: 1h, merge: branch-only, device: never, verify: auto}\nwords: \"\"\nrows:\n  - id: a3-machine\n    packet: a3-machine.md\n"), 0o644)
	q, err := queue.Load(repo, "q.yaml")
	if err != nil {
		t.Fatal(err)
	}
	clock := effects.FakeClock{T: time.Date(2026, 9, 22, 22, 0, 0, 0, time.UTC)}
	l := &effects.FakeLedger{Clock: clock, Night: "2026-09-22-1"}
	fx := effects.Effects{Git: &effects.FakeGit{Roots: map[string]string{repo: "/root"},
		Branches: map[string]string{"/root": "main"}}, Shell: &effects.FakeShell{}, Clock: clock,
		FS: &effects.FakeFS{}, Ledger: l}
	if _, err := night.BuildPlan(fx, q, night.Options{Repo: repo, State: "/state"}); err != nil {
		t.Fatal(err)
	}
	return l, src
}

func TestTemplateHashRecorded(t *testing.T) {
	h := prompt.TemplateHash()
	if !hex64.MatchString(h) {
		t.Fatalf("template hash %q is not 64 hex chars", h)
	}
	src, err := os.ReadFile("rules.txt")
	if err != nil || !bytes.Equal(src, prompt.Rules()) {
		t.Fatalf("rules.txt is not the embedded RULES: %v", err)
	}
	if prompt.HashOf(src) != h {
		t.Fatal("HashOf the committed rules.txt is not TemplateHash")
	}
	changed := append(append([]byte(nil), src...), '.')
	if prompt.HashOf(changed) == h {
		t.Fatal("a changed copy of rules.txt keeps the template hash")
	}
	l, packet := planOverFakes(t)
	_, rulesHash, promptHash := prompt.Render(prompt.Context{Row: "a3-machine"}, nil, packet)
	l.Append("spawn", map[string]any{"row": "a3-machine", "rules_hash": rulesHash, "prompt_hash": promptHash})
	lines := l.Written()
	if len(lines) != 2 {
		t.Fatalf("ledger = %v", lines)
	}
	for _, line := range lines {
		if ev := line["event"]; (ev == "plan" || ev == "spawn") && line["rules_hash"] != h {
			t.Fatalf("a %s line carries rules_hash %v, want %s", ev, line["rules_hash"], h)
		}
	}
}
