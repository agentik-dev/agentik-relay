package effects

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// FakeGit answers from its maps, keyed by dir (IsAncestor by "a..b",
// FirstParentMerged by "tip..main" in Merged,
// BranchExists and Tips by name), and records every call as "Method dir args".
// A key in Err (the method name) makes that method fail.
type FakeGit struct {
	Toplevels, Roots, Heads, Branches map[string]string
	Tips                              map[string]string
	Statuses                          map[string][]string
	Ancestors                         map[string]bool
	Merged                            map[string]bool
	Existing                          map[string]bool
	Remote                            map[string]string
	LsRemotes                         map[string]map[string]string // remote name (its URL) to its refs; unset is none
	URLs                              map[string][]string          // remote name to its URLs; unset is the name itself
	Err                               map[string]error

	// The write side: WTs is the worktree list (adds append,
	// removes delete by path), Diffs answers DiffNames by "base...branch",
	// Sha is what Commit returns. Excluded holds each dir's patterns.
	WTs      []Worktree
	Diffs    map[string][]string
	Sha      string
	Excluded map[string][]string
	// Hermetics answers Hermetic by "dir args..." (joined by spaces);
	// unset is "".
	Hermetics map[string]string

	mu    sync.Mutex
	calls []string
}

func (g *FakeGit) record(method string, args ...string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, strings.Join(append([]string{method}, args...), " "))
	return g.Err[method]
}

// Calls returns the calls so far, in order.
func (g *FakeGit) Calls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.calls...)
}

func (g *FakeGit) lookup(m map[string]string, method, dir string) (string, error) {
	if err := g.record(method, dir); err != nil {
		return "", err
	}
	v, ok := m[dir]
	if !ok {
		return "", fmt.Errorf("fake git: %s %s: not set", method, dir)
	}
	return v, nil
}

func (g *FakeGit) Toplevel(dir string) (string, error) { return g.lookup(g.Toplevels, "Toplevel", dir) }
func (g *FakeGit) Root(dir string) (string, error)     { return g.lookup(g.Roots, "Root", dir) }
func (g *FakeGit) Head(dir string) (string, error)     { return g.lookup(g.Heads, "Head", dir) }
func (g *FakeGit) Branch(dir string) (string, error)   { return g.lookup(g.Branches, "Branch", dir) }

func (g *FakeGit) Tip(dir, branch string) (string, error) {
	if err := g.record("Tip", dir, branch); err != nil {
		return "", err
	}
	v, ok := g.Tips[branch]
	if !ok {
		return "", fmt.Errorf("fake git: Tip %s: not set", branch)
	}
	return v, nil
}

func (g *FakeGit) Status(dir string) ([]string, error) {
	if err := g.record("Status", dir); err != nil {
		return nil, err
	}
	return append([]string(nil), g.Statuses[dir]...), nil
}

// RootStatus reads Statuses[dir], as Status does.
func (g *FakeGit) RootStatus(dir string) ([]string, error) {
	if err := g.record("RootStatus", dir); err != nil {
		return nil, err
	}
	return append([]string(nil), g.Statuses[dir]...), nil
}

func (g *FakeGit) IsAncestor(dir, a, b string) (bool, error) {
	if err := g.record("IsAncestor", dir, a, b); err != nil {
		return false, err
	}
	return g.Ancestors[a+".."+b], nil
}

func (g *FakeGit) FirstParentMerged(dir, tip, main string) (bool, error) {
	if err := g.record("FirstParentMerged", dir, tip, main); err != nil {
		return false, err
	}
	return g.Merged[tip+".."+main], nil
}

func (g *FakeGit) BranchExists(dir, name string) (bool, error) {
	if err := g.record("BranchExists", dir, name); err != nil {
		return false, err
	}
	return g.Existing[name], nil
}

func (g *FakeGit) RemoteRefs(dir string) (map[string]string, error) {
	if err := g.record("RemoteRefs", dir); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range g.Remote {
		out[k] = v
	}
	return out, nil
}

// RevParseCommit resolves `refs/heads/<name>^{commit}` (or a bare name)
// through Tips; an unset name is an error, as git's would be.
func (g *FakeGit) RevParseCommit(dir, ref string) (string, error) {
	if err := g.record("RevParseCommit", dir, ref); err != nil {
		return "", err
	}
	name := strings.TrimSuffix(strings.TrimPrefix(ref, "refs/heads/"), "^{commit}")
	v, ok := g.Tips[name]
	if !ok {
		return "", fmt.Errorf("fake git: RevParseCommit %s: not set", ref)
	}
	return v, nil
}

