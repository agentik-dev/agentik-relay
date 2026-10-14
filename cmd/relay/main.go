// Command relay drives Claude Code sittings over a queue of packets.
//
// Usage: relay <verb> [--json] [args]. Built: version, doctor, hook, plan
// (the queue, dependencies, the baseline and the spawn lines), gate (§6 by
// hand), clean --merged, run (the night, A8), status, stop, kill, resume and
// report, attach (the night's tmux window, or tail -f the row's log).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/gate"
	"github.com/agentik-dev/agentik-relay/internal/hooks"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/prompt"
	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

const version = "0.1.0-dev"

// Exit codes (design/relay-r1.md §9; D21 moved usage from 1 to 2).
const (
	exitOK       = 0 // ok or green
	exitRed      = 1 // red, or a night that stopped on a failure
	exitUsage    = 2 // usage or a failed pre-check
	exitUnwell   = 2 // doctor found something missing
	exitNotBuilt = 3 // a verb still stubbed
)

// stubs are the verbs the brief names that are not built yet.
var stubs = map[string]bool{}

// lookPath finds doctor's binaries; a variable so a test can fake it.
var lookPath = exec.LookPath

// toplevel finds the repo plan reads; a variable so a test can fake it.
var toplevel = func() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	return strings.TrimSpace(string(out)), err
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	verb, asJSON := args[0], false
	for _, a := range args[1:] {
		if a == "--json" {
			asJSON = true
		}
	}
	// AR22 act 1 (ADV13-2, -3): the pinned env, once at start before any
	// measure or spawn: absolute git, make, sh and ps from dirs the account
	// cannot write, the relay home, and the committer read (Identity);
	// every gate line and runner git runs under it. None is exit 2,
	// unpinned-tool. The hook (inside a sitting) and version need none.
	if verb != "version" && verb != "--version" && verb != "hook" && !stubs[verb] {
		release, err := pinFor()
		if err != nil {
			fmt.Fprintf(stderr, "relay %s: %v\n", verb, err)
			return exitUsage
		}
		defer release()
	}
	switch {
	case verb == "version" || verb == "--version":
		fmt.Fprintf(stdout, "relay %s\n", version)
		return exitOK
	case verb == "doctor":
		f, _, err := flagSet(args[1:], []string{"--verify", "--line"}, []string{"--json"}, false)
		if err == nil && f["--line"] != "" && f["--verify"] == "" {
			err = fmt.Errorf("--line needs --verify")
		}
		if err != nil {
			fmt.Fprintf(stderr, "relay doctor: %v\nusage: relay doctor [--verify <dir> [--line <shell line>]] [--json]\n", err)
			return exitUsage
		}
		if dir := f["--verify"]; dir != "" {
			return doctorVerify(dir, f["--line"], asJSON, stdout, stderr)
		}
		return doctor(asJSON, stdout)
	case verb == "plan":
		return plan(args[1:], stdout, stderr)
	case verb == "run":
		return runVerb(args[1:], stdout, stderr)
	case verb == "status":
		return statusVerb(args[1:], stdout, stderr)
	case verb == "stop" || verb == "kill":
		return controlVerb(verb, args[1:], stdout, stderr)
	case verb == "resume":
		return resumeVerb(args[1:], stdout, stderr)
	case verb == "attach":
		return attachVerb(args[1:], stdout, stderr)
	case verb == "report":
		return reportVerb(args[1:], stdout, stderr)
	case verb == "gate":
		return gateVerb(args[1:], stdout, stderr)
	case verb == "clean":
		return clean(args[1:], stdout, stderr)
	case verb == "hook":
		// The plugin's hooks/run shim lands here; every hook exits 0.
		event := ""
		if len(args) > 1 {
			event = args[1]
		}
		return hooks.Run(event, stdin, stdout, os.Getenv, hooks.OS{})
	case stubs[verb]:
		if asJSON {
			writeJSON(stdout, struct {
				Error string `json:"error"`
				Verb  string `json:"verb"`
			}{relay.ErrNotImplemented.Error(), verb})
		} else {
			fmt.Fprintf(stderr, "relay %s: %s\n", verb, relay.ErrNotImplemented)
		}
		return exitNotBuilt
	case verb == "-h" || verb == "--help" || verb == "help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "relay: unknown verb %q\n", verb)
		usage(stderr)
		return exitUsage
	}
}

type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Warn   bool   `json:"warn"`
	Detail string `json:"detail"`
}

// claudeProbe runs `claude --max-turns 1 --version` and reads exit 0: the
// flag the spawn leans on is accepted even where --help hides it (2.1.280).
// The probe is not a sitting: it runs under effects.DropSitting's env, no
// CLAUDE* and no RELAY_ROW of a sitting doctor was run from (AR24 act 3,
// ADV14-5). A variable so a test can fake it.
var claudeProbe = func(path string) error {
	cmd := exec.Command(path, "--max-turns", "1", "--version")
	cmd.Env = effects.DropSitting(os.Environ())
	return cmd.Run()
}

// gitWorktree is `git worktree list` in the repo; a variable so a test can
// fake it.
var gitWorktree = func(repo string) error {
	return exec.Command("git", "-C", repo, "worktree", "list").Run()
}

// doctor runs §9's checks in order. A warning is ok with warn set (tmux
// absent: file windows, D16); unknown is never a pass. Exit 0 when no check
// is not ok, 2 otherwise.
func doctor(asJSON bool, stdout io.Writer) int {
	var checks []check
	add := func(name string, ok, warn bool, format string, a ...any) {
		checks = append(checks, check{name, ok, warn, fmt.Sprintf(format, a...)})
	}
	repo, err := toplevel()
	if err != nil || repo == "" {
		repo = "."
	}
	cfg, cfgErr := effects.LoadConfig(effects.OSFS{}, getenv, repo)

	if p, err := lookPath("claude"); err != nil {
		add("claude", false, false, "not on PATH")
	} else if err := claudeProbe(p); err != nil {
		add("claude", false, false, "%s --max-turns 1 --version: %v", p, err)
	} else {
		add("claude", true, false, "%s; accepts --max-turns 1 --version", p)
	}
	if p, err := lookPath("git"); err != nil {
		add("git", false, false, "not on PATH")
	} else {
		add("git", true, false, "%s", p)
	}
	if err := gitWorktree(repo); err != nil {
		add("git worktree", false, false, "git worktree list in %s: %v", repo, err)
	} else {
		add("git worktree", true, false, "git worktree list runs in %s", repo)
	}
	if p, err := lookPath("caffeinate"); err == nil {
		add("caffeinate", true, false, "%s", p)
	} else if cfgErr == nil && !cfg.Caffeinate {
		add("caffeinate", true, true, "not on PATH; the config turns it off")
	} else {
		add("caffeinate", false, false, "not on PATH")
	}
	mode, modeErr := cfg.TmuxMode()
	switch p, err := lookPath("tmux"); {
	case mode == "never":
		add("tmux", true, false, "never: the night writes file windows (attach tails the log)")
	case err != nil && mode == "required":
		add("tmux", false, false, "not on PATH, and the config requires it")
	case err != nil:
		add("tmux", true, true, "not on PATH: the night writes file windows (attach tails the log)")
	default:
		add("tmux", true, false, "%s (%s)", p, mode)
	}
	if cfg.Ntfy.URL != "" && cfg.Ntfy.Topic != "" {
		add("ntfy", true, false, "configured %s", cfg.Ntfy.Topic)
	} else {
		add("ntfy", true, false, "unset: notices go to the log")
	}
	state := effects.StateDir(getenv)
	fx := effectsFor(state, "")
	if err := writable(fx.FS, state); err != nil {
		add("state", false, false, "%s: %v", state, err)
	} else {
		add("state", true, false, "%s writable", state)
	}
	qpath := filepath.Join(repo, effects.DefaultQueue)
	if cfgErr == nil { // the config's queue path is under the repo (ar32a review round 2)
		qpath = filepath.Join(repo, cfg.Queue)
	}
	if !fx.FS.Exists(qpath) {
		add("queue", true, false, "none at %s", qpath)
	} else if q, err := queue.Load(repo, qpath); err != nil {
		add("queue", false, false, "%v", err)
	} else if ps := q.Validate(); len(ps) > 0 {
		add("queue", true, true, "%s parses, %d rows; %d problems (relay plan names them)", qpath, len(q.Rows), len(ps))
	} else {
		add("queue", true, false, "%s parses, %d rows, valid", qpath, len(q.Rows))
	}
	if root, err := fx.Git.Root(repo); err != nil {
		add("root", false, false, "the root is unknown: %v", err)
	} else if status, err := fx.Git.RootStatus(root); err != nil {
		add("root", false, false, "%s: status unknown: %v", root, err)
	} else if len(status) > 0 {
		add("root", false, false, "%s is dirty (%d paths): the baseline and the merge need it clean", root, len(status))
	} else if branch, err := fx.Git.Branch(root); err != nil || branch != "main" {
		add("root", true, true, "%s is clean but on %q, not main: the baseline will be unrun", root, branch)
	} else {
		add("root", true, false, "%s clean, on main", root)
	}
	// AR29 act 4 (ADV17-4): what the breaker reads a helper's executable
	// by; with neither /proc nor a pinned lsof it counts a claude by its
	// comm alone, a warning, never a fail.
	pinned, _ := effects.Current()
	if r := effects.ExeReader(pinned, "/proc"); r != "" {
		add("breaker", true, false, "a helper's executable read by %s", r)
	} else {
		add("breaker", true, true, "no /proc and no pinned lsof: the breaker counts a claude by its comm alone")
	}
	// AR49 act 2: a pinned python3 that does not honor the bytecode prefix
	// keeps its cache files counted: a warning, and no row otherwise.
	if note := effects.PycacheNote(pinned); note != "" {
		add("python3", true, true, "%s", note)
	}
	add("template", true, false, "sha256 %s", prompt.TemplateHash())
	switch {
	case cfgErr != nil:
		add("config", false, false, "%v", cfgErr)
	case len(cfg.Unknown) > 0 || modeErr != nil:
		unknown := cfg.Unknown
		if modeErr != nil {
			unknown = append(unknown, modeErr.Error())
		}
		add("config", true, true, "unknown keys or values: %s", strings.Join(append(unknown, cfg.Warnings...), "; "))
	case len(cfg.Warnings) > 0: // agentik-r1 §6.3 c4, c6: an old name warns, never fails
		add("config", true, true, "%d files read, old names: %s", len(cfg.Sources), strings.Join(cfg.Warnings, "; "))
	default:
		add("config", true, false, "%d files read, no unknown keys", len(cfg.Sources))
	}

	allOK := true
	for _, c := range checks {
		allOK = allOK && c.OK
	}
	if asJSON {
		writeJSON(stdout, struct {
			Checks []check `json:"checks"`
			OK     bool    `json:"ok"`
		}{checks, allOK})
	} else {
		failed, warned := 0, 0
		for _, c := range checks {
			if !c.OK {
				failed++
			} else if c.Warn {
				warned++
			}
		}
		verdict := fmt.Sprintf("all %d checks ok", len(checks))
		if failed > 0 {
			verdict = fmt.Sprintf("%d of %d checks failed", failed, len(checks))
		}
		if warned > 0 {
			verdict += fmt.Sprintf(", %d with a warning", warned)
		}
		fmt.Fprintf(stdout, "doctor: %s\n", verdict)
		home := getenv("HOME")
		for _, c := range checks {
			mark := "ok"
			if !c.OK {
				mark = "FAIL"
			} else if c.Warn {
				mark = "warn"
			}
			detail := c.Detail
			if home != "" {
				detail = strings.ReplaceAll(detail, home+"/", "~/")
			}
			for i, l := range wrapWords(detail, 99-18) {
				if i == 0 {
					fmt.Fprintf(stdout, "%-4s %-12s %s\n", mark, c.Name, l)
				} else {
					fmt.Fprintf(stdout, "%18s%s\n", "", l)
				}
			}
		}
	}
	if !allOK {
		return exitUnwell
	}
	return exitOK
}

