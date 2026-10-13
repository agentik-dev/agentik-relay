package queue

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAR14_2_RelativePacketResolvesAgainstRepo: a row's relative `packet`
// joins the queue's repo, never the cwd (ruling 2026-09-23 14:55 (1); §3).
func TestAR14_2_RelativePacketResolvesAgainstRepo(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "packet", "testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "design", "packets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "design", "packets", "ok-minimal.md"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "relay"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := "version: 1\ndefaults: {model: claude-opus-5-5, turns: 10, context: 20000, usd: 5, wall: 1h,\n  merge: branch-only, device: never, verify: auto}\nrows:\n  - id: ok-minimal\n    packet: design/packets/ok-minimal.md\n"
	if err := os.WriteFile(filepath.Join(repo, "relay", "queue.yaml"), []byte(queue), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	q, err := Load(repo, "relay/queue.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if ps := q.Validate(); len(ps) != 0 {
		t.Fatalf("problems: %+v", ps)
	}
	if len(q.Rows) != 1 || q.Rows[0].Packet == nil || q.Rows[0].Packet.ID != "ok-minimal" {
		t.Fatalf("rows = %+v, want ok-minimal's packet parsed", q.Rows)
	}
}
