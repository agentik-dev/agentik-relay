package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// ak8World is one describe test's machine: a temp HOME, a temp repo (or a
// folder in none) as the cwd, the env, and every path describe asked of
// its file system.
type ak8World struct {
	home, repo, cwd string
	env             map[string]string
	asked           *[]string
}

// ak8FS records each path asked of the file system it wraps.
type ak8FS struct {
	effects.FS
	asked *[]string
}

func (f ak8FS) Exists(p string) bool { *f.asked = append(*f.asked, p); return f.FS.Exists(p) }
func (f ak8FS) ReadFile(p string) ([]byte, error) {
	*f.asked = append(*f.asked, p)
	return f.FS.ReadFile(p)
}

// ak8Setup fakes getenv, the cwd and the file system describe reads, and
// pinFor to fail the test: describe takes no pin.
func ak8Setup(t *testing.T, inRepo bool) *ak8World {
	t.Helper()
	w := &ak8World{home: t.TempDir(), asked: &[]string{}}
	w.env = map[string]string{"HOME": w.home, effects.MeasureDepthVar: "1"}
	oldEnv, oldPin, oldCwd, oldFS := getenv, pinFor, describeCwd, describeFS
	t.Cleanup(func() { getenv, pinFor, describeCwd, describeFS = oldEnv, oldPin, oldCwd, oldFS })
	getenv = func(k string) string { return w.env[k] }
	pinFor = func() (func(), error) {
		t.Errorf("describe called pinFor")
		return func() {}, errors.New("no pin")
	}
	w.repo = filepath.Join(t.TempDir(), "site")
	w.cwd = w.repo
	if inRepo {
		os.MkdirAll(filepath.Join(w.repo, ".git"), 0o755)
		w.cwd = filepath.Join(w.repo, "sub", "deeper")
	}
	os.MkdirAll(w.cwd, 0o755)
	describeCwd = func() (string, error) { return w.cwd, nil }
	describeFS = ak8FS{effects.OSFS{}, w.asked}
	return w
}

func (w *ak8World) userFile() string {
	return filepath.Join(w.home, ".config", "agentik", "relay", "config.yaml")
}
func (w *ak8World) repoFile() string { return filepath.Join(w.repo, ".claude", "agentik", "config.yaml") }

func ak8Write(t *testing.T, path, text string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ak8Run runs the verb in process and decodes stdout as one object.
func ak8Run(t *testing.T, args ...string) (int, map[string]any, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errb)
	var m map[string]any
	if out.Len() > 0 {
		if !strings.HasSuffix(out.String(), "}\n") || strings.Count(out.String(), "\n") != 1 || strings.ContainsRune(out.String(), 0x1b) {
			t.Errorf("%v: stdout is not one object and a newline: %q", args, out.String())
		}
		if err := json.Unmarshal(out.Bytes(), &m); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
	}
	return code, m, out.String(), errb.String()
}

func ak8Keys(t *testing.T, what string, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("%s: no key %q in %v", what, k, m)
		}
	}
}

func ak8Settings(t *testing.T, m map[string]any) map[string]map[string]any {
	t.Helper()
	list, ok := m["settings"].([]any)
	if !ok {
		t.Fatalf("settings is not a list: %v", m)
	}
	out := map[string]map[string]any{}
	for _, s := range list {
		row := s.(map[string]any)
		out[row["key"].(string)] = row
	}
	return out
}

func ak8Layers(m map[string]any) []string {
	var names []string
	for _, l := range m["layers"].([]any) {
		names = append(names, l.(map[string]any)["name"].(string))
	}
	return names
}

func ak8ValueLayers(s map[string]any) []string {
	var names []string
	for _, v := range s["values"].([]any) {
		names = append(names, v.(map[string]any)["layer"].(string))
	}
	return names
}

