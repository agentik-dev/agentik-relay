package packet

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// p1Body splits a plugin markdown file at its frontmatter's closing line:
// the frontmatter's lines and the body after it.
func p1Body(t *testing.T, path string) (string, string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(b), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatalf("%s: no frontmatter", path)
	}
	return parts[1], parts[2]
}

// TestP1_1_CommandsHidden (packet P1, hq design/09 §2 and §9 step 4; R3):
// every command is one only the owner types, so each sets
// disable-model-invocation: true, and its body is the pin's byte for byte.
func TestP1_1_CommandsHidden(t *testing.T) {
	bodies := map[string]string{ // sha256 of each body at a3c501d
		"doctor": "89d0f340e6ae5867a567d22300ad57b1333fad419c62e2aaa5d669db4a451bcd",
		"gate":   "e476c09556c56ddcd151691c353766946f3d42a70a70793f333fb34adf83a05f",
		"plan":   "a7bf3f21f809db282551df2e46cb40d516cd84f06d38b8f5cd3b024c91c3db50",
		"report": "d6d5f720f6b223d845bf232a9d9594b73d523b8e1d1dd77049bac50730f13453",
		"resume": "fbbb5fd706f51d81fc49b0e391e345f55f52d0daf7a56af23e51ff74036e93b8",
		"status": "6cb54aec78fa65ae14b32e87d38d5572d7c99700af81848802ce56ed5a651dcb",
	}
	cmds, _ := filepath.Glob(filepath.Join("..", "..", "plugin", "commands", "*.md"))
	if len(cmds) != len(bodies) {
		t.Fatalf("commands = %v, want the six", cmds)
	}
	for _, c := range cmds {
		front, body := p1Body(t, c)
		if !strings.Contains("\n"+front, "\ndisable-model-invocation: true\n") {
			t.Errorf("%s: want disable-model-invocation: true", c)
		}
		sum := sha256.Sum256([]byte(body))
		name := strings.TrimSuffix(filepath.Base(c), ".md")
		if got := hex.EncodeToString(sum[:]); got != bodies[name] {
			t.Errorf("%s: body sha256 %s, want the pin's %s", c, got, bodies[name])
		}
	}
}

// TestP1_2_AgentsDescribed (R4, R5): each agent keeps a description and
// lists its tools, none of them Skill or Agent, so its sessions carry no
// descriptions.
func TestP1_2_AgentsDescribed(t *testing.T) {
	agents, _ := filepath.Glob(filepath.Join("..", "..", "plugin", "agents", "*.md"))
	if len(agents) != 3 {
		t.Fatalf("agents = %v, want three", agents)
	}
	for _, a := range agents {
		front, _ := p1Body(t, a)
		var desc, tools string
		for _, line := range strings.Split(front, "\n") {
			if v, ok := strings.CutPrefix(line, "description: "); ok {
				desc = strings.TrimSpace(v)
			}
			if v, ok := strings.CutPrefix(line, "tools: "); ok {
				tools = v
			}
		}
		if desc == "" {
			t.Errorf("%s: empty description", a)
		}
		if tools == "" {
			t.Errorf("%s: no tools line", a)
		}
		for _, tool := range strings.Split(tools, ",") {
			if n := strings.TrimSpace(tool); n == "Skill" || n == "Agent" {
				t.Errorf("%s: tools holds %s", a, n)
			}
		}
	}
}

// TestP1_3_UnderTheCeiling (R1): the vendored measure is listed in
// scripts/vendored.sha256 and passes on plugin/: at most 1,024 B.
func TestP1_3_UnderTheCeiling(t *testing.T) {
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, "scripts", "vendored.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(root, "scripts", "plugin-description-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(script)
	if !strings.Contains(string(b), hex.EncodeToString(sum[:])+"  scripts/plugin-description-bytes\n") {
		t.Errorf("scripts/vendored.sha256 does not list scripts/plugin-description-bytes at its sum")
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	cmd := exec.Command(py, "-B", filepath.Join("scripts", "plugin-description-bytes"), "plugin")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("plugin-description-bytes: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "total 578 of 1024 B\n") {
		t.Errorf("plugin-description-bytes said\n%s\nwant total 578 of 1024 B", out)
	}
}
