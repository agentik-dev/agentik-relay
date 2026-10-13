package effects

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Idle is the breaker's second key (§15): the idle fraction of all cores
// over a one-second sample, 0 to 1. The breaker reads it only after the
// load has turned its key, and an error leaves the load alone to decide.
type Idle interface {
	Idle() (share float64, err error)
}

// OSIdle reads the last `CPU usage:` line of `top -l 2 -n 0 -s 1` on
// darwin (the second sample is the change over the one-second wait; the
// first's window is not defined) and, elsewhere, two reads of
// /proc/stat's `cpu` line one second apart.
type OSIdle struct{}

// idleSample is the wait between OSIdle's two readings; idleWait is how
// long it lets `top` take before it gives the read up.
const (
	idleSample = time.Second
	idleWait   = 10 * time.Second
)

func (OSIdle) Idle() (float64, error) {
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), idleWait)
		defer cancel()
		out, err := exec.CommandContext(ctx, "top", "-l", "2", "-n", "0", "-s", "1").Output()
		if err != nil {
			return 0, fmt.Errorf("idle: top -l 2: %w", err)
		}
		return ParseTopIdle(string(out))
	}
	before, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, fmt.Errorf("idle: %w", err)
	}
	time.Sleep(idleSample)
	after, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, fmt.Errorf("idle: %w", err)
	}
	return ParseStatIdle(string(before), string(after))
}

// topShare is one share of top's `CPU usage:` line, `12.50% user`: plain
// digits with a point, so NaN, Inf, a sign, an exponent and a comma
// decimal are each no share.
var topShare = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)% ([a-z]+)$`)

// ParseTopIdle is the idle share of top's last `CPU usage:` line, as in
// `CPU usage: 12.50% user, 15.25% sys, 72.25% idle`. Text with no such
// line, a last line that is not wholly a list of such shares (a comma
// decimal splits into parts that are none), one with no `% idle`, or a
// value outside 0 to 100 is an error: unreadable, never misread.
func ParseTopIdle(s string) (float64, error) {
	last, found := "", false
	for _, line := range strings.Split(s, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "CPU usage:"); ok {
			last, found = rest, true
		}
	}
	if !found {
		return 0, fmt.Errorf("idle: no `CPU usage:` line in %q", capText(s, 80))
	}
	share, found := 0.0, false
	for _, part := range strings.Split(last, ",") {
		m := topShare.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return 0, fmt.Errorf("idle: %q in top's `CPU usage:` line is not a share", capText(strings.TrimSpace(part), 40))
		}
		if m[2] != "idle" {
			continue
		}
		pct, err := strconv.ParseFloat(m[1], 64)
		if err != nil || !(pct >= 0 && pct <= 100) {
			return 0, fmt.Errorf("idle: %q is not an idle share", capText(strings.TrimSpace(part), 40))
		}
		share, found = pct/100, true
	}
	if !found {
		return 0, fmt.Errorf("idle: no `%% idle` in %q", capText(strings.TrimSpace(last), 80))
	}
	return share, nil
}

// ParseStatIdle is the idle share between two reads of /proc/stat: of the
// time that passed on the `cpu` line (user, nice, system, idle, iowait,
// irq, softirq, steal), the part spent idle or waiting on I/O. A read
// with no `cpu` line, or a pair over which no time passed, is an error.
func ParseStatIdle(before, after string) (float64, error) {
	i0, t0, err := statCPU(before)
	if err != nil {
		return 0, err
	}
	i1, t1, err := statCPU(after)
	if err != nil {
		return 0, err
	}
	idle, total := i1-i0, t1-t0
	if total <= 0 || idle < 0 || idle > total {
		return 0, fmt.Errorf("idle: /proc/stat moved %v idle of %v between the reads", idle, total)
	}
	return idle / total, nil
}

// statCPU is the idle (idle and iowait) and total time of /proc/stat's
// `cpu` line, in its own ticks.
func statCPU(s string) (idle, total float64, err error) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "cpu" {
			continue
		}
		if len(f) < 5 {
			return 0, 0, fmt.Errorf("idle: /proc/stat's cpu line %q has no idle time", capText(line, 80))
		}
		for i, v := range f[1:min(len(f), 9)] {
			ticks, perr := strconv.ParseUint(v, 10, 64) // a count: no sign, no nan, no inf
			if perr != nil {
				return 0, 0, fmt.Errorf("idle: %q in /proc/stat's cpu line is not a time", capText(v, 40))
			}
			n := float64(ticks)
			total += n
			if i == 3 || i == 4 { // idle, iowait
				idle += n
			}
		}
		return idle, total, nil
	}
	return 0, 0, fmt.Errorf("idle: no cpu line in %q", capText(s, 80))
}