// wrapWords is s broken at spaces into lines of at most n runes; a word
// longer than n stays whole on its own line.
func wrapWords(s string, n int) []string {
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = w
		case len([]rune(line))+1+len([]rune(w)) <= n:
			line += " " + w
		default:
			out = append(out, line)
			line = w
		}
	}
	return append(out, line)
}

// writable proves the state directory takes a file.
func writable(fs effects.FS, state string) error {
	if err := fs.MkdirAll(state, 0o700); err != nil {
		return err
	}
	return fs.WriteFile(filepath.Join(state, ".doctor"), []byte("ok\n"), 0o600)
}

// pinFor pins the env at start (effects.Pin) and returns its release,
// which removes the relay home; a variable so a test keeps its package's
// pin.
var pinFor = func() (func(), error) {
	p, err := effects.Pin()
	if err != nil {
		return func() {}, err
	}
	return func() { p.Release() }, nil
}

// getenv is the environment; a variable so a test can fake it.
var getenv = os.Getenv

// effectsFor is the machine's effects for one night; a variable so a test
// can fake them.
var effectsFor = func(state, night string) effects.Effects {
	clock, groups := effects.SystemClock{}, &effects.Groups{}
	return effects.Effects{Git: effects.OSGit{}, Shell: effects.OSShell{Groups: groups}, Clock: clock,
		FS: effects.OSFS{}, Ledger: effects.NewLedger(state, clock, night), Getenv: getenv, Load: effects.OSLoad{}, Idle: effects.OSIdle{}, Procs: effects.OSProcs{},
		Groups: groups}
}

// plan validates the queue and every packet, resolves dependencies,
// snapshots origin, measures the baseline on main in the root, and prints
// each row's spawn line and the stop conditions (§9); it spawns nothing.
// Exit 0 when valid, 2 when invalid, unreadable, or called wrongly.
func plan(args []string, stdout, stderr io.Writer) int {
	asJSON, path, opts := false, "", night.Options{}
	value := func(i *int, a, name string) (string, bool) {
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v, true
		}
		if a == name && *i+1 < len(args) {
			*i++
			return args[*i], true
		}
		return "", false
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--json" {
			asJSON = true
		} else if v, ok := value(&i, a, "--queue"); ok {
			path = v
		} else if v, ok := value(&i, a, "--rows"); ok {
			for _, id := range strings.Split(v, ",") {
				if id = strings.TrimSpace(id); id != "" {
					opts.Rows = append(opts.Rows, id)
				}
			}
		} else if v, ok := value(&i, a, "--until"); ok {
			opts.Until = v
		} else if v, ok := value(&i, a, "--measure-seconds"); ok {
			if opts.MeasureSeconds, ok = measureSeconds(v); !ok {
				fmt.Fprintf(stderr, "relay plan: --measure-seconds %q: want a whole number of seconds over 0\n", v)
				return exitUsage
			}
		} else {
			fmt.Fprintf(stderr, "relay plan: unknown argument %q\n", a)
			fmt.Fprintln(stderr, "usage: relay plan [--queue PATH] [--rows a,b] [--until HH:MM] [--measure-seconds N] [--json]")
			return exitUsage
		}
	}
	fail := func(err error) int {
		if asJSON {
			writeJSON(stdout, struct {
				Valid    bool            `json:"valid"`
				Problems []queue.Problem `json:"problems"`
			}{false, []queue.Problem{{Msg: err.Error()}}})
		} else {
			fmt.Fprintf(stderr, "relay plan: %v\n", err)
		}
		return exitUsage
	}
	// A plan under a measurement measures nothing, so only a plan that
	// would measure holds the pid file (AR17B); inside a row it never
	// does (AR32A act 1).
	if !effects.Nested(getenv) && !night.InRow(getenv) {
		release, code := holdPid("plan", stderr, nil)
		if release == nil {
			return code
		}
		defer release()
	}
	reg := &effects.Groups{}
	ctx, stop := night.Trap(context.Background(), reg)
	defer stop()
	p, _, _, err := loadPlan(path, opts, reg, false, false)
	if code, ok := trapped(ctx, "plan", stderr); ok {
		stop()
		return code
	}
	if err != nil {
		return fail(err)
	}
	if asJSON {
		writeJSON(stdout, p)
	} else {
		night.Print(stdout, p)
	}
	return p.Exit() // 2 invalid (exitUsage), else exitOK
}

// measureSeconds parses `--measure-seconds` and `--stall-seconds` (§15): a
// whole number over 0.
func measureSeconds(v string) (int, bool) {
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 || fmt.Sprint(n) != strings.TrimSpace(v) {
		return 0, false
	}
	return n, true
}

// planned is what loadPlan found beside the plan: the queue, the repo it
// came from, the state directory and the config; end is the night's
// planning mark's end (AR60 act 1), nil when loadPlan marked nothing.
type planned struct {
	q     *queue.Queue
	repo  string
	state string
	cfg   effects.Config
	end   func(relay.StopReason)
}