// TestAK8_1_Describe (AK8 line 1; R2, R3): one object with every key of
// the contract's describe table, three reads, every top false, no pin;
// describe without --json and with an unknown word exit 2.
func TestAK8_1_Describe(t *testing.T) {
	ak8Setup(t, true)
	start := time.Now()
	code, m, _, _ := ak8Run(t, "describe", "--json")
	if code != exitOK || time.Since(start) > time.Second {
		t.Fatalf("describe --json: exit %d in %v", code, time.Since(start))
	}
	ak8Keys(t, "describe", m, "schema", "ok", "tool", "version", "summary", "repo", "plugin", "needs", "verbs", "reads")
	if m["schema"] != "agentik.describe/1" || m["ok"] != true || m["tool"] != "relay" || m["version"] != version ||
		m["repo"] != "agentik-dev/agentik-relay" || m["plugin"] != "relay@agentik" {
		t.Errorf("describe: %v", m)
	}
	if s, _ := m["summary"].(string); s == "" || len(s) >= 80 || strings.Count(s, ". ") > 0 {
		t.Errorf("summary %q", s)
	}
	needs := map[string]map[string]any{}
	for _, n := range m["needs"].([]any) {
		row := n.(map[string]any)
		ak8Keys(t, "need", row, "name", "kind", "why", "optional")
		needs[row["name"].(string)] = row
	}
	for name, want := range map[string][2]any{"claude": {"program", false}, "git": {"program", false}, "go": {"toolchain", false},
		"tmux": {"program", true}, "caffeinate": {"program", true}} {
		if n := needs[name]; n == nil || n["kind"] != want[0] || n["optional"] != want[1] || n["why"] == "" {
			t.Errorf("need %s = %v", name, n)
		}
	}
	var verbs []string
	for _, v := range m["verbs"].([]any) {
		row := v.(map[string]any)
		ak8Keys(t, "verb", row, "name", "summary", "top")
		if row["top"] != false || row["summary"] == "" {
			t.Errorf("verb %v", row)
		}
		verbs = append(verbs, row["name"].(string))
	}
	if want := []string{"status", "resume", "report", "attach", "plan", "run", "stop", "kill", "gate", "clean", "doctor", "describe"}; !reflect.DeepEqual(verbs, want) {
		t.Errorf("verbs = %q, want %q", verbs, want)
	}
	var usageText bytes.Buffer
	usage(&usageText)
	for _, v := range m["verbs"].([]any) {
		row := v.(map[string]any)
		for _, part := range strings.Split(row["summary"].(string), "; ") {
			if !strings.Contains(usageText.String(), part) {
				t.Errorf("verb %s: %q is not usage's words", row["name"], part)
			}
		}
	}
	reads, _ := json.Marshal(m["reads"])
	if want := `[{"argv":["describe","config","--json"],"kind":"config","name":"settings"},{"argv":["doctor","--json"],"kind":"doctor","name":"doctor"},{"argv":["status","--json"],"kind":"status","name":"run"}]`; string(reads) != want {
		t.Errorf("reads = %s", reads)
	}
	for _, args := range [][]string{{"describe"}, {"describe", "nosuch", "--json"}, {"describe", "config"}} {
		code, _, out, errText := ak8Run(t, args...)
		if code != exitUsage || out != "" || strings.Count(errText, "\n") != 1 || !strings.Contains(errText, "describe --json") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, out, errText)
		}
	}
}

