package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/hooks"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR20_4_NestedByEnv (review test 3; ADV11-9): under a live relay
// holder, a built binary run with XDG_STATE_HOME and HOME changed is still
// refused (the pid file is at the account's home, not the env's), and one
// run with no HOME (env -i) is refused for its relative state root; the
// hook denies go run and go build of any path that cleans to cmd/relay.
func TestAR20_4_NestedByEnv(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	bin, pidHome := filepath.Join(dir, "bin", "relay"), filepath.Join(dir, "pidhome")
	if out, err := exec.Command("go", "build", "-ldflags", lockFlags(pidHome, testLockPort), "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// This test process is the live holder (its port, AR22 act 2; the
	// binary is linked to this binary's lock port), an ancestor of every
	// binary it starts.
	self := os.Getpid()
	release, err := night.PidFile(filepath.Join(pidHome, ".local", "state", "agentik-relay"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	path := "PATH=" + os.Getenv("PATH")
	for _, args := range [][]string{{"run"}, {"plan"}, {"resume"}} {
		for name, env := range map[string][]string{
			"XDG_STATE_HOME changed": {path, "HOME=" + filepath.Join(dir, "h1"), "XDG_STATE_HOME=" + filepath.Join(dir, "s2")},
			"HOME changed":           {path, "HOME=" + filepath.Join(dir, "h2")},
		} {
			c := exec.Command(bin, args...)
			c.Dir, c.Env = dir, env
			out, _ := c.CombinedOutput()
			if code := c.ProcessState.ExitCode(); code != exitUsage || !strings.Contains(string(out), fmt.Sprintf("nested: relay %d is live", self)) {
				t.Errorf("%v with %s: exit %d:\n%s", args, name, code, out)
			}
		}
		c := exec.Command(bin, args...)
		c.Dir, c.Env = dir, []string{path} // env -i: no HOME, a relative state root
		out, _ := c.CombinedOutput()
		if code := c.ProcessState.ExitCode(); code != exitUsage || !strings.Contains(string(out), "is not absolute") {
			t.Errorf("%v under env -i: exit %d:\n%s", args, code, out)
		}
	}
	for _, cmd := range []string{
		"go run ./cmd/./relay run", "go run ./cmd//relay/ resume", "cd cmd && go run ./relay run",
		"go build -o r ./cmd/relay && ./r run", "go build -o r ./cmd/./relay", "go install ./cmd/relay",
		"go run cmd/relay/main.go resume r1", "go build github.com/agentik-dev/agentik-relay/cmd/relay",
		"relay resume r1", "./bin/relay resume",
	} {
		if deny, _ := hooks.Verdict(cmd, true); !deny {
			t.Errorf("the hook allows %q in a sitting", cmd)
		}
	}
	for _, cmd := range []string{"go build ./...", "go test ./cmd/relay -run TestX", "go build -o relay ./internal/x", "go run ./cmd/relay status"} {
		if deny, why := hooks.Verdict(cmd, true); deny {
			t.Errorf("the hook denies %q: %s", cmd, why)
		}
	}
}
