package effects

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Procs is what the watchdog samples and the kill it makes (§15): the
// newest modification time under a tree (.git skipped), a process's CPU
// seconds, a process group's members, SIGKILL to the whole group, a
// process's start time (the dead relay's reap, ADV6-2: a pid whose start
// time moved is another process, never killed) and a process's
// descendants by ppid (ADV6-8: one that left the group is still the
// child's); since ADV10-12 a process's session id, every pid there is,
// and the pids holding a file open (the session sweep, AR17C); since AR25
// act 4 a process's command name (the helper breaker, ADV15-1); since
// AR27C act 2 its executable's path (ADV16-3: the name is argv[0]).
type Procs interface {
	Newest(dir string) (time.Time, error)
	CPUSeconds(pid int) (float64, error)
	Group(pgid int) ([]int, error)
	KillGroup(pgid int) error
	Started(pid int) (string, error)
	Descendants(pid int) ([]int, error)
	Session(pid int) (int, error)
	Pids() ([]int, error)
	Holders(path string) ([]int, error)
	Comm(pid int) (string, error)
	Exe(pid int) (string, error)
}

// OSProcs is the machine's: a walk of the tree, `ps -o time=`, `ps -A -o
// pid=,pgid=`, syscall.Kill(-pgid), `ps -o lstart=`, `ps -A -o
// pid=,ppid=`, getsid(2), `ps -A -o pid=`, /proc's fds or `lsof -t`,
// `ps -o comm=`, and /proc's exe or the pinned `lsof -d txt`.
type OSProcs struct{}

// Newest walks dir, skipping every .git, for the latest mtime.
func (OSProcs) Newest(dir string) (time.Time, error) {
	var newest time.Time
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // a file gone mid-walk is no progress and no failure
		}
		if d.Name() == ".git" && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, err
}

// psOutput is the pinned ps's stdout (AR22 act 2, ADV13-14): Pin's
// absolute Ps under the pinned env, never a PATH lookup; before Pin, an
// error.
func psOutput(args ...string) ([]byte, error) {
	p, ok := Current()
	if !ok || p.Ps == "" {
		return nil, errNotPinned
	}
	c := exec.Command(p.Ps, args...)
	c.Env = p.Env
	return c.Output()
}

// CPUSeconds is `ps -o time= -p pid`, parsed.
func (OSProcs) CPUSeconds(pid int) (float64, error) {
	out, err := psOutput("-o", "time=", "-p", strconv.Itoa(pid))
	if err != nil {
		return 0, fmt.Errorf("ps: pid %d: %w", pid, err)
	}
	return ParsePSTime(string(out))
}

// Group is every pid whose process group is pgid; a group no signal can
// reach (ESRCH) is empty with no ps.
func (OSProcs) Group(pgid int) ([]int, error) {
	if pgid > 1 && errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH) {
		return nil, nil
	}
	out, err := psOutput("-A", "-o", "pid=,pgid=")
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return ParsePSGroup(string(out), pgid), nil
}

// KillGroup sends SIGKILL to the whole group.
func (OSProcs) KillGroup(pgid int) error {
	if pgid <= 1 {
		return fmt.Errorf("kill: refusing process group %d", pgid)
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}

// Started is `ps -o lstart= -p pid`, trimmed; a pid ps does not list is
// an error.
func (OSProcs) Started(pid int) (string, error) {
	out, err := psOutput("-o", "lstart=", "-p", strconv.Itoa(pid))
	if s := strings.TrimSpace(string(out)); err == nil && s != "" {
		return s, nil
	}
	return "", fmt.Errorf("ps: pid %d: no such process", pid)
}

// Comm is `ps -o comm= -p pid`, trimmed (AR25 act 4): the executable's
// name as the process was started, whatever its env; a pid ps does not
// list is an error.
func (OSProcs) Comm(pid int) (string, error) {
	out, err := psOutput("-o", "comm=", "-p", strconv.Itoa(pid))
	if c := strings.TrimSpace(string(out)); err == nil && c != "" {
		return c, nil
	}
	return "", fmt.Errorf("ps: pid %d: no such process", pid)
}

// ExeTimeout bounds one lsof read of Exe (a stale network mount can block
// lsof's stat, and the watchdog's tick waits on it).
const ExeTimeout = 5 * time.Second

// Exe is pid's executable's path (AR27C act 2; ADV16-3): /proc/<pid>/exe's
// target where it exists, else the first n line of the pinned `lsof -a -p
// <pid> -d txt -Fn` (Pinned.Lsof; its first txt is the executable, the
// libraries after it), within ExeTimeout. With neither, or no such line,
// an error: the breaker then reads the comm alone.
func (OSProcs) Exe(pid int) (string, error) {
	if exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe")); err == nil {
		return exe, nil
	}
	p, ok := Current()
	if !ok || p.Lsof == "" {
		return "", fmt.Errorf("exe: pid %d: no /proc and no pinned lsof", pid)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ExeTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, p.Lsof, "-a", "-p", strconv.Itoa(pid), "-d", "txt", "-Fn")
	c.Env = p.Env
	out, _ := c.Output() // lsof exits 1 on a warning too; its lines are read
	for _, l := range strings.Split(string(out), "\n") {
		if len(l) > 1 && l[0] == 'n' {
			return l[1:], nil
		}
	}
	return "", fmt.Errorf("lsof: pid %d: no executable", pid)
}