// TestAK8_2_ConfigDefaults (AK8 line 2; R5, R6): no file anywhere: ten
// settings, every key of the contract, every source the default.
func TestAK8_2_ConfigDefaults(t *testing.T) {
	w := ak8Setup(t, true)
	code, m, _, _ := ak8Run(t, "describe", "config", "--json")
	if code != exitOK || m["schema"] != "agentik.config/1" || m["ok"] != true || m["tool"] != "relay" {
		t.Fatalf("exit %d: %v", code, m)
	}
	if got := ak8Layers(m); !reflect.DeepEqual(got, []string{"default", "user", "repo", "local"}) {
		t.Errorf("layers = %q", got)
	}
	layers := m["layers"].([]any)
	if u := layers[1].(map[string]any); u["path"] != w.userFile() || u["present"] != false || u["travels"] != true {
		t.Errorf("user layer %v", u)
	}
	if r := layers[2].(map[string]any); r["path"] != w.repoFile() || r["travels"] != false {
		t.Errorf("repo layer %v", r)
	}
	set := ak8Settings(t, m)
	if len(m["settings"].([]any)) != 10 || len(set) != 10 {
		t.Fatalf("%d settings: %v", len(set), m["settings"])
	}
	for key, s := range set {
		ak8Keys(t, key, s, "key", "value", "default", "type", "choices", "summary", "source", "change", "live", "values")
		src := s["source"].(map[string]any)
		ak8Keys(t, key+".source", src, "layer", "path", "line")
		if src["layer"] != "default" || src["path"] != nil || src["line"] != nil {
			t.Errorf("%s: source %v", key, src)
		}
		ch := s["change"].(map[string]any)
		ak8Keys(t, key+".change", ch, "kind", "command", "path", "block", "note")
		if ch["kind"] != "edit" || ch["path"] != w.userFile() || ch["note"] == nil || s["live"] != true {
			t.Errorf("%s: change %v live %v", key, ch, s["live"])
		}
		if sum, _ := s["summary"].(string); sum == "" || len(sum) >= 100 {
			t.Errorf("%s: summary %q", key, sum)
		}
	}
	if s := set["tmux"]; s["value"] != "auto" || s["default"] != "auto" || s["type"] != "string" {
		t.Errorf("tmux %v", s)
	}
	if s := set["verify_seconds"]; s["value"] != 900.0 || s["default"] != 900.0 || s["type"] != "integer" {
		t.Errorf("verify_seconds %v", s)
	}
	if s := set["caffeinate"]; s["value"] != true || s["default"] != true || s["type"] != "boolean" {
		t.Errorf("caffeinate %v", s)
	}
	if s := set["ntfy.topic"]; s["value"] != nil || s["type"] != "secret" {
		t.Errorf("ntfy.topic %v", s)
	}
	if s := set["band"]; s["value"] != effects.BandPace || !reflect.DeepEqual(s["choices"], []any{effects.BandPace, effects.BandOff}) {
		t.Errorf("band %v", s)
	}
	if s := set["queue"]; s["value"] != effects.DefaultQueue || s["type"] != "path" {
		t.Errorf("queue %v", s)
	}
	if ws, _ := m["warnings"].([]any); ws == nil || len(ws) != 0 {
		t.Errorf("warnings = %v", m["warnings"])
	}
}

// TestAK8_3_UserLine (AK8 line 3): a user file's tmux at line 2.
func TestAK8_3_UserLine(t *testing.T) {
	w := ak8Setup(t, true)
	ak8Write(t, w.userFile(), "# a made-up user file\ntmux: never\n")
	code, m, _, _ := ak8Run(t, "describe", "config", "--json")
	s := ak8Settings(t, m)["tmux"]
	if code != exitOK || s["value"] != "never" {
		t.Fatalf("exit %d: %v", code, s)
	}
	if src, _ := json.Marshal(s["source"]); string(src) != fmt.Sprintf(`{"layer":"user","line":2,"path":%q}`, w.userFile()) {
		t.Errorf("source %s", src)
	}
	if got := ak8ValueLayers(s); !reflect.DeepEqual(got, []string{"default", "user"}) {
		t.Errorf("values %v", s["values"])
	}
	ch := s["change"].(map[string]any)
	if ch["kind"] != "edit" || ch["path"] != w.userFile() || ch["block"] != "tmux: never" || ch["command"] != nil {
		t.Errorf("change %v", ch)
	}
	if layers := m["layers"].([]any); layers[1].(map[string]any)["present"] != true {
		t.Errorf("user layer %v", layers[1])
	}
}

