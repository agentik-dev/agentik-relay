package night

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ar48Run is one run of a copy of scripts/bounded above its load line: the
// exit code, stderr, whether the command ran and whether anything ran `top`.
type ar48Run struct {
	code   int
	stderr string
	ran    bool
	top    bool
}

// ar48Exit is the command's own exit in every AR48 run: no code of bounded's.
const ar48Exit = 7

// ar48Stop is how each stopping run is cut short: the wait ends after half
// a second, polled every tenth.
var ar48Stop = []string{"BOUNDED_LOAD_MAX=0.5", "BOUNDED_LOAD_POLL=0.1"}

// ar48Quick cuts a table's stopping runs shorter still: two polls.
var ar48Quick = []string{"BOUNDED_LOAD_MAX=0.2", "BOUNDED_LOAD_POLL=0.1"}

// ar48FakeTop writes a fake `top` into a fresh directory and returns it:
// the fake writes its arguments to $AR48_TOP_MARKER, then runs body.
func ar48FakeTop(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > \"$AR48_TOP_MARKER\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, "top"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// ar48Env is the caller's environment without PATH and without any
// BOUNDED_ name, then PATH as given and the names in env.
func ar48Env(path string, env ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "BOUNDED_") && !strings.HasPrefix(kv, "PATH=") {
			out = append(out, kv)
		}
	}
	return append(append(out, "PATH="+path), env...)
}

// ar48Bounded runs a copy of scripts/bounded as `--load 4 -- sh -c ...`
// under a 10 s bound. The load is always fake and far above any machine's
// line (BOUNDED_LOADAVG=100000), and a fake `top` that leaves a marker is
// first on PATH: no AR48 run reads the real load or runs the real `top`.
// env adds to that.
func ar48Bounded(t *testing.T, py string, env ...string) ar48Run {
	t.Helper()
	script := filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")
	dir := t.TempDir()
	ran, top := filepath.Join(dir, "ran"), filepath.Join(dir, "top-ran")
	bin := ar48FakeTop(t, "echo 'CPU usage: 1.00% user, 1.00% sys, 98.00% idle'")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, py, "-B", script, "--load", "4", "--",
		"sh", "-c", fmt.Sprintf(`echo ran > "$1"; exit %d`, ar48Exit), "sh", ran)
	cmd.Env = ar48Env(bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		append([]string{"AR48_TOP_MARKER=" + top, "BOUNDED_LOADAVG=100000"}, env...)...)
	cmd.Stderr = &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	out := ar48Run{stderr: stderr.String()}
	switch {
	case err == nil:
	case errors.As(err, &ee) && ctx.Err() == nil:
		out.code = ee.ExitCode()
	default:
		t.Fatalf("bounded %v: %v (%s)", env, err, stderr.String())
	}
	_, ranErr := os.Stat(ran)
	_, topErr := os.Stat(top)
	out.ran, out.top = ranErr == nil, topErr == nil
	return out
}

// ar48Lines is stderr's lines, without the last newline's empty one.
func ar48Lines(stderr string) []string {
	return strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
}

// ar48Ran fails unless the run let the command through: the command ran,
// the exit is the command's own, and stderr has no `stop:` line.
func ar48Ran(t *testing.T, what string, r ar48Run) {
	t.Helper()
	if !r.ran || r.code != ar48Exit || strings.Contains(r.stderr, "stop:") {
		t.Errorf("%s: ran %v, exit %d, stderr %q; want the command run, its exit %d and no stop line", what, r.ran, r.code, r.stderr, ar48Exit)
	}
	if r.top {
		t.Errorf("%s: something ran `top`", what)
	}
}

// ar48Stopped fails unless the run waited and stopped `load`: the command
// did not run, the exit is 2 and the last stderr line is the stop line.
func ar48Stopped(t *testing.T, what string, r ar48Run) {
	t.Helper()
	lines := ar48Lines(r.stderr)
	if r.ran || r.code != 2 || lines[len(lines)-1] != "bounded: stop: load" {
		t.Errorf("%s: ran %v, exit %d, stderr %q; want the command unrun, exit 2 and the last line `bounded: stop: load`", what, r.ran, r.code, r.stderr)
	}
	if r.top {
		t.Errorf("%s: something ran `top`", what)
	}
}

