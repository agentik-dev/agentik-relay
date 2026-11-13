package main

// describe.go is the suite's two reads of this tool (AK8 R2 to R8): what
// the tool is (`describe --json`) and its settings, each with the layer,
// file and line that set it (`describe config --json [KEY]`). Neither
// takes the pin: no git, make, sh or ps runs, no process starts, nothing
// is written.

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/yaml"
)

// describeCwd is where describe looks for the repo; a variable so a test
// can fake it.
var describeCwd = os.Getwd

// describeFS is the file system the config read reads; a variable so a
// test can watch what it asks.
var describeFS effects.FS = effects.OSFS{}

// modulePath is this repo's Go module; its tail is the public repo.
const modulePath = "github.com/agentik-dev/agentik-relay"

const describeUse = "relay describe: want describe --json, or describe config --json [KEY]"

// describeVerb is `describe --json` and `describe config --json [KEY]`;
// anything else exits 2 naming describe --json.
func describeVerb(args []string, stdout, stderr io.Writer) int {
	asJSON, words := false, []string{}
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else {
			words = append(words, a)
		}
	}
	switch {
	case !asJSON:
	case len(words) == 0:
		writeJSON(stdout, describeAnswer())
		return exitOK
	case words[0] == "config" && len(words) <= 2:
		key := ""
		if len(words) == 2 {
			key = words[1]
		}
		return configRead(key, stdout)
	}
	fmt.Fprintln(stderr, describeUse)
	return exitUsage
}

type needRow struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Why      string `json:"why"`
	Optional bool   `json:"optional"`
}

type verbRow struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Top     bool   `json:"top"`
}

type readRow struct {
	Kind string   `json:"kind"`
	Name string   `json:"name"`
	Argv []string `json:"argv"`
}

type describeOut struct {
	Schema  string    `json:"schema"`
	OK      bool      `json:"ok"`
	Tool    string    `json:"tool"`
	Version string    `json:"version"`
	Summary string    `json:"summary"`
	Repo    string    `json:"repo"`
	Plugin  string    `json:"plugin"`
	Needs   []needRow `json:"needs"`
	Verbs   []verbRow `json:"verbs"`
	Reads   []readRow `json:"reads"`
}

func describeAnswer() describeOut {
	return describeOut{
		Schema: "agentik.describe/1", OK: true, Tool: "relay", Version: version,
		Summary: "Runs a queue of rows unattended, each in its own worktree and branch.",
		Repo:    strings.TrimPrefix(modulePath, "github.com/"), Plugin: "relay@agentik",
		Needs: []needRow{
			{"claude", "program", "runs each row's sitting, and must accept --max-turns 1 --version", false},
			{"git", "program", "makes each row's worktree and branch, and reads the repo's root", false},
			{"go", "toolchain", "builds the binary", false},
			{"tmux", "program", "opens a run's windows; without it a run writes file windows", true},
			{"caffeinate", "program", "keeps the machine awake while a run works; the config may turn it off", true},
		},
		Verbs: usageVerbs(),
		Reads: []readRow{
			{"config", "settings", []string{"describe", "config", "--json"}},
			{"doctor", "doctor", []string{"doctor", "--json"}},
			{"status", "run", []string{"status", "--json"}},
		},
	}
}

// usageVerbs is each verb of usage but version, in its order, with
// usage's own words for it: a verb on two lines once, its words joined.
func usageVerbs() []verbRow {
	var b strings.Builder
	usage(&b)
	gap := regexp.MustCompile(`\s{2,}`)
	var rows []verbRow
	at := map[string]int{}
	for _, l := range strings.Split(b.String(), "\n") {
		if !strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "   ") {
			continue
		}
		parts := gap.Split(strings.TrimSpace(l), -1)
		if len(parts) < 2 || parts[0] == "version" {
			continue
		}
		name, words := parts[0], parts[len(parts)-1]
		if i, ok := at[name]; ok {
			rows[i].Summary += "; " + words
			continue
		}
		at[name] = len(rows)
		rows = append(rows, verbRow{name, words, false})
	}
	return rows
}

type layerRow struct {
	Name    string  `json:"name"`
	Path    *string `json:"path"`
	Present bool    `json:"present"`
	Travels bool    `json:"travels"`
}

type sourceRow struct {
	Layer string  `json:"layer"`
	Path  *string `json:"path"`
	Line  *int    `json:"line"`
}

type changeRow struct {
	Kind    string  `json:"kind"`
	Command *string `json:"command"`
	Path    *string `json:"path"`
	Block   *string `json:"block"`
	Note    *string `json:"note"`
}

type valueRow struct {
	Layer string  `json:"layer"`
	Value any     `json:"value"`
	Path  *string `json:"path,omitempty"`
	Line  *int    `json:"line,omitempty"`
}