func (g *FakeGit) LsRemote(dir, remote string) (map[string]string, error) {
	if err := g.record("LsRemote", dir, remote); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range g.LsRemotes[remote] {
		out[k] = v
	}
	return out, nil
}

// RemoteURLs is URLs[remote], or the name itself when unset (so
// LsRemotes keyed by name answers the URL).
func (g *FakeGit) RemoteURLs(dir, remote string) ([]string, error) {
	if err := g.record("RemoteURLs", dir, remote); err != nil {
		return nil, err
	}
	if u, ok := g.URLs[remote]; ok {
		return append([]string(nil), u...), nil
	}
	return []string{remote}, nil
}

// Remotes is the remote names in Remote's refs/remotes/<name>/ keys, sorted.
func (g *FakeGit) Remotes(dir string) ([]string, error) {
	if err := g.record("Remotes", dir); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for k := range g.Remote {
		if name, _, ok := strings.Cut(strings.TrimPrefix(k, "refs/remotes/"), "/"); ok && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (g *FakeGit) WorktreeAdd(dir, path, branch, base string) error {
	if err := g.record("WorktreeAdd", dir, path, branch, base); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.WTs = append(g.WTs, Worktree{Path: path, Branch: branch})
	if g.Existing == nil {
		g.Existing = map[string]bool{}
	}
	g.Existing[branch] = true
	return nil
}

func (g *FakeGit) WorktreeAddDetached(dir, path, ref string) error {
	if err := g.record("WorktreeAddDetached", dir, path, ref); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.WTs = append(g.WTs, Worktree{Path: path})
	return nil
}

// Clone records the call, each ref as name@sha; it makes no worktree.
func (g *FakeGit) Clone(dir, path string, refs ...Ref) error {
	args := []string{dir, path}
	for _, r := range refs {
		args = append(args, r.Name+"@"+r.Sha)
	}
	return g.record("Clone", args...)
}

func (g *FakeGit) drop(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	var keep []Worktree
	for _, w := range g.WTs {
		if w.Path != path {
			keep = append(keep, w)
		}
	}
	g.WTs = keep
}

func (g *FakeGit) WorktreeRemove(dir, path string) error {
	if err := g.record("WorktreeRemove", dir, path); err != nil {
		return err
	}
	g.drop(path)
	return nil
}

func (g *FakeGit) WorktreeDiscard(dir, path string) error {
	if err := g.record("WorktreeDiscard", dir, path); err != nil {
		return err
	}
	g.drop(path)
	return nil
}

func (g *FakeGit) Worktrees(dir string) ([]Worktree, error) {
	if err := g.record("Worktrees", dir); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]Worktree(nil), g.WTs...), nil
}

func (g *FakeGit) DiffNames(dir, base, branch string) ([]string, error) {
	if err := g.record("DiffNames", dir, base, branch); err != nil {
		return nil, err
	}
	return append([]string(nil), g.Diffs[base+"..."+branch]...), nil
}

func (g *FakeGit) MergeNoCommit(dir, branch, name, email string) error {
	return g.record("MergeNoCommit", dir, branch)
}

func (g *FakeGit) MergeAbort(dir string) error { return g.record("MergeAbort", dir) }

func (g *FakeGit) Overlay(dir, sha string, paths ...string) error {
	return g.record("Overlay", append([]string{dir, sha}, paths...)...)
}

func (g *FakeGit) Commit(dir, msg, name, email string) (string, error) {
	if err := g.record("Commit", dir); err != nil {
		return "", err
	}
	return g.Sha, nil
}

func (g *FakeGit) Hermetic(dir string, args ...string) (string, error) {
	if err := g.record("Hermetic", append([]string{dir}, args...)...); err != nil {
		return "", err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Hermetics[strings.Join(append([]string{dir}, args...), " ")], nil
}

func (g *FakeGit) InstallPrePush(path string) error { return g.record("InstallPrePush", path) }

func (g *FakeGit) Exclude(dir, pattern string) error {
	if err := g.record("Exclude", dir, pattern); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Excluded == nil {
		g.Excluded = map[string][]string{}
	}
	for _, p := range g.Excluded[dir] {
		if p == pattern {
			return nil
		}
	}
	g.Excluded[dir] = append(g.Excluded[dir], pattern)
	return nil
}

// ShellResult is one canned answer of FakeShell.
type ShellResult struct {
	Exit int
	Last string
	Err  error
}

// FakeShell answers each line from Results (a missing line exits 0 with no
// output) and records "dir: line" for every call.
type FakeShell struct {
	Results map[string]ShellResult

	mu    sync.Mutex
	calls []string
}

func (s *FakeShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, dir+": "+line)
	s.mu.Unlock()
	r := s.Results[line]
	return r.Exit, capText(r.Last, LastCap), r.Err
}