// loadPlan is plan's body, shared with run: the config, the queue (path
// relative to the cwd, else the config's relative to the repo), the first
// free night, and BuildPlan over the machine's effects. The repo is the
// root (Git.Root, main's tree), never the cwd's toplevel: a row's
// worktree never supplies its own queue, config or packet (ADV6-10).
// Inside a row the plan is a check that measures and writes nothing, so
// a --queue path's packets resolve in the worktree holding that queue,
// where a row the sitting added has its packet (ar32a review round 1).
// Under run, the measure reads the night's control file, and the owner's
// stop or kill ends it and signals reg's groups (AR34 act 2); plan's never.
// With mark (run, not --dry), the night's status reads `planning` from its
// name on, before the plan's measure (AR60 act 1; night.MarkPlanning), and
// planned.end ends it when no night opens.
func loadPlan(path string, opts night.Options, reg *effects.Groups, run, mark bool) (*night.Plan, planned, effects.Effects, error) {
	var pd planned
	repo, state, cfg, err := repoConfig()
	if err != nil {
		return nil, pd, effects.Effects{}, err
	}
	packets := repo
	if path == "" {
		path = cfg.Queue // relative to the repo
	} else if abs, err := filepath.Abs(path); err == nil {
		path = abs // a flag's path is relative to the cwd
		if night.InRow(getenv) {
			if top, err := effectsFor(state, "").Git.Toplevel(filepath.Dir(abs)); err == nil && filepath.IsAbs(top) {
				packets = top
			}
		}
	}
	q, err := queue.Load(packets, path)
	if err != nil {
		return nil, pd, effects.Effects{}, err
	}
	probe := effectsFor(state, "")
	opts.Now = probe.Clock.Now()
	opts.Night = night.NightName(probe.FS, state, opts.Now)
	opts.Repo, opts.State = repo, state
	opts.AllowedTools, opts.Caffeinate = cfg.AllowedTools, cfg.Caffeinate
	opts.VerifyLimit = cfg.VerifyLimit() // the verify baseline's (AR59 act 2)
	if run {
		opts.Control, opts.Groups = night.ControlPath(state, opts.Night), reg
	}
	fx := withGroups(effectsFor(state, opts.Night), reg)
	if night.InRow(getenv) { // a row checks its queue edit: nothing measured, nothing written (AR32A act 1)
		fx = rowPlanEffects(fx)
	}
	pd = planned{q: q, repo: repo, state: state, cfg: cfg}
	if mark {
		pd.end = night.MarkPlanning(fx, state, opts.Night, repo)
	}
	defer night.WatchGroups(fx, reg, state, opts.Night, repo)() // every measured line on record (AR17C), with the root (AR37 fix)
	p, err := night.BuildPlan(fx, q, opts)
	return p, pd, fx, err
}

// repoConfig is the repo's root, the state directory and the config.
func repoConfig() (repo, state string, cfg effects.Config, err error) {
	repo, err = toplevel()
	if err != nil || repo == "" {
		repo = "."
	}
	state = effects.StateDir(getenv)
	if root, err := effectsFor(state, "").Git.Root(repo); err == nil && root != "" {
		repo = root
	}
	cfg, err = effects.LoadConfig(effects.OSFS{}, getenv, repo)
	return repo, state, cfg, err
}

// orchestrateRefusal is why `relay run --orchestrate` refuses before the
// lock and the plan (orchestrator r1 §8, AR32A act 5), "" when it may
// start: no ntfy URL and topic, no orchestrator row, or a red or failed
// sitting of the row today; branch-only defaults start (ar32b act 4). A
// sitting is red by its `sitting` line, by a refused `merge` line (its
// sitting line was written green before the merge), or by a `stop` line
// naming it among the resume ids (ar32a review round 1). The ledger is the
// account's, so a line counts only when it names this root, or names none
// (ar32a review round 2): see lineRoot.
func orchestrateRefusal() string { return orchestrateRefusalFor(false) }

// orchestrateRefusalFor is orchestrateRefusal with --no-notify said or not:
// a run started with it keeps every notice in its log, so it needs no ntfy
// topic (the ledger's entry 7, quiet hours).
func orchestrateRefusalFor(noNotify bool) string {
	repo, state, cfg, err := repoConfig()
	if err != nil {
		return err.Error()
	}
	if !noNotify && (cfg.Ntfy.URL == "" || cfg.Ntfy.Topic == "") {
		return "no ntfy url and topic in the config: an orchestrated night sends its notices there (or start it with --no-notify)"
	}
	q, err := queue.Load(repo, cfg.Queue)
	if err != nil {
		return err.Error()
	}
	r, ok := q.Orchestrator()
	if !ok {
		return "the queue " + cfg.Queue + " has no orchestrator row"
	}
	fx := effectsFor(state, "")
	lines, err := fx.Ledger.Lines()
	if err != nil {
		return "the ledger: " + err.Error()
	}
	now := fx.Clock.Now()
	opened := map[string]string{} // each night's root, from its night-open line
	for _, l := range lines {
		if l["event"] == "night-open" {
			id, _ := l["night"].(string)
			opened[id], _ = l["root"].(string)
		}
	}
	for _, l := range lines {
		ts, _ := l["ts"].(string)
		if at, err := time.Parse(time.RFC3339, ts); err != nil || at.In(now.Location()).Format("2006-01-02") != now.Format("2006-01-02") {
			continue
		}
		if lr := lineRoot(l, opened); lr != "" && filepath.Clean(lr) != filepath.Clean(repo) {
			continue // another repo's line
		}
		for _, red := range redSittings(l) {
			if id := red[0]; strings.HasPrefix(id, r.ID+".") {
				return fmt.Sprintf("the row %s is parked for today: %s was %s (relay resume %s)", r.ID, id, red[1], id)
			}
		}
	}
	return ""
}

// lineRoot is the root a ledger line names: a `merge` or `stop` line's
// root, a `sitting` line's repo, else its night's root from opened; ""
// when it names none, which the refusal counts as this root's.
func lineRoot(l map[string]any, opened map[string]string) string {
	for _, k := range []string{"root", "repo"} {
		if s, _ := l[k].(string); s != "" {
			return s
		}
	}
	id, _ := l["night"].(string)
	return opened[id]
}

// redSittings is each row a ledger line reads red or failed, with how:
// a red or failed `sitting` line, a `merge` line refused with one of
// relay.RefusedMerges, and each id in a `stop` line's resume lines. A
// `sitting` line with `limit: paused` reads none (AR66 ruling 3): the night
// paused on it, so a recovered night parks nothing; a night that then
// stopped names the row in its `stop` line's resume lines, which park it.
func redSittings(l map[string]any) [][2]string {
	id, _ := l["row"].(string)
	switch l["event"] {
	case "sitting":
		if lim, _ := l["limit"].(string); lim == "paused" {
			return nil
		}
		if st, _ := l["state"].(string); st == "red" || st == "failed" {
			return [][2]string{{id, st}}
		}
	case "merge":
		res, _ := l["result"].(string)
		if slices.Contains(relay.RefusedMerges, relay.StopReason(res)) {
			return [][2]string{{id, "red: its merge was refused, " + res}}
		}
	case "stop":
		var lines []string // []any read from the ledger's JSON, []string in memory
		switch v := l["resume"].(type) {
		case []any:
			for _, x := range v {
				if s, ok := x.(string); ok {
					lines = append(lines, s)
				}
			}
		case []string:
			lines = v
		}
		var out [][2]string
		for _, s := range lines {
			if id, ok := strings.CutPrefix(s, "relay resume "); ok {
				out = append(out, [2]string{id, "red or failed at the night's stop"})
			}
		}
		return out
	}
	return nil
}

// rowPlanEffects is fx for a plan inside a row (orchestrator r1 §6): its
// environment reads as under a measurement, so nothing is measured, and
// its ledger reads and never writes.
func rowPlanEffects(fx effects.Effects) effects.Effects {
	env := fx.Getenv
	fx.Getenv = func(k string) string {
		if k == effects.MeasureDepthVar {
			return "1"
		}
		if env == nil {
			return ""
		}
		return env(k)
	}
	fx.Ledger = readOnlyLedger{fx.Ledger}
	return fx
}

// readOnlyLedger reads the ledger and drops every Append.
type readOnlyLedger struct{ effects.Ledger }

func (readOnlyLedger) Append(string, map[string]any) error { return nil }

// withGroups is fx with reg as its group registry, the one the verb's trap
// kills (AR17C, ADV10-5): its Groups and, when it is the machine's shell,
// the shell's, so every measured line registers before it starts.
func withGroups(fx effects.Effects, reg *effects.Groups) effects.Effects {
	if reg == nil {
		return fx
	}
	fx.Groups = reg
	if sh, ok := fx.Shell.(effects.OSShell); ok {
		sh.Groups = reg
		fx.Shell = sh
	}
	return fx
}

// trapped is a measuring verb's exit after its trap fired (AR17C): the
// line naming the signal, then 1, once every group it started is reaped.
func trapped(ctx context.Context, verb string, stderr io.Writer) (int, bool) {
	sig := night.Signalled(ctx)
	if sig == "" {
		return 0, false
	}
	fmt.Fprintf(stderr, "relay %s: stopped by %s\n", verb, sig)
	return exitRed, true
}

func writeJSON(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "%s\n", b)
}