// TestAR48_1_HighIdleRuns (AR48 ruling 1): above the load line, an idle
// share above the floor runs the command at once behind one line that names
// the load and the idle share; one at or under the floor waits, and the
// waiting line names both.
func TestAR48_1_HighIdleRuns(t *testing.T) {
	py := boundedPython(t)

	high := ar48Bounded(t, py, "BOUNDED_IDLE=70")
	ar48Ran(t, "idle 70", high)
	lines := ar48Lines(high.stderr)
	if len(lines) != 1 || !strings.Contains(lines[0], "load 100000.00") || !strings.Contains(lines[0], "idle 70%") {
		t.Errorf("idle 70: stderr %q; want one line that names the load 100000.00 and the idle share 70%%", high.stderr)
	}

	low := ar48Bounded(t, py, append([]string{"BOUNDED_IDLE=10"}, ar48Stop...)...)
	ar48Stopped(t, "idle 10", low)
	waiting := ar48Lines(low.stderr)[0]
	if !strings.Contains(waiting, "load 100000.00") || !strings.Contains(waiting, "idle 10%") || !strings.Contains(waiting, "waiting up to") {
		t.Errorf("idle 10: the waiting line is %q; want it to name the load 100000.00 and the idle share 10%%", waiting)
	}
}

// TestAR48_2_TwoPollsClose (AR48 ruling 2): while it waits, two polls in a
// row with the idle share above the floor end the wait and run the command;
// one such poll does not.
func TestAR48_2_TwoPollsClose(t *testing.T) {
	py := boundedPython(t)

	two := ar48Bounded(t, py, "BOUNDED_IDLE=10,60,60", "BOUNDED_LOAD_POLL=0.1")
	ar48Ran(t, "idle 10,60,60", two)
	if lines := ar48Lines(two.stderr); len(lines) != 2 || !strings.Contains(lines[0], "idle 10%") || !strings.Contains(lines[1], "idle 60%") {
		t.Errorf("idle 10,60,60: stderr %q; want the waiting line at idle 10%%, then the line that ends the wait at idle 60%%", two.stderr)
	}

	one := ar48Bounded(t, py, append([]string{"BOUNDED_IDLE=10,60,10"}, ar48Stop...)...)
	ar48Stopped(t, "idle 10,60,10", one)
}

// ar48Today is stderr of a load wait that ran out before AR48, whole: the
// waiting line, the refusal and the stop line, with no idle share in them.
var ar48Today = regexp.MustCompile(`\Abounded: load 100000\.00 above [0-9]+\.[0-9]{2}; waiting up to [0-9]+ s\n` +
	`bounded: load 100000\.00 above [0-9]+\.[0-9]{2} \(4 x [0-9]+ cores\) for [0-9]+ s\n` +
	`bounded: stop: load\n\z`)

// TestAR48_3_UnreadableAsToday (AR48 ruling 3): with no idle share to read,
// the load alone decides and stderr is the old one. A faked load with no
// BOUNDED_IDLE reads nothing, so `top` never runs; a BOUNDED_IDLE that is
// no number, a NaN or outside 0 to 100 is no reading either. The control
// is the same run with a share that reads: it runs the command.
func TestAR48_3_UnreadableAsToday(t *testing.T) {
	py := boundedPython(t)

	for _, c := range []struct {
		what string
		env  []string
	}{
		{"BOUNDED_IDLE unset", nil},
		{"BOUNDED_IDLE empty", []string{"BOUNDED_IDLE="}},
		{"BOUNDED_IDLE=nan", []string{"BOUNDED_IDLE=nan"}},
		{"BOUNDED_IDLE=x", []string{"BOUNDED_IDLE=x"}},
		{"BOUNDED_IDLE=101", []string{"BOUNDED_IDLE=101"}},
		{"BOUNDED_IDLE=-70", []string{"BOUNDED_IDLE=-70"}},
		{"BOUNDED_IDLE=inf", []string{"BOUNDED_IDLE=inf"}},
	} {
		r := ar48Bounded(t, py, append(c.env, ar48Quick...)...)
		ar48Stopped(t, c.what, r)
		if !ar48Today.MatchString(r.stderr) {
			t.Errorf("%s: stderr %q; want the lines of a load wait before the idle key, and no others", c.what, r.stderr)
		}
	}

	ar48Ran(t, "the control, BOUNDED_IDLE=70", ar48Bounded(t, py, "BOUNDED_IDLE=70"))
}

// ar48Driver loads scripts/bounded as a module (it keeps its main guard, and
// has no .py suffix, so by its path) and calls the functions named on stdin,
// `[[name, [args]], ...]`: their results on stdout, a None as null.
const ar48Driver = `
import importlib.machinery, importlib.util, json, sys
loader = importlib.machinery.SourceFileLoader("bounded", sys.argv[1])
mod = importlib.util.module_from_spec(importlib.util.spec_from_loader("bounded", loader))
loader.exec_module(mod)
json.dump([getattr(mod, name)(*args) for name, args in json.load(sys.stdin)], sys.stdout)
`