type settingRow struct {
	Key     string     `json:"key"`
	Value   any        `json:"value"`
	Default any        `json:"default"`
	Type    string     `json:"type"`
	Choices []string   `json:"choices"`
	Summary string     `json:"summary"`
	Source  sourceRow  `json:"source"`
	Change  changeRow  `json:"change"`
	Live    bool       `json:"live"`
	Values  []valueRow `json:"values"`
}

type warningRow struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
	Value  any    `json:"value"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
}

type configOut struct {
	Schema   string       `json:"schema"`
	OK       bool         `json:"ok"`
	Tool     string       `json:"tool"`
	Layers   []layerRow   `json:"layers"`
	Settings []settingRow `json:"settings"`
	Warnings []warningRow `json:"warnings"`
}

type readError struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Remedy  *string `json:"remedy"`
}

type configFailed struct {
	Schema string    `json:"schema"`
	OK     bool      `json:"ok"`
	Tool   string    `json:"tool"`
	Error  readError `json:"error"`
}

// setting is one of the ten keys (R6, R8): its type, choices, summary,
// its value in a Config, and a layer's written text as that type.
type setting struct {
	key, typ, summary string
	choices           []string
	value             func(effects.Config) any
	written           func(string) any
}

func orNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func asWritten(s string) any { return s }

func secret(s string) any {
	if s == "" {
		return nil
	}
	return "(set)"
}

// settings are the ten, in the order a person meets them. Every default is
// what LoadConfig gives with no file (shipped), never typed here.
var settings = []setting{
	{"tmux", "string", "Whether a run opens tmux windows: when tmux is there, never, or always.",
		[]string{"auto", "never", "required"}, func(c effects.Config) any { return c.Tmux }, asWritten},
	{"allowed_tools", "string", "The tools a row's child may use without asking, passed to Claude Code as written.",
		nil, func(c effects.Config) any { return c.AllowedTools }, asWritten},
	{"caffeinate", "boolean", "Whether a run keeps the machine awake while it works.",
		nil, func(c effects.Config) any { return c.Caffeinate }, func(s string) any {
			if b, ok := (yaml.Node{Kind: yaml.Scalar, Str: s}).Bool(); ok {
				return b
			}
			return s
		}},
	{"band", "string", "Whether a run paces its spawns by the session tool's usage band or runs unpaced.",
		[]string{effects.BandPace, effects.BandOff}, func(c effects.Config) any { return c.Band }, asWritten},
	{"queue", "path", "The queue file a run reads, as a path inside the repo.",
		nil, func(c effects.Config) any { return c.Queue }, asWritten},
	{"merge", "string", "Parsed and kept, and read by no code yet: each row's own merge decides how its branch lands.",
		nil, func(c effects.Config) any { return orNull(c.Merge) }, asWritten},
	{"packets", "path", "A folder of packets the gate compares each row's committed packet against.",
		nil, func(c effects.Config) any { return orNull(c.Packets) }, asWritten},
	{"verify_seconds", "integer", "The seconds a row's verify line may run before it is cut and read red.",
		nil, func(c effects.Config) any { return int(c.VerifyLimit() / time.Second) }, func(s string) any {
			if n, ok := (yaml.Node{Kind: yaml.Scalar, Str: s}).Int(); ok {
				return n
			}
			return s
		}},
	{"ntfy.url", "string", "The server a run's notices are sent to; unset, they go to the log.",
		nil, func(c effects.Config) any { return orNull(c.Ntfy.URL) }, asWritten},
	{"ntfy.topic", "secret", "The topic a run's notices are sent to.",
		nil, func(c effects.Config) any { return secret(c.Ntfy.Topic) }, secret},
}

const changeNote = "A run that is going keeps what it started with; the next run reads the change."

// noFiles is a file system holding nothing: LoadConfig over it gives what
// ships.
type noFiles struct{ effects.FS }

func (noFiles) Exists(string) bool { return false }

// absOnly answers a relative path as absent: outside a repo, LoadConfig's
// repo files are relative names and are never read under the cwd (R2).
type absOnly struct{ effects.FS }

func (f absOnly) Exists(p string) bool { return filepath.IsAbs(p) && f.FS.Exists(p) }
func (f absOnly) ReadFile(p string) ([]byte, error) {
	if !filepath.IsAbs(p) {
		return nil, fs.ErrNotExist
	}
	return f.FS.ReadFile(p)
}

// repoAbove is the first folder from dir up holding `.git`, a folder or a
// file; "" when none does.
func repoAbove(dir string) string {
	dir = filepath.Clean(dir)
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		up := filepath.Dir(dir)
		if up == dir {
			return ""
		}
		dir = up
	}
}

// configRead is `describe config --json [KEY]`: the layers, the settings
// (KEY's alone, or none when no setting is KEY) and the warnings.
func configRead(key string, stdout io.Writer) int {
	repo := ""
	if cwd, err := describeCwd(); err == nil && filepath.IsAbs(cwd) {
		repo = repoAbove(cwd)
	}
	fsys := describeFS
	if repo == "" {
		fsys = absOnly{fsys}
	}
	cfg, err := effects.LoadConfig(fsys, getenv, repo)
	if err != nil {
		writeJSON(stdout, configFailed{"agentik.config/1", false, "relay",
			readError{"bad_file", strings.Join(strings.Fields(err.Error()), " "), nil}})
		return exitUsage
	}
	shipped, err := effects.LoadConfig(noFiles{}, func(string) string { return "" }, "")
	if err != nil { // what ships always loads; a failure here is a bug
		writeJSON(stdout, configFailed{"agentik.config/1", false, "relay", readError{"internal", err.Error(), nil}})
		return exitUsage
	}
	paths := effects.ConfigPaths(fsys, getenv, repo)
	names := map[string]string{paths[0]: "user"}
	layers := []layerRow{{"default", nil, true, false}, {"user", &paths[0], fsys.Exists(paths[0]), true}}
	if repo != "" {
		names[paths[1]], names[paths[2]] = "repo", "local"
		layers = append(layers, layerRow{"repo", &paths[1], fsys.Exists(paths[1]), false},
			layerRow{"local", &paths[2], fsys.Exists(paths[2]), false})
	}
	if len(paths) > 3 {
		read := false
		for _, s := range cfg.Sources {
			read = read || s == paths[3]
		}
		names[paths[3]] = "session"
		layers = append(layers, layerRow{"session", &paths[3], read, false})
	}
	out := configOut{"agentik.config/1", true, "relay", layers, []settingRow{}, []warningRow{}}
	for _, s := range settings {
		row := settingRow{Key: s.key, Value: s.value(cfg), Default: s.value(shipped), Type: s.typ, Choices: s.choices,
			Summary: s.summary, Source: sourceRow{Layer: "default"}, Live: true}
		if row.Default != nil {
			row.Values = append(row.Values, valueRow{Layer: "default", Value: row.Default})
		}
		for _, kl := range cfg.Lines {
			if kl.Known && kl.Key == s.key {
				path, line := kl.Path, kl.Line
				row.Values = append(row.Values, valueRow{names[path], s.written(kl.Value), &path, &line})
				row.Source = sourceRow{names[path], &path, &line}
			}
		}
		if row.Values == nil {
			row.Values = []valueRow{}
		}
		row.Change = editFor(s, row, paths[0])
		if key == "" || key == s.key {
			out.Settings = append(out.Settings, row)
		}
	}
	for _, kl := range cfg.Lines {
		if !kl.Known {
			out.Warnings = append(out.Warnings, warningRow{kl.Key, "unknown", nil, kl.Path, kl.Line})
		}
	}
	if _, err := cfg.TmuxMode(); err != nil { // the line in force holds the value refused
		for _, row := range settingRows(cfg, "tmux") {
			out.Warnings = append(out.Warnings, warningRow{"tmux", "choice", cfg.Tmux, row.Path, row.Line})
		}
	}
	writeJSON(stdout, out)
	return exitOK
}

// settingRows is the one line in force of a known key, none when no layer
// sets it.
func settingRows(cfg effects.Config, key string) []effects.KeyLine {
	var last []effects.KeyLine
	for _, kl := range cfg.Lines {
		if kl.Known && kl.Key == key {
			last = []effects.KeyLine{kl}
		}
	}
	return last
}

// editFor is the edit that sets s (R6): the file of the layer in force
// when a person edits it (user, repo, local), else the user's; the block
// at the top level of the user's file, under `relay:` in a repo's.
func editFor(s setting, row settingRow, userFile string) changeRow {
	path := userFile
	switch row.Source.Layer {
	case "user", "repo", "local":
		path = *row.Source.Path
	}
	indent, block := "", []string{}
	if path != userFile {
		block, indent = append(block, "relay:"), "  "
	}
	segs := strings.Split(s.key, ".")
	for i, seg := range segs[:len(segs)-1] {
		block = append(block, indent+strings.Repeat("  ", i)+seg+":")
	}
	block = append(block, indent+strings.Repeat("  ", len(segs)-1)+segs[len(segs)-1]+": "+blockValue(s, row.Value))
	text, note := strings.Join(block, "\n"), changeNote
	return changeRow{Kind: "edit", Path: &path, Block: &text, Note: &note}
}

// blockValue is v as it would stand in a YAML file: VALUE for a secret or
// an unset key, a string quoted where the plain form would read otherwise.
func blockValue(s setting, v any) string {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case string:
		if s.typ == "secret" {
			break
		}
		if x == "" || x != strings.TrimSpace(x) || strings.ContainsAny(x, ":#{}[],&*!|>'\"%@`\\") || strings.HasPrefix(x, "-") || strings.HasPrefix(x, "?") {
			return strconv.Quote(x)
		}
		return x
	}
	return "VALUE"
}
