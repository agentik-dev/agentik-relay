package night

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// GroupsFile is the night's live process groups (ADV6-2), under its night
// directory: written at every spawn and line, removed at the night's exit,
// so a file that outlives its relay names what a killed relay left.
const GroupsFile = "groups.json"

// groupsDoc is groups.json: the relay that keeps it (its pid and start
// time, so the next start leaves a live relay's file alone), the root it
// runs in (AR37 fix: written from the measure on, so a night with no
// status.json yet still names its repo) and its groups.
type groupsDoc struct {
	PID     int             `json:"pid"`
	Started string          `json:"started"`
	Root    string          `json:"root,omitempty"`
	Groups  []effects.Group `json:"groups"`
}

// signalName is a trapped signal's name in the stop line's detail.
func signalName(sig os.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGHUP:
		return "SIGHUP"
	}
	return sig.String()
}

// selfStarted is this process's start time, "" when Procs cannot read it.
func selfStarted(p effects.Procs) string {
	if p == nil {
		return ""
	}
	s, _ := p.Started(os.Getpid())
	return s
}

// Reap is the next start's cleanup of a dead relay (ADV6-2): every night's
// groups.json under state whose relay is gone (its pid no longer has its
// start time) is read; a group whose leader still has the recorded start
// time is killed, with a `reaped` line; the rest (gone, or a reused pid,
// never signalled) are dropped; the file is removed. A night's planning
// mark whose relay is gone (deadPlan) is removed too (AR60 act 2): a relay
// killed by SIGKILL in its plan opened no night, and no stop reason is
// known. Its directory stays, so NightName never hands its number out again
// while the ledger holds its lines (the review's block 1); Latest skips it.
// Without Procs no start time can be read: nothing is killed and every
// file stays.
func Reap(fx effects.Effects, state string) {
	if fx.Procs == nil || state == "" {
		return
	}
	nights, _ := fx.FS.List(filepath.Join(state, "nights"))
	for _, nt := range nights {
		reapGroups(fx, state, nt)
		reapPlan(fx, state, nt)
	}
}

// reapGroups is Reap's work on one night's groups.json.
func reapGroups(fx effects.Effects, state, nt string) {
	file := filepath.Join(NightDir(state, nt), GroupsFile)
	b, err := fx.FS.ReadFile(file)
	if err != nil {
		return
	}
	var doc groupsDoc
	json.Unmarshal(b, &doc)
	if doc.PID > 1 && doc.Started != "" {
		if s, err := fx.Procs.Started(doc.PID); err == nil && s == doc.Started {
			return // a live relay's night: its groups are its own
		}
	}
	for _, g := range doc.Groups {
		if g.Pgid <= 1 || g.Started == "" {
			continue
		}
		if s, err := fx.Procs.Started(g.Pgid); err != nil || s != g.Started {
			continue
		}
		m := map[string]any{"pgid": g.Pgid, "row": g.Row, "of_night": nt, "started": g.Started}
		if err := fx.Procs.KillGroup(g.Pgid); err != nil {
			m["error"] = err.Error()
		}
		fx.Ledger.Append("reaped", m)
	}
	os.Remove(file)
}

// reapPlan removes one night's status.json when it is a dead plan's mark
// (deadPlan), and only that: the directory keeps the night's number spent.
// The status is read as written: a root is not looked up.
func reapPlan(fx effects.Effects, state, nt string) {
	dir := NightDir(state, nt)
	file := filepath.Join(dir, "status.json")
	b, err := fx.FS.ReadFile(file)
	if err != nil {
		return
	}
	var st Status
	if json.Unmarshal(b, &st) != nil || !deadPlan(fx.Procs, st) {
		return
	}
	fx.FS.Remove(file)
}

// deadPlan is a planning mark whose relay is gone: state `planning` under a
// pid that is not this process's, whose start time p does not read (the
// pid is gone) or reads as another (a reused pid). A mark with no start
// time under a live pid cannot be told from its relay's, so it stays.
func deadPlan(p effects.Procs, st Status) bool {
	if st.State != string(relay.Planning) || st.PID <= 1 || st.PID == os.Getpid() {
		return false
	}
	s, err := p.Started(st.PID)
	return err != nil || (st.Started != "" && s != st.Started)
}
