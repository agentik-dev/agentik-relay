package effects

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

// OSShell runs lines with the pinned sh -c, under the pinned env alone
// (AR22 act 1: Pin's allowlist, never this process's env), in their own session (Setsid: the
// session's and the group's id are the shell's pid, AR17C), so a timeout
// ends the line and everything it started, and the session sweep finds
// what left the group (ADV10-12). Every line sees
// RELAY_MEASURE_DEPTH=1 (the recursion guard, §15). Its output goes
// through pipes it reads itself, so the Wait is the shell's alone: Run
// returns within a second of the shell's exit whatever holds the pipes
// (§15, the wait); a pipe still held then is the line's error (ADV6-8). Groups, when set, tracks each line's group while it
// runs (ADV6-2: a signal to the relay reaches it; a killed relay's next
// start reaps it).
//
// DenyPush names the remotes whose pushurl each line has denied, as
// ChildEnv denies them to the child (AR20 act 3, ADV11-6): the gate's lines
// run the branch's code. A line keeps the last OutputCap bytes of each of
// its stdout and stderr (ADV11-17); RunLog also keeps its whole output in
// a file (AR31 act 2, log.go).
//
// Scratch is the scratch home every line runs under (Scratched; AR25 act
// 1): "" gives each line a fresh one of its own, removed at its end. Tmp
// is its temp dir, set with it (AR31 act 1): a Scratch without its Tmp
// runs nothing, so no line gets the long TMPDIR back.
type OSShell struct {
	Groups   *Groups
	DenyPush []string
	Scratch  string
	Tmp      string // the scratch's dir in the pin's temp dir, set with Scratch (Scratched); its t/ is TMPDIR
}

// OutputCap is how much of a line's stdout, and of its stderr, Run keeps:
// the tail, where the last line and bounded's stop line are.
const OutputCap = 64 << 10

// DenyingPush is sh with remotes' pushurl denied to every line it runs,
// when sh is the machine's shell; any other shell is sh.
func DenyingPush(sh Shell, remotes []string) Shell {
	if o, ok := sh.(OSShell); ok {
		o.DenyPush = append([]string(nil), remotes...)
		return o
	}
	return sh
}

// tail is a writer keeping the last max bytes written to it.
type tail struct {
	max int
	b   []byte
}

func (t *tail) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= t.max {
		t.b = append(t.b[:0], p[len(p)-t.max:]...)
		return n, nil
	}
	if over := len(t.b) + len(p) - t.max; over > 0 {
		t.b = append(t.b[:0], t.b[over:]...)
	}
	t.b = append(t.b, p...)
	return n, nil
}

func (t *tail) String() string { return string(t.b) }

// PipeGrace is how long Run reads the pipes after the shell's exit and the
// sweep; whatever still holds them then (a process that left the group) is
// cut off.
var PipeGrace = 300 * time.Millisecond

func (sh OSShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	exit, last, _, err := sh.run(dir, line, timeout, "")
	return exit, last, err
}

// RunLog is Run with the line's whole output kept in a new file in the
// pin's Logs (AR31 act 2), named by label ("" is the line) and returned as
// log; "" when none could be made, and the line runs as Run runs it.
func (sh OSShell) RunLog(dir, line string, timeout time.Duration, label string) (int, string, string, error) {
	return sh.run(dir, line, timeout, cmp.Or(label, line, "line"))
}

