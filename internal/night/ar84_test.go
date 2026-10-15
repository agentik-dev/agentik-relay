package night_test

import (
	"bytes"
	"context"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

// AR84: the package's tests run as the shards of testdata/shards.txt, one
// go test process a line, all at once (the Makefile's test rule; relay-r1
// §11). Each shard is its own process with its own TestMain (state, HOME,
// pin, lock port), so no test changes.

// shardWall is the slowest shard's bound (the packet's 300 s).
const shardWall = 300 * time.Second

// shardPatterns reads testdata/shards.txt: one -run pattern a line, the
// file ending in a newline, each line starting ^Test with no / or blank,
// each compiling, two lines at least.
func shardPatterns(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "shards.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		t.Fatal("testdata/shards.txt: want each line ending in a newline")
	}
	var pats []string
	for i, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if !strings.HasPrefix(l, "^Test") || strings.Contains(l, "/") || strings.IndexFunc(l, unicode.IsSpace) >= 0 {
			t.Fatalf("testdata/shards.txt:%d: %q: want ^Test first, no / and no blank", i+1, l)
		}
		if _, err := regexp.Compile(l); err != nil {
			t.Fatalf("testdata/shards.txt:%d: %v", i+1, err)
		}
		pats = append(pats, l)
	}
	if len(pats) < 2 {
		t.Fatalf("testdata/shards.txt: %d lines, want two at least", len(pats))
	}
	return pats
}

// testNames lists every top-level func TestX(t *testing.T) in the
// package's *_test.go, as go test reads a test's name: Test, then no
// lower-case letter.
func testNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("*_test.go: %d files, %v", len(files), err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Type.TypeParams != nil || !isTestName(fd.Name.Name) {
				continue
			}
			if ps := fd.Type.Params.List; len(ps) == 1 && len(ps[0].Names) <= 1 && isTestingT(ps[0].Type) {
				names = append(names, fd.Name.Name)
			}
		}
	}
	return names
}

func isTestName(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name[len("Test"):])
	return len(name) == len("Test") || !unicode.IsLower(r)
}

func isTestingT(e ast.Expr) bool {
	st, ok := e.(*ast.StarExpr)
	if !ok {
		return false
	}
	se, ok := st.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := se.X.(*ast.Ident)
	return ok && pkg.Name == "testing" && se.Sel.Name == "T"
}

// shardsOf names the shards (1-based) whose pattern matches name, as
// go test -run matches a top-level name (a pattern with no /).
func shardsOf(res []*regexp.Regexp, name string) []int {
	var in []int
	for i, re := range res {
		if re.MatchString(name) {
			in = append(in, i+1)
		}
	}
	return in
}

// TestAR84_1_ShardsPartition (AR84 ruling 5): every test of the package,
// and a planted name for each shape a new packet's test may take, falls in
// exactly one shard.
func TestAR84_1_ShardsPartition(t *testing.T) {
	pats, names := shardPatterns(t), testNames(t)
	if len(names) < 300 {
		t.Fatalf("%d tests in *_test.go, want 300 at least", len(names))
	}
	res := make([]*regexp.Regexp, len(pats))
	for i, p := range pats {
		res[i] = regexp.MustCompile(p)
	}
	planted := []string{"TestAR99_1_X", "TestAR100_1_X", "TestAR7Fix_X", "TestZed", "TestAgentikR2_1_X"}
	for _, n := range append(append([]string{}, names...), planted...) {
		if in := shardsOf(res, n); len(in) != 1 {
			t.Errorf("%s: in shards %v of testdata/shards.txt, want exactly one", n, in)
		}
	}
}

// topResult reads a -test.v run's top-level result lines: a subtest's are
// indented, a top-level test's start the line.
var topResult = regexp.MustCompile(`(?m)^--- (PASS|SKIP): (Test\S*) \(`)

