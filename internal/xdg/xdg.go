// Package xdg is the one state home the relay and its hooks read (AR76;
// session-layer-r1 §2.4). It imports the standard library only, so the
// hooks, which import nothing else of the relay, read it too.
package xdg

import (
	"path/filepath"
	"strings"
)

// StateHome is ${XDG_STATE_HOME:-$HOME/.local/state}: a value of `~` or one
// starting `~/` is read under HOME, as the session plugin expands it; any
// other value is taken as written (`~x`, a relative path). It is "" when
// the value needs HOME and HOME is empty.
func StateHome(env func(string) string) string {
	base, home := env("XDG_STATE_HOME"), env("HOME")
	tilde := base == "~" || strings.HasPrefix(base, "~/")
	switch {
	case base != "" && !tilde:
		return base
	case home == "":
		return ""
	case tilde:
		return filepath.Join(home, base[1:])
	}
	return filepath.Join(home, ".local", "state")
}
