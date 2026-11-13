package main

// The fix round of AK8's review 1: each finding's test, the review's probe
// among them.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ak8Text runs the verb in process, whatever its output's shape.
func ak8Text(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

// ak8Doctor fakes what doctor runs (the programs, the claude probe, git's
// worktree list and effectsFor's git and state) and points its repo and
// its describe at one world.
func ak8Doctor(t *testing.T) *ak8World {
	t.Helper()
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	w := ak8Setup(t, true)
	pinFor = func() (func(), error) { return func() {}, nil } // doctor and status take the package's pin (TestMain)
	w.env["XDG_STATE_HOME"] = filepath.Join(w.home, "state")
	fakeTop(t, w.repo)
	return w
}

// ak8Sum is path's sha256 as a session file records it, "null" when absent.
func ak8Sum(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "null"
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// ak8PlugReads are the verbs the plug adds, and doctor in both forms.
var ak8PlugReads = [][]string{
	{"describe", "--json"}, {"describe", "config", "--json"}, {"describe", "config", "--json", "ntfy.topic"},
	{"describe", "config", "--json", "ntfy.url"}, {"describe"}, {"doctor"}, {"doctor", "--json"},
	{"status", "--json"}, {"status"},
}

// ak8NoText runs every plug read and fails on any of words in any byte of
// stdout or stderr; it returns doctor's two outputs.
func ak8NoText(t *testing.T, what string, words ...string) (text, js string) {
	t.Helper()
	for _, args := range ak8PlugReads {
		_, out, errb := ak8Text(args...)
		for _, word := range words {
			if strings.Contains(out, word) || strings.Contains(errb, word) {
				t.Errorf("%s: %v prints %q:\n%s%s", what, args, word, out, errb)
			}
		}
		switch strings.Join(args, " ") {
		case "doctor":
			text = out
		case "doctor --json":
			js = out
		}
	}
	return text, js
}

// TestAK8_F2_NoTopicAnywhere (review 1, R1-F2): a marked topic in each
// layer that can hold one, by the new names and by the old, each layer in
// force in turn, reaches no byte of any plug read or of doctor's text;
// doctor's ntfy line reads `configured (topic set)` in both forms.
func TestAK8_F2_NoTopicAnywhere(t *testing.T) {
	const id = "made-up-session"
	ntfy := func(indent, topic string) string {
		return indent + "ntfy:\n" + indent + "  url: https://ntfy.example.invalid\n" + indent + "  topic: " + topic + "\n"
	}
	for _, names := range []string{"new", "old"} {
		for depth := 1; depth <= 4; depth++ {
			w := ak8Doctor(t)
			user, repo, sec := w.userFile(), w.repoFile(), "relay:\n"
			sess := filepath.Join(w.home, "state", "agentik-session", "sessions", id, "resolved.yaml")
			if names == "old" {
				user = filepath.Join(w.home, ".config", "agent-relay", "config.yaml")
				repo = filepath.Join(w.repo, ".claude", "claude-code-session", "config.yaml")
				sess = filepath.Join(w.home, "state", "claude-code-session", "sessions", id, "resolved.yaml")
				sec = "agent-relay:\n"
			}
			local := filepath.Join(filepath.Dir(repo), "config.local.yaml")
			marks := []string{"zq-" + names + "-user-topic", "zq-" + names + "-repo-topic", "zq-" + names + "-local-topic", "zq-" + names + "-session-topic"}
			ak8Write(t, user, ntfy("", marks[0]))
			if depth >= 2 {
				ak8Write(t, repo, "# made-up\n"+sec+ntfy("  ", marks[1]))
			}
			if depth >= 3 {
				ak8Write(t, local, sec+ntfy("  ", marks[2]))
			}
			if depth >= 4 {
				w.env["CLAUDE_CODE_SESSION_ID"] = id
				ak8Write(t, sess, "provider:\n  inputs:\n    - path: "+repo+"\n      sha256: "+ak8Sum(repo)+
					"\n    - path: "+local+"\n      sha256: "+ak8Sum(local)+"\n"+sec+ntfy("  ", marks[3]))
			}
			what := names + " names, " + []string{"", "user", "repo", "local", "session"}[depth] + " in force"
			_, m, _, _ := ak8Run(t, "describe", "config", "--json")
			if src := ak8Settings(t, m)["ntfy.topic"]["source"].(map[string]any); src["layer"] != []string{"", "user", "repo", "local", "session"}[depth] {
				t.Errorf("%s: the topic's source %v", what, src)
			}
			text, js := ak8NoText(t, what, marks...)
			if !strings.Contains(text, "configured (topic set)") || !strings.Contains(js, `"detail":"configured (topic set)"`) {
				t.Errorf("%s: doctor's ntfy line:\n%s\n%s", what, text, js)
			}
		}
	}
}

// TestAK8_F2_ReviewProbe is review 1's probe of R1-F2 as written: the
// topic in the user file, doctor --json.
func TestAK8_F2_ReviewProbe(t *testing.T) {
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	lookPath = func(n string) (string, error) { return "/fake/" + n, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	ak8Write(t, filepath.Join(getenv("HOME"), ".config/agentik/relay/config.yaml"), "ntfy:\n  url: https://ntfy.example\n  topic: s3cr3t-topic-zq\n")
	_, _, out, _ := ak8Run(t, "doctor", "--json")
	for _, line := range strings.Split(out, "{") {
		if strings.Contains(line, `"name":"ntfy"`) {
			t.Logf("ntfy check: {%s", strings.TrimSuffix(strings.TrimSuffix(line, ","), "]}\n"))
		}
	}
	if strings.Contains(out, "s3cr3t-topic-zq") {
		t.Errorf("doctor --json prints the topic")
	}
}

// TestAK8_N4_ParseErrorQuotesNoTopic (review 1, N4): a parse error on the
// topic's line names the line and what is wrong, and quotes no character
// of the topic, in the config read and in doctor.
func TestAK8_N4_ParseErrorQuotesNoTopic(t *testing.T) {
	for _, c := range []struct{ topic, quoted string }{
		{`"zq-made-up\Q-topic"`, `\Q`},
		{"@Zq-made-up-topic", "'@'"},
		{"`Zq-made-up-topic", "'`'"},
	} {
		w := ak8Doctor(t)
		ak8Write(t, w.userFile(), "ntfy:\n  url: https://ntfy.example.invalid\n  topic: "+c.topic+"\n")
		code, m, _, _ := ak8Run(t, "describe", "config", "--json")
		msg, _ := m["error"].(map[string]any)["message"].(string)
		t.Logf("%s: bad_file %q", c.topic, msg)
		if code != exitUsage || !strings.Contains(msg, w.userFile()) || !strings.Contains(msg, "line 3") {
			t.Errorf("%s: exit %d, message %q", c.topic, code, msg)
		}
		ak8NoText(t, c.topic, c.quoted, "Zq", "zq")
	}
}