// ar48Signal loads scripts/bounded the same way and runs its main as
// `bounded --load 4 -- true` with a 1-minute load no machine has, read from
// a patched os.getloadavg and never from the machine.
const ar48Signal = `
import importlib.machinery, importlib.util, os, sys
loader = importlib.machinery.SourceFileLoader("bounded", sys.argv[1])
mod = importlib.util.module_from_spec(importlib.util.spec_from_loader("bounded", loader))
loader.exec_module(mod)
os.getloadavg = lambda: (100000.0, 100000.0, 100000.0)
sys.exit(mod.main(["bounded", "--load", "4", "--", "true"]))
`

// ar48Call is one call through ar48Driver.
type ar48Call struct {
	what string
	name string
	args []string
	want float64 // the share in percent; NaN for unreadable
}

// ar48Calls runs the calls through ar48Driver on a copy of scripts/bounded,
// with path as the whole PATH, and compares each result.
func ar48Calls(t *testing.T, py, path string, env []string, calls []ar48Call) {
	t.Helper()
	script := filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")
	in := make([][]any, len(calls))
	for i, c := range calls {
		args := c.args
		if args == nil {
			args = []string{}
		}
		in[i] = []any{c.name, args}
	}
	stdin, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, py, "-B", "-c", ar48Driver, script)
	cmd.Env = ar48Env(path, env...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("the driver: %v (%s)", err, strings.TrimSpace(stderr.String()))
	}
	var got []*float64
	if err := json.Unmarshal(stdout, &got); err != nil || len(got) != len(calls) {
		t.Fatalf("the driver wrote %q for %d calls: %v", stdout, len(calls), err)
	}
	for i, c := range calls {
		switch {
		case math.IsNaN(c.want) && got[i] != nil:
			t.Errorf("%s: %s read %v; want it unreadable", c.what, c.name, *got[i])
		case !math.IsNaN(c.want) && got[i] == nil:
			t.Errorf("%s: %s was unreadable; want %v", c.what, c.name, c.want)
		case !math.IsNaN(c.want) && math.Abs(*got[i]-c.want) > 1e-9:
			t.Errorf("%s: %s read %v; want %v", c.what, c.name, *got[i], c.want)
		}
	}
}

