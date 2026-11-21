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
	bodies := map[string]string{ // sha256 of each body at a3c501d; IR1: the not-on-PATH line names the README's go install
		"doctor": "de95be1ef39110926d22121dd7cec87aa980a1ffad02c517656ba49fd0a9e019", // AK8 R10: its last sentence; RV1 review 1, F2: --plugin-root
		"gate":   "8ada68533e1b7990d5c447f386415ace8a0a94e473b53b3ff03569532b12487f",
		"plan":   "d91bcdadc466bf338c734ba5d48a881be2671bc96f54b869c9dff4bfffaac509",
		"report": "3403cb291e67a5b6a99daf2707f5fc6a8bc27e29dfae673bfe63c6c26af1a570",
		"resume": "0c10271b792a9069fa7d493608b06fd0810c0f08f2927223846e461fda63c7cf",
		"status": "e86b3d239c92ed12231b21d2ee873c2663a1f62708b0f93b86624c73a6ae5af3",
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