// TestAK8_4_RepoLine (AK8 line 4): the repo's relay section over the user.
func TestAK8_4_RepoLine(t *testing.T) {
	w := ak8Setup(t, true)
	ak8Write(t, w.userFile(), "# a made-up user file\ntmux: never\n")
	ak8Write(t, w.repoFile(), "# a made-up repo\nsession:\n  model: x\nrelay:\n  tmux: required\n")
	code, m, _, _ := ak8Run(t, "describe", "config", "--json")
	s := ak8Settings(t, m)["tmux"]
	if code != exitOK || s["value"] != "required" {
		t.Fatalf("exit %d: %v", code, s)
	}
	if src, _ := json.Marshal(s["source"]); string(src) != fmt.Sprintf(`{"layer":"repo","line":5,"path":%q}`, w.repoFile()) {
		t.Errorf("source %s", src)
	}
	if got := ak8ValueLayers(s); !reflect.DeepEqual(got, []string{"default", "user", "repo"}) {
		t.Errorf("values %v", s["values"])
	}
	if ch := s["change"].(map[string]any); ch["path"] != w.repoFile() || ch["block"] != "relay:\n  tmux: required" {
		t.Errorf("change %v", ch)
	}
	if ws := m["warnings"].([]any); len(ws) != 0 {
		t.Errorf("warnings %v", ws)
	}
}

// TestAK8_5_SecretTopic (AK8 line 5): a topic reads "(set)" and its text
// is nowhere in the output.
func TestAK8_5_SecretTopic(t *testing.T) {
	w := ak8Setup(t, true)
	const topic = "made-up-topic-q7"
	ak8Write(t, w.userFile(), "ntfy:\n  url: https://ntfy.example.invalid\n  topic: "+topic+"\n")
	code, m, out, _ := ak8Run(t, "describe", "config", "--json")
	set := ak8Settings(t, m)
	s := set["ntfy.topic"]
	if code != exitOK || s["value"] != "(set)" || strings.Contains(out, topic) {
		t.Fatalf("exit %d: %v\n%s", code, s, out)
	}
	if src := s["source"].(map[string]any); src["line"] != 3.0 || src["layer"] != "user" {
		t.Errorf("source %v", src)
	}
	for _, v := range s["values"].([]any) {
		if v.(map[string]any)["value"] != "(set)" {
			t.Errorf("values %v", s["values"])
		}
	}
	if u := set["ntfy.url"]; u["value"] != "https://ntfy.example.invalid" || u["source"].(map[string]any)["line"] != 2.0 {
		t.Errorf("ntfy.url %v", u)
	}
}

// TestAK8_6_KeyedWarningsBadFile (AK8 line 6).
func TestAK8_6_KeyedWarningsBadFile(t *testing.T) {
	w := ak8Setup(t, true)
	code, m, _, _ := ak8Run(t, "describe", "config", "--json", "tmux")
	if s := m["settings"].([]any); code != exitOK || len(s) != 1 || s[0].(map[string]any)["key"] != "tmux" || len(m["layers"].([]any)) != 4 {
		t.Errorf("keyed: exit %d: %v", code, m)
	}
	code, m, _, _ = ak8Run(t, "describe", "config", "nosuch", "--json")
	if s, ok := m["settings"].([]any); code != exitOK || !ok || len(s) != 0 || m["ok"] != true {
		t.Errorf("nosuch: exit %d: %v", code, m)
	}
	ak8Write(t, w.userFile(), "# made-up\ntmxu: never\n")
	_, m, _, _ = ak8Run(t, "describe", "config", "--json")
	if ws := m["warnings"].([]any); len(ws) != 1 {
		t.Errorf("unknown: %v", ws)
	} else if row, _ := json.Marshal(ws[0]); string(row) != fmt.Sprintf(`{"key":"tmxu","line":2,"path":%q,"reason":"unknown","value":null}`, w.userFile()) {
		t.Errorf("unknown row %s", row)
	}
	ak8Write(t, w.userFile(), "tmux: sometimes\n")
	_, m, _, _ = ak8Run(t, "describe", "config", "--json")
	if ws := m["warnings"].([]any); len(ws) != 1 {
		t.Errorf("choice: %v", ws)
	} else if row, _ := json.Marshal(ws[0]); string(row) != fmt.Sprintf(`{"key":"tmux","line":1,"path":%q,"reason":"choice","value":"sometimes"}`, w.userFile()) {
		t.Errorf("choice row %s", row)
	}
	ak8Write(t, w.userFile(), "ntfy:\n\turl: x\n")
	code, m, _, _ = ak8Run(t, "describe", "config", "--json")
	e, _ := m["error"].(map[string]any)
	if code != exitUsage || m["ok"] != false || m["schema"] != "agentik.config/1" || m["tool"] != "relay" || e == nil ||
		e["code"] != "bad_file" || !strings.Contains(e["message"].(string), w.userFile()) ||
		!strings.Contains(e["message"].(string), "line 2") || e["remedy"] != nil {
		t.Errorf("bad file: exit %d: %v", code, m)
	}
}