// TestAR48_4_ParseIdle (AR48 ruling 4): the two pure parsers. Two `CPU
// usage:` lines give the second line's share; text with no `% idle`, a NaN,
// a comma decimal or a value outside 0 to 100 is unreadable, never misread;
// two /proc/stat reads give the idle share of their difference. On darwin
// the reader runs `top -l 2 -n 0 -s 1` (a fake, the only `top` on its
// PATH), and a TERM during that read ends bounded with 143 and no traceback.
func TestAR48_4_ParseIdle(t *testing.T) {
	py := boundedPython(t)
	unreadable := math.NaN()
	top := func(lines ...string) []string {
		return []string{"Processes: 612 total, 3 running, 609 sleeping, 4014 threads\nLoad Avg: 72.34, 61.02, 48.77\n" + strings.Join(lines, "\nSharedLibs: 812M resident, 151M data, 96M linkedit.\n\nProcesses: 612 total\n") + "\nSharedLibs: 812M resident\n"}
	}
	stat := func(before, after string) []string {
		return []string{before + "\ncpu0 1 2 3 4 5 6 7 8 9 10\nintr 99\n", after + "\ncpu0 2 3 4 5 6 7 8 9 10 11\nintr 100\n"}
	}

	t.Run("the parsers", func(t *testing.T) {
		ar48Calls(t, py, os.Getenv("PATH"), nil, []ar48Call{
			{"two CPU usage lines", "parse_top_idle", top("CPU usage: 3.10% user, 4.90% sys, 92.0% idle", "CPU usage: 12.50% user, 15.25% sys, 72.25% idle"), 72.25},
			{"a whole number", "parse_top_idle", top("CPU usage: 50% user, 50% sys, 0% idle"), 0},
			{"all idle", "parse_top_idle", top("CPU usage: 0.0% user, 0.0% sys, 100.0% idle"), 100},
			{"no % idle", "parse_top_idle", top("CPU usage: 12.50% user, 15.25% sys"), unreadable},
			{"no % idle on the last line", "parse_top_idle", top("CPU usage: 3.10% user, 4.90% sys, 92.0% idle", "CPU usage: 12.50% user, 15.25% sys"), unreadable},
			{"a NaN", "parse_top_idle", top("CPU usage: 12.50% user, 15.25% sys, nan% idle"), unreadable},
			{"an inf", "parse_top_idle", top("CPU usage: 12.50% user, 15.25% sys, inf% idle"), unreadable},
			{"a comma decimal", "parse_top_idle", top("CPU usage: 12,50% user, 15,25% sys, 72,25% idle"), unreadable},
			{"above 100", "parse_top_idle", top("CPU usage: 12.50% user, 15.25% sys, 172.25% idle"), unreadable},
			{"below 0", "parse_top_idle", top("CPU usage: 12.50% user, 15.25% sys, -72.25% idle"), unreadable},
			{"an exponent", "parse_top_idle", top("CPU usage: 12.50% user, 15.25% sys, 7e1% idle"), unreadable},
			{"no CPU usage line", "parse_top_idle", []string{"Processes: 612 total\nLoad Avg: 72.34, 61.02, 48.77\n"}, unreadable},
			{"no text", "parse_top_idle", []string{""}, unreadable},

			{"two /proc/stat reads", "parse_stat_idle", stat("cpu  100 0 100 700 100 0 0 0 0 0", "cpu  150 0 150 1300 200 0 0 0 0 0"), 87.5},
			{"no idle time passed", "parse_stat_idle", stat("cpu  100 0 100 700 100 0 0 0", "cpu  200 0 200 700 100 0 0 0"), 0},
			{"no time passed", "parse_stat_idle", stat("cpu  100 0 100 700 100 0 0 0", "cpu  100 0 100 700 100 0 0 0"), unreadable},
			{"time ran back", "parse_stat_idle", stat("cpu  150 0 150 1300 200 0 0 0", "cpu  100 0 100 700 100 0 0 0"), unreadable},
			{"a time that is no count", "parse_stat_idle", stat("cpu  100 0 100 700 100 0 0 0", "cpu  150 0 150 nan 200 0 0 0"), unreadable},
			{"no cpu line", "parse_stat_idle", []string{"intr 99\n", "intr 100\n"}, unreadable},
			{"a short cpu line", "parse_stat_idle", []string{"cpu 1 2 3\n", "cpu 2 3 4\n"}, unreadable},
		})
	})

	t.Run("the reader runs top", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("the reader runs `top` on darwin only")
		}
		marker := filepath.Join(t.TempDir(), "top-args")
		bin := ar48FakeTop(t, "echo 'CPU usage: 3.10% user, 4.90% sys, 92.0% idle'\necho 'CPU usage: 12.50% user, 15.25% sys, 72.25% idle'")
		ar48Calls(t, py, bin, []string{"AR48_TOP_MARKER=" + marker}, []ar48Call{{"the fake top", "os_idle", nil, 72.25}})
		if args, _ := os.ReadFile(marker); strings.TrimSpace(string(args)) != "-l 2 -n 0 -s 1" {
			t.Errorf("the reader ran `top %s`; want `top -l 2 -n 0 -s 1`", strings.TrimSpace(string(args)))
		}
		noisy := ar48FakeTop(t, "echo 'CPU usage: 12.50% user, 15.25% sys, 72.25% idle'\nexit 1")
		ar48Calls(t, py, noisy, []string{"AR48_TOP_MARKER=" + marker}, []ar48Call{{"a top that fails", "os_idle", nil, unreadable}})
		ar48Calls(t, py, t.TempDir(), nil, []ar48Call{{"no top on PATH", "os_idle", nil, unreadable}})
	})

	t.Run("a TERM during the read", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("the reader runs `top` on darwin only")
		}
		script := filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")
		marker := filepath.Join(t.TempDir(), "top-args")
		bin := ar48FakeTop(t, "exec /bin/sleep 2")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, py, "-B", "-c", ar48Signal, script)
		cmd.Env = ar48Env(bin, "AR48_TOP_MARKER="+marker)
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		for start := time.Now(); time.Since(start) < 5*time.Second; time.Sleep(20 * time.Millisecond) {
			if _, err := os.Stat(marker); err == nil {
				break
			}
		}
		if _, err := os.Stat(marker); err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			t.Fatalf("bounded never ran `top` (%s)", strings.TrimSpace(stderr.String()))
		}
		cmd.Process.Signal(syscall.SIGTERM)
		err := cmd.Wait()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 128+int(syscall.SIGTERM) || stderr.Len() != 0 {
			t.Errorf("a TERM during the idle read: %v, stderr %q; want exit 143 and nothing on stderr", err, stderr.String())
		}
	})
}