// run is Run, with a log when label is set.
func (sh OSShell) run(dir, line string, timeout time.Duration, label string) (exit int, last, log string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	pin, perr := pinnedEnv()
	if perr != nil {
		return -1, "", "", perr
	}
	scratch, tmp := sh.Scratch, sh.Tmp
	switch {
	case scratch != "" && tmp == "":
		return -1, "", "", errors.New("a scratch home without its temp dir (effects.Scratched)")
	case scratch == "":
		h, t, err := newScratch()
		if err != nil {
			return -1, "", "", err
		}
		defer removeScratch(h, t)
		scratch, tmp = h, t
	}
	cmd := exec.CommandContext(ctx, pin.Sh, "-c", line)
	cmd.Dir = dir
	cmd.Env = scratchEnv(depthEnv(slices.Clone(pin.Env)), scratch, tmp)
	if len(sh.DenyPush) > 0 {
		cmd.Env = PushDenyEnv(cmd.Env, sh.DenyPush)
	}
	out, errOut := tail{max: OutputCap}, tail{max: OutputCap}
	outR, outW, err := os.Pipe()
	if err != nil {
		return -1, "", "", err
	}
	defer outR.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		outW.Close()
		return -1, "", "", err
	}
	defer errR.Close()
	cmd.Stdout, cmd.Stderr = outW, errW // files: no copy for the Wait to wait on
	// The session sweep's mark and snapshot (AR17C, ADV10-12): fd 3 is a
	// file only this line's processes inherit, and every pid alive before
	// the start is never the line's.
	mark, err := os.CreateTemp("", "relay-line-*")
	if err != nil {
		outW.Close()
		errW.Close()
		return -1, "", "", err
	}
	defer os.Remove(mark.Name())
	defer mark.Close()
	before := map[int]bool{}
	if pids, perr := (OSProcs{}).Pids(); perr == nil {
		for _, pid := range pids {
			before[pid] = true
		}
	}
	cmd.ExtraFiles = []*os.File{mark}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	// The line's log (AR31 act 2), made just before the start: the relay's
	// own fd, close-on-exec, never the line's. None made, the line runs
	// as it does without.
	var ll *lineLog
	if label != "" {
		if f, ferr := logFile(pin.Logs, label); ferr == nil {
			ll = &lineLog{f: f, cap: LogCap}
		}
	}
	err = cmd.Start()
	outW.Close()
	errW.Close()
	mark.Close() // the line's copies alone hold it now
	if err != nil {
		if ll != nil {
			ll.f.Close()
			os.Remove(ll.f.Name())
		}
		return -1, "", "", err
	}
	read := func(r *os.File, b io.Writer) <-chan error {
		if ll != nil {
			b = io.MultiWriter(b, ll)
		}
		done := make(chan error, 1)
		go func() { _, err := io.Copy(b, r); done <- err }()
		return done
	}
	pgid := cmd.Process.Pid
	if sh.Groups != nil {
		started, _ := OSProcs{}.Started(pgid)
		sh.Groups.Add(Group{Pgid: pgid, Started: started}, groupSignal(pgid))
	}
	outDone, errDone := read(outR, &out), read(errR, &errOut)
	err = cmd.Wait()
	// Whatever the line left running in its group dies with it: the orphan
	// sweep (§15), three rounds; a survivor is an error.
	if _, left := Sweep(OSProcs{}, cmd.Process.Pid); len(left) > 0 && err == nil {
		err = fmt.Errorf("orphans: %d of the line's group survived %d kills", len(left), SweepRounds)
	}
	// Then the session sweep (AR17C, ADV10-12): what left the group, by
	// session or by the mark, is killed; any found is the line's error, the
	// check unrun: orphans (never green), the count in its reason.
	if found, left := SweepSession(OSProcs{}, pgid, before, mark.Name()); found > 0 && (err == nil || !strings.HasPrefix(err.Error(), "orphans: ")) {
		err = fmt.Errorf("orphans: %d of the line's processes escaped its group (setsid or setpgid), swept; %d survived %d kills", found, len(left), SweepRounds)
	}
	sh.Groups.Done(pgid)
	// The pipes close once their last holder is gone; one that left the
	// group is cut off after PipeGrace, and the line's output is not all
	// read: its error, the check unrun (ADV6-8).
	deadline := time.Now().Add(PipeGrace)
	outR.SetReadDeadline(deadline)
	errR.SetReadDeadline(deadline)
	e1, e2 := <-outDone, <-errDone
	if ll != nil { // both reads are done: every byte read is in it
		ll.close(out.String(), errOut.String())
		log = ll.f.Name()
	}
	last = LastLine(out.String())
	if last == "" {
		last = LastLine(errOut.String())
	}
	if errors.Is(e1, os.ErrDeadlineExceeded) || errors.Is(e2, os.ErrDeadlineExceeded) {
		return -1, last, log, fmt.Errorf("orphans: the line's output still held %s after its exit", PipeGrace)
	}
	if ctx.Err() == context.DeadlineExceeded {
		return 124, last, log, nil
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, last, log, nil
	case errors.As(err, &ee):
		// bounded's stop (conventions §5): the check never ran, so the
		// line is unrun, never red; every reader maps a Run error there.
		if stop := StopLine(errOut.String()); stop != "" && ee.ExitCode() != 0 {
			return ee.ExitCode(), last, log, &BoundedStop{Line: stop}
		}
		return ee.ExitCode(), last, log, nil
	default:
		return -1, last, log, err
	}
}

// LastLine is the last non-empty line of s, trimmed and capped at LastCap.
func LastLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return capText(l, LastCap)
		}
	}
	return ""
}

// BoundedStop is a line that bounded stopped before or while it ran (its
// last stderr line `bounded: stop: lock|load|wall`, exit 2, 2 or 124;
// conventions §5): unrun, never red.
type BoundedStop struct{ Line string }

func (e *BoundedStop) Error() string { return e.Line }

var (
	stopLine    = regexp.MustCompile(`^bounded: stop: (lock|load|wall)$`)
	makeTrailer = regexp.MustCompile(`^g?make(\[[0-9]+\])?: \*\*\* `)
)

// StopLine is stderr's last line that is not make's own trailer when it is
// bounded's stop line, else "".
func StopLine(stderr string) string {
	lines := strings.Split(stderr, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || makeTrailer.MatchString(l) {
			continue
		}
		if stopLine.MatchString(l) {
			return l
		}
		return ""
	}
	return ""
}
