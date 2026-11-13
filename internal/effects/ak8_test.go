package effects

import (
	"reflect"
	"testing"
)

// ak8Fields is every field of Config the pin had, Lines left out.
type ak8Fields struct {
	Ntfy          struct{ URL, Topic string }
	Tmux          string
	AllowedTools  string
	Caffeinate    bool
	Band          string
	Queue, Merge  string
	Packets       string
	VerifySeconds int
	Unknown       []string
	Sources       []string
	Warnings      []string
	Err           string
}

func ak8Of(c Config, err error) ak8Fields {
	f := ak8Fields{Ntfy: c.Ntfy, Tmux: c.Tmux, AllowedTools: c.AllowedTools, Caffeinate: c.Caffeinate, Band: c.Band,
		Queue: c.Queue, Merge: c.Merge, Packets: c.Packets, VerifySeconds: c.VerifySeconds,
		Unknown: c.Unknown, Sources: c.Sources, Warnings: c.Warnings}
	if err != nil {
		f.Err = err.Error()
	}
	return f
}

// TestAK8_8_PinFieldsAndLines (AK8 line 8, R4): for each of
// TestLoadConfig's three fixtures, LoadConfig's fields are what fa29bcc's
// code returned (recorded at the pin), and Lines names each key's file,
// line and value as written, lowest first, the unknown key among them.
func TestAK8_8_PinFieldsAndLines(t *testing.T) {
	const user, repo, sess = "/h/.config/agent-relay/config.yaml", "/r/.claude/claude-code-session/config.yaml",
		"/h/.local/state/claude-code-session/sessions/s1/resolved.yaml"
	old := []string{repo + ": the old path; move it to .claude/agentik/config.yaml"}
	defaults := ak8Fields{Tmux: "auto", AllowedTools: DefaultAllowedTools, Caffeinate: true, Band: "pace", Queue: "relay/queue.yaml"}

	env := map[string]string{"HOME": "/h"}
	get := func(k string) string { return env[k] }
	fs := &FakeFS{}
	c, err := LoadConfig(fs, get, "/r")
	if got := ak8Of(c, err); !reflect.DeepEqual(got, defaults) {
		t.Errorf("defaults = %+v\nwant %+v", got, defaults)
	}
	if len(c.Lines) != 0 {
		t.Errorf("defaults: Lines = %+v", c.Lines)
	}

	fs.WriteFile(user, []byte(
		"ntfy: {url: https://ntfy.example, topic: night}\ntmux: never\ncaffeinate: false\ncolour: blue\n"), 0o600)
	fs.WriteFile(repo, []byte(
		"other: {x: 1}\nagent-relay:\n  queue: q.yaml\n  merge: branch-only\n  tmux: auto\n"), 0o600)
	env["CLAUDE_CODE_SESSION_ID"] = "s1"
	fs.WriteFile(sess, []byte(
		"agent-relay: {queue: s.yaml, allowed_tools: Bash}\n"+
			inputs(fs, repo, "/r/.claude/claude-code-session/config.local.yaml")), 0o600)
	c, err = LoadConfig(fs, get, "/r")
	layered := ak8Fields{Tmux: "auto", AllowedTools: "Bash", Caffeinate: false, Band: "pace", Queue: "s.yaml", Merge: "branch-only",
		Unknown: []string{user + ": colour"}, Sources: []string{user, repo, sess}, Warnings: old}
	layered.Ntfy.URL, layered.Ntfy.Topic = "https://ntfy.example", "night"
	if got := ak8Of(c, err); !reflect.DeepEqual(got, layered) {
		t.Errorf("layered = %+v\nwant %+v", got, layered)
	}
	want := []KeyLine{
		{Key: "ntfy.url", Path: user, Line: 1, Value: "https://ntfy.example", Known: true},
		{Key: "ntfy.topic", Path: user, Line: 1, Value: "night", Known: true},
		{Key: "tmux", Path: user, Line: 2, Value: "never", Known: true},
		{Key: "caffeinate", Path: user, Line: 3, Value: "false", Known: true},
		{Key: "colour", Path: user, Line: 4, Value: "blue"},
		{Key: "queue", Path: repo, Line: 3, Value: "q.yaml", Known: true},
		{Key: "merge", Path: repo, Line: 4, Value: "branch-only", Known: true},
		{Key: "tmux", Path: repo, Line: 5, Value: "auto", Known: true},
		{Key: "queue", Path: sess, Line: 1, Value: "s.yaml", Known: true},
		{Key: "allowed_tools", Path: sess, Line: 1, Value: "Bash", Known: true},
	}
	if !reflect.DeepEqual(c.Lines, want) {
		t.Errorf("Lines = %+v\nwant %+v", c.Lines, want)
	}

	fs.WriteFile(user, []byte("caffeinate: maybe\n"), 0o600)
	c, err = LoadConfig(fs, get, "/r")
	bad := defaults
	bad.Sources, bad.Warnings = []string{user}, old
	bad.Err = "config " + user + ": line 1: caffeinate: want true or false"
	if got := ak8Of(c, err); !reflect.DeepEqual(got, bad) {
		t.Errorf("bad shape = %+v\nwant %+v", got, bad)
	}
}

// TestAK8_8_NestedUnknownLine: an unknown key under ntfy keeps its line,
// and so does each key of a block mapping.
func TestAK8_8_NestedUnknownLine(t *testing.T) {
	fs := &FakeFS{}
	fs.WriteFile("/h/.config/agentik/relay/config.yaml", []byte("# made-up\nntfy:\n  url: https://ntfy.example\n  room: x\nverify_seconds: 600\n"), 0o600)
	c, err := LoadConfig(fs, func(k string) string { return map[string]string{"HOME": "/h"}[k] }, "/r")
	if err != nil {
		t.Fatal(err)
	}
	p := "/h/.config/agentik/relay/config.yaml"
	want := []KeyLine{{Key: "ntfy.url", Path: p, Line: 3, Value: "https://ntfy.example", Known: true},
		{Key: "ntfy.room", Path: p, Line: 4, Value: "x"}, {Key: "verify_seconds", Path: p, Line: 5, Value: "600", Known: true}}
	if !reflect.DeepEqual(c.Lines, want) || !reflect.DeepEqual(c.Unknown, []string{p + ": ntfy.room"}) {
		t.Fatalf("Lines = %+v, Unknown = %q", c.Lines, c.Unknown)
	}
}