// TestAK8_7_OutsideRepoAndSession (AK8 line 7): no repo or local layer
// and no relative path asked, though the cwd holds a repo file; a session
// layer, absent, when CLAUDE_CODE_SESSION_ID is set.
func TestAK8_7_OutsideRepoAndSession(t *testing.T) {
	w := ak8Setup(t, false)
	ak8Write(t, filepath.Join(w.cwd, ".claude", "agentik", "config.yaml"), "relay:\n  tmux: required\n")
	code, m, _, _ := ak8Run(t, "describe", "config", "--json")
	if got := ak8Layers(m); code != exitOK || !reflect.DeepEqual(got, []string{"default", "user"}) {
		t.Errorf("exit %d, layers %q", code, got)
	}
	if s := ak8Settings(t, m)["tmux"]; s["value"] != "auto" {
		t.Errorf("tmux %v: a file under the cwd was read", s)
	}
	for _, p := range *w.asked {
		if !filepath.IsAbs(p) || strings.HasPrefix(p, w.cwd) {
			t.Errorf("asked %q outside a repo", p)
		}
	}
	w.env["CLAUDE_CODE_SESSION_ID"] = "made-up-session"
	code, m, _, _ = ak8Run(t, "describe", "config", "--json")
	layers := m["layers"].([]any)
	last := layers[len(layers)-1].(map[string]any)
	if code != exitOK || last["name"] != "session" || last["present"] != false || last["travels"] != false || !strings.HasPrefix(last["path"].(string), w.home) {
		t.Errorf("exit %d, session layer %v", code, last)
	}
}

// TestAK8_9_DoctorRead (AK8 line 9; R9): the doctor read's new keys; ok
// true even with a check failing, failed counting it, exit 2.
func TestAK8_9_DoctorRead(t *testing.T) {
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	read := func(wantCode, wantFailed int) {
		t.Helper()
		code, m, _, _ := ak8Run(t, "doctor", "--json")
		ak8Keys(t, "doctor", m, "schema", "ok", "tool", "failed", "warned", "checks")
		failed, warned := 0, 0
		for _, c := range m["checks"].([]any) {
			row := c.(map[string]any)
			ak8Keys(t, "check", row, "name", "ok", "warn", "detail", "status", "remedy")
			want := "ok"
			if row["ok"] != true {
				want = "fail"
				failed++
			} else if row["warn"] == true {
				want = "warn"
				warned++
			}
			if row["status"] != want {
				t.Errorf("check %v: status want %s", row, want)
			}
		}
		if code != wantCode || m["schema"] != "agentik.doctor/1" || m["ok"] != true || m["tool"] != "relay" ||
			m["failed"] != float64(failed) || failed != wantFailed || m["warned"] != float64(warned) {
			t.Errorf("doctor --json: exit %d, failed %d: %v", code, failed, m)
		}
	}
	read(exitOK, 0)
	var text bytes.Buffer
	run([]string{"doctor"}, strings.NewReader(""), &text, &bytes.Buffer{})
	if !strings.HasPrefix(text.String(), "doctor: all ") {
		t.Errorf("doctor's text: %s", text.String())
	}
	claudeProbe = func(string) error { return errors.New("made-up failure") }
	read(exitUnwell, 1)
}