// usage names the verbs a morning starts with first: status, then the
// resume door. Every verb takes --json.
func usage(w io.Writer) {
	fmt.Fprint(w, `usage: relay <verb> [--json] [args]
  status  [--night D]                 the night once: state, rows, burn, resume lines
  resume  [<row>]                     reopen a red or failed row's sitting (claude --resume)
  report  [--night D] [--write PATH]  the morning read, in markdown
  attach  [<row>] [--print]           watch the night: its tmux window, or tail -f a row's log
  plan    [--rows a,b] [--until HH:MM]  what a night would run, and whether it is valid
  run     [--rows a,b] [--until HH:MM] [--dry]  the night
          [--max-sittings N] [--max-usd X] [--parallel N] [--words T] [--no-tmux]
  stop                                ask the night to stop after the running sittings
  kill    [<row>]                     stop a row now
  gate    <row>                       the gate by hand
  clean   --merged                    remove the worktrees of merged rows
  clean   --stale                     remove dead relays' locks and their untouched worktrees
  doctor                              check claude, git, tmux, the state directory, the config
  version
every verb takes --json
`)
}

// gateReport is `gate --json`: the gate's facts, its result and reason, and
// what stopped it short.
type gateReport struct {
	relay.Gate
	Row    string           `json:"row"`
	Result relay.Result     `json:"result"`
	Reason relay.StopReason `json:"reason"`
	Error  string           `json:"error,omitempty"`
}

// gateVerb is `relay gate <row> [--json]`: §6 by hand, the night's own code
// (gate.Hand). The queue, the config and the row's packet are read in the
// root (Git.Root), never the cwd's toplevel: from a row's worktree, the
// child's own packet never judges it (ADV6-10). Exit 0 green, 1 red or failed, 2 unrun
// or called wrongly.
func gateVerb(args []string, stdout, stderr io.Writer) int {
	var id string
	asJSON := false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case id == "" && !strings.HasPrefix(a, "-"):
			id = a
		default:
			fmt.Fprintf(stderr, "relay gate: unknown argument %q\nusage: relay gate <row> [--json]\n", a)
			return exitUsage
		}
	}
	rep := gateReport{Row: id, Result: relay.Unrun}
	finish := func(err error) int {
		if err != nil {
			rep.Error = err.Error()
		}
		if asJSON {
			writeJSON(stdout, rep)
		} else {
			printGate(stdout, rep)
		}
		switch rep.Result {
		case relay.GreenResult:
			return exitOK
		case relay.RedResult, relay.FailedResult:
			return exitRed
		}
		return exitUsage
	}
	if id == "" {
		fmt.Fprintln(stderr, "usage: relay gate <row> [--json]")
		return exitUsage
	}
	// The hand gate runs branch code: the account's one relay while it
	// runs (AR22 act 2, ADV13-14).
	release, code := holdPid("gate", stderr, nil)
	if release == nil {
		return code
	}
	defer release()
	repo, err := toplevel()
	if err != nil || repo == "" {
		repo = "."
	}
	state := effects.StateDir(getenv)
	reg := &effects.Groups{}
	ctx, stop := night.Trap(context.Background(), reg)
	defer stop()
	fx := withGroups(effectsFor(state, "hand"), reg)
	defer night.WatchGroups(fx, reg, state, "hand", "")()
	root, err := fx.Git.Root(repo)
	if err != nil {
		return finish(fmt.Errorf("the root: %w", err))
	}
	cfg, err := effects.LoadConfig(effects.OSFS{}, getenv, root)
	if err != nil {
		return finish(err)
	}
	q, err := queue.Load(root, cfg.Queue)
	if err != nil {
		return finish(err)
	}
	var row *queue.Row
	for i := range q.Rows {
		if q.Rows[i].ID == id {
			row = &q.Rows[i]
		}
	}
	if row == nil {
		return finish(fmt.Errorf("row %s: not in the queue", id))
	}
	if row.Packet == nil && row.PacketPath != "" {
		p := filepath.Join(root, row.PacketPath)
		if src, err := fx.FS.ReadFile(p); err == nil {
			row.Packet, err = packet.Parse(p, src)
			if err != nil {
				return finish(err)
			}
		}
	}
	res, why, g, err := gate.Hand(fx, root, gate.Verify{Source: cfg.Packets, Limit: cfg.VerifyLimit()}, *row, q)
	if code, ok := trapped(ctx, "gate", stderr); ok {
		stop()
		return code
	}
	rep.Gate, rep.Result, rep.Reason = g, res, why
	return finish(err)
}

// printGate is `relay gate`'s text: a check that kept its log names it
// (AR31 act 2), and a gate stopped after its lines ran names theirs under
// its error.
func printGate(w io.Writer, r gateReport) {
	if r.Error != "" {
		fmt.Fprintf(w, "gate %s: %s: %s\n", r.Row, r.Result, r.Error)
		for _, l := range r.Gate.Logs() {
			fmt.Fprintf(w, "  log: %s\n", l)
		}
		return
	}
	logged := func(c relay.Check) string {
		if c.BranchLog == "" {
			return ""
		}
		return "; log: " + c.BranchLog
	}
	g := r.Gate
	fmt.Fprintf(w, "gate %s: %s %s\n", r.Row, r.Result, r.Reason)
	fact := func(name string, ok bool) string {
		for _, s := range g.Skipped {
			if s == name {
				return "skipped"
			}
		}
		return fmt.Sprint(ok)
	}
	fmt.Fprintf(w, "  exit %s, commit %v, clean %v, origin %s, outside %v\n",
		fact("exit", g.ResultSeen && g.ExitOK), g.Commit, g.Clean, fact("origin", g.OriginSame), g.Outside)
	for i, c := range g.Lines {
		fmt.Fprintf(w, "  %d. %s: exit %d, last %q; want %s%s\n", i+1, c.Status, c.BranchExit, c.BranchLast, c.Expect, logged(c))
	}
	if c := g.Verify; c != nil && c.Status != relay.CheckSkipped {
		fmt.Fprintf(w, "  verify %s: exit %d, last %q; %s%s\n", c.Name, c.BranchExit, c.BranchLast, c.Status, logged(*c))
	}
}

// clean is `relay clean --merged|--stale [--json]` (§8, §9): --merged
// removes each worktree under the root's .claude/worktrees/ whose branch
// is an ancestor of main, unless its stage lock has a live holder
// (night.LiveHolder, AR66: a paused row's tree is at its cut); --stale removes each stage lock whose holder is
// gone (a dead or restarted pid, R6 §4) and the worktree it held when that
// worktree's tip equals its cut (the stage line's head_before) and its
// row's last exit line is not the account's stop (run 10 ruling 1),
// printing each path. Never forced (a dirty worktree stays, named), never the
// working tree it runs in, and never a branch. Exit 0 when every candidate
// went, 1 when one stayed, 2 when called wrongly.
func clean(args []string, stdout, stderr io.Writer) int {
	merged, stale, asJSON, bad := false, false, false, false
	for _, a := range args {
		switch a {
		case "--merged":
			merged = true
		case "--stale":
			stale = true
		case "--json":
			asJSON = true
		default:
			bad = true
		}
	}
	if !merged && !stale || bad {
		fmt.Fprintln(stderr, "usage: relay clean --merged|--stale [--json]")
		return exitUsage
	}
	type kept struct {
		Path string `json:"path"`
		Why  string `json:"why"`
	}
	out := struct {
		Removed []string `json:"removed"`
		Kept    []kept   `json:"kept"`
		Locks   []string `json:"locks,omitempty"`
	}{[]string{}, []kept{}, nil}
	repo, err := toplevel()
	if err != nil || repo == "" {
		repo = "."
	}
	fx := effectsFor(effects.StateDir(getenv), "")
	code := exitOK
	root, err := fx.Git.Root(repo)
	w := fx.Git
	var wts []effects.Worktree
	if err == nil {
		wts, err = w.Worktrees(root)
	}
	if err != nil {
		fmt.Fprintf(stderr, "relay clean: %v\n", err)
		return exitUsage
	}
	here, _ := filepath.Abs(repo)
	under := filepath.Join(root, ".claude", "worktrees") + string(filepath.Separator)
	held := map[string]bool{} // the stale locks' worktrees
	if stale {
		gone, err := night.ReapStale(fx)
		if err != nil {
			fmt.Fprintf(stderr, "relay clean: %v\n", err)
			code = exitRed
		}
		for _, h := range gone {
			out.Locks = append(out.Locks, h.Worktree)
			held[filepath.Clean(h.Worktree)] = true
		}
	}
	var lines []map[string]any
	if stale && fx.Ledger != nil {
		lines, _ = fx.Ledger.Lines()
	}
	for _, wt := range wts {
		if !strings.HasPrefix(wt.Path, under) || wt.Branch == "" {
			continue
		}
		switch {
		case stale && held[filepath.Clean(wt.Path)]:
			if r := night.AccountKept(lines, filepath.Base(wt.Path)); r != "" {
				out.Kept = append(out.Kept, kept{wt.Path, "its last exit was the account's stop (" + string(r) + "): kept for the next attempt"})
				code = exitRed
				continue
			}
			st := night.StagedFromLedger(lines, filepath.Base(wt.Path))
			tip, err := fx.Git.Head(wt.Path)
			if st == nil || st.Path != wt.Path || err != nil || tip != st.HeadBefore {
				out.Kept = append(out.Kept, kept{wt.Path, "its tip is not its cut: commits past the stage"})
				code = exitRed
				continue
			}
		case !merged:
			continue
		default:
			if anc, err := fx.Git.IsAncestor(root, wt.Branch, "main"); err != nil || !anc {
				continue
			}
			if h, live := night.LiveHolder(fx.Procs, wt.Path); live { // a paused row's tree (AR66 ruling 4)
				out.Kept = append(out.Kept, kept{wt.Path, fmt.Sprintf("its stage lock is held by pid %d", h.Pid)})
				code = exitRed
				continue
			}
		}
		if wt.Path == here {
			out.Kept = append(out.Kept, kept{wt.Path, "the working tree clean runs in"})
			code = exitRed
			continue
		}
		if err := w.WorktreeRemove(root, wt.Path); err != nil {
			out.Kept = append(out.Kept, kept{wt.Path, err.Error()})
			code = exitRed
			continue
		}
		out.Removed = append(out.Removed, wt.Path)
	}
	if asJSON {
		writeJSON(stdout, out)
	} else {
		for _, p := range out.Locks {
			fmt.Fprintln(stdout, "lock "+p)
		}
		for _, p := range out.Removed {
			fmt.Fprintln(stdout, p)
		}
		for _, k := range out.Kept {
			fmt.Fprintf(stderr, "relay clean: kept %s: %s\n", k.Path, k.Why)
		}
	}
	return code
}

