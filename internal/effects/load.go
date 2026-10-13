package effects

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Load is the machine's load as the breaker reads it (§15): the 1-minute
// load average and the cores it is measured against.
type Load interface {
	Load() (avg1 float64, cores int, err error)
}

// OSLoad reads `sysctl -n vm.loadavg` (`/proc/loadavg` where there is no
// sysctl) and the cores Go sees (hw.ncpu on darwin).
type OSLoad struct{}

func (OSLoad) Load() (float64, int, error) {
	cores := runtime.NumCPU()
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		b, ferr := os.ReadFile("/proc/loadavg")
		if ferr != nil {
			return 0, cores, fmt.Errorf("load: sysctl vm.loadavg: %w", err)
		}
		out = b
	}
	avg, err := ParseLoadavg(string(out))
	return avg, cores, err
}

// ParseLoadavg is the 1-minute load from sysctl's `{ 6.28 27.38 139.23 }`
// or /proc/loadavg's `6.28 27.38 139.23 2/900 4242`.
func ParseLoadavg(s string) (float64, error) {
	f := strings.Fields(strings.NewReplacer("{", " ", "}", " ").Replace(s))
	if len(f) == 0 {
		return 0, fmt.Errorf("load: no load average in %q", capText(s, 80))
	}
	avg, err := strconv.ParseFloat(f[0], 64)
	if err != nil || avg < 0 {
		return 0, fmt.Errorf("load: %q is not a load average", capText(f[0], 40))
	}
	return avg, nil
}
