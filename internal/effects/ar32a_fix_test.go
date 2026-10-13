package effects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR32AFix_QueuePathIsRepoRelative is ar32a review round 2 (F1): the
// config's queue path is made a clean repo-relative path once, where it is
// loaded, so the gate's build-row guard and the orchestrator's queue check
// both see the one spelling. `./relay/queue.yaml` reads `relay/queue.yaml`;
// an absolute path under the repo (its symlinks resolved, as git prints a
// root) reads relative to it; a path that leaves the repo is a config error.
func TestAR32AFix_QueuePathIsRepoRelative(t *testing.T) {
	repo := t.TempDir()
	real, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(repo, "relay"), 0o755)
	os.WriteFile(filepath.Join(repo, "relay", "queue.yaml"), []byte("version: 1\n"), 0o644)
	env := map[string]string{"HOME": t.TempDir()}
	get := func(k string) string { return env[k] }
	for _, c := range []struct{ queue, want, err string }{
		{"relay/queue.yaml", "relay/queue.yaml", ""},
		{"./relay/queue.yaml", "relay/queue.yaml", ""},
		{"relay//x/../queue.yaml", "relay/queue.yaml", ""},
		{filepath.Join(repo, "relay", "queue.yaml"), "relay/queue.yaml", ""},
		{filepath.Join(real, "relay", "queue.yaml"), "relay/queue.yaml", ""},
		{"../queue.yaml", "", "inside the repo"},
		{filepath.Join(filepath.Dir(repo), "queue.yaml"), "", "inside the repo"},
		{"''", "", "inside the repo"},
	} {
		fs := &FakeFS{}
		fs.WriteFile(filepath.Join(repo, repoConfig), []byte("agent-relay:\n  queue: "+c.queue+"\n"), 0o600)
		got, err := LoadConfig(fs, get, repo)
		switch {
		case c.err == "" && (err != nil || got.Queue != c.want):
			t.Errorf("queue %q: Queue = %q, %v; want %q", c.queue, got.Queue, err, c.want)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("queue %q: Queue = %q, %v; want an error naming %q", c.queue, got.Queue, err, c.err)
		}
	}
}
