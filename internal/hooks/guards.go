package hooks

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Guard denies a Bash command when Pattern matches one of its pipeline
// segments as Normalise writes it: `<command> <args>`, the command word's
// base name after any wrapper, git's global options dropped (a config one
// that defines a command kept), and the spaces inside a quoted word turned
// to `_`, so a commit message never reads as a verb.
type Guard struct {
	Name    string
	Pattern *regexp.Regexp
	Why     string
	Unless  *regexp.Regexp            // a segment Pattern matched that this matches is let through (nil: none)
	Shape   func(words []string) bool // set: the guard reads the segment's words, not its line (Pattern nil)
}

// deviceVerbs are the remote-shell commands; the owner's device verbs come
// from RELAY_DEVICE_VERBS at run time, never from this table.
const deviceVerbs = "ssh|scp|sftp|rsync|mosh|telnet"

// Push is the push guard (relay-r1 §10): a sitting (RELAY_ROW set, as
// ChildEnv sets it) never pushes; outside one the owner may (the owner,
// 2026-09-24), so guards adds it only in a row, ahead of every other.
var Push = Guard{"push", regexp.MustCompile(`^(git (push|send-pack|http-push|subtree push)|git-(push|send-pack|http-push))(\s|$)`), "pushing is the owner's act, never a sitting's", nil, nil}

// Guards is the one table; device is widened by RELAY_DEVICE_VERBS in guards.
var Guards = []Guard{
	// git takes any unambiguous prefix of a long option (ADV6-6).
	{"amend", regexp.MustCompile(`^git (commit(\s.*)?\s--am(e|en|end)?(\s|=|$)|commit-tree(\s|$)|reset(\s.*)?\s--s(o(ft?)?)?(\s|$))`), "history is append-only, so make a new commit instead", nil, nil},
	{"rebase", regexp.MustCompile(`^git (rebase(\s|$)|pull(\s.*)?\s(--reb(a|as|ase)?|-r)(\s|=|$)|-c (?i:pull\.rebase|branch\.\S+\.rebase)(=|\s|$)|config(\s.*)?\s(?i:pull\.rebase|branch\.\S+\.rebase)(\s|$))`), "history is append-only, so leave rewriting it to the owner", nil, nil},
	{"no-verify", regexp.MustCompile(`^git (\S+(\s.*)?\s--no-veri(f|fy)?(\s|=|$)|commit(\s.*)?\s-[aeiopqsvz]*n[a-zA-Z]*(\s|$))`), "the hooks are the relay's checks, so let them run", nil, nil},
	{"alias", regexp.MustCompile(`^git (-c (?i:alias|include|includeif)\.|config(\s.*)?\s(?i:alias|include|includeif)\.)`), "an alias hides the verb from these guards", nil, nil},
	{"interpreter", regexp.MustCompile(`^(python[0-9.]*|perl[0-9.]*|ruby|node|nodejs|bun|php|lua|osascript|tclsh)\s(.*\s)?-[a-zA-Z]*[ceE]\S*\s(.*[^A-Za-z0-9-])?git([^A-Za-z0-9-]|$)`), "a one-liner that runs git hides it from these guards, so run git itself", nil, nil},
	{"device", devicePattern(nil), "a sitting never touches a device", nil, nil},
}

// NestedRun is the run guard's hook (relay-r1 §15, AR11, AR17B; C4): in a
// sitting (RELAY_ROW set, as ChildEnv sets it) no relay starts a night,
// gates, resumes, or plans without --rows. It reads the argv, not the
// spelling (ADV10-6, -18): a binary named relay or a cmd/relay path, with
// run, plan, gate or resume anywhere after it; a plan with --rows passes
// unless the segment sets or drops RELAY_*. go build and go install of any
// path that cleans to cmd/relay are denied whatever follows, since the
// binary may run under any name (ADV11-9). The binary refuses too
// (night.PidFile); this denies the call before it is made.
var NestedRun = Guard{"nested-run", nil, "a sitting never builds the relay, starts a night, gates, resumes, nor plans without --rows", nil, nestedRun}

// relayPath reports a word naming the relay's main package: a path (or a
// .go file in one) that filepath.Cleans to cmd/agentik-relay or the old
// cmd/relay, or ends in either; with bare, also one that cleans to
// agentik-relay or relay (`cd cmd && go run ./agentik-relay`). The old
// spelling stays denied (agentik-r1 §6.3 c12).
func relayPath(w string, bare bool) bool {
	c := filepath.Clean(w)
	if strings.HasSuffix(c, ".go") {
		c = filepath.Dir(c)
	}
	for _, m := range relayMains {
		if c == "cmd/"+m || strings.HasSuffix(c, "/cmd/"+m) || strings.Contains(w, "cmd/"+m) || bare && c == m {
			return true
		}
	}
	return false
}

// relayMains are the relay's main package names, the new and the old.
var relayMains = []string{"agentik-relay", "relay"}

// relayName reports words[j], the command word, naming the relay under any
// of its spellings (agentik-r1 §6.3 c12): a binary named agentik-relay or
// relay, or the front door agentik with relay next, whose relay word is
// then the binary (at is j+1).
func relayName(words []string, j int) (at int, ok bool) {
	switch filepath.Base(words[j]) {
	case "agentik-relay", "relay":
		return j, true
	case "agentik":
		if j+1 < len(words) && words[j+1] == "relay" {
			return j + 1, true
		}
	}
	return -1, false
}

// HelperRelay is the helper's run guard (relay-r1 §15, AR40): a Bash call
// whose hook input carries an agent_id or agent_type (a subagent's, of any
// session, in a row or out of one) builds no relay and runs no relay verb
// that writes. It reads the argv as NestedRun does (relayBin), with plan
// --rows denied too. preToolUse adds it; guards never does, since the
// helper is named in the payload, not the env.
var HelperRelay = Guard{"helper-relay", nil, "a helper never builds the relay, starts, gates, resumes, plans, stops, kills or cleans a night; the session that spawned it does", nil, helperRelay}

// helperVerbs are the relay verbs that write a night: HelperRelay denies
// each wherever it follows the binary, as nestedRun reads its own.
var helperVerbs = map[string]bool{"run": true, "plan": true, "gate": true, "resume": true, "stop": true, "kill": true, "clean": true}

// relayBin finds the relay in a segment's argv (ADV10-6, -18; ADV11-9). i
// is the command word. build is a go build or go install of a path that
// cleans to cmd/agentik-relay or cmd/relay, the binary may then run under
// any name; otherwise bin is go run's package, a command word naming the
// relay (relayName: agentik-relay, relay, or agentik relay, whose bin is
// the relay word), or any such path, and -1 when there is none.
func relayBin(words []string) (i, bin int, build bool) {
	i = commandWord(words)
	bin = -1
	if i+1 < len(words) && filepath.Base(words[i]) == "go" {
		switch verb := words[i+1]; verb {
		case "build", "install", "run":
			for j := i + 2; j < len(words); j++ {
				if words[j] == "-o" {
					j++ // the output's name, never a package
					continue
				}
				if relayPath(words[j], true) {
					if verb != "run" {
						return i, -1, true
					}
					bin = j
					break
				}
			}
		}
	}
	for j := i; bin < 0 && j < len(words); j++ {
		if j == i {
			if at, ok := relayName(words, j); ok {
				bin = at
				break
			}
		}
		if relayPath(words[j], false) {
			bin = j
		}
	}
	return i, bin, false
}

func helperRelay(words []string) bool {
	_, bin, build := relayBin(words)
	if build || bin < 0 {
		return build
	}
	for _, w := range words[bin+1:] {
		if helperVerbs[w] {
			return true
		}
	}
	return false
}

func nestedRun(words []string) bool {
	i, bin, build := relayBin(words)
	if build {
		return true
	}
	if bin < 0 {
		return false
	}
	verb, rows := "", false
	for _, w := range words[bin+1:] {
		switch {
		case verb == "" && (w == "run" || w == "plan" || w == "gate" || w == "resume"):
			verb = w
		case w == "--rows" || strings.HasPrefix(w, "--rows="):
			rows = true
		}
	}
	if verb != "plan" || !rows {
		return verb != ""
	}
	for j, w := range words[:i] {
		if isAssignment(w) && strings.HasPrefix(w, "RELAY_") || strings.HasPrefix(w, "-u") &&
			(strings.HasPrefix(w[2:], "RELAY_") || w == "-u" && j+1 < i && strings.HasPrefix(words[j+1], "RELAY_")) {
			return true
		}
	}
	return false
}

// RowShapes are the by-shape guards a sitting adds (relay-r1 §7, AR17B;
// ADV10-3): a command word the guards cannot read is denied, never
// enumerated by spelling. Outside a row they are absent.
var RowShapes = []Guard{
	{"literal", nil, "a command word that is not a literal word hides the command from these guards", nil,
		func(words []string) bool { return !wordsLiteral(words) }},
	{"stdin-shell", nil, "a shell fed from stdin, a here-string or a pipe hides its commands from these guards", nil, stdinShell},
	// ADV10-1's door: main moves only by the gate's merge.
	{"branch-move", regexp.MustCompile(`^git (update-ref(\s|$)|branch\s(.*\s)?(-[a-zA-Z]*[fMDC][a-zA-Z]*|--force)(\s|$)|(checkout|switch)\s(.*\s)?(-[a-zA-Z]*[BC]|--force-create)(\s|=|$)|fetch\s(.*\s)?[^\s:]*:[^/\s]\S*)`),
		"a branch moves only by the gate's merge, never a sitting's", nil, nil},
	{"other-tree", nil, "a sitting's git runs in its own tree, so name no other", nil, gitElsewhere},
	// ADV10-11: the relay worktree's hooks are the repo's plus its pre-push.
	{"hooks-path", regexp.MustCompile(`^git (-c (?i:core\.hookspath)|config(\s.*)?\s(?i:core\.hookspath)(\s|$))`),
		"the hooks are the relay's checks, so leave core.hooksPath alone", nil, nil},
}

// gitEnvName is git's environment that names another tree or config: a
// sitting that set it would step past ChildEnv's pushurl or its own tree.
var gitEnvName = regexp.MustCompile(`^(GIT_DIR|GIT_WORK_TREE|GIT_COMMON_DIR|GIT_INDEX_FILE|GIT_CONFIG[A-Z0-9_]*)$`)

