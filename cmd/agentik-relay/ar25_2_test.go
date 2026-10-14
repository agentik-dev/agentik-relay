package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR25_2_AttachNamesSocket is ADV15-2, -5: `relay attach` runs the
// machine's tmux (newTmux, a fake tmux by lookPath) on the relay's own
// socket, <state>/tmux/relay.sock, and the line it prints names it.
func TestAR25_2_AttachNamesSocket(t *testing.T) {
	home := t.TempDir()
	getenv = fakeEnv(home)
	oldLook, oldExec := lookPath, execAttach
	t.Cleanup(func() { getenv, lookPath, execAttach = os.Getenv, oldLook, oldExec })
	state := effects.StateDir(getenv)
	const id = "2026-09-24-1"
	os.MkdirAll(night.NightDir(state, id), 0o700)
	os.WriteFile(filepath.Join(night.NightDir(state, id), "status.json"),
		[]byte(`{"night":"`+id+`","state":"running","rows":[{"id":"r1","state":"running"}]}`), 0o600)
	dir := t.TempDir()
	log, bin := filepath.Join(dir, "argv"), filepath.Join(dir, "tmux")
	os.WriteFile(bin, []byte("#!/bin/sh\necho \"$*\" >> "+log+"\nexit 0\n"), 0o755)
	lookPath = func(name string) (string, error) { return bin, nil }
	execAttach = func([]string) error { t.Fatal("--print ran attach"); return nil }
	var out, errb bytes.Buffer
	if code := run([]string{"attach", "r1", "--print"}, strings.NewReader(""), &out, &errb); code != exitOK {
		t.Fatalf("attach = %d %q", code, errb.String())
	}
	sock := filepath.Join(state, "tmux", "relay.sock")
	want := "-S ~/" + strings.TrimPrefix(sock, home+"/") + " attach -t '=relay-" + id + ":r1'"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("attach printed %q, want the socket: %q", out.String(), want)
	}
	b, _ := os.ReadFile(log)
	if calls := strings.TrimSpace(string(b)); calls == "" || !strings.HasPrefix(calls, "-S "+sock+" has-session") {
		t.Fatalf("tmux calls %q, want has-session on %s", calls, sock)
	}
}
