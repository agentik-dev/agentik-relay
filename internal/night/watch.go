package night

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// Clear is the watch's redraw (AR33 act 1): the cursor home and the screen
// erased, ANSI, never a `clear` process.
const Clear = "\x1b[H\x1b[2J"

// StatusArgv is the session's first window (AR33 act 1): `<relay> status
// --night <night> --watch 5 --hold`, no shell, so the window watches its own
// night, stops with it and keeps the last table. <relay> is the running
// binary (os.Executable), else `relay` on PATH.
func StatusArgv(night string) []string {
	return []string{relayBinary(), "status", "--night", night, "--watch", "5", "--hold"}
}

func relayBinary() string {
	if p, err := os.Executable(); err == nil && p != "" {
		return p
	}
	return "relay"
}

// RelayOf is the pid and start time of the relay that runs the night, as
// the night records them while it runs: status.json's, else groups.json's
// (written before the first status); 0 and "" when neither records one.
func RelayOf(fs effects.FS, state string, st Status) (int, string) {
	if st.PID > 0 {
		return st.PID, st.Started
	}
	if st.Night == "" {
		return 0, ""
	}
	b, err := fs.ReadFile(filepath.Join(NightDir(state, st.Night), GroupsFile))
	if err != nil {
		return 0, ""
	}
	var g groupsDoc
	if json.Unmarshal(b, &g) != nil {
		return 0, ""
	}
	return g.PID, g.Started
}

// Liveness is a watch's test of the night's relay: PidStarted names the
// recorded pid's start time once, when the pid or its recorded start is new
// to the watch (a pid live under another start time is not the night's
// relay; an empty recorded start asks only that it is live); after that a
// signal 0 keeps it, a syscall, so a tick spawns no `ps`. No pid is gone.
func Liveness() func(pid int, started string) bool {
	seen, seenStarted := 0, ""
	return func(pid int, started string) bool {
		if pid <= 0 {
			return false
		}
		if pid == seen && started == seenStarted {
			err := syscall.Kill(pid, 0)
			return err == nil || errors.Is(err, syscall.EPERM)
		}
		s, err := PidStarted(pid)
		if err != nil || (started != "" && s != started) {
			return false
		}
		seen, seenStarted = pid, started
		return true
	}
}

// Watch is `agentik-relay status --watch` (AR33 act 1): the night's dashboard,
// redrawn in-process after Clear every `every`, until the night's state is
// terminal (`stopped:<reason>`) or its relay is gone (live false for
// RelayOf's pid); then the dashboard once more, the relay's last write, and
// Watch returns it. A night with no status yet is watched while its relay
// is live. sleep and live are the caller's, so a test needs no real time.
func Watch(w io.Writer, fs effects.FS, state, night string, every time.Duration, sleep func(time.Duration), live func(pid int, started string) bool) Status {
	for {
		st := ReadStatus(fs, state, night)
		ended := relay.NightTerminal(relay.NightState(st.State))
		if !ended {
			if pid, started := RelayOf(fs, state, st); !live(pid, started) {
				ended = true
				st = ReadStatus(fs, state, night) // a last write after the read above
			}
		}
		fmt.Fprint(w, Clear)
		PrintStatus(w, st)
		if ended {
			return st
		}
		sleep(every)
	}
}
