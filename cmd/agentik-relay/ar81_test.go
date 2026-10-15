package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR81_3_DoctorBand (AR81 ruling 6): doctor's `band` check, after
// caffeinate and never asking, on bandFor and bandVersion faked: present
// reads ok, `~/<file>: <--version's first line>`; `off` ok; absent ok and
// warn; a failing --version ok and warn, `spawns unchecked`. Doctor exits
// 0 each time and the FakeBand is never asked.
func TestAR81_3_DoctorBand(t *testing.T) {
	planFakes(t, "main")
	oldLook, oldProbe, oldWT, oldBand, oldVersion := lookPath, claudeProbe, gitWorktree, bandFor, bandVersion
	t.Cleanup(func() {
		lookPath, claudeProbe, gitWorktree, bandFor, bandVersion = oldLook, oldProbe, oldWT, oldBand, oldVersion
	})
	home := getenv("HOME")
	lookPath = func(name string) (string, error) { return home + "/bin/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	file := home + "/.local/bin/agentik-session"
	fake := &effects.FakeBand{}
	for _, c := range []struct {
		name, resolved string
		version        string
		err            error
		want           string
	}{
		{"present", file, "agentik-session 2.0.0-alpha.14", nil, "ok: ~/.local/bin/agentik-session: agentik-session 2.0.0-alpha.14"},
		{"off", effects.BandOff, "", nil, "ok: off: the config turns pacing off"},
		{"absent", "absent", "", nil, "warn: absent: agentik-session not on PATH; spawns unpaced"},
		{"failing", file, "", errors.New("exit 2: unknown flag"), "warn: ~/.local/bin/agentik-session --version: exit 2: unknown flag; spawns unchecked"},
	} {
		var modes, versions []string
		bandFor = func(mode string, _ func(string) string) (effects.Band, string) {
			modes = append(modes, mode)
			if c.resolved == file {
				return fake, file
			}
			return nil, c.resolved
		}
		bandVersion = func(f string) (string, error) { versions = append(versions, f); return c.version, c.err }
		var out, errb bytes.Buffer
		code := run([]string{"doctor", "--json"}, strings.NewReader(""), &out, &errb)
		var doc struct{ Checks []check }
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, out.String())
		}
		got, at, caff := "", -1, -1
		for i, k := range doc.Checks {
			switch k.Name {
			case "caffeinate":
				caff = i
			case "band":
				mark := "ok"
				if !k.OK {
					mark = "FAIL"
				} else if k.Warn {
					mark = "warn"
				}
				got, at = mark+": "+k.Detail, i
			}
		}
		wantVersions := "[]"
		if c.resolved == file {
			wantVersions = "[" + file + "]"
		}
		if code != 0 || got != c.want || at != caff+1 || fmt.Sprint(modes) != "[pace]" || fmt.Sprint(versions) != wantVersions {
			t.Fatalf("%s: exit %d, band %q at %d (caffeinate %d), modes %v, versions %v\n%s%s", c.name, code, got, at, caff, modes, versions, out.String(), errb.String())
		}
	}
	if len(fake.Asks()) != 0 {
		t.Fatalf("doctor asked the band: %v", fake.Asks())
	}
}