// gitElsewhere reports a segment that points git at a tree or config
// other than the sitting's own: git's -C, --git-dir or --work-tree, or
// git's environment set, exported, unset or dropped (envEdits).
func gitElsewhere(words []string) bool {
	if edits, _ := envEdits(words, gitEnvName.MatchString); len(edits) > 0 {
		return true
	}
	i := commandWord(words)
	if i == len(words) || filepath.Base(words[i]) != "git" {
		return false
	}
	for _, w := range words[i+1:] {
		if !strings.HasPrefix(w, "-") {
			break
		}
		if w == "-C" || w == "--git-dir" || w == "--work-tree" ||
			strings.HasPrefix(w, "--git-dir=") || strings.HasPrefix(w, "--work-tree=") {
			return true
		}
	}
	return false
}

// envEdit is one change a segment makes to a name in its environment: set
// to value, or dropped (set false).
type envEdit struct {
	name, value string
	set         bool
}

// envBuiltins set, export, unset or shadow a shell variable by name.
var envBuiltins = map[string]bool{"unset": true, "export": true, "declare": true, "typeset": true, "readonly": true, "local": true}

// envEdits reads the changes a segment makes to the names named reports:
// a NAME=value word anywhere (a prefix, env's, export's, make's); env's
// -u NAME, -uNAME, --unset=NAME or --unset NAME; and a builtin of
// envBuiltins naming NAME, a drop when bare or after an option (`export
// -n`, `declare +x`). all reports env -i, -, --ignore-environment, or
// exec -c: every name dropped.
func envEdits(words []string, named func(string) bool) (edits []envEdit, all bool) {
	i := commandWord(words)
	builtin, opts := i < len(words) && envBuiltins[filepath.Base(words[i])], false
	drop := func(name string) {
		if named(name) {
			edits = append(edits, envEdit{name: name})
		}
	}
	for j, w := range words {
		if builtin && j > i && (strings.HasPrefix(w, "-") || strings.HasPrefix(w, "+")) {
			opts = true
		}
		if isAssignment(w) {
			name, value, _ := strings.Cut(w, "=")
			if named(name) {
				edits = append(edits, envEdit{name, value, !(builtin && j > i && opts)})
			}
			continue
		}
		if builtin && j > i {
			drop(w)
			continue
		}
		if j >= i {
			continue
		}
		switch filepath.Base(w) {
		case "env":
			// env's options end at its first word with no dash; a lone `-`
			// (-i) is never read as an option by commandWord, so read past i
			for k := j + 1; k < len(words) && strings.HasPrefix(words[k], "-"); k++ {
				a := words[k]
				switch {
				case a == "-" || a == "--ignore-environment":
					all = true
				case a == "--":
					k = len(words)
				case a == "--unset" || a == "--chdir" || a == "--split-string":
					if k++; a == "--unset" && k < len(words) {
						drop(words[k])
					}
				case strings.HasPrefix(a, "--unset="):
					drop(a[len("--unset="):])
				case strings.HasPrefix(a, "--"):
				default:
					for n := 1; n < len(a); n++ {
						if a[n] == 'i' {
							all = true
						}
						if strings.IndexByte(wrappers["env"].valued, a[n]) < 0 {
							continue
						}
						v := a[n+1:]
						if v == "" && k+1 < len(words) {
							k++
							v = words[k]
						}
						if a[n] == 'u' {
							drop(v)
						}
						break
					}
				}
			}
		case "exec":
			for k := j + 1; k < i && strings.HasPrefix(words[k], "-") && words[k] != "--"; k++ {
				if words[k] == "-a" {
					k++
				} else if strings.Contains(words[k], "c") {
					all = true
				}
			}
		}
	}
	return edits, all
}

// MaxHelperDepth is the deepest a helper runs (AR25 act 0; ADV15-1): a row
// at depth 1 runs one at depth 2; a helper runs none.
const MaxHelperDepth = 2

// markerNames are the sitting's markers (effects.ChildEnv sets them): the
// row, the depth and the turns the session meter counts; and a helper's
// mark (AR27B act 1; ADV16-1), which makes it a row (inRow).
var markerNames = map[string]bool{"RELAY_ROW": true, "CLAUDE_SESSION_DEPTH": true, "CLAUDE_SESSION_MAX_TURNS": true, "CLAUDE_SESSION_HELPER": true}

// markers is the markers guard at the hook's depth d (AR25 act 0;
// ADV15-1): a segment that drops, blanks or reassigns RELAY_ROW or
// CLAUDE_SESSION_MAX_TURNS, drops, blanks or lowers CLAUDE_SESSION_DEPTH
// below d, or drops CLAUDE_SESSION_HELPER or sets it but to 1, in any form
// envEdits reads, is denied; env -i drops them all. Raising the depth is
// let through. AR27B act 1 (ADV16-1): so is any word that names a marker
// but to read it (namesMarker), since read, for, print -v, (( )) and kin
// assign by name.
func markers(d int) Guard {
	return Guard{"markers", nil, "the sitting's markers ride every command, so never drop, blank or reassign RELAY_ROW, CLAUDE_SESSION_MAX_TURNS or CLAUDE_SESSION_HELPER, nor drop or lower CLAUDE_SESSION_DEPTH; name one only to read it ($NAME or ${NAME}), or search for it with grep or rg (a commit message naming one goes by `git commit -F - <<'EOF'`)", nil,
		func(words []string) bool {
			edits, all := envEdits(words, func(n string) bool { return markerNames[n] })
			for _, e := range edits {
				n, err := strconv.Atoi(e.value)
				switch {
				case !e.set:
					return true
				case e.name == "CLAUDE_SESSION_DEPTH" && err == nil && n >= d:
				case e.name == "CLAUDE_SESSION_HELPER" && e.value == "1":
				default:
					return true
				}
			}
			return all || namesMarker(words)
		}}
}

// namesMarker reports a word of a segment naming a marker, the name bounded
// by non-identifier characters anywhere in the word, other than as a $NAME
// or ${NAME} read (AR27B act 1; ADV16-1). The helper line's two prefix
// words (the helper guard judges them) and a search's operands (a segment
// whose command word is grep or rg, which assigns nothing) are let
// through; a redirection's target never is.
func namesMarker(words []string) bool {
	i := commandWord(words)
	cmd := ""
	if i < len(words) {
		cmd = filepath.Base(words[i])
	}
	for j, w := range words {
		if j < i && cmd == "claude" && isAssignment(w) {
			if name, _, _ := strings.Cut(w, "="); name == "CLAUDE_SESSION_DEPTH" || name == "CLAUDE_SESSION_HELPER" {
				continue
			}
		}
		if j > i && searchOperand(words, i, j) {
			continue
		}
		if markerIn(w) {
			return true
		}
	}
	return false
}

// searchOperand reports whether words[j], after the command word at i, is
// an operand of a search (grep or rg) and no redirection's target.
func searchOperand(words []string, i, j int) bool {
	if i == len(words) || j <= i {
		return false
	}
	if c := filepath.Base(words[i]); c != "grep" && c != "rg" {
		return false
	}
	return !redirection(words, j)
}

// redirOp is a redirection operator at the start of a word, with its fd.
var redirOp = regexp.MustCompile(`^[0-9]*(>>|>\||>&|<>|<&|<<<|>|<)`)

// redirection reports whether words[j] is a redirection (an operator,
// bare or glued to its target) or the target of a bare one before it.
func redirection(words []string, j int) bool {
	if redirOp.MatchString(words[j]) {
		return true
	}
	return j > 0 && redirOp.FindString(words[j-1]) == words[j-1] && words[j-1] != ""
}

// markerIn reports whether w names a marker other than as $NAME or ${NAME}.
func markerIn(w string) bool {
	for name := range markerNames {
		for k := 0; ; {
			n := strings.Index(w[k:], name)
			if n < 0 {
				break
			}
			at, end := k+n, k+n+len(name)
			k = at + 1
			if at > 0 && identChar(w[at-1]) || end < len(w) && identChar(w[end]) {
				continue // part of a longer name
			}
			if at > 0 && w[at-1] == '$' || at > 1 && w[at-2:at] == "${" && end < len(w) && w[end] == '}' {
				continue // a read
			}
			return true
		}
	}
	return false
}