// TestAR48_5_FloorMatchesRelay (AR48 ruling 5): the script's default floor
// is the relay's DefaultIdleFloor, and a reading at the floor counts as
// under it. BOUNDED_IDLE_FLOOR moves it; a value that is no number in 0 to
// 100 leaves the default.
func TestAR48_5_FloorMatchesRelay(t *testing.T) {
	py := boundedPython(t)
	floor := float64(DefaultIdleFloor * 100)
	at, above := fmt.Sprintf("BOUNDED_IDLE=%g", floor), fmt.Sprintf("BOUNDED_IDLE=%g", floor+1)
	named := fmt.Sprintf("floor %g%%", floor)

	r := ar48Bounded(t, py, append([]string{at}, ar48Stop...)...)
	ar48Stopped(t, "idle at the floor", r)
	if waiting := ar48Lines(r.stderr)[0]; !strings.Contains(waiting, named) {
		t.Errorf("idle at the floor: the waiting line is %q; want it to name the %s", waiting, named)
	}
	r = ar48Bounded(t, py, above)
	ar48Ran(t, "idle one point above the floor", r)
	if !strings.Contains(r.stderr, named) {
		t.Errorf("idle one point above the floor: stderr %q; want it to name the %s", r.stderr, named)
	}

	r = ar48Bounded(t, py, append([]string{"BOUNDED_IDLE=70", "BOUNDED_IDLE_FLOOR=80"}, ar48Stop...)...)
	ar48Stopped(t, "idle 70 under a floor of 80", r)
	if waiting := ar48Lines(r.stderr)[0]; !strings.Contains(waiting, "floor 80%") {
		t.Errorf("idle 70 under a floor of 80: the waiting line is %q; want it to name the floor 80%%", waiting)
	}
	ar48Ran(t, "idle 10 above a floor of 5", ar48Bounded(t, py, "BOUNDED_IDLE=10", "BOUNDED_IDLE_FLOOR=5"))

	for _, bad := range []string{"x", "nan", "150", "-1", ""} {
		ar48Ran(t, "BOUNDED_IDLE_FLOOR="+bad+" one point above the default", ar48Bounded(t, py, above, "BOUNDED_IDLE_FLOOR="+bad))
		ar48Stopped(t, "BOUNDED_IDLE_FLOOR="+bad+" at the default", ar48Bounded(t, py, append([]string{at, "BOUNDED_IDLE_FLOOR=" + bad}, ar48Quick...)...))
	}
}

// ar48FixMain loads scripts/bounded as ar48Signal does and runs its main
// with the arguments after the script's path. The 1-minute load is never the
// machine's: it reads 100000, and 0 inside $AR48_DIP's window (`from,to`,
// seconds from the start) when that is set.
const ar48FixMain = `
import importlib.machinery, importlib.util, os, sys, time
loader = importlib.machinery.SourceFileLoader("bounded", sys.argv[1])
mod = importlib.util.module_from_spec(importlib.util.spec_from_loader("bounded", loader))
loader.exec_module(mod)
dip = [float(x) for x in os.environ.get("AR48_DIP", "0,0").split(",")]
start = time.monotonic()
os.getloadavg = lambda: (0.0 if dip[0] <= time.monotonic() - start < dip[1] else 100000.0,) * 3
sys.exit(mod.main(["bounded"] + sys.argv[2:]))
`

// ar48Fix is one run through ar48FixMain: the fake `top`'s body (it has
// written its pid by then), bounded's options before `--load 4 --`, the
// environment, and whether bounded gets a TERM once the fake `top` runs.
type ar48Fix struct {
	top  string
	head []string
	env  []string
	term bool
}

// ar48FixRun is what such a run left: the exit code, stderr, whether the
// command ran, how long bounded took, and the fake `top`'s pid (0: none ran).
type ar48FixRun struct {
	code   int
	stderr string
	ran    bool
	took   time.Duration
	topPid int
}