// Calls returns the calls so far, in order.
func (s *FakeShell) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// FakeClock is a clock that reads T.
type FakeClock struct{ T time.Time }

func (c FakeClock) Now() time.Time { return c.T }

// After fires at once for a non-positive wait; else never, since T never
// moves (StepClock is the clock that does).
func (c FakeClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.T
	}
	return ch
}

// FakeFS is a file system of Files (path to bytes) and Dirs.
type FakeFS struct {
	Files map[string][]byte
	Dirs  map[string]bool
	Err   map[string]error // by path: that path's reads and writes fail

	mu    sync.Mutex
	temps int
	locks map[string]*sync.Mutex
}

func (f *FakeFS) init() {
	if f.Files == nil {
		f.Files = map[string][]byte{}
	}
	if f.Dirs == nil {
		f.Dirs = map[string]bool{}
	}
}

func (f *FakeFS) ReadFile(p string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.Err[p]; err != nil {
		return nil, err
	}
	b, ok := f.Files[p]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: p, Err: os.ErrNotExist}
	}
	return append([]byte(nil), b...), nil
}

func (f *FakeFS) WriteFile(p string, data []byte, perm fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.init()
	if err := f.Err[p]; err != nil {
		return err
	}
	f.Files[p] = append([]byte(nil), data...)
	return nil
}

func (f *FakeFS) MkdirAll(p string, perm fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.init()
	if err := f.Err[p]; err != nil {
		return err
	}
	for d := p; d != "/" && d != "." && d != ""; d = path.Dir(d) {
		f.Dirs[d] = true
	}
	return nil
}

func (f *FakeFS) Exists(p string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, file := f.Files[p]
	return file || f.Dirs[p]
}

// Rename moves a file's bytes from one path to another.
func (f *FakeFS) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.init()
	if _, ok := f.Files[path]; !ok {
		return &fs.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}
	delete(f.Files, path)
	return nil
}

func (f *FakeFS) Rename(from, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.init()
	if err := f.Err[to]; err != nil {
		return err
	}
	b, ok := f.Files[from]
	if !ok {
		return &fs.PathError{Op: "rename", Path: from, Err: os.ErrNotExist}
	}
	delete(f.Files, from)
	f.Files[to] = b
	return nil
}

// WriteTemp writes data under a fresh name in dir: pattern's `*` (or its
// end) replaced by a counter.
func (f *FakeFS) WriteTemp(dir, pattern string, data []byte) (string, error) {
	f.mu.Lock()
	f.temps++
	n := fmt.Sprint(f.temps)
	f.mu.Unlock()
	name := pattern + n
	if i := strings.LastIndex(pattern, "*"); i >= 0 {
		name = pattern[:i] + n + pattern[i+1:]
	}
	p := path.Join(dir, name)
	if err := f.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

// Lock is a mutex per path, in this process only.
func (f *FakeFS) Lock(p string) (func(), error) {
	f.mu.Lock()
	if err := f.Err[p]; err != nil {
		f.mu.Unlock()
		return nil, err
	}
	if f.locks == nil {
		f.locks = map[string]*sync.Mutex{}
	}
	mu := f.locks[p]
	if mu == nil {
		mu = &sync.Mutex{}
		f.locks[p] = mu
	}
	f.mu.Unlock()
	mu.Lock()
	return mu.Unlock, nil
}

func (f *FakeFS) List(dir string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := map[string]bool{}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	add := func(p string) {
		if rest, ok := strings.CutPrefix(p, prefix); ok && rest != "" {
			seen[strings.SplitN(rest, "/", 2)[0]] = true
		}
	}
	for p := range f.Files {
		add(p)
	}
	for p := range f.Dirs {
		add(p)
	}
	if len(seen) == 0 && !f.Dirs[dir] {
		return nil, &fs.PathError{Op: "open", Path: dir, Err: os.ErrNotExist}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// FakeLedger keeps its lines in memory, each one round-tripped through JSON
// as the real one writes it. Prior holds lines of earlier nights. Err fails
// every append, or with FailOn set only that event's.
type FakeLedger struct {
	Clock  Clock
	Night  string
	Prior  []map[string]any
	Err    error
	FailOn string

	mu    sync.Mutex
	lines []map[string]any
}

func (l *FakeLedger) Append(event string, fields map[string]any) error {
	if l.Err != nil && (l.FailOn == "" || l.FailOn == event) {
		return l.Err
	}
	b, err := encodeLine(l.Clock.Now(), l.Night, event, fields)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	l.mu.Lock()
	l.lines = append(l.lines, m)
	l.mu.Unlock()
	return nil
}

func (l *FakeLedger) Lines() ([]map[string]any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append(append([]map[string]any(nil), l.Prior...), l.lines...), nil
}

// Written returns only the lines appended through this ledger.
func (l *FakeLedger) Written() []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]map[string]any(nil), l.lines...)
}