func identChar(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// helperRefused are the words that put a claude off the helper line: the
// reader's mark of a line that backgrounds (bgMark), nohup, setsid, xargs,
// parallel and coproc, which run it outside the sitting's foreground, and
// sudo and doas, which reset the env the markers ride in.
var helperRefused = map[string]bool{bgMark: true, "nohup": true, "setsid": true, "xargs": true, "parallel": true, "coproc": true, "sudo": true, "doas": true}

// helper is the helper guard at the hook's depth d under its turns (AR25
// act 0; ADV15-1, -7): a claude command word (k.is, AR27C act 1) in a row
// is let through only as the one helper line (helperLine), in the
// foreground, and only while d+1 <= MaxHelperDepth. parallel and coproc
// are no wrappers to commandWord, so a claude among their words is
// denied. exec -a, which names argv[0] (all ps's comm reads), is denied.
func helper(d, turns int, k claudeKey) Guard {
	why := fmt.Sprintf("a sitting's claude is one helper in the foreground, so run exactly `CLAUDE_SESSION_DEPTH=%d CLAUDE_SESSION_HELPER=1 claude -p … --max-turns N --max-budget-usd X` (1 <= N <= CLAUDE_SESSION_MAX_TURNS, X > 0; --model, --output-format and --verbose may join) and no other word, never backgrounded, never by another name or path, and no exec -a", d+1)
	if d+1 > MaxHelperDepth {
		why = fmt.Sprintf("a sitting at depth %d runs no claude: helpers stop at depth %d (MaxHelperDepth)", d, MaxHelperDepth)
	}
	return Guard{"helper", nil, why, nil, func(words []string) bool {
		i := commandWord(words)
		if execNames(words, i) {
			return true
		}
		if i == len(words) {
			return false
		}
		if helperRefused[filepath.Base(words[i])] {
			for _, w := range words[i+1:] {
				if k.is(w) {
					return true
				}
			}
			return false
		}
		return k.is(words[i]) && !claudeQuery(words, i) && !helperLine(words, i, d, turns)
	}}
}

// claudeQuery reports that the claude at words[i] is asked of, not
// started (AR27C act 3; ADV16-8): `command -v` or -V names it, or it has
// --version or -v alone (redirections aside); and no refused word
// (helperRefused: xargs appends to it) comes before it.
func claudeQuery(words []string, i int) bool {
	for _, w := range words[:i] {
		if helperRefused[filepath.Base(w)] {
			return false
		}
	}
	for j := 0; j < i; j++ {
		if filepath.Base(words[j]) != "command" {
			continue
		}
		for _, o := range words[j+1 : i] {
			if !strings.HasPrefix(o, "-") || o == "-" || o == "--" {
				break
			}
			if strings.ContainsAny(o[1:], "vV") {
				return true
			}
		}
	}
	var args []string
	for k := i + 1; k < len(words); k++ {
		if !redirection(words, k) {
			args = append(args, words[k])
		}
	}
	return len(args) == 1 && (args[0] == "--version" || args[0] == "-v")
}

// backgrounded is the helper guard for a Bash call the tool itself runs in
// the background (AR27C act 3; ADV16-4: run_in_background, which no word
// of cmd shows): in a row, a segment whose command word is a claude
// (claudeKey.is) that is no query (claudeQuery) is denied by helper.
func backgrounded(cmd string, env func(string) string) (Guard, bool) {
	if !inRow(env) {
		return Guard{}, false
	}
	k := claudeKeyOf(env)
	for _, seg := range segments(cmd, 0) {
		if i := commandWord(seg); i < len(seg) && k.is(seg[i]) && !claudeQuery(seg, i) {
			return helper(rowDepth(env), hookTurns(env), k), true
		}
	}
	return Guard{}, false
}

// BackgroundAgent and RootWrite are the orchestrator's row guards
// (orchestrator r1 §3), held in every row: a helper runs in the
// foreground, and only the relay writes the root checkout.
var (
	BackgroundAgent = Guard{"background-agent", nil,
		"a sitting's helper runs in the foreground: the turn that starts one waits for its result, so never set run_in_background", nil, nil}
	RootWrite = Guard{"root-write", nil,
		"a sitting's git runs in its own worktree, never in the root checkout, which only the relay writes; cd to your worktree by a literal path, in a command of its own if the shell's directory is the root", nil, nil}
)

// relayTree is the root checkout and the worktree when proj is a relay
// worktree, `<root>/.claude/worktrees/<name>`.
func relayTree(proj string) (root, tree string, ok bool) {
	if !filepath.IsAbs(proj) {
		return "", "", false
	}
	tree = filepath.Clean(proj)
	wts := filepath.Dir(tree)
	if filepath.Base(wts) != "worktrees" || filepath.Base(filepath.Dir(wts)) != ".claude" || filepath.Base(tree) == "worktrees" {
		return "", "", false
	}
	return filepath.Dir(filepath.Dir(wts)), tree, true
}

// cdTarget is the directory a cd or pushd with args moves to from dir, ""
// when the guard cannot read it: `-`, a stack entry, a word that expands
// ($, backtick, a ~user), two operands, an unknown dir behind a relative
// one, or a relative one a CDPATH may resolve (not `.`, `..`, `./…` or
// `../…`; ar32a review round 1). No operand is home.
func cdTarget(args []string, dir, home, cdpath string) string {
	var ops []string
	opts := true
	for i, a := range args {
		switch {
		case redirection(args, i):
		case opts && a == "--":
			opts = false
		case opts && len(a) > 1 && a[0] == '-':
			if strings.Trim(a[1:], "LPeq") != "" { // a stack entry (-2) or an option the guard does not know
				return ""
			}
		default:
			opts = false
			ops = append(ops, a)
		}
	}
	switch {
	case len(ops) == 0:
		return home
	case len(ops) > 1, ops[0] == "-", strings.ContainsAny(ops[0], "$`"), strings.HasPrefix(ops[0], "+"):
		return ""
	case ops[0] == "~" || strings.HasPrefix(ops[0], "~/"):
		if !filepath.IsAbs(home) {
			return ""
		}
		return filepath.Join(home, strings.TrimPrefix(ops[0], "~"))
	case strings.HasPrefix(ops[0], "~"):
		return ""
	case filepath.IsAbs(ops[0]):
		return filepath.Clean(ops[0])
	case dir == "":
		return ""
	case cdpath != "" && ops[0] != "." && ops[0] != ".." && !strings.HasPrefix(ops[0], "./") && !strings.HasPrefix(ops[0], "../"):
		return ""
	}
	return filepath.Join(dir, ops[0])
}

// cdVars matches a word naming HOME or CDPATH (zsh's tied cdpath too) as a
// shell identifier: an assignment, an export, a declare or a read of one,
// after which the guard cannot read a cd (ar32a review round 1).
var cdVars = regexp.MustCompile(`(^|[^A-Za-z0-9_])(HOME|CDPATH|cdpath)([^A-Za-z0-9_]|$)`)

// rootWrite is the root-write guard (§3): in a row whose project dir is a
// relay worktree, a git segment is denied when any directory it may run
// in resolves inside the root checkout and outside the worktree, or is
// one the guard cannot read. Those are the hook's cwd and every cd, pushd
// or popd before it in the command, since a subshell's cd need not hold.
// A cd target is read from the hook's env and the disk now, so it is one
// the guard cannot read after a word naming HOME or CDPATH or an `ln` or
// `mv` earlier in the command, or when it is not there (a link the
// command makes; ar32a review round 1).
func rootWrite(cmd, cwd string, env func(string) string) (Guard, bool) {
	if !inRow(env) {
		return Guard{}, false
	}
	root, tree, ok := relayTree(projectDir(env, cwd))
	if !ok {
		return Guard{}, false
	}
	root, tree = resolved(root), resolved(tree)
	start := cwd
	if start == "" {
		start = tree
	}
	dirs := []string{start}
	moved := false // the command may have changed what a cd means
	for _, seg := range segments(cmd, 0) {
		for _, w := range seg {
			moved = moved || cdVars.MatchString(w)
		}
		i := commandWord(seg)
		if i == len(seg) {
			continue
		}
		switch w := filepath.Base(seg[i]); {
		case w == "cd" || w == "pushd":
			d := cdTarget(seg[i+1:], dirs[len(dirs)-1], env("HOME"), env("CDPATH"))
			if _, err := os.Stat(d); moved || err != nil {
				d = ""
			}
			dirs = append(dirs, d)
		case w == "popd":
			dirs = append(dirs, "")
		case w == "ln" || w == "mv":
			moved = true
		case w == "git" || strings.HasPrefix(w, "git-"):
			for _, d := range dirs {
				if d == "" || !filepath.IsAbs(d) {
					return RootWrite, true
				}
				if d = resolved(d); within(root, d) && !within(tree, d) {
					return RootWrite, true
				}
			}
		}
	}
	return Guard{}, false
}

// truthy reads a tool input's flag, failing closed (AR29 act 2,
// ADV17-3): false for nil, false, a number 0, and a string empty, "false"
// or "0" (any case, trimmed); anything else, "yes", 1, an object or a
// list included, is true, and never fails the input's parse.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case json.Number:
		f, err := x.Float64()
		return err != nil || f != 0
	case string:
		x = strings.TrimSpace(x)
		return x != "" && x != "0" && !strings.EqualFold(x, "false")
	}
	return true
}

// execNames reports an exec before the command word words[i] naming its
// argv[0] by an -a in its options (AR27C act 1; ADV16-3).
func execNames(words []string, i int) bool {
	for j := 0; j < i; j++ {
		if filepath.Base(words[j]) != "exec" {
			continue
		}
		for _, o := range words[j+1 : i] {
			if !strings.HasPrefix(o, "-") || o == "-" || o == "--" {
				break
			}
			if strings.ContainsRune(o[1:], 'a') {
				return true
			}
		}
	}
	return false
}

// claudeKey keys a claude by its real file (AR27C act 1; ADV16-3): real is
// the real file of claude on the hook's PATH ("" when none resolves, and a
// claude is then known by its base name alone); path, pwd and home are the
// hook's, which a command word is resolved on.
type claudeKey struct{ real, path, pwd, home string }

// claudeKeyOf is the hook's claudeKey.
func claudeKeyOf(env func(string) string) claudeKey {
	k := claudeKey{path: env("PATH"), pwd: env("PWD"), home: env("HOME")}
	k.real = k.resolve("claude")
	return k
}

// is reports whether the command word w is a claude: its base is claude,
// or its real file (resolve) is the hook's claude's.
func (k claudeKey) is(w string) bool {
	return filepath.Base(w) == "claude" || k.real != "" && k.resolve(w) == k.real
}

// resolve is the real file of the command word w, "" when it names no
// executable file: a bare word looked up on k.path, a path word (`~/`
// read under k.home) joined with k.pwd when relative and globbed, each
// then by filepath.EvalSymlinks. A word with $ or a backtick is not read.
func (k claudeKey) resolve(w string) string {
	if w == "" || strings.ContainsAny(w, "$`") {
		return ""
	}
	var cands []string
	switch {
	case !strings.Contains(w, "/"):
		for _, dir := range filepath.SplitList(k.path) {
			if dir == "" {
				dir = "."
			}
			if !filepath.IsAbs(dir) {
				if !filepath.IsAbs(k.pwd) {
					continue
				}
				dir = filepath.Join(k.pwd, dir)
			}
			cands = append(cands, filepath.Join(dir, w))
		}
	default:
		if strings.HasPrefix(w, "~/") && filepath.IsAbs(k.home) {
			w = filepath.Join(k.home, w[2:])
		}
		if !filepath.IsAbs(w) {
			if !filepath.IsAbs(k.pwd) {
				return ""
			}
			w = filepath.Join(k.pwd, w)
		}
		cands, _ = filepath.Glob(w)
	}
	for _, c := range cands {
		r, err := filepath.EvalSymlinks(c)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(r); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
			return r
		}
	}
	return ""
}

// helperFormats are the --output-format values a helper line may name.
var helperFormats = map[string]bool{"text": true, "json": true, "stream-json": true}