// ar48FixBounded runs bounded's main through ar48FixMain under a 10 s
// bound, as `<head> --load 4 -- /bin/sh -c ...`, with the fake `top` the
// only thing on PATH: the real reader, never the real `top` or load.
func ar48FixBounded(t *testing.T, py string, c ar48Fix) ar48FixRun {
	t.Helper()
	script := filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")
	dir := t.TempDir()
	ran, args, pidFile := filepath.Join(dir, "ran"), filepath.Join(dir, "top-args"), filepath.Join(dir, "top-pid")
	bin := ar48FakeTop(t, "echo $$ > \"$AR48_TOP_PID\"\n"+c.top)
	topPid := func() int {
		b, _ := os.ReadFile(pidFile)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		return pid
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	argv := append([]string{"-c", ar48FixMain, script}, c.head...)
	argv = append(argv, "--load", "4", "--", "/bin/sh", "-c", fmt.Sprintf(`echo ran > "$1"; exit %d`, ar48Exit), "sh", ran)
	cmd := exec.CommandContext(ctx, py, append([]string{"-B"}, argv...)...)
	cmd.Env = ar48Env(bin, append([]string{"AR48_TOP_MARKER=" + args, "AR48_TOP_PID=" + pidFile}, c.env...)...)
	cmd.Stderr = &stderr
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if c.term {
		for time.Since(start) < 5*time.Second && topPid() == 0 {
			time.Sleep(20 * time.Millisecond)
		}
		if topPid() == 0 {
			cmd.Process.Kill()
			cmd.Wait()
			t.Fatalf("bounded never ran `top` (%s)", strings.TrimSpace(stderr.String()))
		}
		cmd.Process.Signal(syscall.SIGTERM)
	}
	err := cmd.Wait()
	out := ar48FixRun{stderr: stderr.String(), took: time.Since(start), topPid: topPid()}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee) && ctx.Err() == nil:
		out.code = ee.ExitCode()
	default:
		t.Fatalf("bounded %v %v: %v (%s)", c.head, c.env, err, stderr.String())
	}
	_, ranErr := os.Stat(ran)
	out.ran = ranErr == nil
	return out
}

// ar48AsMain fails unless the run is a load wait that ran out as it did
// before the idle key: exit 2, the command unrun, the three old lines and
// no others, inside the bound.
func ar48AsMain(t *testing.T, what string, r ar48FixRun, bound time.Duration) {
	t.Helper()
	if r.ran || r.code != 2 || !ar48Today.MatchString(r.stderr) {
		t.Errorf("%s: ran %v, exit %d, stderr %q; want the command unrun, exit 2 and the lines of a load wait before the idle key", what, r.ran, r.code, r.stderr)
	}
	if r.took > bound {
		t.Errorf("%s: bounded took %v; want under %v", what, r.took.Round(10*time.Millisecond), bound)
	}
}

// TestAR48Fix_SlowTopUnderWall (AR48 ruling 6, B1): the idle read is held
// by the wall. A `top` that takes 3 s to say the CPU is idle, under a wall
// of 0.3 s, is no reading: bounded stops `load` at the wall as it did.
func TestAR48Fix_SlowTopUnderWall(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the reader runs `top` on darwin only")
	}
	r := ar48FixBounded(t, boundedPython(t), ar48Fix{
		top:  "/bin/sleep 3\necho 'CPU usage: 1.00% user, 1.00% sys, 98.00% idle'",
		head: []string{"--wall", "0.3"},
	})
	ar48AsMain(t, "a 3 s top under a 0.3 s wall", r, 2500*time.Millisecond)
	if r.topPid == 0 {
		t.Error("a 3 s top under a 0.3 s wall: `top` never ran, so the read was not cut short")
	}
}

// TestAR48Fix_HungTopAtLimit (AR48 ruling 6, B2): the idle read is held by
// the wait's limit. A `top` that hangs is cut at the limit and is no
// reading: stderr is the old one, the waiting line included.
func TestAR48Fix_HungTopAtLimit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the reader runs `top` on darwin only")
	}
	r := ar48FixBounded(t, boundedPython(t), ar48Fix{
		top: "exec /bin/sleep 15",
		env: []string{"BOUNDED_LOAD_MAX=0.3"},
	})
	ar48AsMain(t, "a hung top at a 0.3 s limit", r, 4*time.Second)
	if r.topPid == 0 {
		t.Error("a hung top at a 0.3 s limit: `top` never ran, so the read was not cut short")
	}
}

// TestAR48Fix_LoadDipsDuringRead (AR48 ruling 6, S1): a read's time comes
// out of its poll's sleep, so the load is sampled when it was before the
// idle key. With a 1 s `top` and a 1 s poll, a load that is under its line
// from 0.4 s to 1.8 s is seen by the second sample, and the command runs.
func TestAR48Fix_LoadDipsDuringRead(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the reader runs `top` on darwin only")
	}
	r := ar48FixBounded(t, boundedPython(t), ar48Fix{
		top: "/bin/sleep 1\necho 'CPU usage: 90.00% user, 5.00% sys, 5.00% idle'",
		env: []string{"AR48_DIP=0.4,1.8", "BOUNDED_LOAD_POLL=1", "BOUNDED_LOAD_MAX=2.5"},
	})
	if !r.ran || r.code != ar48Exit || strings.Contains(r.stderr, "stop:") {
		t.Errorf("a load that dips during the read: ran %v, exit %d, stderr %q; want the command run at the second sample, its exit %d and no stop line", r.ran, r.code, r.stderr, ar48Exit)
	}
}