// runReport is one row of `run --json`: its spawn line and hashes, and
// after the night its sitting, log and resume line.
type runReport struct {
	Row        string           `json:"row"`
	Spawn      []string         `json:"spawn"`
	Worktree   string           `json:"worktree"`
	SessionID  string           `json:"session_id"`
	PromptHash string           `json:"prompt_hash"`
	RulesHash  string           `json:"rules_hash"`
	State      relay.State      `json:"state,omitempty"`
	Reason     relay.StopReason `json:"reason,omitempty"`
	Log        string           `json:"log,omitempty"`
	Resume     string           `json:"resume,omitempty"`
}

// runOutput is `run --json`: the night, how it stopped, each row, the
// resume lines.
type runOutput struct {
	Night  string      `json:"night"`
	State  string      `json:"state"`
	Reason string      `json:"reason"`
	Rows   []runReport `json:"rows"`
	Resume []string    `json:"resume"`
	Error  string      `json:"error,omitempty"`
}

// newTmux is the machine's tmux on the relay's own socket under state
// (AR25 act 2; a socket refused leaves tmux absent); a variable so a test
// can fake it.
var newTmux = func(state string) effects.Tmux {
	sock, _ := effects.TmuxSocket(state)
	return effects.OSTmux{LookPath: lookPath, Socket: sock}
}

// runTrap is run's trap, night.Trap; a variable so a test can hand in a
// context already signalled, and no signal is sent (AR60 fix round, N2).
var runTrap = night.Trap

// spawner is the night's; a variable so a test can fake it.
var spawner = func(cfg effects.Config) effects.Spawner { return effects.OSSpawner{Caffeinate: cfg.Caffeinate} }

// flagSet parses args: the names in values take one value (`--x v` or
// `--x=v`), the names in bools none, and at most one positional is allowed
// when positional is true.
func flagSet(args []string, values, bools []string, positional bool) (map[string]string, string, error) {
	got, pos := map[string]string{}, ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, v, eq := strings.Cut(a, "=")
		switch {
		case contains(bools, a):
			got[a] = "true"
		case contains(values, name) && eq:
			got[name] = v
		case contains(values, a) && i+1 < len(args):
			i++
			got[a] = args[i]
		case positional && pos == "" && !strings.HasPrefix(a, "-"):
			pos = a
		default:
			return nil, "", fmt.Errorf("unknown argument %q", a)
		}
	}
	return got, pos, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if x == y {
			return true
		}
	}
	return false
}

