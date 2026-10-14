package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR17B_2_ResumeNested: `agentik-relay resume` refuses in a sitting as `relay
// run` does (ADV10-18), before it reads the ledger.
func TestAR17B_2_ResumeNested(t *testing.T) {
	home := t.TempDir()
	old := getenv
	t.Cleanup(func() { getenv = old })
	inner := fakeEnv(home)
	getenv = func(k string) string {
		if k == night.RowVar {
			return "ar17b-guards-r10"
		}
		return inner(k)
	}
	for _, args := range [][]string{{"resume"}, {"resume", "r1"}} {
		var out, errb bytes.Buffer
		if code := run(args, strings.NewReader(""), &out, &errb); code != exitUsage ||
			!strings.HasPrefix(errb.String(), "relay resume: nested: ") {
			t.Fatalf("%v: exit %d, stderr %q", args, code, errb.String())
		}
	}
}

// TestAR17B_3_NoBinaryDeniesInRow: the plugin's hook shim fails closed
// (ADV10-15). With no RELAY_BIN and no relay on PATH it denies a sitting's
// Bash call and says {} outside one; with RELAY_BIN set it runs that binary.
func TestAR17B_3_NoBinaryDeniesInRow(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "plugin", "hooks", "run"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	shim := filepath.Join(dir, "run")
	os.WriteFile(shim, src, 0o755)
	fake := filepath.Join(dir, "fake-relay")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"fake $*\"\n"), 0o755)
	in := `{"tool_name":"Bash","tool_input":{"command":"git push"}}`
	for _, c := range []struct {
		env  []string
		want string
	}{
		{[]string{"RELAY_ROW=r1"}, `"permissionDecision":"deny"`},
		{nil, "{}\n"},
		{[]string{"RELAY_ROW=r1", "RELAY_BIN=" + filepath.Join(dir, "missing")}, `"permissionDecision":"deny"`},
		{[]string{"RELAY_ROW=r1", "RELAY_BIN=" + fake}, "fake hook PreToolUse\n"},
	} {
		cmd := exec.Command("/bin/sh", shim, "PreToolUse")
		cmd.Env = append([]string{"PATH=/usr/bin:/bin", "HOME=" + dir}, c.env...)
		cmd.Stdin = strings.NewReader(in)
		out, err := cmd.Output()
		if err != nil || !strings.Contains(string(out), c.want) || c.want == "{}\n" && string(out) != c.want {
			t.Errorf("%v: %q (%v), want %q", c.env, out, err, c.want)
		}
	}
}
