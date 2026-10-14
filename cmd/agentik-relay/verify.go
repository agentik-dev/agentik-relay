package main

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/gate"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// verifyReport is `agentik-relay doctor --verify <dir>`'s answer (--json).
type verifyReport struct {
	Dir     string  `json:"dir"`
	Branch  string  `json:"branch"`
	Sha     string  `json:"sha"`
	Line    string  `json:"line"`
	Exit    int     `json:"exit"`
	Last    string  `json:"last"`
	Seconds float64 `json:"seconds"`
	OK      bool    `json:"ok"`
	Error   string  `json:"error,omitempty"`
	Log     string  `json:"log,omitempty"` // the line's whole output when it erred or was red (AR31 act 2)
	// Limit is the repo's verify limit in seconds (verify_seconds, else
	// 900; 0, unknown, when the config does not load), and Suggest the
	// verify_seconds suggested for the verify line when it exited 0
	// (effects.SuggestVerifySeconds; never for --line), 0 for none (AR59
	// act 4 and its fix round).
	Limit   int `json:"limit"`
	Suggest int `json:"suggest"`
}

// doctorVerify is `agentik-relay doctor --verify <dir>` (AR25 act 3, ADV15-4):
// dir's verify line measured as a gate measures it. Under run's pin (a
// night's), a fresh clone of dir's HEAD branch at its sha (gate.Fresh, one
// ref, under the state root's gate/); the line is `auto` resolved there
// (`make check` when `make -n check` succeeds) and run by the gate's
// runner (one scratch home) under effects.MaxVerifySeconds, never the
// repo's limit, so a line past its limit is still measured (AR59 act 4);
// exit, last line and seconds
// are printed, and the clone is removed. The repo's limit is printed with
// its source (`limit: 900 s (unset)`, `limit: 1800 s (verify_seconds)`),
// and a line that exits 0 gets a suggested verify_seconds
// (effects.SuggestVerifySeconds); a red or cut line suggests nothing and
// says why. line, when set (--line), is run
// there instead (a measure under the pin: `go list -m all`). dir is only
// read. Exit 0 when the
// line exits 0, 1 when it is red, 2 when it cannot run (nested, a detached
// HEAD, no verify line, a clone refused). A line that erred or exited
// non-zero keeps its whole output in a log (AR31 act 2, <state>/logs,
// labelled <dir>-doctor), named in `log` and printed; the probe keeps none.
func doctorVerify(dir, line string, asJSON bool, stdout, stderr io.Writer) int {
	rep := verifyReport{Dir: dir, Exit: -1, Limit: int(effects.Timeout / time.Second)}
	from, why := "unset", "" // the limit's source; why no suggestion
	probe := line != ""      // --line: not the verify line, so no suggestion
	done := func(code int, err error) int {
		if err != nil {
			rep.Error = err.Error()
		}
		switch {
		case asJSON:
			writeJSON(stdout, rep)
		case err != nil:
			fmt.Fprintf(stderr, "relay doctor --verify: %v\n", err)
		default:
			// The limit and the suggestion end the exit line (AR59 act 4), so
			// a red line's text keeps its four lines, `log:` the fourth.
			suggest := "none: " + why
			if rep.Suggest > 0 {
				suggest = fmt.Sprintf("verify_seconds: %d in %s, section relay", rep.Suggest, effects.RepoConfigFile)
			}
			limit := fmt.Sprintf("%d s (%s)", rep.Limit, from)
			if rep.Limit == 0 {
				limit = "unknown (" + from + ")"
			}
			fmt.Fprintf(stdout, "verify %s (%s at %.12s): `%s` in a fresh clone under the pin\nexit %d in %.0f s; limit: %s; suggest: %s\nlast: %s\n",
				rep.Dir, rep.Branch, rep.Sha, rep.Line, rep.Exit, rep.Seconds, limit, suggest, rep.Last)
			if rep.Log != "" {
				fmt.Fprintf(stdout, "log: %s\n", rep.Log)
			}
		}
		return code
	}
	fx := effectsFor(effects.StateDir(getenv), "")
	if effects.Nested(fx.Getenv) {
		return done(exitUsage, fmt.Errorf("nested (%s set): the verify line is never measured", effects.MeasureDepthVar))
	}
	var err error
	if rep.Branch, err = fx.Git.Branch(dir); err != nil || rep.Branch == "" {
		return done(exitUsage, fmt.Errorf("%s: no branch at HEAD (%v): a detached HEAD is not cloned", dir, err))
	}
	if rep.Sha, err = fx.Git.Head(dir); err != nil {
		return done(exitUsage, fmt.Errorf("%s: HEAD: %v", dir, err))
	}
	root := dir
	if r, err := fx.Git.Root(dir); err == nil && r != "" {
		root = r
	}
	// A config that does not load leaves the limit unknown (0 in --json)
	// and the line is measured anyway (AR59's fix round).
	if cfg, err := effects.LoadConfig(effects.OSFS{}, getenv, root); err != nil {
		rep.Limit, from = 0, effects.LastLine(err.Error())
	} else if rep.Limit = int(cfg.VerifyLimit() / time.Second); cfg.VerifySeconds > 0 {
		from = "verify_seconds"
	}
	tmp, remove, err := gate.Fresh(fx, dir, effects.Ref{Name: "refs/heads/" + rep.Branch, Sha: rep.Sha})
	if err != nil {
		return done(exitUsage, fmt.Errorf("the clone of %s: %v", dir, err))
	}
	defer remove()
	sh, release, err := effects.Scratched(fx.Shell)
	if err != nil {
		return done(exitUsage, err)
	}
	defer release()
	if line == "" {
		line = "auto"
	}
	if rep.Line, err = night.ResolveVerify(sh, tmp, line); err != nil || rep.Line == "" {
		return done(exitUsage, fmt.Errorf("%s has no verify line (`make -n check` in the clone: %v)", dir, err))
	}
	label := "repo"
	if abs, err := filepath.Abs(dir); err == nil {
		label = filepath.Base(abs)
	}
	start := time.Now()
	measure := effects.MaxVerifySeconds * time.Second // never the repo's limit (AR59 act 4)
	rep.Exit, rep.Last, rep.Log, err = effects.RunLogged(sh, tmp, rep.Line, measure, label+"-doctor")
	took := time.Since(start)
	rep.Seconds = took.Seconds()
	rep.Log = effects.KeepLog(rep.Log, err != nil || rep.Exit != 0)
	if err != nil {
		sfx := ""
		if rep.Log != "" {
			sfx = "; log: " + rep.Log
		}
		return done(exitUsage, fmt.Errorf("`%s` could not run: %v%s", rep.Line, err, sfx))
	}
	rep.OK = rep.Exit == 0
	switch {
	case effects.Cut(rep.Exit, took, measure):
		why = fmt.Sprintf("the line was cut at %d s", effects.MaxVerifySeconds)
	case !rep.OK:
		why = fmt.Sprintf("the line exited %d", rep.Exit)
	case probe:
		why = "--line is not the verify line"
	default:
		rep.Suggest = effects.SuggestVerifySeconds(took)
	}
	if !rep.OK {
		return done(exitRed, nil)
	}
	return done(exitOK, nil)
}