// FakeSpawner hands every Start the canned Out as the stream and records
// each Spawn and each signal; its processes exit with Exit. Outs, when
// set, gives the n-th Start Outs[n]'s stream and exit instead (AR67): past
// the last, Out and Exit. Err makes Start fail.
type FakeSpawner struct {
	Out  []byte
	Exit int
	Outs []FakeRun
	Err  error
	PID  int // every process's Pid

	mu      sync.Mutex
	spawns  []Spawn
	signals []os.Signal
}

func (s *FakeSpawner) Start(sp Spawn) (Process, io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.spawns)
	s.spawns = append(s.spawns, sp)
	if s.Err != nil {
		return nil, nil, s.Err
	}
	if n < len(s.Outs) {
		r := s.Outs[n]
		return fakeProcess{s, &r}, io.NopCloser(bytes.NewReader(r.Out)), nil
	}
	return fakeProcess{s, nil}, io.NopCloser(bytes.NewReader(s.Out)), nil
}

// FakeRun is one Start's canned stream and exit (AR67).
type FakeRun struct {
	Out  []byte
	Exit int
}

// Spawns returns the recorded spawns in order.
func (s *FakeSpawner) Spawns() []Spawn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Spawn(nil), s.spawns...)
}

// Signals returns the signals sent to its processes in order.
func (s *FakeSpawner) Signals() []os.Signal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]os.Signal(nil), s.signals...)
}

// fakeProcess exits with its run's Exit, else the spawner's.
type fakeProcess struct {
	s   *FakeSpawner
	run *FakeRun
}

func (p fakeProcess) Signal(sig os.Signal) error {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	p.s.signals = append(p.s.signals, sig)
	return nil
}

func (p fakeProcess) Wait() (int, error) {
	if p.run != nil {
		return p.run.Exit, nil
	}
	return p.s.Exit, nil
}
func (p fakeProcess) Pid() int { return p.s.PID }

// FakeTmux records every call as "Method args" and reports Alive false once
// it has answered AliveAfter Alive calls (0: always alive). List is what
// Sessions answers (AR33 act 2), with ListErr its error; Kill fails with
// KillErr and else drops the session from List.
type FakeTmux struct {
	IsPresent  bool
	AliveAfter int
	Calls      []string
	List       []string
	ListErr    error
	KillErr    error

	mu     sync.Mutex
	alives int
}

func (t *FakeTmux) record(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Calls = append(t.Calls, s)
}

func (t *FakeTmux) Present() bool { return t.IsPresent }

func (t *FakeTmux) NewSession(name, dir string, argv []string) error {
	t.record("NewSession " + name + " " + dir + " " + strings.Join(argv, " "))
	return nil
}

func (t *FakeTmux) NewWindow(session, window, dir string, argv []string) error {
	t.record("NewWindow " + session + " " + window + " " + dir + " " + strings.Join(argv, " "))
	return nil
}

func (t *FakeTmux) Alive(session string) (bool, error) {
	t.record("Alive " + session)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.alives++
	return t.AliveAfter == 0 || t.alives <= t.AliveAfter, nil
}

func (t *FakeTmux) AttachArgv(session, window string) []string {
	return AttachArgv("tmux", "", session, window)
}

func (t *FakeTmux) Sessions() ([]string, error) {
	t.record("Sessions")
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ListErr != nil {
		return nil, t.ListErr
	}
	return append([]string(nil), t.List...), nil
}

func (t *FakeTmux) Kill(session string) error {
	t.record("Kill " + session)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.KillErr != nil {
		return t.KillErr
	}
	t.List = slices.DeleteFunc(t.List, func(s string) bool { return s == session })
	return nil
}