// TestAR48Fix_NotConfirmedLine (AR48 ruling 6, S2): at or past the limit
// bounded refuses before any read, so a wait that the wall ends is a `load`
// stop as it was, with no line about a load not confirmed; and that line
// never says the command runs, since the wall may still stop it.
func TestAR48Fix_NotConfirmedLine(t *testing.T) {
	py := boundedPython(t)
	r := ar48FixBounded(t, py, ar48Fix{
		head: []string{"--wall", "0.2"},
		env:  []string{"BOUNDED_IDLE=10,60,60", "BOUNDED_LOAD_MAX=0.9", "BOUNDED_LOAD_POLL=0.1"},
	})
	lines := ar48Lines(r.stderr)
	if r.ran || r.code != 2 || lines[len(lines)-1] != "bounded: stop: load" || len(lines) != 3 {
		t.Errorf("idle 10,60,60 under a 0.2 s wall: ran %v, exit %d, stderr %q; want exit 2 and three lines, the last `bounded: stop: load`", r.ran, r.code, r.stderr)
	}
	high := ar48Bounded(t, py, "BOUNDED_IDLE=70")
	ar48Ran(t, "idle 70", high)
	if strings.Contains(high.stderr, "run") || !strings.Contains(high.stderr, "not confirmed") {
		t.Errorf("idle 70: stderr %q; want a line that says the load is not confirmed and nothing about the command running", high.stderr)
	}
}

// TestAR48Fix_QuietRunResets (AR48 ruling 6, S5): two polls above the floor
// end the wait only in a row. A low poll between them, or one with no
// reading, starts the count again.
func TestAR48Fix_QuietRunResets(t *testing.T) {
	py := boundedPython(t)
	for _, shares := range []string{"10,60,10,60,10", "10,60,x,60,10"} {
		r := ar48Bounded(t, py, "BOUNDED_IDLE="+shares, "BOUNDED_LOAD_MAX=0.7", "BOUNDED_LOAD_POLL=0.1")
		ar48Stopped(t, "idle "+shares, r)
	}
}

// TestAR48Fix_TermKillsTop (AR48 ruling 6, S3): a TERM during an idle read
// kills the reader's child before bounded exits: 143, nothing on stderr,
// and no `top` left behind.
func TestAR48Fix_TermKillsTop(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the reader runs `top` on darwin only")
	}
	r := ar48FixBounded(t, boundedPython(t), ar48Fix{top: "exec /bin/sleep 5", term: true})
	if r.code != 128+int(syscall.SIGTERM) || r.stderr != "" || r.ran {
		t.Errorf("a TERM during the read: ran %v, exit %d, stderr %q; want exit 143 and nothing on stderr", r.ran, r.code, r.stderr)
	}
	gone := false
	for start := time.Now(); time.Since(start) < 2*time.Second && !gone; time.Sleep(20 * time.Millisecond) {
		gone = errors.Is(syscall.Kill(r.topPid, 0), syscall.ESRCH)
	}
	if !gone {
		syscall.Kill(r.topPid, syscall.SIGKILL) // the fake top this test started, still in its sleep
		t.Errorf("a TERM during the read: the fake `top` (pid %d) outlived bounded", r.topPid)
	}
}

// TestAR48Fix_FloorTakesFloats (AR48 ruling 6, S4): BOUNDED_IDLE_FLOOR takes
// any number float reads in 0 to 100; a NaN, an inf or a value outside
// leaves the default.
func TestAR48Fix_FloorTakesFloats(t *testing.T) {
	py := boundedPython(t)
	for _, floor := range []string{"+80", "8e1", "80.", " 80 "} {
		r := ar48Bounded(t, py, append([]string{"BOUNDED_IDLE=70", "BOUNDED_IDLE_FLOOR=" + floor}, ar48Quick...)...)
		ar48Stopped(t, "idle 70 under BOUNDED_IDLE_FLOOR="+floor, r)
		if waiting := ar48Lines(r.stderr)[0]; !strings.Contains(waiting, "floor 80%") {
			t.Errorf("BOUNDED_IDLE_FLOOR=%s: the waiting line is %q; want it to name the floor 80%%", floor, waiting)
		}
	}
	above := fmt.Sprintf("BOUNDED_IDLE=%g", float64(DefaultIdleFloor*100)+1)
	for _, bad := range []string{"nan", "inf", "+150", "-0.5", "1e3"} {
		ar48Ran(t, "BOUNDED_IDLE_FLOOR="+bad+" one point above the default", ar48Bounded(t, py, above, "BOUNDED_IDLE_FLOOR="+bad))
	}
}

