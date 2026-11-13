package main

// The fix round of AK8's review 1: each finding's test, the review's probe
// among them.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
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

// TestAK8_F1_ValueIsWhatTheRunReads (review 1, R1-F1): each of the ten
// keys, set in the user file to a form whose raw field could differ from
// what the run uses, reads as the run reads it; `changed` (value against
// default) and the block follow from that value.
func TestAK8_F1_ValueIsWhatTheRunReads(t *testing.T) {
	for _, c := range []struct {
		file, key string
		value     any
		changed   bool
		block     string
	}{
		{"tmux:\n", "tmux", "auto", false, "tmux: auto"},
		{"tmux: ''\n", "tmux", "auto", false, "tmux: auto"},
		{"allowed_tools: ''\n", "allowed_tools", effects.DefaultAllowedTools, false, "allowed_tools: " + effects.DefaultAllowedTools},
		{"allowed_tools:\n", "allowed_tools", effects.DefaultAllowedTools, false, "allowed_tools: " + effects.DefaultAllowedTools},
		{"caffeinate: false\n", "caffeinate", false, true, "caffeinate: false"},
		{"band: off\n", "band", "off", true, "band: off"},
		{"queue: ./relay/../relay/queue.yaml\n", "queue", effects.DefaultQueue, false, "queue: " + effects.DefaultQueue},
		{"merge: ''\n", "merge", nil, false, "merge: VALUE"},
		{"packets: ''\n", "packets", nil, false, "packets: VALUE"},
		{"verify_seconds: 900\n", "verify_seconds", 900.0, false, "verify_seconds: 900"},
		{"ntfy:\n  url: ''\n", "ntfy.url", nil, false, "ntfy:\n  url: VALUE"},
		{"ntfy:\n  topic: ''\n", "ntfy.topic", nil, false, "ntfy:\n  topic: VALUE"},
	} {
		w := ak8Setup(t, true)
		ak8Write(t, w.userFile(), c.file)
		code, m, _, _ := ak8Run(t, "describe", "config", "--json", c.key)
		s := ak8Settings(t, m)[c.key]
		changed := s["default"] != nil && !reflect.DeepEqual(s["value"], s["default"])
		block := s["change"].(map[string]any)["block"]
		if code != exitOK || !reflect.DeepEqual(s["value"], c.value) || changed != c.changed || block != c.block {
			t.Errorf("%q: exit %d, value %#v (want %#v), changed %v, block %q (want %q)", c.file, code, s["value"], c.value, changed, block, c.block)
		}
		if src := s["source"].(map[string]any); src["layer"] != "user" {
			t.Errorf("%q: source %v", c.file, src)
		}
		if ws := m["warnings"].([]any); len(ws) != 0 {
			t.Errorf("%q: warnings %v", c.file, ws)
		}
	}
}

// TestAK8_F1_ReviewProbe is review 1's probe of R1-F1 as written.
func TestAK8_F1_ReviewProbe(t *testing.T) {
	for _, s := range []string{"tmux:\n", "allowed_tools: ''\n"} {
		w := ak8Setup(t, true)
		ak8Write(t, w.userFile(), s)
		_, m, _, _ := ak8Run(t, "describe", "config", "--json")
		k := map[string]string{"tmux:\n": "tmux", "allowed_tools: ''\n": "allowed_tools"}[s]
		r := ak8Settings(t, m)[k]
		t.Logf("%s value %q default %q block %q", k, r["value"], r["default"], r["change"].(map[string]any)["block"])
		if r["value"] != r["default"] {
			t.Errorf("%s: value %q is not what the run reads", k, r["value"])
		}
	}
}

// TestAK8_F3_UnknownBlockKeyLine (review 1, R1-F3): an unknown key that
// holds a block is reported at its own line; the review's three inputs.
func TestAK8_F3_UnknownBlockKeyLine(t *testing.T) {
	for _, c := range []struct {
		file, key string
		line      float64
	}{
		{"ntfi:\n  topic: t\n", "ntfi", 1},
		{"relay:\n\n  tmux: never\n", "relay", 1},
		{"tmux: never\nextra:\n- a\n", "extra", 2},
		{"# made-up\nntfy:\n  room:\n    a: 1\n", "ntfy.room", 3},
	} {
		w := ak8Setup(t, true)
		ak8Write(t, w.userFile(), c.file)
		_, m, _, _ := ak8Run(t, "describe", "config", "--json")
		ws := m["warnings"].([]any)
		t.Logf("%q %v", c.file, ws)
		if len(ws) != 1 {
			t.Errorf("%q: warnings %v", c.file, ws)
			continue
		}
		if row := ws[0].(map[string]any); row["key"] != c.key || row["line"] != c.line || row["reason"] != "unknown" {
			t.Errorf("%q: row %v, want %s at line %v", c.file, row, c.key, c.line)
		}
	}
}