// Recorded returns the calls in order.
func (t *FakeTmux) Recorded() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.Calls...)
}

// FakeNotify keeps every notice sent.
type FakeNotify struct {
	Sent []Notice
	mu   sync.Mutex
}

func (f *FakeNotify) Send(n Notice) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sent = append(f.Sent, n)
	return nil
}

// Notices returns the notices sent in order.
func (f *FakeNotify) Notices() []Notice {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Notice(nil), f.Sent...)
}

// FakeLoad answers each Load with the next of Series, then its last for
// ever, over Cores (16 when unset); Err makes every Load fail.
type FakeLoad struct {
	Series []float64
	Cores  int
	Err    error

	mu sync.Mutex
	n  int
}

func (l *FakeLoad) Load() (float64, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cores := l.Cores
	if cores == 0 {
		cores = 16
	}
	if l.Err != nil {
		return 0, cores, l.Err
	}
	l.n++
	if len(l.Series) == 0 {
		return 0, cores, nil
	}
	return l.Series[min(l.n, len(l.Series))-1], cores, nil
}

// Reads is how many times Load was called.
func (l *FakeLoad) Reads() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n
}

// FakeProcs answers Newest with Mtime, CPUSeconds with the next of CPU (then
// its last for ever), Group with Groups[pgid] (else just pgid), and records
// every KillGroup, calling OnKill; Started answers from Starts (a pid not
// there is gone; with Mortal, a pid KillGroup reached is gone too),
// Descendants from Desc; Session from Sessions (else the pid itself), Pids
// from All (with Mortal, less every pid KillGroup reached) and Holders
// from Held[path] (AR17C).
type FakeProcs struct {
	Mtime  time.Time
	CPU    []float64
	Groups map[int][]int
	Starts map[int]string
	Desc   map[int][]int
	Mortal bool

	Sessions map[int]int
	All      []int
	Held     map[string][]int
	Comms    map[int]string
	Exes     map[int]string

	OnKill func(pgid int)

	mu    sync.Mutex
	reads int
	kills []int
}

func (p *FakeProcs) Newest(string) (time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Mtime, nil
}

func (p *FakeProcs) CPUSeconds(int) (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	if len(p.CPU) == 0 {
		return 0, nil
	}
	return p.CPU[min(p.reads, len(p.CPU))-1], nil
}

func (p *FakeProcs) Group(pgid int) ([]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if g, ok := p.Groups[pgid]; ok {
		return append([]int(nil), g...), nil
	}
	return []int{pgid}, nil
}

func (p *FakeProcs) KillGroup(pgid int) error {
	p.mu.Lock()
	p.kills = append(p.kills, pgid)
	if p.Mortal {
		delete(p.Starts, pgid)
	}
	f := p.OnKill
	p.mu.Unlock()
	if f != nil {
		f(pgid)
	}
	return nil
}

func (p *FakeProcs) Started(pid int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.Starts[pid]; ok {
		return s, nil
	}
	return "", fmt.Errorf("ps: pid %d: no such process", pid)
}

func (p *FakeProcs) Descendants(pid int) ([]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.Desc[pid]...), nil
}

func (p *FakeProcs) Session(pid int) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.Sessions[pid]; ok {
		return s, nil
	}
	return pid, nil
}

func (p *FakeProcs) Pids() ([]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []int
	for _, pid := range p.All {
		if !p.Mortal || !slices.Contains(p.kills, pid) {
			out = append(out, pid)
		}
	}
	return out, nil
}

// Comm is Comms[pid]; a pid absent there, or killed under Mortal, is no
// process.
func (p *FakeProcs) Comm(pid int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.Comms[pid]; ok && !(p.Mortal && slices.Contains(p.kills, pid)) {
		return c, nil
	}
	return "", fmt.Errorf("ps: pid %d: no such process", pid)
}

// Exe is Exes[pid]; a pid absent there, or killed under Mortal, has none.
func (p *FakeProcs) Exe(pid int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.Exes[pid]; ok && !(p.Mortal && slices.Contains(p.kills, pid)) {
		return e, nil
	}
	return "", fmt.Errorf("lsof: pid %d: no executable", pid)
}

func (p *FakeProcs) Holders(path string) ([]int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.Held[path]...), nil
}

// Kills is every group KillGroup was sent, in order.
func (p *FakeProcs) Kills() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.kills...)
}

// Reads is how many CPU readings were taken.
func (p *FakeProcs) Reads() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads
}