// ExeReader names what Exe reads (AR29 act 4, ADV17-4): proc (the /proc
// root, "/proc" on a host) when <proc>/self/exe reads as a link, else the
// pinned lsof (p.Lsof), else "": with neither, Exe errors and the breaker
// counts a claude by its comm alone, which doctor's breaker row warns of.
func ExeReader(p Pinned, proc string) string {
	if proc != "" {
		if _, err := os.Readlink(filepath.Join(proc, "self", "exe")); err == nil {
			return proc
		}
	}
	return p.Lsof
}

// IsClaude reports whether a process of comm (a Comm reading) and exe (an
// Exe reading, "" unread) is a `claude` (AR25 act 4, ADV15-1; AR27C act 2,
// ADV16-3): comm's base name is claude or comm is the pinned claude's real
// file (the version file bin/claude links to); or exe, its symlinks
// resolved, is that real file; or exe has that file's size (as Pin read
// it) and its sha256 is the sum Pin bound (a copy; hashed only on a size
// match). A claude's own renamed worker (`claude bg-pty-host`) is not one:
// a helper is a claude started. Another version's file is none (§15).
func IsClaude(comm, exe string) bool {
	comm = strings.TrimSpace(comm)
	if comm != "" && filepath.Base(comm) == "claude" {
		return true
	}
	p, ok := Current()
	real := p.Tools["claude"]
	if !ok || real == "" {
		return false
	}
	if comm == real {
		return true
	}
	if exe == "" {
		return false
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if exe == real {
		return true
	}
	k, ok := p.stats.get("claude")
	fi, err := os.Stat(exe)
	if !ok || err != nil || !fi.Mode().IsRegular() || fi.Size() != k.size {
		return false
	}
	sum, err := fileSum(exe)
	return err == nil && sum == p.sums["claude"]
}

// Descendants is every process under pid by ppid, `ps -A -o pid=,ppid=`
// walked from pid (pid itself not among them).
func (OSProcs) Descendants(pid int) ([]int, error) {
	out, err := psOutput("-A", "-o", "pid=,ppid=")
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return ParsePSTree(string(out), pid), nil
}

// Session is getsid(2) by its raw syscall (darwin's ps has no usable
// session column and the stdlib no Getsid; AR17C, ADV10-12); a pid with
// no process is an error.
func (OSProcs) Session(pid int) (int, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("getsid: pid %d", pid)
	}
	sid, _, errno := syscall.Syscall(syscall.SYS_GETSID, uintptr(pid), 0, 0)
	if errno != 0 {
		return 0, fmt.Errorf("getsid: pid %d: %w", pid, errno)
	}
	return int(sid), nil
}

// Pids is every pid there is, `ps -A -o pid=`.
func (OSProcs) Pids() ([]int, error) {
	out, err := psOutput("-A", "-o", "pid=")
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// Holders is every pid with path open: /proc/<pid>/fd read where there is
// a /proc (linux), else `lsof -t` (whose exit 1 with no output is none).
func (OSProcs) Holders(path string) ([]int, error) {
	if _, err := os.Stat("/proc/self/fd"); err == nil {
		return procHolders(path)
	}
	out, err := exec.Command("lsof", "-t", "--", path).Output()
	var ee *exec.ExitError
	if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 1 && len(out) == 0) {
		return nil, fmt.Errorf("lsof: %w", err)
	}
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// procHolders reads every /proc/<pid>/fd link for path; a process gone or
// unreadable mid-walk holds nothing.
func procHolders(path string) ([]int, error) {
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		fds, _ := os.ReadDir(filepath.Join("/proc", d.Name(), "fd"))
		for _, fd := range fds {
			if l, err := os.Readlink(filepath.Join("/proc", d.Name(), "fd", fd.Name())); err == nil && l == path {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids, nil
}

// ParsePSTime is ps's cumulative CPU time, `[[dd-]hh:]mm:ss[.cc]`, in
// seconds.
func ParsePSTime(s string) (float64, error) {
	s = strings.TrimSpace(s)
	bad := fmt.Errorf("ps: %q is not a CPU time", capText(s, 40))
	days := 0.0
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, bad
		}
		days, s = float64(n), rest
	}
	parts := strings.Split(s, ":")
	if s == "" || len(parts) > 3 {
		return 0, bad
	}
	total := 0.0
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil || v < 0 {
			return 0, bad
		}
		total = total*60 + v
	}
	return days*86400 + total, nil
}