// TestAK8_N1_BlockNamesTheFilesSection (review 1, N1): an edit in a repo
// or local file names the section that file already uses, as LoadConfig
// reads it (relay when both stand, else agent-relay), so a paste never
// gives the file a second section.
func TestAK8_N1_BlockNamesTheFilesSection(t *testing.T) {
	for _, c := range []struct {
		dir, name, text, block string
	}{
		{"claude-code-session", "config.yaml", "# made-up\nagent-relay:\n  queue: relay/q.yaml\n  tmux: required\n", "agent-relay:\n  tmux: required"},
		{"claude-code-session", "config.local.yaml", "agent-relay:\n  tmux: required\n", "agent-relay:\n  tmux: required"},
		{"agentik", "config.yaml", "agent-relay:\n  tmux: required\n", "agent-relay:\n  tmux: required"},
		{"agentik", "config.yaml", "agent-relay:\n  band: off\nrelay:\n  tmux: required\n", "relay:\n  tmux: required"},
		{"agentik", "config.local.yaml", "relay:\n  tmux: required\n", "relay:\n  tmux: required"},
	} {
		w := ak8Setup(t, true)
		ak8Write(t, w.userFile(), "tmux: never\n")
		file := filepath.Join(w.repo, ".claude", c.dir, c.name)
		if c.dir == "claude-code-session" && c.name != "config.yaml" { // the old folder is read only beside its repo file
			ak8Write(t, filepath.Join(filepath.Dir(file), "config.yaml"), "agent-relay:\n  band: off\n")
		}
		ak8Write(t, file, c.text)
		_, m, _, _ := ak8Run(t, "describe", "config", "--json", "tmux")
		s := ak8Settings(t, m)["tmux"]
		ch := s["change"].(map[string]any)
		t.Logf("%s/%s: path %v block %q", c.dir, c.name, ch["path"], ch["block"])
		if s["value"] != "required" || ch["path"] != file || ch["block"] != c.block {
			t.Errorf("%s/%s: value %v, change %v, want block %q", c.dir, c.name, s["value"], ch, c.block)
		}
	}
}

// TestAK8_R2F1_NoVerbQuotesTheTopic (review 2, R2-F1): a topic the reader
// refuses by quoting one of its characters (an escape, an indicator), in
// each of the five files the relay reads, block and flow. plan, its --json,
// and run --orchestrate's refusal (with --no-notify and without) name the
// file, the line and the fault, and no byte of the topic. run and gate take
// the account's pid file, so no test runs them: each reads its config
// through loadConfig, as plan does (TestAK8_R2F1_EveryVerbReadsMasked).
func TestAK8_R2F1_NoVerbQuotesTheTopic(t *testing.T) {
	const url = "url: https://ntfy.example.invalid"
	for _, shape := range []struct{ block, flow, fault string }{
		{`topic: "ab\qxzv"`, "", "unsupported escape"},
		{"topic: @qxzv", "", "reserved indicator"},
		{"", `{` + url + `, topic: "qxzv\z"}`, "unsupported escape"},
	} {
		for _, file := range []string{"user", "old user", "repo", "old repo", "local"} {
			w := ak8Doctor(t)
			path, sec := w.userFile(), ""
			switch file {
			case "old user":
				path = filepath.Join(w.home, ".config", "agent-relay", "config.yaml")
			case "repo":
				path, sec = w.repoFile(), "relay:\n"
			case "old repo":
				path, sec = filepath.Join(w.repo, ".claude", "claude-code-session", "config.yaml"), "agent-relay:\n"
			case "local":
				path, sec = filepath.Join(filepath.Dir(w.repoFile()), "config.local.yaml"), "relay:\n"
			}
			ind, line := "", 1
			if sec != "" {
				ind, line = "  ", 2
			}
			text := sec + ind + "ntfy: " + shape.flow + "\n"
			if shape.flow == "" {
				text, line = sec+ind+"ntfy:\n"+ind+"  "+url+"\n"+ind+"  "+shape.block+"\n", line+2
			}
			ak8Write(t, path, text)
			what := file + " " + strings.TrimSpace(shape.block+shape.flow)
			want := []string{path, "line " + string(rune('0'+line)) + ": " + shape.fault}
			outs := map[string]string{}
			_, out, errb := ak8Text("plan")
			outs["plan"] = out + errb
			_, out, errb = ak8Text("plan", "--json")
			outs["plan --json"] = out + errb
			outs["orchestrate"] = orchestrateRefusalFor(false)
			outs["orchestrate --no-notify"] = orchestrateRefusalFor(true)
			for verb, s := range outs {
				for _, word := range want {
					if !strings.Contains(s, word) {
						t.Errorf("%s, %s: no %q in %q", what, verb, word, s)
					}
				}
				for _, leak := range []string{"qxzv", `\q`, `\z`, "@"} {
					if strings.Contains(s, leak) {
						t.Errorf("%s, %s: prints %q: %q", what, verb, leak, s)
					}
				}
			}
		}
	}
}

// TestAK8_R2F1_EveryVerbReadsMasked (review 2, R2-F1): the package reads
// its config through loadConfig alone, so gate and run, which no test runs,
// print a parse refusal as plan does; describe's read of the shipped
// defaults (no file) is the one other call. loadConfig itself masks.
func TestAK8_R2F1_EveryVerbReadsMasked(t *testing.T) {
	srcs, _ := filepath.Glob("*.go")
	calls := 0
	for _, src := range srcs {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "effects.LoadConfig(") {
				calls++
				if src != "describe.go" || !(strings.Contains(l, "noFiles{}") || strings.Contains(l, "LoadConfig(fsys, env, repo)")) {
					t.Errorf("%s:%d reads the config past loadConfig: %s", src, i+1, strings.TrimSpace(l))
				}
			}
		}
	}
	if calls != 2 {
		t.Errorf("effects.LoadConfig called %d times in the package, want 2 (loadConfig and the shipped read)", calls)
	}
	home := t.TempDir()
	ak8Write(t, filepath.Join(home, ".config", "agentik", "relay", "config.yaml"), "ntfy:\n  topic: \"zq\\qxzv\"\n")
	_, err := loadConfig(effects.OSFS{}, func(k string) string { return map[string]string{"HOME": home}[k] }, "")
	if err == nil || strings.Contains(err.Error(), "qxzv") || !strings.HasSuffix(err.Error(), ": line 2: unsupported escape") {
		t.Errorf("loadConfig: %v", err)
	}
}
