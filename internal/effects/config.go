package effects

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/xdg"
	"github.com/agentik-dev/agentik-relay/internal/yaml"
)

// Config is the relay's configuration after the §9 layers, lowest first:
// the shipped defaults; the user file; the repo's `relay` section; its
// config.local.yaml's; the session's resolved file when present and fresh
// (session-layer-r1 §2). Flags are the caller's, last.
// Unknown lists keys no layer knows ("<file>: <key>"), reported by doctor and
// never fatal (D23); Sources lists the files read. Packets is the packets
// source (key `packets`, AR29 act 3): an absolute dir the relay only reads,
// against which the gate compares each row's committed packet; relative is
// an error, unset compares nothing. Queue is the queue's clean path under
// the repo (ar32a review round 2): LoadConfig makes it so once, an absolute
// path under the repo relative to it, and a path that leaves the repo an
// error, so every reader of the queue sees the one spelling. Warnings are
// the old names a layer still uses (agentik-r1 §6.3 c4, c6): the repo's old
// file whenever it exists, and a file holding both sections; and a session
// layer not read, with why (session-layer-r1 §2.3); doctor's config check
// warns on them, never fails.
// VerifySeconds is the repo's verify limit (key `verify_seconds`, AR59 act
// 1, the settled entry 10): an integer from 1 to MaxVerifySeconds; 0 is
// unset, and VerifyLimit is then Timeout.
// Band is the session band's mode (key `band`, band-pacing-r1 §2.1; AR80):
// BandPace, the default, or BandOff; any other value is an error naming its
// line.
type Config struct {
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
}

// The shipped defaults (§7's tools, §3's queue path).
const (
	// DefaultAllowedTools keeps its bare `Bash` (AR90 ruling 1). The docs
	// say a run that starts in auto mode drops a bare `Bash` allow entry,
	// the classifier judging each command; the probes of 2026-10-16 on CLI
	// 2.1.302 (`-p --permission-mode auto`) could not measure that: `echo`,
	// `sudo -n true` and `git push --dry-run` each ran, `permission_denials`
	// empty, with the entry bare, as `Bash(<cmd>:*)` or absent, so nothing
	// was refused. Where auto mode is unavailable the session falls back to
	// Manual, and there the entry is what pre-allows a headless child's Bash
	// calls, which would otherwise be denied. A configured `allowed_tools`
	// passes as written.
	DefaultAllowedTools = "Bash Edit Write MultiEdit NotebookEdit"
	DefaultQueue        = "relay/queue.yaml"
)

// The layer files, each new name beside its old (agentik-r1 §6.3 c1, c3,
// c4, c6). The repo's is the session plugin's config, whose `relay`
// section (else the old `agent-relay`) the relay reads; its local file is
// the one in the same folder (session-layer-r1 §2.2); the session's
// resolved file is named by $CLAUDE_CODE_SESSION_ID (R25-91 (3)). The state
// dir's two names are StateIn's.
const (
	userConfig       = ".config/agentik/relay/config.yaml"
	oldUserConfig    = ".config/agent-relay/config.yaml"
	repoConfig       = ".claude/agentik/config.yaml"
	oldRepoConfig    = ".claude/claude-code-session/config.yaml"
	localConfig      = "config.local.yaml"
	sessionConfig    = "agentik-session/sessions/%s/resolved.yaml"
	oldSessionConfig = "claude-code-session/sessions/%s/resolved.yaml"
	section          = "relay"
	oldSection       = "agent-relay"
	stateName        = "agentik-relay"
	oldStateName     = "agent-relay"
)

// RepoConfigFile is the repo's layer, relative to its root, whose `relay`
// section the relay reads; `doctor --verify` names it in its suggestion
// (AR59 act 4).
const RepoConfigFile = repoConfig

// sessionLayer is the session's place in ConfigPaths.
const sessionLayer = 3

// ConfigPaths returns the layer files LoadConfig reads, lowest first: the
// user's, the repo's and the session's each the new file when it exists in
// fs, else the old when it exists, else the new (NewElseOld); the local
// file in the picked repo file's folder (session-layer-r1 §2.2); the
// session's only when env names a session, under xdg.StateHome, `~`
// expanded (§2.4; AR76).
func ConfigPaths(fs FS, env func(string) string, repo string) []string {
	home := env("HOME")
	repoFile := NewElseOld(fs.Exists, filepath.Join(repo, repoConfig), filepath.Join(repo, oldRepoConfig))
	paths := []string{
		NewElseOld(fs.Exists, filepath.Join(home, userConfig), filepath.Join(home, oldUserConfig)),
		repoFile,
		filepath.Join(filepath.Dir(repoFile), localConfig),
	}
	if id := env("CLAUDE_CODE_SESSION_ID"); id != "" {
		state := xdg.StateHome(env)
		id = filepath.Base(id)
		paths = append(paths, NewElseOld(fs.Exists, filepath.Join(state, fmt.Sprintf(sessionConfig, id)),
			filepath.Join(state, fmt.Sprintf(oldSessionConfig, id))))
	}
	return paths
}

