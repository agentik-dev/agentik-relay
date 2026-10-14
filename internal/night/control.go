package night

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// ControlPath is the night's control file (§9): `agentik-relay stop` and `relay
// kill` append to it, the night's loop reads it after every event.
func ControlPath(state, night string) string {
	return filepath.Join(state, "nights", night, "control")
}

// ReadControl reads the control file: stop when a line reads `stop`, and
// every `kill <row>` line's row (`kill` alone is "*", every row). A missing
// file is no command.
func ReadControl(fs effects.FS, path string) (stop bool, kills []string) {
	b, err := fs.ReadFile(path)
	if err != nil {
		return false, nil
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		switch {
		case len(f) == 1 && f[0] == "stop":
			stop = true
		case len(f) == 1 && f[0] == "kill":
			kills = append(kills, "*")
		case len(f) == 2 && f[0] == "kill":
			kills = append(kills, f[1])
		}
	}
	return stop, kills
}

// OpenControl is the night's open over its control file (AR34 act 1): one
// modified at or after since (the plan's start) was written for this
// night, during its plan, and is read with ReadControl and kept; an older
// one is a leftover and is removed, and reads as no command. The file's
// time is the machine's (os.Stat), as the remove was.
func OpenControl(fs effects.FS, path string, since time.Time) (stop bool, kills []string) {
	stop, kills, old := controlSince(fs, path, since)
	if old {
		os.Remove(path)
	}
	return stop, kills
}

// controlSince is ReadControl over a control file modified at or after
// since; an older one (old) or none on the machine's disk reads as no
// command. The measure's watch reads it so too (AR34 act 2).
func controlSince(fs effects.FS, path string, since time.Time) (stop bool, kills []string, old bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, nil, false
	}
	if fi.ModTime().Before(since) {
		return false, nil, true
	}
	stop, kills = ReadControl(fs, path)
	return stop, kills, false
}

// controlLines is what ReadControl read, as the owner wrote it: `stop`,
// then each `kill <row>` (`kill` alone for "*"), comma-separated.
func controlLines(stop bool, kills []string) string {
	var out []string
	if stop {
		out = append(out, "stop")
	}
	for _, k := range kills {
		if k == "*" {
			out = append(out, "kill")
		} else {
			out = append(out, "kill "+k)
		}
	}
	return strings.Join(out, ", ")
}

// WriteControl appends one command line to the control file.
func WriteControl(fs effects.FS, path, line string) error {
	old, _ := fs.ReadFile(path)
	return fs.WriteFile(path, append(old, []byte(line+"\n")...), 0o600)
}