// TestAR48Fix_GroupKillTakesTop (AR48 ruling 7, R1): `top` stays in
// bounded's process group, so a caller that ends a line by a KILL of its
// group during an idle read leaves no `top` behind. bounded runs in a group
// of its own here, so the kill cannot reach the test.
func TestAR48Fix_GroupKillTakesTop(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the reader runs `top` on darwin only")
	}
	py := boundedPython(t)
	script := filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "top-pid")
	bin := ar48FakeTop(t, "echo $$ > \"$AR48_TOP_PID\"\nexec /bin/sleep 20")
	topPid := func() int {
		b, _ := os.ReadFile(pidFile)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		return pid
	}
	var stderr bytes.Buffer
	cmd := exec.Command(py, "-B", "-c", ar48FixMain, script, "--load", "4", "--", "/bin/sh", "-c", "exit 0")
	cmd.Env = ar48Env(bin, "AR48_TOP_MARKER="+filepath.Join(dir, "top-args"), "AR48_TOP_PID="+pidFile)
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	bounded := cmd.Process.Pid
	for start := time.Now(); time.Since(start) < 5*time.Second && topPid() == 0; {
		time.Sleep(20 * time.Millisecond)
	}
	top := topPid()
	if pgid, err := syscall.Getpgid(bounded); top == 0 || err != nil || pgid != bounded {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("bounded (pid %d, group %d, %v) never ran `top` in a group of its own (%s)", bounded, pgid, err, strings.TrimSpace(stderr.String()))
	}
	syscall.Kill(-bounded, syscall.SIGKILL) // the group this test made, bounded's own
	cmd.Wait()
	gone := false
	for start := time.Now(); time.Since(start) < 2*time.Second && !gone; time.Sleep(20 * time.Millisecond) {
		gone = errors.Is(syscall.Kill(top, 0), syscall.ESRCH)
	}
	if !gone {
		syscall.Kill(top, syscall.SIGKILL) // the fake top this test started, still in its sleep
		t.Errorf("a KILL of bounded's group during the read: the fake `top` (pid %d) outlived it", top)
	}
}

// ar48TurnsMain loads scripts/bounded as ar48FixMain does and runs its main
// with a load of 100000, never the machine's, then writes how many times
// the load was read (one per turn of the wait's loop) to $AR48_TURNS.
const ar48TurnsMain = `
import importlib.machinery, importlib.util, os, sys
loader = importlib.machinery.SourceFileLoader("bounded", sys.argv[1])
mod = importlib.util.module_from_spec(importlib.util.spec_from_loader("bounded", loader))
loader.exec_module(mod)
turns = [0]
def load():
    turns[0] += 1
    return (100000.0,) * 3
os.getloadavg = load
code = 70
try:
    code = mod.main(["bounded"] + sys.argv[2:])
finally:
    with open(os.environ["AR48_TURNS"], "w") as fh:
        fh.write(str(turns[0]))
sys.exit(code)
`

// TestAR48Fix_PollNotPositive (AR48 ruling 7, P1): a BOUNDED_LOAD_POLL
// that is zero, negative or a NaN is the default, 30 s. The wait does not
// spin: over a limit of 0.4 s the loop turns twice, and the stop is `load`.
func TestAR48Fix_PollNotPositive(t *testing.T) {
	py := boundedPython(t)
	for _, poll := range []string{"-1", "0", "nan"} {
		script := filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")
		dir := t.TempDir()
		turnsFile := filepath.Join(dir, "turns")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, py, "-B", "-c", ar48TurnsMain, script, "--load", "4", "--", "/bin/sh", "-c", "exit 0")
		cmd.Env = ar48Env(ar48FakeTop(t, "exit 1"), "AR48_TOP_MARKER="+filepath.Join(dir, "top-args"), "AR48_TURNS="+turnsFile,
			"BOUNDED_IDLE=10", "BOUNDED_LOAD_MAX=0.4", "BOUNDED_LOAD_POLL="+poll)
		cmd.Stderr = &stderr
		err := cmd.Run()
		cancel()
		var ee *exec.ExitError
		code := 0
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("BOUNDED_LOAD_POLL=%s: %v (%s)", poll, err, stderr.String())
		}
		b, _ := os.ReadFile(turnsFile)
		turns, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		lines := ar48Lines(stderr.String())
		if code != 2 || lines[len(lines)-1] != "bounded: stop: load" {
			t.Errorf("BOUNDED_LOAD_POLL=%s: exit %d, stderr's last line %q; want exit 2 and `bounded: stop: load`", poll, code, lines[len(lines)-1])
		}
		if turns < 1 || turns > 5 {
			t.Errorf("BOUNDED_LOAD_POLL=%s: the wait's loop turned %d times in 0.4 s; want 2, and 5 at most", poll, turns)
		}
	}
}