// runVerb is `relay run [--rows a,b] [--until HH:MM] [--max-sittings N]
// [--max-usd X] [--parallel N] [--words T] [--measure-seconds N]
// [--stall-seconds N] [--no-tmux] [--no-notify] [--dry] [--json] [--orchestrate]`
// (§9): plan, then with --dry each row's spawn line and the rendered
// prompt's hashes, staging and spawning nothing; else the night (night.Run:
// A6's gate, merge-on-green through gate.MergeIfGreen, the live lines to
// <state>/nights/<night>/<row>.log, tmux windows over them per the config's
// tmux value, --no-tmux forcing never; the notices to the output lines and
// to ntfy when configured), the dashboard and
// every red or failed row's resume line. Exit 0 on queue-empty,
// count-reached, until-reached or owner-stop with no red; 1 otherwise; 2 on
// usage, a plan not valid, or a machine pair refused.
func runVerb(args []string, stdout, stderr io.Writer) int {
	const use = "usage: relay run [--rows a,b] [--until HH:MM] [--max-sittings N] [--max-usd X] [--parallel N] [--words T] [--measure-seconds N] [--stall-seconds N] [--no-tmux] [--no-notify] [--dry] [--json] [--orchestrate]"
	// The run guard (relay-r1 §15, AR11): before any stage, as night.Run.
	if night.Nested(getenv) {
		fmt.Fprintf(stderr, "relay run: %v\n", night.ErrNested)
		return exitUsage
	}
	// AR21 act 0 (ADV12-1): the merge's committer was read at start, by
	// Pin (AR22 act 1), before any measure or spawn, while ~/.gitconfig is
	// still the owner's; the runner reads no global config after.
	f, _, err := flagSet(args, []string{"--rows", "--until", "--max-sittings", "--max-usd", "--parallel", "--words", "--measure-seconds", "--stall-seconds"},
		[]string{"--no-tmux", "--dry", "--json", "--orchestrate", "--no-notify"}, false)
	if err != nil {
		fmt.Fprintf(stderr, "relay run: %v\n%s\n", err, use)
		return exitUsage
	}
	asJSON := f["--json"] != ""
	no := night.NightOptions{Until: f["--until"], Words: f["--words"], NoTmux: f["--no-tmux"] != ""}
	for _, id := range strings.Split(f["--rows"], ",") {
		if id = strings.TrimSpace(id); id != "" {
			no.Rows = append(no.Rows, id)
		}
	}
	for name, dst := range map[string]*int{"--max-sittings": &no.MaxSittings, "--parallel": &no.Parallel} {
		if v, ok := f[name]; ok {
			if _, err := fmt.Sscanf(v, "%d", dst); err != nil || *dst < 0 {
				fmt.Fprintf(stderr, "relay run: %s %q: want a whole number\n%s\n", name, v, use)
				return exitUsage
			}
		}
	}
	// ADV6-13: more slots than cores is a burst the load breaker reads late.
	if cores := runtime.NumCPU(); no.Parallel > cores {
		fmt.Fprintf(stderr, "relay run: --parallel %d: over the machine's %d cores\n%s\n", no.Parallel, cores, use)
		return exitUsage
	}
	if v, ok := f["--max-usd"]; ok {
		if _, err := fmt.Sscanf(v, "%g", &no.MaxUSD); err != nil || no.MaxUSD < 0 {
			fmt.Fprintf(stderr, "relay run: --max-usd %q: want dollars\n%s\n", v, use)
			return exitUsage
		}
	}
	measure := 0
	if v, ok := f["--measure-seconds"]; ok {
		if measure, ok = measureSeconds(v); !ok {
			fmt.Fprintf(stderr, "relay run: --measure-seconds %q: want a whole number of seconds over 0\n%s\n", v, use)
			return exitUsage
		}
	}
	if v, ok := f["--stall-seconds"]; ok {
		if no.StallSeconds, ok = measureSeconds(v); !ok {
			fmt.Fprintf(stderr, "relay run: --stall-seconds %q: want a whole number of seconds over 0\n%s\n", v, use)
			return exitUsage
		}
	}
	if f["--orchestrate"] != "" { // AR32A act 5 (orchestrator r1 §8): before the lock and the plan
		why := ""
		switch _, until := f["--until"]; {
		case !until && no.MaxUSD == 0:
			why = "want --max-usd or --until: a standing row needs a bound"
		case no.Parallel > 1:
			why = fmt.Sprintf("--parallel %d: an orchestrated night runs one child at a time", no.Parallel)
		case len(no.Rows) > 0:
			why = "--rows: it takes the orchestrator row and every unparked build row"
		default:
			why = orchestrateRefusalFor(f["--no-notify"] != "")
		}
		if why != "" {
			fmt.Fprintf(stderr, "relay run --orchestrate: %s\n", why)
			return exitUsage
		}
		no.Orchestrate = true
	}
	release, code := holdPid("run", stderr, nil)
	if release == nil {
		return code
	}
	defer release()
	reg := &effects.Groups{}
	ctx, stop := runTrap(context.Background(), reg)
	defer stop()
	out := runOutput{Rows: []runReport{}, Resume: []string{}}
	var problems []string // a plan not valid: one printed line each
	finish := func(code int, err error) int {
		if err != nil {
			out.Error = err.Error()
		}
		if asJSON {
			writeJSON(stdout, out)
			return code
		}
		if err != nil && len(problems) > 0 {
			fmt.Fprintf(stderr, "relay run: the plan is not valid, %d problems (relay plan shows each row):\n", len(problems))
			for _, p := range problems {
				fmt.Fprintf(stderr, "  %s\n", p)
			}
		} else if err != nil {
			fmt.Fprintf(stderr, "relay run: %v\n", err)
		}
		if f["--dry"] != "" && len(out.Rows) > 0 {
			fmt.Fprintf(stdout, "run --dry: night %s, %d row(s); nothing staged, nothing spawned\n", out.Night, len(out.Rows))
			home := getenv("HOME")
			for _, r := range out.Rows {
				fmt.Fprintf(stdout, "row %s:\n%s\n", r.Row, strings.Join(night.WrapSpawn(r.Worktree, r.Spawn, home), "\n"))
				fmt.Fprintf(stdout, "  prompt sha256 %s\n  rules sha256  %s\n", r.PromptHash, r.RulesHash)
			}
		}
		if len(out.Resume) > 0 {
			fmt.Fprintln(stdout, "to resume the red or failed rows:")
			for _, l := range out.Resume {
				fmt.Fprintln(stdout, l)
			}
		}
		return code
	}
	p, pd, fx, err := loadPlan("", night.Options{Rows: no.Rows, Until: no.Until, UUID: night.NewUUID, MeasureSeconds: measure, Orchestrate: no.Orchestrate}, reg, true, f["--dry"] == "")
	// AR60 act 1: the end of the planning mark, unless night.Run returns a
	// Night (its status is the record from the open): a signal during the
	// plan is the owner's kill (Run's reason for one), any other way out
	// before the open a queue that did not load, validate or plan (§1).
	end := pd.end
	defer func() {
		if end != nil {
			why := relay.QueueInvalid
			if night.Signalled(ctx) != "" {
				why = relay.OwnerKill
			}
			end(why)
		}
	}()
	if code, ok := trapped(ctx, "run", stderr); ok {
		stop()
		return code
	}
	if err != nil {
		return finish(exitUsage, err)
	}
	out.Night = p.Night
	words := no.Words
	if words == "" {
		words = pd.q.Words
	}
	rows := map[string]queue.Row{}
	for _, r := range pd.q.Rows {
		rows[r.ID] = r
	}
	if len(p.Rows) == 0 || !p.Valid {
		for _, pr := range p.Problems {
			problems = append(problems, strings.TrimSpace(pr.Row+" "+pr.Msg))
		}
		return finish(exitUsage, fmt.Errorf("the plan is not valid: %s", strings.Join(problems, "; ")))
	}
	// Packets are files in this working tree, else the root: read from disk.
	disk := fx
	disk.FS = effects.OSFS{}
	for i := range p.Rows {
		rp := &p.Rows[i]
		rep := runReport{Row: rp.ID, Spawn: rp.Spawn, Worktree: rp.Worktree, SessionID: rp.SessionID}
		if r, ok := rows[rp.ID]; ok && r.Packet != nil {
			if _, rep.RulesHash, rep.PromptHash, err = night.RenderPrompt(disk, p, r, rp, words, pd.repo); err != nil {
				return finish(exitUsage, err)
			}
		}
		out.Rows = append(out.Rows, rep)
	}
	if f["--dry"] != "" {
		return finish(exitOK, nil)
	}
	lines := stdout
	if asJSON {
		lines = stderr
	}
	no.Plan, no.State, no.Out = p, pd.state, lines
	mode, err := pd.cfg.TmuxMode()
	if err != nil {
		return finish(exitUsage, err)
	}
	no.Tmux, no.TmuxMode = newTmux(pd.state), mode
	no.Notify = effects.LogNotify{W: lines}
	if f["--no-notify"] == "" && pd.cfg.Ntfy.URL != "" && pd.cfg.Ntfy.Topic != "" {
		no.Notify = effects.Multi(no.Notify, effects.Ntfy{URL: pd.cfg.Ntfy.URL, Topic: pd.cfg.Ntfy.Topic})
	}
	if no.Orchestrate {
		no.QueuePath = pd.cfg.Queue // clean and repo-relative: LoadConfig made it so
	}
	no.Row = func(r queue.Row, rp *night.RowPlan) ([]byte, night.RunOptions, error) {
		v := gate.Verify{Name: r.Verify, Baseline: p.Verify, Harness: r.Harness, Source: pd.cfg.Packets, QueuePath: no.QueuePath, Limit: pd.cfg.VerifyLimit()}
		pk, ro := r.Packet, night.RunOptions{}
		var stdin []byte
		var err error
		if rp.Sitting != nil { // an orchestrator sitting (AR32A act 5): its prompt, its gate
			stdin, ro.RulesHash, _ = night.RenderSitting(p, r, rp, words)
			ro.Block = p.BlockSource
			v.Orchestrator, v.CopyInSkipped = &gate.Orchestrated{Row: r, Night: pd.q, Began: rp.Sitting.Began}, true
			pk = &packet.Packet{ID: rp.ID}
		} else {
			stdin, _, _, err = night.RenderPrompt(disk, p, r, rp, words, pd.repo)
		}
		ro.Gate = func(st *night.Staged, seen, isError bool) (relay.Result, relay.StopReason, relay.Gate) {
			res, why, g, _ := gate.Run(fx, st, pk, rp.Baseline, &gate.ExitFacts{Seen: seen, IsError: isError}, v) // the gate line holds the detail
			return res, why, g
		}
		return stdin, ro, err
	}
	no.Merge = func(r queue.Row, g relay.Gate) (string, error) {
		return gate.MergeIfGreen(fx, p.Root, g, r.ID, pd.q.Defaults.Verify, p.Verify)
	}
	n, err := night.Run(fx, spawner(pd.cfg), pd.q, no)
	if n == nil {
		return finish(exitUsage, err)
	}
	end = nil // the night opened: Run's status replaced the mark
	out.State, out.Reason = string(n.State), string(n.Reason)
	for i := range out.Rows {
		r := &out.Rows[i]
		if s := n.Sittings[r.Row]; s != nil {
			r.State, r.Reason = s.State, s.Reason
			r.Log = filepath.Join(night.NightDir(pd.state, n.ID), r.Row+".log")
			if s.State == relay.Red || s.State == relay.Failed {
				r.Resume = "relay resume " + r.Row
				out.Resume = append(out.Resume, r.Resume)
			}
		}
	}
	if err != nil {
		return finish(exitUsage, err)
	}
	if !asJSON {
		night.PrintStatus(stdout, night.ReadStatus(effects.OSFS{}, pd.state, n.ID))
		out.Resume = nil // the dashboard printed them
	}
	if n.Reason == relay.LoadOpen { // the breaker still open before a spawn: a pre-check (§15)
		return finish(exitUsage, nil)
	}
	if night.Clean(n) {
		return finish(exitOK, nil)
	}
	return finish(exitRed, nil)
}

// stateAndNight is the state directory and the night a verb reads: --night
// when given, else the latest ("" when there is none).
func stateAndNight(f map[string]string) (string, string) {
	state := effects.StateDir(getenv)
	if n := f["--night"]; n != "" {
		return state, n
	}
	return state, night.Latest(effects.OSFS{}, state)
}

