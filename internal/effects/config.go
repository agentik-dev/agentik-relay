package effects

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/yaml"
)

// Config is the relay's configuration after the §9 layers, lowest first:
// the shipped defaults; the user file; the repo's `agent-relay` section; the
// session's resolved file when present. Flags are the caller's, last.
// Unknown lists keys no layer knows ("<file>: <key>"), reported by doctor and
// never fatal (D23); Sources lists the files read. Packets is the packets
// source (key `packets`, AR29 act 3): an absolute dir the relay only reads,
// against which the gate compares each row's committed packet; relative is
// an error, unset compares nothing. Queue is the queue's clean path under
// the repo (ar32a review round 2): LoadConfig makes it so once, an absolute
// path under the repo relative to it, and a path that leaves the repo an
// error, so every reader of the queue sees the one spelling.
// VerifySeconds is the repo's verify limit (key `verify_seconds`, AR59 act
// 1, the settled entry 10): an integer from 1 to MaxVerifySeconds; 0 is
// unset, and VerifyLimit is then Timeout.
type Config struct {
	Ntfy          struct{ URL, Topic string }
	Tmux          string
	AllowedTools  string
	Caffeinate    bool
	Queue, Merge  string
	Packets       string
	VerifySeconds int
	Unknown       []string
	Sources       []string
}

// The shipped defaults (§7's tools, §3's queue path).
const (
	DefaultAllowedTools = "Bash Edit Write MultiEdit NotebookEdit"
	DefaultQueue        = "relay/queue.yaml"
)

// The layer files. The repo's is the session plugin's config, whose
// `agent-relay` section the relay reads; the session's resolved file is
// named by $CLAUDE_CODE_SESSION_ID (R25-91 (3)).
const (
	userConfig    = ".config/agent-relay/config.yaml"
	repoConfig    = ".claude/claude-code-session/config.yaml"
	sessionConfig = "claude-code-session/sessions/%s/resolved.yaml"
	section       = "agent-relay"
)

// RepoConfigFile is the repo's layer, relative to its root, whose
// `agent-relay` section the relay reads; `doctor --verify` names it in its
// suggestion (AR59 act 4).
const RepoConfigFile = repoConfig

// ConfigPaths returns the layer files LoadConfig reads, lowest first; the session's
// only when env names a session.
func ConfigPaths(env func(string) string, repo string) []string {
	paths := []string{filepath.Join(env("HOME"), userConfig), filepath.Join(repo, repoConfig)}
	if id := env("CLAUDE_CODE_SESSION_ID"); id != "" {
		state := env("XDG_STATE_HOME")
		if state == "" {
			state = filepath.Join(env("HOME"), ".local", "state")
		}
		paths = append(paths, filepath.Join(state, fmt.Sprintf(sessionConfig, filepath.Base(id))))
	}
	return paths
}

// LoadConfig applies the layers in order. An absent file is no layer; a file
// that does not parse, or a known key of the wrong shape, is an error.
func LoadConfig(fs FS, env func(string) string, repo string) (Config, error) {
	c := Config{Tmux: "auto", AllowedTools: DefaultAllowedTools, Caffeinate: true, Queue: DefaultQueue}
	from := "defaults" // the layer that set the queue path
	for i, p := range ConfigPaths(env, repo) {
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
		c.Sources = append(c.Sources, p)
		was := c.Queue
		node := root
		if i > 0 { // the repo's and the session's files hold a section
			sec, ok := root.Get(section)
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
