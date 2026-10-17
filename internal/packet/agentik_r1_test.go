package packet

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentikR1_5_PluginIsRelay (agentik-r1 §5.1, D2; §6.3 c11; ruling 8):
// the plugin is `relay` at 0.2.0, the repo holds no marketplace (the
// commons' lists it), every command allows `Bash(agentik-relay *)`, and the
// hook shim hands the event to $RELAY_BIN, else agentik-relay, else relay.
func TestAgentikR1_5_PluginIsRelay(t *testing.T) {
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, "plugin", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Name, Version, Description string }
	if err := json.Unmarshal(b, &m); err != nil || m.Name != "relay" || m.Version != "0.2.0" ||
		!strings.Contains(m.Description, "/relay:resume") {
		t.Errorf("plugin.json = %+v, %v; want relay at 0.2.0 naming /relay:resume", m, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude-plugin", "marketplace.json")); !os.IsNotExist(err) {
		t.Errorf("the repo's marketplace.json is still here (%v)", err)
	}
	cmds, _ := filepath.Glob(filepath.Join(root, "plugin", "commands", "*.md"))
	if len(cmds) == 0 {
		t.Fatal("no plugin commands")
	}
	for _, c := range cmds {
		b, _ := os.ReadFile(c)
		if !strings.Contains(string(b), "\nallowed-tools: Bash(agentik-relay *)\n") {
			t.Errorf("%s: want allowed-tools: Bash(agentik-relay *)", c)
		}
	}
	shim, err := filepath.Abs(filepath.Join(root, "plugin", "hooks", "run"))
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for _, n := range []string{"agentik-relay", "relay", "custom"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\necho "+n+" \"$@\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	says := func(relayBin string) string {
		cmd := exec.Command("/bin/sh", shim, "Stop")
		cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "RELAY_BIN=" + relayBin}
		cmd.Stdin = strings.NewReader("{}")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("the shim: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	for _, step := range []struct{ relayBin, rm, want string }{
		{filepath.Join(bin, "custom"), "", "custom hook Stop"},
		{"", "", "agentik-relay hook Stop"},
		{"", "agentik-relay", "relay hook Stop"},
		{"", "relay", "{}"},
	} {
		if step.rm != "" {
			os.Remove(filepath.Join(bin, step.rm))
		}
		if got := says(step.relayBin); got != step.want {
			t.Errorf("the shim with RELAY_BIN %q, %s removed: %q, want %q", step.relayBin, step.rm, got, step.want)
		}
	}
}
