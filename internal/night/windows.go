package night

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// Windows is the night's viewers (§9, D16): under tmux a session
// relay-<night> whose first window watches the night's status (StatusArgv)
// and a window per row tailing the row's log; under never, file windows only (the logs, which
// `attach` tails). A window never holds the child's terminal.
type Windows struct {
	Mode    string // auto, never or required; auto falls to never when tmux is lost
	Session string
	Tmux    effects.Tmux
	State   string
	Closed  []string // the old nights' sessions Open killed (AR33 act 2)
	Warn    []string // a failed list or kill, `relay: tmux: <err>`, for the caller to print

	lost bool
}

// SessionName is the night's tmux session.
func SessionName(night string) string { return "relay-" + night }

// RowLog is the row's live log: <state>/nights/<night>/<row>.log.
func RowLog(state, night, row string) string {
	return filepath.Join(NightDir(state, night), row+".log")
}

// Open starts the night's session under auto or required when tmux is
// present, after it kills every other night's session on the relay's
// socket (sweep). Under auto an absent tmux or a session that will not
// start is file windows; under required either is an error. Under never
// it touches nothing.
func Open(tm effects.Tmux, mode, night, state string) (*Windows, error) {
	w := &Windows{Mode: mode, Tmux: tm, State: state}
	switch mode {
	case "never":
		return w, nil
	case "auto", "required":
	default:
		return nil, fmt.Errorf("tmux: %q: want auto, never or required", mode)
	}
	if tm == nil || !tm.Present() {
		if mode == "required" {
			return nil, fmt.Errorf("tmux: required, and not on PATH")
		}
		w.Mode = "never"
		return w, nil
	}
	name := SessionName(night)
	w.sweep(name)
	if err := tm.NewSession(name, state, StatusArgv(night)); err != nil {
		if mode == "required" {
			return nil, err
		}
		w.Mode = "never"
		return w, nil
	}
	w.Session = name
	return w, nil
}

// sweep kills each session on the relay's socket named relay-<anything>
// but own (AR33 act 2): there is one relay per account (§8), so when a
// night opens no other night is live, and its window would only watch a
// stopped night. A failed list or kill is a Warn line, never the night's
// error.
func (w *Windows) sweep(own string) {
	names, err := w.Tmux.Sessions()
	if err != nil {
		w.Warn = append(w.Warn, "relay: tmux: "+err.Error())
		return
	}
	for _, s := range names {
		if !strings.HasPrefix(s, "relay-") || s == own {
			continue
		}
		if err := w.Tmux.Kill(s); err != nil {
			w.Warn = append(w.Warn, "relay: tmux: "+err.Error())
			continue
		}
		w.Closed = append(w.Closed, s)
	}
}

// RowWindow opens window <row> tailing the row's log; nothing under never.
func (w *Windows) RowWindow(row, logPath string) error {
	if w == nil || w.Mode == "never" {
		return nil
	}
	return w.Tmux.NewWindow(w.Session, row, w.State, []string{"tail", "-f", logPath})
}

// Check asks whether the session is still there, after every loop event.
// Lost under auto: Mode becomes never and fallback is true, once. Lost under
// required: gone is true, once.
func (w *Windows) Check() (fallback bool, gone bool) {
	if w == nil || w.Mode == "never" || w.lost {
		return false, false
	}
	if ok, err := w.Tmux.Alive(w.Session); ok && err == nil {
		return false, false
	}
	w.lost = true
	if w.Mode == "required" {
		return false, true
	}
	w.Mode = "never"
	return true, false
}

// Attach is what `relay attach [<row>]` runs for the night in st (§9, D16):
// `tmux attach` to the session, or to the row's window, when the session is
// alive; else `tail -f` the row's log (no row: every row's). An error names
// a night with no rows or a row the night does not hold.
func Attach(tm effects.Tmux, st Status, state, row string) ([]string, error) {
	var logs []string
	for _, r := range st.Rows {
		if row == "" || r.ID == row {
			logs = append(logs, RowLog(state, st.Night, r.ID))
		}
	}
	switch {
	case len(logs) == 0 && row == "":
		return nil, fmt.Errorf("night %s has no rows", st.Night)
	case len(logs) == 0:
		return nil, fmt.Errorf("night %s has no row %s", st.Night, row)
	}
	if tm != nil && tm.Present() {
		session := SessionName(st.Night)
		if ok, err := tm.Alive(session); ok && err == nil {
			return tm.AttachArgv(session, row), nil
		}
	}
	return append([]string{"tail", "-f"}, logs...), nil
}