// ParsePSGroup is the pids of `ps -o pid=,pgid=` lines whose pgid is pgid.
func ParsePSGroup(out string, pgid int) []int {
	var pids []int
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		g, e2 := strconv.Atoi(f[1])
		if e1 == nil && e2 == nil && g == pgid {
			pids = append(pids, pid)
		}
	}
	return pids
}

// ParsePSTree is the pids under root in `ps -o pid=,ppid=` lines,
// breadth first.
func ParsePSTree(out string, root int) []int {
	kids := map[int][]int{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		if e1 == nil && e2 == nil && pid != ppid {
			kids[ppid] = append(kids[ppid], pid)
		}
	}
	var pids []int
	seen := map[int]bool{root: true}
	for queue := []int{root}; len(queue) > 0; queue = queue[1:] {
		for _, k := range kids[queue[0]] {
			if !seen[k] {
				seen[k] = true
				pids = append(pids, k)
				queue = append(queue, k)
			}
		}
	}
	return pids
}

// SweepRounds is how many times Sweep kills a group and reads it again.
const SweepRounds = 3

// SweepPause is the wait between Sweep's kill and its next read (SIGKILL
// lands asynchronously); a variable so a test can shorten it.
var SweepPause = 50 * time.Millisecond

// Sweep is the orphan sweep (§15, D6), run after the Wait on a group's
// leader: it reads the group's members other than the leader; none, it is
// done; else it kills the group and reads again, up to SweepRounds kills.
// orphans is how many the first read found; left is what survived the
// last round (nil: swept). An unreadable group is taken as swept.
func Sweep(p Procs, pgid int) (orphans int, left []int) {
	for round := 0; ; round++ {
		pids, err := p.Group(pgid)
		if err != nil {
			return orphans, nil
		}
		var rest []int
		for _, pid := range pids {
			if pid != pgid {
				rest = append(rest, pid)
			}
		}
		if round == 0 {
			orphans = len(rest)
		}
		if len(rest) == 0 {
			return orphans, nil
		}
		if round == SweepRounds {
			return orphans, rest
		}
		p.KillGroup(pgid)
		time.Sleep(SweepPause)
	}
}

// SweepEscaped is the sweep over the descendants the watchdog saw (ADV6-8),
// each with the start time it had then, run after Sweep: a pid whose start
// time still reads the same is alive and the child's, killed by its own
// group (a descendant that left the child's group leads one: setsid or
// setpgid) and read again, up to SweepRounds kills; one whose start time
// moved or is gone is never touched. found is how many the first read
// found alive; left what outlived the last round.
func SweepEscaped(p Procs, seen map[int]string) (found int, left []int) {
	var pids []int
	for pid := range seen {
		pids = append(pids, pid)
	}
	for round := 0; ; round++ {
		var alive []int
		for _, pid := range pids {
			if s, err := p.Started(pid); err == nil && s == seen[pid] {
				alive = append(alive, pid)
			}
		}
		if round == 0 {
			found = len(alive)
		}
		if len(alive) == 0 {
			return found, nil
		}
		if round == SweepRounds {
			return found, alive
		}
		for _, pid := range alive {
			p.KillGroup(pid)
		}
		pids = alive
		time.Sleep(SweepPause)
	}
}

// SweepSession is the session sweep (AR17C, ADV10-12, D2 §9), run after
// the Wait on a line that leads its own session (Setsid: sid == pgid ==
// the shell's pid) and after Sweep. The rule: a pid is the line's when it
// is absent from before (the snapshot taken before the line started, so
// nothing that existed then is ever touched) and either reads session sid
// (a descendant that only left the group) or holds mark open (the file the
// line's shell got as fd 3, inherited through fork, setsid and exec: the
// daemon that left the session too; a start-time or ppid-1 rule would take
// the owner's own launchd-started apps). Each is killed by its own group
// and the set read again, up to SweepRounds kills. found is how many the
// first read found; left what outlived the last round. An unreadable
// snapshot is taken as swept.
func SweepSession(p Procs, sid int, before map[int]bool, mark string) (found int, left []int) {
	for round := 0; ; round++ {
		pids, err := p.Pids()
		if err != nil {
			return found, nil
		}
		held := map[int]bool{}
		if mark != "" {
			hs, _ := p.Holders(mark)
			for _, h := range hs {
				held[h] = true
			}
		}
		var ours []int
		for _, pid := range pids {
			if pid <= 1 || before[pid] || pid == sid {
				continue
			}
			if s, err := p.Session(pid); (err == nil && s == sid) || held[pid] {
				ours = append(ours, pid)
			}
		}
		if round == 0 {
			found = len(ours)
		}
		if len(ours) == 0 {
			return found, nil
		}
		if round == SweepRounds {
			return found, ours
		}
		for _, pid := range ours {
			p.KillGroup(pid) // a member of another's group lives: left, the line unrun
		}
		time.Sleep(SweepPause)
	}
}