// TestAK8_10_StatusRead (AK8 line 10; R11): status --json opens with
// schema and ok, every key of night.Status after them in its order.
func TestAK8_10_StatusRead(t *testing.T) {
	home := t.TempDir()
	old := getenv
	t.Cleanup(func() { getenv = old })
	getenv = fakeEnv(home)
	code, m, out, _ := ak8Run(t, "status", "--json")
	if code != exitOK || m["schema"] != "agentik.relay.run/1" || m["ok"] != true {
		t.Fatalf("exit %d: %s", code, out)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	dec.Token()
	var keys []string
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var skip json.RawMessage
		dec.Decode(&skip)
	}
	want := []string{"schema", "ok", "night", "state", "reason", "rows", "burn", "next", "resume", "pid", "started", "root"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %q, want %q", keys, want)
	}
}

// TestAK8_11_SuiteReads (AK8 line 11): the suite's launcher over the built
// binary, in a temp HOME holding line 3's user file. Skipped unless
// AGENTIK_COMMONS names a commons checkout; it runs in make check only.
func TestAK8_11_SuiteReads(t *testing.T) {
	commons := os.Getenv("AGENTIK_COMMONS")
	suite := filepath.Join(commons, "plugins", "suite", "bin", "agentik-suite")
	if fi, err := os.Stat(suite); commons == "" || err != nil || fi.IsDir() {
		t.Skip("AGENTIK_COMMONS names no commons checkout with plugins/suite/bin/agentik-suite")
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	bin, home, cwd := filepath.Join(dir, "bin"), filepath.Join(dir, "home"), filepath.Join(dir, "work")
	os.MkdirAll(bin, 0o755)
	os.MkdirAll(cwd, 0o755)
	if out, err := exec.Command("go", "build", "-ldflags", pidHomeFlag(dir), "-o", filepath.Join(bin, "agentik-relay"), ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	ak8Write(t, filepath.Join(home, ".config", "agentik", "relay", "config.yaml"), "# a made-up user file\ntmux: never\n")
	suiteRun := func(args ...string) string {
		cmd := exec.Command(suite, args...)
		cmd.Dir = cwd
		cmd.Env = []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin", "TMPDIR=" + dir, "LANG=C.UTF-8"}
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("agentik-suite %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	keyed, listed := suiteRun("config", "relay.tmux", "--json"), suiteRun("config")
	if p := os.Getenv("AK8_SUITE_OUT"); p != "" {
		os.WriteFile(p, []byte("$ agentik config relay.tmux --json\n"+keyed+"\n$ agentik config\n"+listed), 0o644)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(keyed), &m); err != nil {
		t.Fatalf("config relay.tmux --json: %v\n%s", err, keyed)
	}
	s, _ := m["setting"].(map[string]any)
	if m["ok"] != true || m["tool"] != "relay" || m["key"] != "tmux" || s == nil ||
		s["source"].(map[string]any)["line"] != 2.0 || s["change"].(map[string]any)["kind"] != "edit" {
		t.Errorf("config relay.tmux --json: %s", keyed)
	}
	line := ""
	for _, l := range strings.Split(listed, "\n") {
		if strings.Contains(l, "relay") && strings.Contains(l, "answers") {
			line = l
		}
	}
	if line == "" || !strings.Contains(listed, "10 settings") || !strings.Contains(listed, "1 changed") {
		t.Errorf("config: relay not listed as answers, 10 settings, 1 changed:\n%s", listed)
	}
}
