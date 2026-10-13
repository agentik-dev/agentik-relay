package effects

import (
	"errors"
	"testing"
	"time"
)

// TestAR18_2_StopLineError (conventions §5): a line whose last stderr line
// is bounded's stop, lock, load or wall, exiting 2 or 124, returns a
// *BoundedStop carrying that line, make's own trailer skipped; stdout's last
// line stays last.
func TestAR18_2_StopLineError(t *testing.T) {
	var sh OSShell
	for _, c := range []struct {
		line string
		exit int
		stop string
	}{
		{"echo out; echo 'bounded: stop: lock' >&2; exit 2", 2, "bounded: stop: lock"},
		{"echo 'bounded: stop: load' >&2; exit 2", 2, "bounded: stop: load"},
		{"echo out; echo 'bounded: stop: wall' >&2; exit 124", 124, "bounded: stop: wall"},
		{"echo 'bounded: stop: wall' >&2; echo 'make: *** [check] Error 124' >&2; exit 2", 2, "bounded: stop: wall"},
		{"echo 'bounded: stop: lock' >&2; echo 'gmake[1]: *** [check] Error 2' >&2; echo >&2; exit 2", 2, "bounded: stop: lock"},
	} {
		exit, _, err := sh.Run(t.TempDir(), c.line, 10*time.Second)
		var stop *BoundedStop
		if exit != c.exit || !errors.As(err, &stop) || stop.Line != c.stop || err.Error() != c.stop {
			t.Errorf("%q: exit %d, %v; want exit %d, a BoundedStop %q", c.line, exit, err, c.exit, c.stop)
		}
	}
	if _, last, _ := sh.Run(t.TempDir(), c0, 10*time.Second); last != "out" {
		t.Errorf("last = %q, want stdout's out", last)
	}
	if got := StopLine("x\nbounded: stop: wall\nmake: *** [check] Error 124\n"); got != "bounded: stop: wall" {
		t.Errorf("StopLine = %q", got)
	}
}

const c0 = "echo out; echo 'bounded: stop: lock' >&2; exit 2"

// TestAR18_2_StopOnStdoutIsExit: the stop text on stdout, followed by another
// stderr line, other than a stop word, or under exit 0 is a plain exit.
func TestAR18_2_StopOnStdoutIsExit(t *testing.T) {
	var sh OSShell
	for _, c := range []struct {
		line string
		exit int
	}{
		{"echo 'bounded: stop: wall'; exit 124", 124},
		{"echo 'bounded: stop: wall' >&2; echo later >&2; exit 2", 2},
		{"echo 'bounded: stop: tired' >&2; exit 2", 2},
		{"echo 'bounded: stop: wall ' >&2; echo x; exit 0", 0},
	} {
		exit, _, err := sh.Run(t.TempDir(), c.line, 10*time.Second)
		if exit != c.exit || err != nil {
			t.Errorf("%q: exit %d, %v; want a plain exit %d", c.line, exit, err, c.exit)
		}
	}
}