// failExcerpt is a failed shard's output from its first race, panic or
// failure on (a race's head names the memory raced on), 12 KB at most,
// else its last 4 KB.
func failExcerpt(out string) string {
	from := -1
	for _, m := range []string{"WARNING: DATA RACE", "panic: ", "--- FAIL: "} {
		if i := strings.Index(out, m); i >= 0 && (from < 0 || i < from) {
			from = i
		}
	}
	if from < 0 {
		return "..." + out[max(0, len(out)-4096):]
	}
	return out[from:min(len(out), from+12288)]
}

// TestAR84_2_ShardWalls (AR84 ruling 5): the shards of testdata/shards.txt,
// each its own -race test process, all at once, each exits 0, between them
// they pass or skip every test once, and the slowest ends under 300 s. It
// runs only when named alone, so no shard and no plain go test starts it.
func TestAR84_2_ShardWalls(t *testing.T) {
	if f := flag.Lookup("test.run"); f == nil || f.Value.String() != "^TestAR84_2_" {
		t.Skip("runs only under -run '^TestAR84_2_'")
	}
	pats, names := shardPatterns(t), testNames(t)
	pkg, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "night.test")
	if out, err := exec.Command("go", "test", "-c", "-race", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go test -c -race: %v\n%s", err, out)
	}
	// Each shard picks its own lock port (TestMain), never this binary's.
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, lockPortEnv+"=") {
			env = append(env, e)
		}
	}
	type shard struct {
		cmd  *exec.Cmd
		out  bytes.Buffer
		err  error
		wall time.Duration
		done chan struct{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), shardWall)
	defer cancel()
	start := time.Now()
	shards := make([]*shard, len(pats))
	for i, p := range pats {
		s := &shard{done: make(chan struct{})}
		s.cmd = exec.CommandContext(ctx, bin, "-test.run", p, "-test.count=1", "-test.v", "-test.timeout", "15m")
		s.cmd.Dir, s.cmd.Env, s.cmd.Stdout, s.cmd.Stderr = pkg, env, &s.out, &s.out
		s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		s.cmd.Cancel = func() error { return syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL) }
		s.cmd.WaitDelay = 10 * time.Second
		if err := s.cmd.Start(); err != nil {
			t.Fatalf("shard %d: %v", i+1, err)
		}
		go func() { s.err = s.cmd.Wait(); s.wall = time.Since(start); close(s.done) }()
		shards[i] = s
	}
	seen := map[string]int{}
	var slowest time.Duration
	for i, s := range shards {
		<-s.done
		ms := topResult.FindAllStringSubmatch(s.out.String(), -1)
		for _, m := range ms {
			seen[m[2]]++
		}
		slowest = max(slowest, s.wall)
		var user, sys time.Duration
		if ps := s.cmd.ProcessState; ps != nil {
			user, sys = ps.UserTime(), ps.SystemTime()
		}
		t.Logf("shard %d %s: %d tests, %s (user %s, sys %s)", i+1, pats[i], len(ms),
			s.wall.Round(time.Second), user.Round(time.Second), sys.Round(time.Second))
		if s.err != nil {
			t.Errorf("shard %d %s: %v after %s\n%s", i+1, pats[i], s.err, s.wall.Round(time.Second), failExcerpt(s.out.String()))
		}
	}
	t.Logf("slowest %s, %d tests over %d shards", slowest.Round(time.Second), len(names), len(pats))
	var missing []string
	for _, n := range names {
		if seen[n] != 1 {
			missing = append(missing, n)
		}
		delete(seen, n)
	}
	if len(missing) > 0 || len(seen) > 0 {
		t.Errorf("passed or skipped other than once: %d of %d tests (%s); not in *_test.go: %v",
			len(missing), len(names), strings.Join(missing[:min(len(missing), 20)], " "), seen)
	}
	if slowest >= shardWall {
		t.Errorf("slowest shard %s, want under %s", slowest.Round(time.Second), shardWall)
	}
}
