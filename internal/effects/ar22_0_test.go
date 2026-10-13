package effects

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestAR22_0_CommitNeverSigns is ADV13-9's repro: a root whose local
// config asks for an ssh signature (commit.gpgSign, gpg.format=ssh and
// gpg.ssh.defaultKeyCommand, a key the program-key list lacks) never runs
// the key command at the merge's commit: the runner commits with
// commit.gpgSign=false.
func TestAR22_0_CommitNeverSigns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, _ := repoWithRemote(t)
	marker := filepath.Join(t.TempDir(), "ran")
	cmd := filepath.Join(t.TempDir(), "keycmd")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\necho ran > '"+marker+"'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "config", "commit.gpgSign", "true")
	gitIn(t, root, "config", "gpg.format", "ssh")
	gitIn(t, root, "config", "gpg.ssh.defaultKeyCommand", cmd)
	os.WriteFile(filepath.Join(root, "c.txt"), []byte("c\n"), 0o644)
	gitIn(t, root, "add", "c.txt")
	var g OSGit
	if _, err := g.Commit(root, "unsigned", "t", "t@t"); err != nil {
		t.Fatalf("the runner's commit tried to sign: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the commit ran gpg.ssh.defaultKeyCommand (%v)", err)
	}
}