// LoadConfig applies the layers in order. An absent file is no layer; a file
// that does not parse, or a known key of the wrong shape, is an error. A
// session layer sessionFresh does not pass is skipped whole with a warning
// (session-layer-r1 §2.3).
func LoadConfig(fs FS, env func(string) string, repo string) (Config, error) {
	c := Config{Tmux: "auto", AllowedTools: DefaultAllowedTools, Caffeinate: true, Band: BandPace, Queue: DefaultQueue}
	from := "defaults" // the layer that set the queue path
	// c4: doctor flags the old repo file whenever it exists.
	if old := filepath.Join(repo, oldRepoConfig); fs.Exists(old) {
		c.Warnings = append(c.Warnings, fmt.Sprintf("%s: the old path; move it to %s", old, RepoConfigFile))
	}
	paths := ConfigPaths(fs, env, repo)
	for i, p := range paths {
		if !fs.Exists(p) {
			continue
		}
		src, err := fs.ReadFile(p)
		if err != nil {
			return c, err
		}
		root, err := yaml.Parse(src)
		if err != nil {
			return c, fmt.Errorf("config %s: %w", p, err)
		}
		if i == sessionLayer {
			if why := sessionFresh(fs, root, paths[1], paths[2]); why != "" {
				c.Warnings = append(c.Warnings, fmt.Sprintf("%s: not read: %s", p, why))
				continue
			}
		}
		c.Sources = append(c.Sources, p)
		was := c.Queue
		node := root
		if i > 0 { // the repo's, the local and the session's files hold a section
			sec, ok := root.Get(section)
			old, oldOK := root.Get(oldSection)
			switch { // c6: with both present, relay wins with a warning
			case ok && oldOK:
				c.Warnings = append(c.Warnings, fmt.Sprintf("%s: sections %s and %s; %s wins", p, section, oldSection, section))
			case oldOK:
				sec, ok = old, true
			}
			if !ok {
				continue
			}
			node = sec
		}
		if err := c.apply(p, node); err != nil {
			return c, fmt.Errorf("config %s: %w", p, err)
		}
		if c.Queue != was {
			from = p
		}
	}
	q, err := underRepo(repo, c.Queue)
	if err != nil {
		return c, fmt.Errorf("config %s: queue: %w", from, err)
	}
	c.Queue = q
	return c, nil
}

// recordedSum is a sha256 as a session file records it.
var recordedSum = regexp.MustCompile(`^[0-9a-f]{64}$`)

// sessionFresh is "" when the session layer root may be read, else why not
// (session-layer-r1 §2.1, §2.3): its provider.inputs must list repoFile and
// localFile, each at the sha256 it has now through fs, a null matching only
// a file still absent. Paths compare after filepath.Clean; an item with a
// relative path is skipped; the other inputs are not checked. An inputs
// that is YAML's null is none (AR76 ruling 3); a path unlisted while its
// other-folder twin is listed was resolved before the folder moved (ruling 2).
func sessionFresh(fs FS, root yaml.Node, repoFile, localFile string) string {
	const malformed = "malformed provider.inputs"
	prov, _ := root.Get("provider")
	list, ok := prov.Get("inputs")
	if !ok || null(list) {
		return "no provider.inputs"
	}
	if list.Kind != yaml.List {
		return malformed
	}
	sums := map[string][]string{} // a clean path's recorded sums
	for _, item := range list.List {
		pn, ok := item.Get("path")
		path, scalar := pn.String()
		if !ok || !scalar {
			return malformed
		}
		if !filepath.IsAbs(path) {
			continue
		}
		sn, ok := item.Get("sha256")
		sum, scalar := sn.String()
		if !ok || !scalar || (sum != "null" && !recordedSum.MatchString(sum)) {
			return malformed
		}
		path = filepath.Clean(path)
		sums[path] = append(sums[path], sum)
	}
	want := []string{filepath.Clean(repoFile), filepath.Clean(localFile)}
	for _, p := range want {
		if len(sums[p]) > 0 {
			continue
		}
		if tw := twin(p); tw != "" && len(sums[tw]) > 0 {
			return "resolved before the config folder moved: " + tw
		}
		return fmt.Sprintf("resolved for another checkout: %s not listed", p)
	}
	for _, p := range want {
		now := sumNow(fs, p)
		for _, sum := range sums[p] {
			if sum != now {
				return fmt.Sprintf("%s changed since the session resolved it", p)
			}
		}
	}
	return ""
}

// null is whether n is YAML's null: a scalar `null`, `Null`, `NULL`, `~`,
// or no value (the strict reader keeps each as its text).
func null(n yaml.Node) bool {
	s, ok := n.String()
	return ok && (s == "" || s == "~" || s == "null" || s == "Null" || s == "NULL")
}