// helperLine reports whether the claude at words[i] is the helper line at
// the hook's depth d under its turns (AR27B act 2; ADV16-2: an allowlist,
// since --settings, --setting-sources, CLAUDE_CONFIG_DIR and kin start a
// helper whose hooks never load). Before claude exactly the two prefix
// words, each once, nothing else (no wrapper, exec, other assignment or
// background mark); the command word literally claude; after it only -p
// or --print (required), --max-turns N (1 <= N <= turns; turns 0, the
// hook's unreadable, denies), --max-budget-usd X (required, above 0,
// finite), --model M, --output-format text|json|stream-json and
// --verbose, each valued one in its = form too (a value never starts with
// -), at most one prompt word not starting with -, and redirections.
func helperLine(words []string, i, d, turns int) bool {
	if d+1 > MaxHelperDepth || words[i] != "claude" || i != 2 {
		return false
	}
	depth, mark := "CLAUDE_SESSION_DEPTH="+strconv.Itoa(d+1), "CLAUDE_SESSION_HELPER=1"
	if !(words[0] == depth && words[1] == mark || words[0] == mark && words[1] == depth) {
		return false
	}
	print, maxTurns, budget, prompt := false, false, false, false
	args := words[i+1:]
	for k := 0; k < len(args); k++ {
		a := args[k]
		if redirection(args, k) {
			continue
		}
		name, v, eq := strings.Cut(a, "=")
		switch name {
		case "-p", "--print", "--verbose":
			if eq {
				return false
			}
			print = print || name != "--verbose"
			continue
		case "--max-turns", "--max-budget-usd", "--model", "--output-format":
			if !eq {
				if k++; k == len(args) {
					return false
				}
				v = args[k]
			}
		default:
			if strings.HasPrefix(a, "-") || prompt {
				return false
			}
			prompt = true
			continue
		}
		if v == "" || strings.HasPrefix(v, "-") {
			return false
		}
		switch name {
		case "--max-turns":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > turns {
				return false
			}
			maxTurns = true
		case "--max-budget-usd":
			x, err := strconv.ParseFloat(v, 64)
			if err != nil || !(x > 0) || math.IsInf(x, 0) {
				return false
			}
			budget = true
		case "--output-format":
			if !helperFormats[v] {
				return false
			}
		}
	}
	return print && maxTurns && budget
}

// DetachedServer and NestedSession are the row's process doors (AR25 act
// 0; ADV15-2, -5): a multiplexer's window is its server's child, never the
// sitting's, so the watchdog and the sweep never see it and it outlives
// the row; and a row starts no sitting of its own. NestedSession matches
// the session tool under all three spellings, the old one still denied
// (agentik-r1 §6.3 c12), and lets the charter verb through under each:
// the orchestrator's rules tell a row to open its charter, which starts
// no sitting.
var (
	DetachedServer = Guard{"detached-server", regexp.MustCompile(`^(tmux|screen|byobu|dtach|abduco|zellij)(\s|$)`),
		"a detached server's child is never the sitting's, so the sweep never sees it; run the command itself", nil, nil}
	NestedSession = Guard{"nested-session", regexp.MustCompile(`^(claude-session|agentik-session|agentik session)(\s|$)`),
		"a row starts no sitting, so do the work in this one (or run one helper)",
		regexp.MustCompile(`^(claude-session|agentik-session|agentik session) charter(\s|$)`), nil}
)

func devicePattern(extra []string) *regexp.Regexp {
	alt := deviceVerbs
	for _, w := range extra {
		alt += "|" + regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`^(` + alt + `)(\s|$)`)
}

// guards is Guards with the device pattern widened by RELAY_DEVICE_VERBS,
// and Push, NestedRun, RowShapes and the marker guards (rowMarkers, at the
// hook's rowDepth) added in a row (inRow).
func guards(env func(string) string) []Guard {
	var extra []string
	for _, w := range strings.Split(env("RELAY_DEVICE_VERBS"), ",") {
		if w = strings.TrimSpace(w); w != "" {
			extra = append(extra, w)
		}
	}
	row := inRow(env)
	if len(extra) == 0 && !row {
		return Guards
	}
	var gs []Guard
	if row {
		gs = append(gs, Push, Unreadable)
	}
	gs = append(gs, Guards...)
	for i := range gs {
		if gs[i].Name == "device" && len(extra) > 0 {
			gs[i].Pattern = devicePattern(extra)
		}
	}
	if row {
		gs = append(append(append(gs, NestedRun), RowShapes...), rowMarkers(rowDepth(env), hookTurns(env), claudeKeyOf(env))...)
		gs = append(gs, claudeConfig(projectDir(env, env("PWD")), env("PWD")))
	}
	return gs
}

// inRow is the row test every guard makes (AR27B act 1; ADV16-1): RELAY_ROW
// or CLAUDE_SESSION_HELPER non-empty, so a helper whose RELAY_ROW was
// blanked is still a row: the helper fails closed. inSitting, the Stop
// and start.json test, is not this: a helper is no sitting.
func inRow(env func(string) string) bool {
	return env("RELAY_ROW") != "" || env("CLAUDE_SESSION_HELPER") != ""
}

// rowDepth is the hook's own CLAUDE_SESSION_DEPTH in a row as strconv.Atoi
// reads it; unreadable or below 1 it reads MaxHelperDepth (AR27B act 1;
// ADV16-1), so a row that lost its depth runs no helper.
func rowDepth(env func(string) string) int {
	d, err := strconv.Atoi(env("CLAUDE_SESSION_DEPTH"))
	if err != nil || d < 1 {
		return MaxHelperDepth
	}
	return d
}

// hookTurns is the hook's own CLAUDE_SESSION_MAX_TURNS as strconv.Atoi
// reads it (AR27B act 2): a helper's --max-turns is at most it; 0 when
// unreadable or below 1, so no helper line passes.
func hookTurns(env func(string) string) int {
	n, err := strconv.Atoi(env("CLAUDE_SESSION_MAX_TURNS"))
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// rowMarkers are a row's guards at depth d under the hook's turns, keying
// a claude by k (AR25 act 0; AR27C act 1): the markers ride, a claude is
// the helper line, and no detached server or new sitting.
func rowMarkers(d, turns int, k claudeKey) []Guard {
	return []Guard{markers(d), helper(d, turns, k), DetachedServer, NestedSession}
}

// Verdict is the one entry of the PreToolUse guards (AR17B): whether cmd is
// denied, in a sitting when inRow, and why. A row is read at depth 1, the
// depth ChildEnv gives it. The owner's device verbs are not read here
// (preToolUse adds them from the environment).
func Verdict(cmd string, inRow bool) (deny bool, why string) {
	return VerdictEnv(cmd, func(k string) string {
		switch {
		case inRow && k == "RELAY_ROW":
			return "row"
		case inRow && k == "CLAUDE_SESSION_DEPTH":
			return "1"
		}
		return ""
	})
}

// VerdictEnv is Verdict under a hook's env as preToolUse reads it (AR25 act
// 0): RELAY_ROW, CLAUDE_SESSION_DEPTH and RELAY_DEVICE_VERBS.
func VerdictEnv(cmd string, env func(string) string) (deny bool, why string) {
	g, ok := match(cmd, guards(env))
	if !ok {
		return false, ""
	}
	return true, "relay: " + g.Name + ": " + g.Why
}

// CommandWordLiteral reports whether every segment of cmd names its
// command, and git its verb, by a literal word: no expansion ($, a
// backtick, a glob or a brace), no redirection glued on.
func CommandWordLiteral(cmd string) bool {
	for _, seg := range Segments(cmd) {
		if !wordsLiteral(seg) {
			return false
		}
	}
	return true
}

func literal(w string) bool {
	return w == "[" || w == "[[" || !strings.ContainsAny(w, "$`*?[{<>")
}

// wordsLiteral is CommandWordLiteral for one segment. git run by xargs with
// no verb takes its verb from stdin, so it is not literal either.
func wordsLiteral(words []string) bool {
	i := commandWord(words)
	if i == len(words) {
		return true
	}
	if !literal(words[i]) {
		return false
	}
	if filepath.Base(words[i]) != "git" {
		return true
	}
	f := strings.Fields(Normalise(words))
	if len(f) < 2 {
		for _, w := range words[:i] {
			if filepath.Base(w) == "xargs" {
				return false
			}
		}
		return true
	}
	return f[1] == "-c" || literal(f[1])
}

// stdinShell reports a shell segment that reads its commands from stdin: a
// redirection into it, -s, `-c` with no command, or no script operand.
func stdinShell(words []string) bool {
	i := commandWord(words)
	if i == len(words) || !shells[filepath.Base(words[i])] {
		return false
	}
	args := words[i+1:]
	for _, a := range args {
		if a == "<" || a == "<<" || a == "<<<" {
			return true
		}
	}
	for j := 0; j < len(args); j++ {
		a := args[j]
		switch {
		case a == "--":
			return j+1 == len(args)
		case a == "-o" || a == "-O" || a == "+o" || a == "+O" || a == "--rcfile" || a == "--init-file":
			j++
		case strings.HasPrefix(a, "--"):
		case strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+"):
			if strings.Contains(a, "s") {
				return true
			}
			if strings.Contains(a, "c") {
				return j+1 == len(args)
			}
			if strings.HasSuffix(a, "o") || strings.HasSuffix(a, "O") {
				j++
			}
		default:
			return false
		}
	}
	return true
}

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true}

// keywords open a compound command; the command word follows them.
var keywords = map[string]bool{"!": true, "{": true, "}": true, "if": true, "then": true, "else": true,
	"elif": true, "fi": true, "do": true, "done": true, "while": true, "until": true}

// match returns the first guard any segment of command matches.
func match(command string, gs []Guard) (Guard, bool) {
	for _, seg := range segments(command, 0) {
		line := Normalise(seg)
		for _, g := range gs {
			if g.Shape != nil {
				if g.Shape(seg) {
					return g, true
				}
				continue
			}
			if g.Pattern.MatchString(line) && (g.Unless == nil || !g.Unless.MatchString(line)) {
				return g, true
			}
		}
	}
	return Guard{}, false
}

type denyOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func preToolUse(b []byte, env func(string) string) any {
	var in struct {
		ToolName  string `json:"tool_name"`
		Cwd       string `json:"cwd"`
		AgentID   string `json:"agent_id"`   // set: the call is a subagent's (AR40)
		AgentType string `json:"agent_type"` // set: likewise
		ToolInput struct {
			Command         string `json:"command"`
			FilePath        string `json:"file_path"`
			NotebookPath    string `json:"notebook_path"`
			RunInBackground any    `json:"run_in_background"`
		} `json:"tool_input"`
	}
	if json.Unmarshal(b, &in) != nil {
		return nil
	}
	if hook := env; in.Cwd != "" {
		env = func(k string) string {
			if k == "PWD" {
				return in.Cwd // the claude-config guard joins a relative word with it
			}
			return hook(k)
		}
	}
	var g Guard
	var ok bool
	switch in.ToolName {
	case "Bash":
		g, ok = bashDoor(in.ToolInput.Command, in.Cwd, truthy(in.ToolInput.RunInBackground), in.AgentID != "" || in.AgentType != "", env)
	case "Agent":
		g, ok = BackgroundAgent, inRow(env) && truthy(in.ToolInput.RunInBackground)
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		path := in.ToolInput.FilePath
		if in.ToolName == "NotebookEdit" {
			path = in.ToolInput.NotebookPath
		}
		g, ok = fileDoor(path, in.Cwd, env)
	}
	if !ok {
		return nil
	}
	return struct {
		HookSpecificOutput denyOutput `json:"hookSpecificOutput"`
	}{denyOutput{"PreToolUse", "deny", "relay: " + g.Name + ": " + g.Why}}
}

// Unreadable answers a Bash command the guards panicked on (AR44 act 2): a
// command they cannot read is denied, never let through unread.
// It is also a guard of the table in a row: it denies the reader's mark
// (unreadMark) for a line the reader could not read whole.
var Unreadable = Guard{Name: "unreadable", Why: "the guards could not read this command, so write it more simply: one plain command, no nested quotes or substitutions",
	Shape: func(words []string) bool { return holds(words, unreadMark) }}

// unreadableBound is Unreadable at a subagent's door (AR46's fix round):
// outside a row it denies only the line the reader gave up on (boundMark:
// the depth bound, or a panic the reader recovered). Any other unread line
// is judged there by its trees, its loose reading and the helper's guard;
// in a row Unreadable has answered first.
var unreadableBound = Guard{Name: Unreadable.Name, Why: Unreadable.Why,
	Shape: func(words []string) bool { return holds(words, boundMark) }}

// holds is whether one of words is mark.
func holds(words []string, mark string) bool {
	for _, w := range words {
		if w == mark {
			return true
		}
	}
	return false
}

// bashDoor puts the guards to a Bash command: the table (guards, and
// HelperRelay and unreadableBound for a subagent's call), then backgrounded
// and rootWrite. A panic on the way is answered Unreadable when a guard is
// in play: in a row (inRow), whose guards the command would have met, or on
// a subagent's call, which meets the helper guard. With neither the panic
// goes on to answer's recover and its empty answer, so a hook never breaks
// the owner's own session.
func bashDoor(cmd, cwd string, background, subagent bool, env func(string) string) (g Guard, ok bool) {
	if inRow(env) || subagent {
		defer func() {
			if recover() != nil {
				g, ok = Unreadable, true
			}
		}()
	}
	gs := guards(env)
	if subagent {
		gs = append(gs[:len(gs):len(gs)], HelperRelay, unreadableBound) // a copy: guards may return Guards itself
	}
	g, ok = match(cmd, gs)
	if !ok && background {
		g, ok = backgrounded(cmd, env)
	}
	if !ok {
		g, ok = rootWrite(cmd, cwd, env)
	}
	return g, ok
}

// The file tools' guards (ADV11-11): Write, Edit, MultiEdit and
// NotebookEdit reach .git and the ledger by no command the Bash guards read.
var (
	GitDir    = Guard{Name: "git-dir", Why: "a sitting writes nothing under .git, so let git write it"}
	StateRoot = Guard{Name: "state-root", Why: "the relay's state root is the relay's, never a sitting's"}
	PidLock   = Guard{Name: "pid-dir", Why: "the account's relay.pid is the one relay's lock, never a sitting's"}
	// ClaudeConfig is the .claude door (AR27B act 3; ADV16-2, ADV14-7): the
	// file door's and the Bash guard's (claudeConfig) name and reason.
	ClaudeConfig = Guard{Name: "claude-config", Why: "a sitting reads and writes nothing under a .claude directory but inside its own project dir: the settings there turn the hooks off"}
)

// PidDir is the account's pid dir (night.PidDir, which sets it at init:
// hooks imports nothing of the relay); nil denies no pid dir (ADV12-8).
var PidDir func() (string, error)

// fileDoor is the file tools' guard in a row (inRow): path,
// `~/` read under HOME, resolved against cwd when relative and Cleaned, is
// denied when an element is .git (a worktree's .git file, a root's .git
// dir; any case, as the Mac's file system folds it), or it lies in the
// state root (stateRoot, as the relay resolves it) or the account's pid
// dir (PidDir), or it names a .claude element (claudeIn, AR27B act 3).
// Roots and path are compared as spelled and again with
// their existing prefixes' symlinks resolved (a /tmp root written through
// /private/tmp); a symlink made after the call still walks past it: the
// guards are advisory (§10).
func fileDoor(path, cwd string, env func(string) string) (Guard, bool) {
	if !inRow(env) || path == "" {
		return Guard{}, false
	}
	if home := env("HOME"); filepath.IsAbs(home) && (path == "~" || strings.HasPrefix(path, "~/")) {
		path = filepath.Join(home, path[1:])
	}
	if !filepath.IsAbs(path) && cwd != "" {
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	for _, e := range strings.Split(path, string(filepath.Separator)) {
		if strings.EqualFold(e, ".git") {
			return GitDir, true
		}
	}
	if claudeIn(path, projectDir(env, cwd)) {
		return ClaudeConfig, true
	}
	real := resolved(path)
	under := func(root string) bool {
		if !filepath.IsAbs(root) {
			return false
		}
		return within(root, path) || within(resolved(root), real)
	}
	if under(stateRoot(env)) {
		return StateRoot, true
	}
	if PidDir != nil {
		if dir, err := PidDir(); err == nil && under(dir) {
			return PidLock, true
		}
	}
	return Guard{}, false
}

// within is whether path is root or lies under it, by their words.
func within(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// projectDir is the project dir the .claude door judges below (AR27B act
// 3): CLAUDE_PROJECT_DIR when absolute, else cwd (the input's).
func projectDir(env func(string) string, cwd string) string {
	if p := env("CLAUDE_PROJECT_DIR"); filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return cwd
}

// claudeIn reports whether the absolute path names a .claude element
// (claudeElem): by its part below proj when it lies there, else whole; as
// spelled and again with symlinks resolved (resolved). So a worktree under
// <root>/.claude/worktrees/ passes, and a link inside it to a .claude
// directory does not.
func claudeIn(path, proj string) bool {
	rpath, rproj := resolved(path), resolved(proj)
	below := func(root, p string) (string, bool) {
		if !filepath.IsAbs(root) || !within(root, p) {
			return p, false
		}
		rel, _ := filepath.Rel(filepath.Clean(root), p)
		return rel, true
	}
	spelled, ok := below(proj, path)
	if !ok {
		spelled, _ = below(rproj, rpath)
	}
	real, _ := below(rproj, rpath)
	return claudeElems(spelled) || claudeElems(real)
}

// claudeElems reports whether a path, read by its own words, has a .claude
// element (claudeElem).
func claudeElems(p string) bool {
	for _, e := range strings.Split(p, "/") {
		if claudeElem(e) {
			return true
		}
	}
	return false
}

// claudeElem reports an element that is, or may expand to, .claude in any
// case (the Mac's file system folds it): the name itself, or a glob that
// matches it and does not start with * or ? (which match no leading dot).
func claudeElem(e string) bool {
	e = strings.ToLower(e)
	if e == ".claude" {
		return true
	}
	if !strings.ContainsAny(e, "*?[") || strings.HasPrefix(e, "*") || strings.HasPrefix(e, "?") {
		return false
	}
	ok, _ := filepath.Match(e, ".claude")
	return ok
}

// claudeConfig is the claude-config Bash guard in a row (AR27B act 3;
// ADV16-2, ADV14-7): a word that is a path naming a .claude element is
// denied, a read or a write. A word (a redirection's operator dropped,
// braces expanded, and each value after an =) is a path when it holds /
// or is itself a .claude element. One with $ or a tilde is judged by its
// own elements; a relative one is joined with cwd, and it and an absolute
// one are judged as the file door judges them (claudeIn under proj). A
// search's operands (searchOperand) are exempt; a redirection's target
// never is.
func claudeConfig(proj, cwd string) Guard {
	g := ClaudeConfig
	g.Shape = func(words []string) bool {
		i := commandWord(words)
		for j, w := range words {
			if j > i && searchOperand(words, i, j) {
				continue
			}
			w = w[len(redirOp.FindString(w)):]
			for _, b := range braces(w, 64) {
				cands := []string{b}
				for k := range b {
					if b[k] == '=' {
						cands = append(cands, b[k+1:])
					}
				}
				for _, c := range cands {
					if claudeWord(c, proj, cwd) {
						return true
					}
				}
			}
		}
		return false
	}
	return g
}

// claudeWord reports whether the word c, a path, names a .claude element.
func claudeWord(c, proj, cwd string) bool {
	if !strings.Contains(c, "/") && !claudeElem(c) {
		return false
	}
	if strings.ContainsAny(c, "$~") || !filepath.IsAbs(c) && !filepath.IsAbs(cwd) {
		return claudeElems(c)
	}
	if !filepath.IsAbs(c) {
		c = filepath.Join(cwd, c)
	}
	return claudeIn(filepath.Clean(c), proj)
}

// braces is w with its brace expansions made, as the shells make them
// (`a{b,c}d` is abd and acd); at most max words, w itself when there are
// more or none.
func braces(w string, max int) []string {
	open := strings.IndexByte(w, '{')
	if open < 0 {
		return []string{w}
	}
	depth, parts, start := 0, []string(nil), open+1
	for k := open; k < len(w); k++ {
		switch w[k] {
		case '{':
			depth++
		case ',':
			if depth == 1 {
				parts, start = append(parts, w[start:k]), k+1
			}
		case '}':
			if depth--; depth > 0 {
				continue
			}
			if parts == nil {
				// no comma: a literal brace; expand what follows it
				var out []string
				for _, rest := range braces(w[k+1:], max) {
					out = append(out, w[:k+1]+rest)
				}
				return out
			}
			parts = append(parts, w[start:k])
			var out []string
			for _, p := range parts {
				for _, x := range braces(w[:open]+p+w[k+1:], max) {
					if out = append(out, x); len(out) > max {
						return []string{w}
					}
				}
			}
			return out
		}
	}
	return []string{w}
}

// resolved is the absolute path p with the symlinks of its longest
// existing prefix resolved and the rest appended; p when none resolves.
func resolved(p string) string {
	rest := ""
	for q := filepath.Clean(p); ; {
		if r, err := filepath.EvalSymlinks(q); err == nil {
			return filepath.Join(r, rest)
		}
		up := filepath.Dir(q)
		if up == q {
			return p
		}
		rest, q = filepath.Join(filepath.Base(q), rest), up
	}
}

// wrappers precede the command word without being it (ADV6-6): each names
// its one-letter options that take the next word as a value, and how many
// operands come before the command (timeout's duration).
var wrappers = map[string]struct {
	valued   string
	operands int
}{
	"sudo": {"ugpCDhRrtTU", 0}, "doas": {"uC", 0}, "env": {"uCSP", 0}, "command": {"", 0},
	"exec": {"a", 0}, "nohup": {"", 0}, "time": {"", 0}, "nice": {"n", 0}, "builtin": {"", 0},
	"timeout": {"sk", 1}, "gtimeout": {"sk", 1}, "stdbuf": {"ioe", 0}, "xargs": {"IELnPsdJRa", 0},
	"ionice": {"cnp", 0}, "setsid": {"", 0}, "caffeinate": {"tw", 0}, "unbuffer": {"", 0},
}

// commandWord is the index of words' command word: past assignments and
// wrappers with their options and operands; len(words) when there is none.
func commandWord(words []string) int {
	i := 0
	for i < len(words) {
		if isAssignment(words[i]) || keywords[words[i]] || words[i] == "<<" || words[i] == bgMark {
			i++
			continue
		}
		if words[i] == "<" || words[i] == "<<<" {
			i += 2
			continue
		}
		w, ok := wrappers[filepath.Base(words[i])]
		if !ok {
			return i
		}
		for i++; i < len(words) && strings.HasPrefix(words[i], "-") && words[i] != "-"; i++ {
			if words[i] == "--" {
				i++
				break
			}
			if len(words[i]) == 2 && strings.IndexByte(w.valued, words[i][1]) >= 0 {
				i++
			}
		}
		for n := 0; n < w.operands && i < len(words) && !isAssignment(words[i]); n++ {
			i++
		}
	}
	// a bare `<` or `<<<`, or a wrapper's valued option, that ends the
	// segment has no word to step over (AR43's fix round): the index stays
	// inside words, so no caller slices past them
	return min(i, len(words))
}

// gitValued are git's global options that take the next word as a value.
var gitValued = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true,
	"--namespace": true, "--exec-path": true, "--config-env": true,
}

// Normalise writes one segment as the guards read it (Guard); "" when it
// has no command word. Of git's global options, a config one (-c,
// --config-env) naming alias.* or include* is kept as `-c <key=value>`: it
// can define the verb the guards look for.
func Normalise(words []string) string {
	i := commandWord(words)
	if i == len(words) {
		return ""
	}
	out := []string{filepath.Base(words[i])}
	rest := words[i+1:]
	if out[0] == "git" {
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			opt, val := rest[0], ""
			if gitValued[opt] && len(rest) > 1 {
				val, rest = rest[1], rest[1:]
			}
			config := opt == "-c" || opt == "--config-env"
			if v, ok := strings.CutPrefix(opt, "--config-env="); ok {
				config, val = true, v
			} else if v, ok := strings.CutPrefix(opt, "-c"); ok && v != "" && v[0] != '-' {
				config, val = true, v
			}
			if config && definesCommand(val) {
				out = append(out, "-c", val)
			}
			rest = rest[1:]
		}
	}
	for _, w := range rest {
		out = append(out, strings.Join(strings.Fields(w), "_"))
	}
	return strings.Join(out, " ")
}

