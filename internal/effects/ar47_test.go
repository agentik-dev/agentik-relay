package effects

import (
	"math"
	"testing"
)

// topTwo is `top -l 2 -n 0 -s 1` on darwin, cut to the lines round each
// sample's `CPU usage:`: the first sample's window is not defined, the
// second is the change over the one-second wait.
const topTwo = `Processes: 612 total, 3 running, 609 sleeping, 3407 threads
2026/10/01 07:12:01
Load Avg: 52.70, 31.02, 16.10
CPU usage: 5.00% user, 10.00% sys, 85.00% idle
SharedLibs: 631M resident, 121M data, 87M linkedit.

Processes: 612 total, 9 running, 603 sleeping, 3411 threads
2026/10/01 07:12:02
Load Avg: 52.70, 31.02, 16.10
CPU usage: 12.50% user, 15.25% sys, 72.25% idle
SharedLibs: 631M resident, 121M data, 87M linkedit.
`

// TestAR47_1_ParseIdle (breaker-signal-r1 §2; AR47 act 1): the idle share
// is a pure parse. Two `CPU usage:` lines give the second's share; a line
// with no `% idle`, or a value outside 0 to 100, is an error; two
// /proc/stat `cpu` lines give the share of their difference.
func TestAR47_1_ParseIdle(t *testing.T) {
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	if got, err := ParseTopIdle(topTwo); err != nil || !near(got, 0.7225) {
		t.Fatalf("two CPU usage lines: %v %v, want the second's 0.7225", got, err)
	}
	for name, text := range map[string]string{
		"no % idle":     "CPU usage: 12.50% user, 15.25% sys\n",
		"no CPU usage":  "Load Avg: 52.70, 31.02, 16.10\n",
		"empty":         "",
		"above 100":     "CPU usage: 1.00% user, 2.00% sys, 120.00% idle\n",
		"below 0":       "CPU usage: 1.00% user, 2.00% sys, -3.00% idle\n",
		"not a number":  "CPU usage: 1.00% user, 2.00% sys, lots% idle\n",
		"last has none": "CPU usage: 5.00% user, 10.00% sys, 85.00% idle\nCPU usage: 12.50% user\n",
	} {
		if got, err := ParseTopIdle(text); err == nil {
			t.Fatalf("%s: %v with no error", name, got)
		}
	}

	// user nice system idle iowait irq softirq steal: 800 jiffies passed,
	// 550 idle and 50 waiting on I/O.
	const before = "cpu  100 0 100 700 100 0 0 0 0 0\ncpu0 50 0 50 350 50 0 0 0 0 0\nintr 9\n"
	const after = "cpu  200 0 200 1250 150 0 0 0 0 0\ncpu0 100 0 100 600 75 0 0 0 0 0\nintr 12\n"
	if got, err := ParseStatIdle(before, after); err != nil || !near(got, 0.75) {
		t.Fatalf("two /proc/stat reads: %v %v, want 0.75", got, err)
	}
	for name, pair := range map[string][2]string{
		"no cpu line":   {"intr 9\n", after},
		"no time moved": {before, before},
		"backwards":     {after, before},
		"short line":    {"cpu  100 0 100\n", after},
	} {
		if got, err := ParseStatIdle(pair[0], pair[1]); err == nil {
			t.Fatalf("%s: %v with no error", name, got)
		}
	}

	// The seam: OSIdle is an Idle, and the bundle carries one.
	var _ Idle = OSIdle{}
	if fx := (Effects{Idle: OSIdle{}}); fx.Idle == nil {
		t.Fatal("Effects.Idle not set")
	}
}