// statusVerb is `relay status [--night D] [--json] [--watch N [--hold]]`:
// the night's status.json, printed as the dashboard once; with no night, an
// empty one. --watch N redraws it in-process every N seconds (N from 1 to
// maxWatch) until the night stops or its relay is gone, prints it once more and
// exits 0; --hold then waits for a signal so the pane keeps the last table
// (AR33 act 1, the session's first window).
func statusVerb(args []string, stdout, stderr io.Writer) int {
	const use = "usage: relay status [--night D] [--json] [--watch N [--hold]]"
	f, _, err := flagSet(args, []string{"--night", "--watch"}, []string{"--json", "--hold"}, false)
	every := 0
	if err == nil {
		switch w, watching := f["--watch"]; {
		case watching:
			if every, err = strconv.Atoi(w); err != nil || every < 1 || every > maxWatch {
				err = fmt.Errorf("--watch %q: want seconds, 1 to %d", w, maxWatch)
			} else if f["--json"] != "" {
				err = fmt.Errorf("--watch prints the dashboard, not --json")
			}
		case f["--hold"] != "":
			err = fmt.Errorf("--hold needs --watch")
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "relay status: %v\n%s\n", err, use)
		return exitUsage
	}
	state, n := stateAndNight(f)
	if every > 0 {
		night.Watch(stdout, effects.OSFS{}, state, n, time.Duration(every)*time.Second, watchSleep, watchLive())
		if f["--hold"] != "" {
			watchHold()
		}
		return exitOK
	}
	st := night.ReadStatus(effects.OSFS{}, state, n)
	// AR56 ruling 2: a row merged by hand after the night reads merged at
	// git's sha, with no resume line; a ledger it cannot read leaves it.
	if lines, err := effects.NewLedger(state, effects.SystemClock{}, "").Lines(); err == nil {
		st.TakeMerges(night.HandMerges(effects.OSGit{}, lines, n))
	}
	if f["--json"] != "" {
		writeJSON(stdout, st)
	} else {
		night.PrintStatus(stdout, st)
	}
	return exitOK
}

// maxWatch is --watch's ceiling in seconds, a day: above about 9.2e9
// seconds time.Duration overflows to a negative sleep and the watch would
// spin a core (AR33 fix 2).
const maxWatch = 86400

// The watch's effects (AR33 act 1), a test's to replace: the sleep between
// redraws, the night's relay's liveness (night.Liveness), and --hold's wait
// for a signal, which uses no CPU.
var (
	watchSleep = time.Sleep
	watchLive  = night.Liveness
	watchHold  = func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
		<-c
	}
)

// controlVerb is `relay stop` and `relay kill [<row>]`: one line appended
// to the latest night's control file, read by the night's loop after every
// event; a night with no status.json yet, or one reading `planning` (its
// plan's mark, AR60 act 2), is still measuring, and its
// measure reads the file within a poll and no row starts (AR34 act 2). The
// line names the night's root from its status (ReadStatus: status.json's,
// else groups.json's, else the ledger's; AR37 fix), and a cwd in another
// repo, by git common directory, adds a note (AR37 act 2 and fix). Exit 2
// when there is no night.
func controlVerb(verb string, args []string, stdout, stderr io.Writer) int {
	positional := verb == "kill"
	f, row, err := flagSet(args, nil, []string{"--json"}, positional)
	if err != nil {
		fmt.Fprintf(stderr, "relay %s: %v\nusage: relay stop | relay kill [<row>]\n", verb, err)
		return exitUsage
	}
	state, n := stateAndNight(f)
	if n == "" {
		fmt.Fprintf(stderr, "relay %s: no night under %s\n", verb, state)
		return exitUsage
	}
	line := strings.TrimSpace(verb + " " + row)
	if err := night.WriteControl(effects.OSFS{}, night.ControlPath(state, n), line); err != nil {
		fmt.Fprintf(stderr, "relay %s: %v\n", verb, err)
		return exitUsage
	}
	// AR37 act 2: the account holds every repo's nights (§8), so the line
	// names the night's root when its status does, and a cwd in another
	// repo gets a note; the file is written all the same.
	st := night.ReadStatus(effects.OSFS{}, state, n)
	root := st.Root
	in := ""
	if root != "" {
		in = " in " + night.ShowRoot(root)
	}
	if f["--json"] != "" {
		writeJSON(stdout, map[string]string{"night": n, "control": line})
		return exitOK
	}
	// A status.json that does not parse is an open night's half write
	// (AR37 fix), so only a missing file reads as measuring, not "".
	if st.State == string(relay.Planning) || !(effects.OSFS{}).Exists(filepath.Join(night.NightDir(state, n), "status.json")) {
		fmt.Fprintf(stdout, "relay %s: night %s%s: %q written; the night is still measuring: it stops within a few seconds, before any row starts\n", verb, n, in, line)
	} else {
		fmt.Fprintf(stdout, "relay %s: night %s%s: %q written; the loop reads it after its next event\n", verb, n, in, line)
	}
	if top, err := toplevel(); root != "" && err == nil && top != "" && !sameRepo(top, root) {
		fmt.Fprintf(stdout, "note: this night runs in %s, not in this repo\n", night.ShowRoot(root))
	}
	return exitOK
}

// sameRepo is two trees of one repo (AR37 fix): the same git common
// directory, so a linked worktree of the night's root is its repo; when
// git cannot read either, the two paths are compared (samePath).
func sameRepo(a, b string) bool {
	ca, errA := commonDir(a)
	cb, errB := commonDir(b)
	if errA == nil && errB == nil {
		return ca == cb
	}
	return samePath(a, b)
}

// commonDir is the git common directory of the tree at dir, absolute,
// cleaned and its symlinks resolved.
func commonDir(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", err
	}
	d := strings.TrimSpace(string(out))
	if d == "" {
		return "", fmt.Errorf("git rev-parse --git-common-dir in %s: empty", dir)
	}
	if !filepath.IsAbs(d) {
		d = filepath.Join(dir, d)
	}
	if r, err := filepath.EvalSymlinks(d); err == nil {
		return r, nil
	}
	return filepath.Clean(d), nil
}

// samePath is two directories that are one: equal once cleaned, or once
// their symlinks resolve (git's top level is resolved, a root may not be).
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// execResume replaces the process with argv in dir; a variable so a test
// can fake it.
var execResume = func(dir string, argv []string) error {
	bin, err := lookPath(argv[0])
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	// The account's lock outlives the exec (AR21 act 2, AR22 act 2): the
	// resumed sitting is the account's one relay until it exits.
	return night.ExecHolding(bin, argv, os.Environ())
}

// execAttach replaces the process with argv; a variable so a test can fake
// it.
var execAttach = func(argv []string) error {
	bin, err := lookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(bin, argv, os.Environ())
}

// attachVerb is `relay attach [<row>] [--night D] [--print]` (§9, D16): with
// the night's tmux session alive, `tmux attach` to it or to the row's
// window; without, `tail -f` the row's log (no row: every row's). The line
// is printed, then run by exec; --print stops after printing. Exit 2 for no
// night or a row the night does not hold.
func attachVerb(args []string, stdout, stderr io.Writer) int {
	f, row, err := flagSet(args, []string{"--night"}, []string{"--print"}, true)
	if err != nil {
		fmt.Fprintf(stderr, "relay attach: %v\nusage: relay attach [<row>] [--night D] [--print]\n", err)
		return exitUsage
	}
	state, n := stateAndNight(f)
	if n == "" {
		fmt.Fprintln(stderr, "relay attach: no night")
		return exitUsage
	}
	st := night.ReadStatus(effects.OSFS{}, state, n)
	st.Night = n
	argv, err := night.Attach(newTmux(state), st, state, row)
	if err != nil {
		fmt.Fprintf(stderr, "relay attach: %v\n", err)
		return exitUsage
	}
	fmt.Fprintln(stdout, night.TildeQuote(argv, getenv("HOME")))
	if f["--print"] != "" {
		return exitOK
	}
	if err := execAttach(argv); err != nil {
		fmt.Fprintf(stderr, "relay attach: %v\n", err)
		return exitUsage
	}
	return exitOK
}

// holdPid is the account's lock (relay-r1 §8, §15; AR17B, AR21 act 2,
// AR22 act 2; ADV10-6, ADV11-9, ADV12-3, ADV13-4, -14): run, a measuring
// plan, resume and gate hold night.PidFile, the loopback port fixed from
// the uid, with relay.pid at night.PidDir (a fixed path from the account's
// home, never the environment) naming the holder, and refuse, exit 2,
// naming a live holder's pid or none, while the port is held, ancestor or
// not, however the environment was changed. There is no bypass flag: --parallel is the concurrency door.
// refused, when set, runs before a live holder's refusal is printed. A
// state root that is not absolute (no HOME, no XDG_STATE_HOME) is refused.
func holdPid(verb string, stderr io.Writer, refused func()) (release func(), code int) {
	if state := effects.StateDir(getenv); !filepath.IsAbs(state) {
		fmt.Fprintf(stderr, "relay %s: the state root %q is not absolute (set HOME or XDG_STATE_HOME)\n", verb, state)
		return nil, exitUsage
	}
	dir, err := night.PidDir()
	if err != nil {
		fmt.Fprintf(stderr, "relay %s: the pid file: %v\n", verb, err)
		return nil, exitUsage
	}
	release, err = night.PidFile(dir)
	if err != nil {
		if ne := (night.NestedError{}); errors.As(err, &ne) && refused != nil {
			refused()
		}
		fmt.Fprintf(stderr, "relay %s: %v\n", verb, err)
		return nil, exitUsage
	}
	return release, exitOK
}

