package effects

import (
	"math"
	"testing"
)

// TestAR47Fix_ParseIdleRefuses (AR47 ruling 6; breaker-signal-r1 §6): a
// share the parsers cannot read is an error, never a number. NaN and Inf
// pass no range check, and a NaN `idle` fails the ledger's append; a
// comma decimal (`5,90% idle`) read as `90% idle` would be 0.9, a busy
// machine called quiet. A whole, well-formed line still parses.
func TestAR47Fix_ParseIdleRefuses(t *testing.T) {
	for name, text := range map[string]string{
		"NaN":             "CPU usage: 1.00% user, 2.00% sys, NaN% idle\n",
		"nan":             "CPU usage: 1.00% user, 2.00% sys, nan% idle\n",
		"Inf":             "CPU usage: 1.00% user, 2.00% sys, Inf% idle\n",
		"+Inf":            "CPU usage: 1.00% user, 2.00% sys, +Inf% idle\n",
		"comma decimals":  "CPU usage: 80,10% user, 14,00% sys, 5,90% idle",
		"one comma value": "CPU usage: 80.10% user, 14.00% sys, 5,90% idle\n",
		"a bare number":   "CPU usage: 80.10% user, 14.00, 5.90% idle\n",
		"an exponent":     "CPU usage: 1.00% user, 2.00% sys, 9e1% idle\n",
	} {
		if got, err := ParseTopIdle(text); err == nil {
			t.Fatalf("top, %s: %v with no error", name, got)
		}
	}
	const good = "cpu  200 0 200 1250 150 0 0 0 0 0\n"
	for name, bad := range map[string]string{
		"nan idle":   "cpu  100 0 100 nan 100 0 0 0 0 0\n",
		"NaN user":   "cpu  NaN 0 100 700 100 0 0 0 0 0\n",
		"inf idle":   "cpu  100 0 100 inf 100 0 0 0 0 0\n",
		"+Inf steal": "cpu  100 0 100 700 100 0 0 +Inf 0 0\n",
		"a fraction": "cpu  100 0 100 700.5 100 0 0 0 0 0\n",
	} {
		if got, err := ParseStatIdle(bad, good); err == nil {
			t.Fatalf("/proc/stat, %s before: %v with no error", name, got)
		}
		if got, err := ParseStatIdle(good, bad); err == nil {
			t.Fatalf("/proc/stat, %s after: %v with no error", name, got)
		}
	}
	// What top and /proc/stat do print still reads.
	if got, err := ParseTopIdle("CPU usage: 80.10% user, 14.00% sys, 5.90% idle \n"); err != nil || math.Abs(got-0.059) > 1e-9 {
		t.Fatalf("a well-formed line: %v %v, want 0.059", got, err)
	}
	if got, err := ParseTopIdle("CPU usage: 0% user, 0% sys, 100% idle"); err != nil || got != 1 {
		t.Fatalf("whole numbers: %v %v, want 1", got, err)
	}
	if got, err := ParseStatIdle("cpu  100 0 100 700 100 0 0 0 0 0\n", good); err != nil || math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("two /proc/stat reads: %v %v, want 0.75", got, err)
	}
}