// definesCommand reports a git config key=value that can make a new verb,
// re-point the hooks (core.hooksPath) or make a pull rebase (AR17B).
func definesCommand(kv string) bool {
	k := strings.ToLower(kv)
	return strings.HasPrefix(k, "alias.") || strings.HasPrefix(k, "include.") || strings.HasPrefix(k, "includeif.") ||
		strings.HasPrefix(k, "core.hookspath") || strings.HasPrefix(k, "pull.rebase") ||
		strings.HasPrefix(k, "branch.") && strings.Contains(k, ".rebase")
}

func isAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for _, c := range w[:eq] {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

const maxDepth = 4

// Segments splits a shell command into its simple commands, each a list of
// words with quotes removed. It is a guard's reading, not a shell: operators
// (; & | && || newline, parentheses, backticks, $( ) end a segment, comments
// and heredoc bodies are dropped, and `$( )` inside double quotes, the
// argument of `sh -c` or `eval`, and the command of `find -exec` are read as
// commands in their own right. `$'…'` is read with its escapes (AR21 act
// 3), and inside `((…))`, `$((…))`, `$[…]` and `${…}` a `<<` is a shift;
// after `let` it is read both ways (see segments). A line it cannot read
// whole carries a segment of one word, unreadMark.
func Segments(cmd string) [][]string {
	all := segments(cmd, 0)
	segs := all[:0]
	for _, s := range all {
		for len(s) > 0 && s[0] == bgMark {
			s = s[1:]
		}
		if len(s) == 1 && s[0] == boundMark {
			continue // the hook door's word; unreadMark stands before it
		}
		segs = append(segs, s)
	}
	return segs
}

// bgMark is the reader's first word on each segment of a line that
// backgrounds a command (a lone `&`, zsh's `&|` and `&!` too; never `&&`,
// `&>`, `>&` or `|&`), so the helper guard sees it; commandWord skips it,
// and Segments drops it. A shell word never reads as it: the NUL `$'\0'`
// could write ends a word in the shell, so a forged mark hides nothing.
const bgMark = "\x00&"

// unreadMark is the reader's one-word segment for a line it could not read
// whole (AR44's fix round): text past the depth bound (maxDepth, in read,
// nested and span), or a `case` in command position inside a substitution
// (substitution). Nothing is dropped unread: the Unreadable guard denies
// the mark in a row, and a subagent's call outside one is denied for
// boundMark. As bgMark, no shell word reads as it; one forged would only
// deny its own line.
const unreadMark = "\x00unreadable"

// boundMark is the reader's one-word segment after unreadMark on a line it
// gave up on (AR46's fix round): text past the depth bound, or a panic the
// reader recovered. A subagent's call outside a row is denied for it alone
// (unreadableBound); any other unread line is judged there by what was
// read of it. Segments drops it, so its contract holds word for word.
const boundMark = "\x00bound"

// reading is one way to read a line where the shells differ, or where the
// guards keep a reading no shell holds so that nothing is read less.
type reading struct {
	letShift bool // a `<<` after `let` is a shift, not a heredoc
	quotes   int  // how a quote inside `"${…}"` and the end of a `$(…)` are read
}

// The readings of a quote inside a `${…}` inside a double-quoted span (AR43
// act 2). A `"` there opens a nested span in every shell; a `'` there is a
// literal in zsh and dash and opens a single-quoted span in bash 3.2. The
// end of a `$(…)` follows the same shells (AR44 act 1, see span): inside
// one, zsh and dash read a `${…}` to its `}` and a `#` after an operator as
// a comment, and bash 3.2 reads neither. The flat reading is the reader
// before AR43 and AR44, kept so that no line it denied is lost.
const (
	nestLiteral = iota // zsh and dash: `"` nests, `'` is a literal
	nestQuoted         // bash 3.2: `"` nests, `'` runs to the next `'`
	flat               // before AR43: the next `"` ends the outer span, and closing and matching read as they did
)

// seen is what one reading met that another reading reads otherwise.
type seen struct {
	let    bool // a `<<` after `let`, read as a heredoc
	single bool // a `'` inside `"${…}"`
	double bool // a `"` inside `"${…}"`
	subst  bool // inside a `$(…)`, a `${` or a `#` after an operator, which bash 3.2 reads on through
	skip   bool // inside a `$(…)` or a bracket pair, a span the reader before AR44 did not skip
	unread bool // span stopped at the depth bound: the line is unreadable (unreadMark)
	nest   int  // how many spans are open, one inside the other
}

// segments is the reader under Segments: the tree reader (AR46,
// treereader.go), which reads cmd through a shell parser under each dialect
// the box speaks and marks what it cannot read whole (unreadMark).
func segments(cmd string, depth int) [][]string {
	return treeSegments(cmd, depth)
}

// handSegments is the reader before AR46, kept whole and unreached (read,
// span, closing, matching, doubleQuoted, readHeredoc, nested, ansiC and
// their helpers; only ansiC serves the tree reader): the guard-parser
// record's packet 3 deletes it.
//
// It is read's reading of cmd. A `<<` after `let` opens a heredoc in
// sh, bash and zsh, but the guards also read it as a shift (ADV12-5): when
// one is met, the reading with every such `<<` a shift is added, so a line
// either reading holds is read. A quote inside `"${…}"` is read the same
// way (AR43 act 2): a `'` there adds the reading where it opens a span, and
// a `"` there adds the reading before AR43, where it ends the outer span,
// so no line that reading denied is lost. The end of a `$(…)` is read the
// same way (AR44 act 1): what bash 3.2 reads otherwise adds its reading, and
// a span the reader before AR44 did not skip adds that reader's. Each
// reading may meet what the first did not, so the readings are followed
// until none is new: six at most.
func handSegments(cmd string, depth int) [][]string {
	var segs [][]string
	done := map[reading]bool{}
	todo := []reading{{}}
	for len(todo) > 0 {
		r := todo[0]
		todo = todo[1:]
		if done[r] {
			continue
		}
		done[r] = true
		more, saw := read(cmd, depth, r)
		segs = append(segs, more...)
		if saw.unread {
			segs = append(segs, []string{unreadMark})
		}
		if saw.let && !r.letShift {
			todo = append(todo, reading{letShift: true, quotes: r.quotes})
		}
		if (saw.single || saw.subst) && r.quotes == nestLiteral {
			todo = append(todo, reading{letShift: r.letShift, quotes: nestQuoted})
		}
		if (saw.double || saw.skip) && r.quotes != flat {
			todo = append(todo, reading{letShift: r.letShift, quotes: flat})
		}
	}
	return segs
}

// read splits cmd into its segments as the reading r reads it; saw reports
// what it met that another reading reads otherwise.
func read(cmd string, depth int, r reading) (segs [][]string, saw seen) {
	if depth > maxDepth {
		return [][]string{{unreadMark}}, saw // past the bound: unreadable, never dropped
	}
	var (
		words    []string
		word     strings.Builder
		inWord   bool
		heredocs []heredoc
		arith    = -1 // a `<<` before this index is a shift
		bg       bool // a lone `&` was met: every segment is marked (bgMark)
	)
	endWord := func() {
		if inWord {
			words = append(words, word.String())
		}
		word.Reset()
		inWord = false
	}
	endSeg := func() {
		endWord()
		if len(words) > 0 {
			segs = append(segs, words)
			segs = append(segs, nested(words, depth)...)
		}
		words = nil
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		// an arithmetic region: `((…))` closed by `))`, `$[…]`, `${…}`
		switch {
		case c == '(' && i+1 < len(cmd) && cmd[i+1] == '(':
			if m := matching(cmd, i+1, '(', ')', r, &saw); m+1 < len(cmd) && cmd[m+1] == ')' {
				arith = max(arith, m+1)
			}
		case c == '$' && i+1 < len(cmd) && (cmd[i+1] == '[' || cmd[i+1] == '{'):
			arith = max(arith, matching(cmd, i+1, cmd[i+1], map[byte]byte{'[': ']', '{': '}'}[cmd[i+1]], r, &saw))
		}
		switch {
		case c == '\\':
			if i+1 < len(cmd) {
				i++
				if cmd[i] != '\n' {
					word.WriteByte(cmd[i])
					inWord = true
				}
			}
		case c == '\'':
			j := strings.IndexByte(cmd[i+1:], '\'')
			if j < 0 {
				j = len(cmd) - i - 1
			}
			word.WriteString(cmd[i+1 : i+1+j])
			inWord = true
			i += j + 1
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '\'':
			i = ansiC(cmd, i+2, &word)
			inWord = true
		case c == '"':
			var inner [][]string
			i, inner = doubleQuoted(cmd, i+1, &word, depth, r, &saw)
			segs = append(segs, inner...)
			inWord = true
		case c == ' ' || c == '\t':
			endWord()
		case c == '\n':
			endSeg()
			for _, h := range heredocs {
				i = h.skip(cmd, i)
			}
			heredocs = nil
		case c == '#' && !inWord:
			for i+1 < len(cmd) && cmd[i+1] != '\n' {
				i++
			}
		case c == '<' && strings.HasPrefix(cmd[i:], "<<<"):
			endWord()
			words = append(words, "<<<")
			i += 2
		case c == '<' && strings.HasPrefix(cmd[i:], "<<"):
			let := isLet(words)
			endWord()
			words = append(words, "<<")
			if i < arith || (let && r.letShift) {
				i++ // a shift: no heredoc
				break
			}
			saw.let = saw.let || let
			var h heredoc
			i, h = readHeredoc(cmd, i+2)
			if h.delim != "" {
				heredocs = append(heredocs, h)
			}
		case c == '<' && !inWord && !strings.HasPrefix(cmd[i:], "<("):
			// a redirection marker, so a shell fed from a file reads as one
			words = append(words, "<")
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
			// the substitution's commands are segments of their own; the
			// word it stands in keeps a `$()` so it never reads as literal
			// read with its parentheses, so `$((` opens `((`
			end := closing(cmd, i+2, r, &saw)
			segs = append(segs, substitution(cmd[i+1:min(end+1, len(cmd))], depth)...)
			word.WriteString("$()")
			inWord = true
			i = end
		case c == '`':
			j := strings.IndexByte(cmd[i+1:], '`')
			if j < 0 {
				j = len(cmd) - i - 1
			}
			segs = append(segs, substitution(cmd[i+1:i+1+j], depth)...)
			word.WriteString("$()")
			inWord = true
			i += j + 1
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			bg = bg || c == '&' && background(cmd, i)
			endSeg()
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	endSeg()
	if bg {
		for k := range segs {
			segs[k] = append([]string{bgMark}, segs[k]...)
		}
	}
	return segs, saw
}

// background reports whether the `&` at cmd[i] backgrounds its command:
// not half of `&&`, not `&>` nor an unescaped `>&`, `<&`, `|&` or `;&`.
func background(cmd string, i int) bool {
	if i+1 < len(cmd) && (cmd[i+1] == '&' || cmd[i+1] == '>') {
		return false
	}
	if i == 0 || strings.IndexByte("&><|;", cmd[i-1]) < 0 {
		return true
	}
	n := 0 // backslashes before the operator: odd, it is escaped
	for k := i - 2; k >= 0 && cmd[k] == '\\'; k-- {
		n++
	}
	return n%2 == 1
}

// substitution reads the commands of a `$(…)` or a backtick span met at
// depth. A `case` in command position there makes the line unreadable
// (unreadMark): its pattern's `)` ends the substitution for closing and not
// for zsh and dash, and the reader holds no case grammar.
func substitution(text string, depth int) [][]string {
	segs := handSegments(text, depth+1)
	for _, seg := range segs {
		if i := commandWord(seg); i < len(seg) && seg[i] == "case" {
			return append(segs, []string{unreadMark})
		}
	}
	return segs
}

// isLet reports whether the segment read so far runs let.
func isLet(words []string) bool {
	if len(words) == 0 {
		return false
	}
	i := commandWord(words)
	return i < len(words) && words[i] == "let"
}

// doubleQuoted appends a double-quoted word's text to word, reading any
// `$( )` or backtick inside as commands; it returns the index of the closing
// quote and those commands' segments. A `${` inside the span opens an
// expansion that runs to its matching `}` (AR43 act 2; a bare `{` nests
// nothing, and a backslash takes the `}` or the quote after it). Inside the
// expansion a `"` opens a nested span, itself read as this one is, and the
// outer span goes on after the `}`; a `'` there is a literal or, as r reads
// it, a span to the next `'`. The flat reading opens no expansion. saw
// records each quote met inside an expansion.
func doubleQuoted(cmd string, i int, word *strings.Builder, depth int, r reading, saw *seen) (int, [][]string) {
	var segs [][]string
	// open holds what is open inside the outer span, innermost last: 'e' an
	// expansion, 'q' a double-quoted span inside one
	var open []byte
	for ; i < len(cmd); i++ {
		c := cmd[i]
		expanding := len(open) > 0 && open[len(open)-1] == 'e'
		switch {
		case c == '"' && expanding:
			saw.double = true
			open = append(open, 'q')
		case c == '"' && len(open) > 0:
			open = open[:len(open)-1]
		case c == '"':
			return i, segs
		case c == '}' && expanding:
			open = open[:len(open)-1]
			word.WriteByte(c)
		case c == '\'' && expanding:
			saw.single = true
			if r.quotes != nestQuoted {
				word.WriteByte(c)
				break
			}
			j := strings.IndexByte(cmd[i+1:], '\'')
			if j < 0 {
				j = len(cmd) - i - 1
			}
			word.WriteString(cmd[i+1 : i+1+j])
			i += j + 1
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '{' && r.quotes != flat:
			open = append(open, 'e')
			word.WriteString("${")
			i++
		case c == '\\' && i+1 < len(cmd):
			i++
			word.WriteByte(cmd[i])
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
			end := closing(cmd, i+2, r, saw)
			segs = append(segs, substitution(cmd[i+1:min(end+1, len(cmd))], depth)...)
			word.WriteString("$()")
			i = end
		case c == '`':
			j := strings.IndexByte(cmd[i+1:], '`')
			if j < 0 {
				j = len(cmd) - i - 1
			}
			segs = append(segs, substitution(cmd[i+1:i+1+j], depth)...)
			word.WriteString("$()")
			i += j + 1
		default:
			word.WriteByte(c)
		}
	}
	return i, segs
}

// closing finds the `)` that closes a `$(` opened just before i, as the
// shell r follows finds it (span); len(cmd) when there is none. The flat
// reading finds it as the reader before AR44 did: a `'…'` span and a
// backslash skipped, a `"` read as any byte.
func closing(cmd string, i int, r reading, saw *seen) int {
	if r.quotes != flat {
		return span(cmd, i, '(', ')', true, r, saw)
	}
	depth := 1
	for ; i < len(cmd); i++ {
		switch cmd[i] {
		case '\\':
			i++
		case '$':
			if i+1 < len(cmd) && cmd[i+1] == '\'' {
				i = ansiC(cmd, i+2, nil)
			}
		case '\'':
			j := strings.IndexByte(cmd[i+1:], '\'')
			if j < 0 {
				return len(cmd)
			}
			i += j + 1
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(cmd)
}

// ansiC reads the body of a `$'…'` quote that starts at i, writing its
// text, escapes decoded as bash decodes them, to w when w is not nil; it
// returns the index of the closing quote, or len(cmd) when there is none,
// so the rest reads as quoted, as an unclosed `'` does. `\c` never takes
// the quote, so where a shell might read the quote as longer, the guards
// read it as shorter.
func ansiC(cmd string, i int, w *strings.Builder) int {
	var b strings.Builder
	defer func() {
		if w != nil {
			w.WriteString(b.String())
		}
	}()
	for ; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' {
			return i
		}
		if c != '\\' || i+1 == len(cmd) {
			b.WriteByte(c)
			continue
		}
		i++
		switch e := cmd[i]; e {
		case 'a', 'b', 'e', 'E', 'f', 'n', 'r', 't', 'v':
			b.WriteByte(map[byte]byte{'a': 7, 'b': 8, 'e': 27, 'E': 27, 'f': 12, 'n': 10, 'r': 13, 't': 9, 'v': 11}[e])
		case '\\', '\'', '"', '?':
			b.WriteByte(e)
		case 'c':
			if i+1 < len(cmd) && cmd[i+1] != '\'' {
				i++
				b.WriteByte(cmd[i] & 0x1f)
			} else {
				b.WriteString(`\c`)
			}
		case 'x', 'u', 'U':
			n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[e]
			v, k := number(cmd[i+1:], n, 16)
			switch {
			case k == 0:
				b.WriteByte('\\')
				b.WriteByte(e)
			case e == 'x':
				b.WriteByte(byte(v))
			default:
				b.WriteRune(rune(v))
			}
			i += k
		case '0', '1', '2', '3', '4', '5', '6', '7':
			v, k := number(cmd[i:], 3, 8)
			b.WriteByte(byte(v))
			i += k - 1
		default:
			b.WriteByte('\\')
			b.WriteByte(e)
		}
	}
	return len(cmd)
}

// number reads at most n digits of base at the start of s: their value and
// how many were read.
func number(s string, n, base int) (v, k int) {
	for ; k < n && k < len(s); k++ {
		ch := s[k]
		if ch >= 'A' && ch <= 'F' {
			ch += 'a' - 'A'
		}
		d := strings.IndexByte("0123456789abcdef"[:base], ch)
		if d < 0 {
			break
		}
		v = v*base + d
	}
	return v, k
}

// span finds the close byte that ends what was opened just before i, as the
// shell finds it (AR44 act 1); len(cmd) when there is none. The quoting
// starts fresh: a backslash takes the next byte, a `'…'`, a `$'…'` and a
// backtick span are skipped, a `"` opens a span read as doubleQuoted reads
// it (so a `"` inside a `$(…)` nests in an outer double-quoted span), and a
// nested `$(…)` or `${…}` is read by this rule to its own end. A bare open
// byte nests (0: none does, as a `{` inside `${…}`). In a substitution
// (command) a `#` at the start of a word is a comment to the end of its
// line. Where the shells differ r decides, and saw records it: a `${…}`
// inside a `$(…)` hides a `)` in zsh and dash and not in bash 3.2, and a
// `#` just after an operator is a comment in zsh and dash and a word in
// bash 3.2. saw also records each span the reader before AR44 read
// otherwise, so segments adds that reading. Spans nest no deeper than read
// does (maxDepth): past it span stops and saw marks the line unreadable.
func span(cmd string, i int, open, close byte, command bool, r reading, saw *seen) int {
	const (
		operator = iota // after `;`, `&`, `|`, `(`, `)` or the opening itself
		blank           // after a blank or a newline
		inWord
	)
	if saw.nest > maxDepth {
		saw.unread = true // past the bound: no deeper, and the line is unreadable
		return len(cmd)
	}
	saw.nest++
	defer func() { saw.nest-- }()
	depth, at := 1, operator
	for ; i < len(cmd); i++ {
		c, was := cmd[i], at
		at = inWord
		switch {
		case c == '\\':
			i++
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '\'':
			i = ansiC(cmd, i+2, nil)
		case c == '\'' || c == '`':
			j := strings.IndexByte(cmd[i+1:], c)
			if j < 0 {
				return len(cmd)
			}
			saw.skip = saw.skip || c == '`'
			i += j + 1
		case c == '"':
			saw.skip = true
			var w strings.Builder
			// at maxDepth the span's own substitutions are skipped, not read
			i, _ = doubleQuoted(cmd, i+1, &w, maxDepth, r, saw)
		case c == '#' && command && was != inWord:
			if was == operator {
				if saw.subst = true; r.quotes == nestQuoted {
					break
				}
			}
			saw.skip = true
			for i+1 < len(cmd) && cmd[i+1] != '\n' {
				i++
			}
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
			saw.skip = saw.skip || open != '('
			i = span(cmd, i+2, '(', ')', true, r, saw)
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '{' && open != '{':
			if command {
				if saw.subst = true; r.quotes == nestQuoted {
					break
				}
			}
			saw.skip = true
			i = span(cmd, i+2, 0, '}', false, r, saw)
		case c == open && open != 0:
			depth++
			at = operator
		case c == close:
			if depth--; depth == 0 {
				return i
			}
			at = operator
		case c == ' ' || c == '\t' || c == '\n':
			at = blank
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			at = operator
		}
	}
	return len(cmd)
}

// matching finds the byte that closes the open byte at i, as span finds it;
// len(cmd) when there is none. The flat reading finds it as the reader
// before AR44 did: quoted text and escapes skipped, a `"…"` span read flat.
func matching(cmd string, i int, open, close byte, r reading, saw *seen) int {
	if r.quotes != flat {
		return span(cmd, i+1, open, close, false, r, saw)
	}
	depth := 0
	for ; i < len(cmd); i++ {
		switch c := cmd[i]; {
		case c == '\\':
			i++
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '\'':
			i = ansiC(cmd, i+2, nil)
		case c == '\'':
			j := strings.IndexByte(cmd[i+1:], '\'')
			if j < 0 {
				return len(cmd)
			}
			i += j + 1
		case c == '"':
			for i++; i < len(cmd) && cmd[i] != '"'; i++ {
				if cmd[i] == '\\' {
					i++
				}
			}
		case c == open:
			depth++
		case c == close:
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(cmd)
}

// nested reads the command argument of `sh -c`, `bash -c` (and kin) and
// eval, and each command `find` runs by -exec, -execdir, -ok or -okdir.
func nested(words []string, depth int) [][]string {
	i := commandWord(words)
	if depth > maxDepth {
		// only find's -exec comes here: what its command would run in turn
		// is past the bound, so unreadable
		_, runs := splitString(words[:i])
		if !runs && i < len(words) {
			base := filepath.Base(words[i])
			runs = shells[base] || base == "find" || base == "eval"
		}
		if runs {
			return [][]string{{unreadMark}}
		}
		return nil
	}
	if s, ok := splitString(words[:i]); ok {
		// env reads the split words as its own: options, then the command
		return handSegments("env "+s+" "+strings.Join(words[i:], " "), depth+1)
	}
	if i == len(words) {
		return nil
	}
	switch filepath.Base(words[i]) {
	case "find":
		var segs [][]string
		for j := i + 1; j < len(words); j++ {
			switch words[j] {
			case "-exec", "-execdir", "-ok", "-okdir":
				k := j + 1
				for k < len(words) && words[k] != ";" && words[k] != "+" {
					k++
				}
				if k > j+1 {
					segs = append(segs, words[j+1:k])
					segs = append(segs, nested(words[j+1:k], depth+1)...)
				}
				j = k
			}
		}
		return segs
	case "eval":
		return handSegments(strings.Join(words[i+1:], " "), depth+1)
	case "sh", "bash", "zsh", "dash", "ksh":
		for j := i + 1; j+1 < len(words); j++ {
			w := words[j]
			if strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "--") && strings.Contains(w, "c") {
				return handSegments(words[j+1], depth+1)
			}
		}
	}
	return nil
}

// splitString is the string `env -S` (--split-string) splits into its
// command, when one of the wrappers before the command word is env with it.
func splitString(pre []string) (string, bool) {
	for j, w := range pre {
		if filepath.Base(w) != "env" {
			continue
		}
		for k := j + 1; k < len(pre) && strings.HasPrefix(pre[k], "-"); k++ {
			a := pre[k]
			switch {
			case (a == "-S" || a == "--split-string") && k+1 < len(pre):
				return pre[k+1], true
			case strings.HasPrefix(a, "--split-string="):
				return strings.TrimPrefix(a, "--split-string="), true
			case strings.HasPrefix(a, "-S"):
				return a[2:], true
			}
		}
	}
	return "", false
}

type heredoc struct {
	delim string
	strip bool
}

// readHeredoc reads the delimiter after `<<` at i, returning the index of
// its last byte. A quoted delimiter runs to its closing quote (AR43 act 1):
// `'` takes every byte up to the next `'`, `"` every byte up to the next
// unescaped `"`, and a backslash the next byte, so a blank inside the
// quotes is in the delimiter, as it is in the shell's. Outside a span the
// delimiter ends at a blank or an operator. A quote that never closes
// would take the rest of the line, which no shell runs; the delimiter is
// then read as it was before AR43 (flatDelimiter), so the rest is still
// read and no line that reading denied is lost.
func readHeredoc(cmd string, i int) (int, heredoc) {
	var h heredoc
	if i < len(cmd) && cmd[i] == '-' {
		h.strip = true
		i++
	}
	for i < len(cmd) && (cmd[i] == ' ' || cmd[i] == '\t') {
		i++
	}
	start := i
	var d strings.Builder
	for ; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\'':
			j := strings.IndexByte(cmd[i+1:], '\'')
			if j < 0 {
				return flatDelimiter(cmd, start, h)
			}
			d.WriteString(cmd[i+1 : i+1+j])
			i += j + 1
			continue
		case c == '"':
			for i++; i < len(cmd) && cmd[i] != '"'; i++ {
				// inside the span a backslash is dropped only before a byte
				// it escapes there; before any other it is the delimiter's
				if cmd[i] == '\\' && i+1 < len(cmd) && strings.IndexByte("\"\\$`", cmd[i+1]) >= 0 {
					i++
				}
				d.WriteByte(cmd[i])
			}
			if i >= len(cmd) {
				return flatDelimiter(cmd, start, h)
			}
			continue
		case c == '\\':
			if i++; i < len(cmd) && cmd[i] != '\n' {
				d.WriteByte(cmd[i])
			}
			continue
		}
		if strings.IndexByte(" \t\n;&|()<>", c) >= 0 {
			break
		}
		d.WriteByte(c)
	}
	i = min(i, len(cmd))
	h.delim = d.String()
	return i - 1, h
}

// flatDelimiter is readHeredoc's reading before AR43, from the delimiter's
// first byte: every quote and backslash is skipped, and the delimiter ends
// at the first blank or operator.
func flatDelimiter(cmd string, i int, h heredoc) (int, heredoc) {
	var d strings.Builder
	for ; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' || c == '"' || c == '\\' {
			continue
		}
		if strings.IndexByte(" \t\n;&|()<>", c) >= 0 {
			break
		}
		d.WriteByte(c)
	}
	h.delim = d.String()
	return i - 1, h
}

// skip drops the heredoc body that starts after the newline at i, returning
// the index of the newline that ends the delimiter line.
func (h heredoc) skip(cmd string, i int) int {
	for i < len(cmd) {
		end := strings.IndexByte(cmd[i+1:], '\n')
		if end < 0 {
			return len(cmd)
		}
		line := cmd[i+1 : i+1+end]
		i += end + 1
		if h.strip {
			line = strings.TrimLeft(line, "\t")
		}
		if line == h.delim {
			return i
		}
	}
	return i
}