// resumeVerb is `relay resume [<row>]`: with a row, `claude --resume
// <uuid>` printed and run (interactively, by exec) in the tree git lists at
// <root>/.claude/worktrees/<row>, the uuid its newest Claude Code
// transcript's; a row with no branch, tree or transcript gets the picker,
// `claude --resume`, in the root (resumeDoor). The ledger's session_id and
// worktree are only printed, never run (AR22 act 3, ADV13-8). With none,
// the latest night's resume lines. Exit 2 for a row with no sitting (the
// ledger may refuse, never direct).
func resumeVerb(args []string, stdout, stderr io.Writer) int {
	// The run guard (ADV10-18): a sitting never resumes one, as `relay run`.
	if night.Nested(getenv) {
		fmt.Fprintf(stderr, "relay resume: %v\n", night.ErrNested)
		return exitUsage
	}
	f, row, err := flagSet(args, nil, []string{"--json"}, true)
	if err != nil {
		fmt.Fprintf(stderr, "relay resume: %v\nusage: relay resume [<row>]\n", err)
		return exitUsage
	}
	// A refused resume prints the line it would exec (as --print does), so
	// the owner's door stays obvious (AR21 act 2).
	release, code := holdPid("resume", stderr, func() {
		if row == "" {
			return
		}
		state, _ := stateAndNight(f)
		if lines, err := effects.NewLedger(state, effects.SystemClock{}, "").Lines(); err == nil {
			if session, tree := resumeTarget(lines, row); session != "" && tree != "" {
				if dir, argv, err := resumeDoor(effectsFor(state, "").Git, row); err == nil {
					printResume(stdout, dir, argv, session, tree)
				}
			}
		}
	})
	if release == nil {
		return code
	}
	defer release()
	_, stop := night.Trap(context.Background(), &effects.Groups{}) // measures nothing; exec resets it
	defer stop()
	state, n := stateAndNight(f)
	lines, err := effects.NewLedger(state, effects.SystemClock{}, "").Lines()
	if err != nil {
		fmt.Fprintf(stderr, "relay resume: the ledger: %v\n", err)
		return exitUsage
	}
	if row == "" {
		r := night.BuildReport(lines, n, effects.OSGit{}) // a hand-merged row has no resume (AR56)
		if f["--json"] != "" {
			writeJSON(stdout, map[string]any{"night": n, "resume": r.Resume})
			return exitOK
		}
		switch {
		case n == "":
			fmt.Fprintln(stdout, "no night yet")
		case len(r.Resume) == 0:
			fmt.Fprintf(stdout, "night %s: nothing to resume\n", n)
		default:
			fmt.Fprintf(stdout, "night %s: %d red or failed row(s) to resume\n", n, len(r.Resume))
		}
		for _, l := range r.Resume {
			fmt.Fprintln(stdout, l)
		}
		return exitOK
	}
	session, tree := resumeTarget(lines, row)
	if session == "" || tree == "" {
		fmt.Fprintf(stderr, "relay resume: row %s: no sitting in the ledger\n", row)
		return exitUsage
	}
	dir, argv, err := resumeDoor(effectsFor(state, "").Git, row)
	if err != nil {
		fmt.Fprintf(stderr, "relay resume: row %s: %v\n", row, err)
		return exitUsage
	}
	printResume(stdout, dir, argv, session, tree)
	if err := execResume(dir, argv); err != nil {
		fmt.Fprintf(stderr, "relay resume: %v\n", err)
		return exitUsage
	}
	return exitOK
}

// resumeTarget is row's last sitting's session and its worktree, from the
// ledger's lines; "" when there is none. They are claims: resume prints
// them and refuses a row with none, never execs them (AR22 act 3).
func resumeTarget(lines []map[string]any, row string) (session, tree string) {
	for _, l := range lines {
		if l["row"] != row {
			continue
		}
		switch l["event"] {
		case "sitting":
			session, _ = l["session_id"].(string)
		case "stage":
			tree, _ = l["worktree"].(string)
		}
	}
	return session, tree
}

// resumeLine is the line resume execs: `cd <dir> && claude --resume [<uuid>]`.
func resumeLine(dir string, argv []string) string {
	return fmt.Sprintf("cd %s && %s", night.TildeQuote([]string{dir}, getenv("HOME")), night.Quote(argv))
}

// printResume prints the line resume execs, then the ledger's claim,
// quoted, as a claim only.
func printResume(w io.Writer, dir string, argv []string, session, tree string) {
	fmt.Fprintln(w, resumeLine(dir, argv))
	fmt.Fprintf(w, "  the ledger's claim (printed, never run): session %q in %q\n", session, tree)
}

// resumeDoor is the dir and argv resume execs for row, from git and
// Claude Code's store alone (AR22 act 3, ADV13-8): the root is the pinned
// git's for this directory; a row with a branch whose tree `git worktree
// list` holds at exactly <root>/.claude/worktrees/<row> (its real path
// that one, no link) and whose store holds a transcript gets `claude
// --resume <uuid>` there, the newest's; any other row the picker,
// `claude --resume`, in the root.
func resumeDoor(g effects.Git, row string) (string, []string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	root, err := g.Root(wd)
	if err != nil || root == "" {
		return "", nil, fmt.Errorf("the root: %v", err)
	}
	picker := []string{"claude", "--resume"}
	if row == "" || row != filepath.Base(row) || row == "." || row == ".." || strings.HasPrefix(row, "-") {
		return root, picker, nil
	}
	if ok, err := g.BranchExists(root, row); err != nil || !ok {
		return root, picker, nil
	}
	wts, err := g.Worktrees(root)
	if err != nil {
		return root, picker, nil
	}
	want := filepath.Join(root, ".claude", "worktrees", row)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return root, picker, nil
	}
	for _, wt := range wts {
		if filepath.Clean(wt.Path) != want {
			continue
		}
		tree, err := filepath.EvalSymlinks(want)
		if err != nil || tree != filepath.Join(realRoot, ".claude", "worktrees", row) {
			return root, picker, nil
		}
		if id := newestTranscript(getenv("HOME"), tree); id != "" {
			return tree, []string{"claude", "--resume", id}, nil
		}
	}
	return root, picker, nil
}

// transcriptID is a Claude Code transcript's stem: a lowercase UUID.
var transcriptID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// newestTranscript is the session id of the newest transcript Claude Code
// keeps for dir under home: <home>/.claude/projects/<dir, each character
// not a letter or digit a '-'>/<uuid>.jsonl, a regular file, the latest
// modified (a tie: the greater name); "" when there is none. A stem that
// is not a UUID is never read as one.
func newestTranscript(home, dir string) string {
	if home == "" {
		return ""
	}
	store := filepath.Join(home, ".claude", "projects", regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(dir, "-"))
	entries, err := os.ReadDir(store)
	if err != nil {
		return ""
	}
	var best string
	var at time.Time
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if !ok || !transcriptID.MatchString(id) || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if m := info.ModTime(); best == "" || m.After(at) || (m.Equal(at) && id > best) {
			best, at = id, m
		}
	}
	return best
}

// reportVerb is `relay report [--night D] [--write PATH] [--json]`: the
// morning read from the ledger, as markdown (the root written `.`, home
// `~`); --write puts it at PATH relative to this repo (D18), the only write
// into a repo. A night with no `night-open` line takes its state from its
// status (TakeStatus): `planning`, or its mark's end (AR60 act 2).
func reportVerb(args []string, stdout, stderr io.Writer) int {
	f, _, err := flagSet(args, []string{"--night", "--write"}, []string{"--json"}, false)
	if err != nil {
		fmt.Fprintf(stderr, "relay report: %v\nusage: relay report [--night D] [--write PATH] [--json]\n", err)
		return exitUsage
	}
	state, n := stateAndNight(f)
	lines, err := effects.NewLedger(state, effects.SystemClock{}, "").Lines()
	if err != nil {
		fmt.Fprintf(stderr, "relay report: the ledger: %v\n", err)
		return exitUsage
	}
	r := night.BuildReport(lines, n, effects.OSGit{})        // a hand merge read from git (AR56)
	r.TakeStatus(night.ReadStatus(effects.OSFS{}, state, n)) // a night with no night-open line: its status's state (AR60 act 2)
	if f["--json"] != "" {
		writeJSON(stdout, r)
		return exitOK
	}
	md := night.Markdown(r, night.RootOf(lines, n), getenv("HOME"))
	if rel := f["--write"]; rel != "" {
		repo, err := toplevel()
		if err != nil || repo == "" {
			fmt.Fprintf(stderr, "relay report: --write needs a repo: %v\n", err)
			return exitUsage
		}
		if _, err := night.WriteReport(effects.OSFS{}, repo, rel, md); err != nil {
			fmt.Fprintf(stderr, "relay report: %v\n", err)
			return exitUsage
		}
		fmt.Fprintf(stdout, "relay report: wrote %s\n", rel)
		return exitOK
	}
	stdout.Write(md)
	return exitOK
}