// twin is clean path p's other-folder twin: the same file name in its
// repo's other config folder (the new for the old and back, agentik-r1
// §6.3 c3), "" for a path in neither folder.
func twin(p string) string {
	dir, name := filepath.Dir(p), filepath.Base(p)
	nw, old := filepath.Dir(repoConfig), filepath.Dir(oldRepoConfig)
	for _, pair := range [2][2]string{{nw, old}, {old, nw}} {
		if strings.HasSuffix(dir, string(filepath.Separator)+pair[0]) {
			return filepath.Join(strings.TrimSuffix(dir, pair[0]), pair[1], name)
		}
	}
	return ""
}

// sumNow is path's sha256 through fs as a session file records it: "null"
// for an absent file, and "", which matches no record, for a file that
// cannot be read.
func sumNow(fs FS, path string) string {
	b, err := fs.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "null"
	} else if err != nil {
		return ""
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// underRepo is path as a clean path relative to repo: a relative path
// cleaned, an absolute one made relative to repo, with the symlinks of both
// resolved when the plain spelling is not under it (git prints a root
// resolved); a path that is empty or leaves the repo is an error.
func underRepo(repo, path string) (string, error) {
	rel := filepath.Clean(path)
	if filepath.IsAbs(path) {
		abs, err := filepath.Abs(repo)
		if err != nil {
			return "", err
		}
		rel, err = filepath.Rel(abs, path)
		if err != nil || !filepath.IsLocal(rel) {
			if r, err := filepath.EvalSymlinks(abs); err == nil {
				p := path
				if e, err := filepath.EvalSymlinks(path); err == nil {
					p = e
				}
				rel, _ = filepath.Rel(r, p)
			}
		}
	}
	if path == "" || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%q: want a path inside the repo", path)
	}
	return rel, nil
}

func (c *Config) apply(p string, n yaml.Node) error {
	if n.Kind != yaml.Map {
		return fmt.Errorf("line %d: want a mapping", n.Line)
	}
	str := func(kv yaml.KV, dst *string) error {
		s, ok := kv.Val.String()
		if !ok {
			return fmt.Errorf("line %d: %s: want a scalar", kv.Val.Line, kv.Key)
		}
		*dst = s
		return nil
	}
	for _, kv := range n.Map {
		var err error
		switch kv.Key {
		case "tmux":
			err = str(kv, &c.Tmux)
		case "allowed_tools":
			err = str(kv, &c.AllowedTools)
		case "queue":
			err = str(kv, &c.Queue)
		case "merge":
			err = str(kv, &c.Merge)
		case "packets":
			if err = str(kv, &c.Packets); err == nil && c.Packets != "" && !filepath.IsAbs(c.Packets) {
				return fmt.Errorf("line %d: packets: %q: want an absolute dir", kv.Val.Line, c.Packets)
			}
		case "verify_seconds": // named as the queue's measure_seconds (AR59 act 1)
			v, ok := kv.Val.Int()
			if !ok || v < 1 || v > MaxVerifySeconds {
				return fmt.Errorf("line %d: verify_seconds: %q: want an integer from 1 to %d", kv.Val.Line, kv.Val.Str, MaxVerifySeconds)
			}
			c.VerifySeconds = v
		case "caffeinate":
			b, ok := kv.Val.Bool()
			if !ok {
				return fmt.Errorf("line %d: caffeinate: want true or false", kv.Val.Line)
			}
			c.Caffeinate = b
		case "band": // beside caffeinate (band-pacing-r1 §2.1; AR80 ruling 1)
			if kv.Val.Kind != yaml.Scalar || (kv.Val.Str != BandPace && kv.Val.Str != BandOff) {
				return fmt.Errorf("line %d: band: %q: want pace or off", kv.Val.Line, kv.Val.Str)
			}
			c.Band = kv.Val.Str
		case "ntfy":
			if kv.Val.Kind != yaml.Map {
				return fmt.Errorf("line %d: ntfy: want a mapping", kv.Val.Line)
			}
			for _, sub := range kv.Val.Map {
				switch sub.Key {
				case "url":
					err = str(sub, &c.Ntfy.URL)
				case "topic":
					err = str(sub, &c.Ntfy.Topic)
				default:
					c.Unknown = append(c.Unknown, p+": ntfy."+sub.Key)
				}
				if err != nil {
					return err
				}
			}
		default:
			c.Unknown = append(c.Unknown, p+": "+kv.Key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// VerifyLimit is the verify line's limit (AR59 act 2): verify_seconds,
// else Timeout. Acceptance lines keep Timeout.
func (c Config) VerifyLimit() time.Duration {
	if c.VerifySeconds <= 0 {
		return Timeout
	}
	return time.Duration(c.VerifySeconds) * time.Second
}

// TmuxMode is the tmux value (D16): auto (the default: tmux when present),
// never (file windows only, `run --no-tmux`) or required (a lost session
// stops the night tmux-gone). Anything else is an error doctor lists.
func (c Config) TmuxMode() (string, error) {
	switch c.Tmux {
	case "", "auto":
		return "auto", nil
	case "never", "required":
		return c.Tmux, nil
	}
	return "", fmt.Errorf("tmux: %q: want auto, never or required", c.Tmux)
}
