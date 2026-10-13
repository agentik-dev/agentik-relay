package effects

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The line's log (AR31 act 2; the owner's word, 2026-09-28). A failed
// line left only its exit and last line, so finding the failing test took
// a second run. RunLog keeps the whole output: OSShell writes every byte
// it reads from the line's stdout and stderr, in the order read, into a
// new file in the pin's Logs (<state>/logs), beside the tails, which are
// unchanged. The caller judges, and KeepLog removes a green line's log at
// once; a kept one is named where the owner reads the result. A log is an
// aid, never a verdict: one that cannot be made leaves the line as it runs
// without, and the relay never reads one back. Each start removes a log
// older than LogKeep (sweepLogs).

// LogCap is how many bytes of a line's output its log keeps; past it the
// log ends with a cut line and both tails. A var so a test proves the cut.
var LogCap int64 = 64 << 20

// LogKeep is how long a kept log lives: each start removes an older one.
const LogKeep = 14 * 24 * time.Hour

// LogShell is a Shell that can keep a line's whole output: log names the
// file (in the pin's Logs), "" when none was made.
type LogShell interface {
	Shell
	RunLog(dir, line string, timeout time.Duration, label string) (exit int, last, log string, err error)
}

// RunLogged is sh's RunLog when sh keeps logs (OSShell), else its Run and
// no log: a fake or the bound shell runs as it does.
func RunLogged(sh Shell, dir, line string, timeout time.Duration, label string) (int, string, string, error) {
	if l, ok := sh.(LogShell); ok {
		return l.RunLog(dir, line, timeout, label)
	}
	exit, last, err := sh.Run(dir, line, timeout)
	return exit, last, "", err
}

// KeepLog is log when keep, else "" with the file removed; "" stays "".
func KeepLog(log string, keep bool) string {
	if log == "" || keep {
		return log
	}
	os.Remove(log)
	return ""
}

// logFile is a new log in dir, 0600, named by the UTC time and label. dir
// is made 0700 and read by Lstat, TmuxSocket's rules: a link or a non-dir
// is an error, a looser mode is set back to 0700.
func logFile(dir, label string) (*os.File, error) {
	if dir == "" {
		return nil, errors.New("the pin has no logs dir (effects.PinAt)")
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	fi, err := os.Lstat(dir)
	switch {
	case err != nil:
		return nil, err
	case fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir():
		return nil, fmt.Errorf("logs: %s is not a dir of the relay's own (%s)", dir, fi.Mode())
	case fi.Mode().Perm() != 0o700:
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, err
		}
	}
	return os.CreateTemp(dir, time.Now().UTC().Format("20060102T150405Z")+"-"+logLabel(label)+"-*.log")
}

// logLabel is s as a file name's part: [A-Za-z0-9._-] kept, any other
// byte '-', cut to 64 bytes.
func logLabel(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '.', c == '_', c == '-':
		default:
			b[i] = '-'
		}
	}
	if len(b) > 64 {
		b = b[:64]
	}
	return string(b)
}

// sweepLogs removes each log in dir older than LogKeep at now: a name
// ending .log that Lstat reads as a regular file (a link is never
// followed or removed) of the account's. dir must be a dir of the
// account's, else nothing is swept. Errors are ignored.
func sweepLogs(dir string, now time.Time) {
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return
	}
	if uid, ok := ownerOf(fi); !ok || int(uid) != os.Getuid() {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		f := filepath.Join(dir, e.Name())
		fi, err := os.Lstat(f)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if uid, ok := ownerOf(fi); !ok || int(uid) != os.Getuid() {
			continue
		}
		if fi.ModTime().Before(now.Add(-LogKeep)) {
			os.Remove(f)
		}
	}
}

// lineLog is the log both of a line's streams write into, in the order
// read. Write never fails, so the tails beside it see every byte: it
// writes what fits under cap and counts the rest in dropped, and after its
// first write error (a full disk) it keeps the error and drops the rest.
type lineLog struct {
	mu              sync.Mutex
	f               *os.File
	n, cap, dropped int64
	err             error
	nl              bool // the last byte kept is a newline
}

func (l *lineLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return len(p), nil
	}
	keep := p[:min(int64(len(p)), max(l.cap-l.n, 0))]
	if len(keep) > 0 {
		k, err := l.f.Write(keep)
		l.n += int64(k)
		if k > 0 {
			l.nl = keep[k-1] == '\n'
		}
		if err != nil {
			l.err = err
			return len(p), nil
		}
	}
	l.dropped += int64(len(p) - len(keep))
	return len(p), nil
}

// close ends the log and closes it: a log cut at cap (and whole on disk
// so far) ends with the cut line and both tails, stdout's first.
func (l *lineLog) close(stdout, stderr string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dropped > 0 && l.err == nil {
		var b strings.Builder
		if l.n > 0 && !l.nl {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "relay: log cut at %d bytes; %d more bytes read, not kept; the last %d bytes of stdout and of stderr follow\n",
			l.cap, l.dropped, OutputCap)
		for _, t := range []struct{ name, s string }{{"stdout", stdout}, {"stderr", stderr}} {
			fmt.Fprintf(&b, "relay: %s's tail\n", t.name)
			b.WriteString(t.s)
			if t.s != "" && !strings.HasSuffix(t.s, "\n") {
				b.WriteByte('\n')
			}
		}
		l.f.WriteString(b.String())
	}
	l.f.Close()
}
