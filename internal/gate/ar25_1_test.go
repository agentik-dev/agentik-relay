package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/packet"
)

// TestAR25_1_RelayHomeBound is review-adv15's test 2 (ADV15-3): a file a
// sitting plants in the relay home's own dirs (go-build, config, tmp)
// reaches no gate line, and each gate's lines run under a fresh scratch
// home in <home>/cache, gone when the gate ends: two gates, two dirs, the
// first gone, and what one gate's line wrote reaches no later line.
// Amended at AR31 act 1: a line's TMPDIR is the t/ of a fresh dir in the
// pin's short temp dir, within TmpMax, gone with the gate.
func TestAR25_1_RelayHomeBound(t *testing.T) {
	w := hermeticWorld(t)
	p, ok := effects.Current()
	if !ok {
		t.Fatal("no pin")
	}
	for _, d := range []string{"cache/go-build", "config", "tmp", "."} {
		if err := os.MkdirAll(filepath.Join(p.Home, d), 0o700); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(p.Home, d), "planted", "x\n")
	}
	log := filepath.Join(t.TempDir(), "log")
	line := fmt.Sprintf(`echo "env $GOCACHE $HOME $TMPDIR $PYTHONNOUSERSITE" >> %[1]s; `+
		`for f in "$GOCACHE/planted" "$XDG_CONFIG_HOME/planted" "$TMPDIR/planted" "$HOME/planted"; do test -e "$f" && echo "seen $f" >> %[1]s; done; `+
		`touch "$GOCACHE/planted" "$XDG_CONFIG_HOME/planted" "$TMPDIR/planted" "$HOME/planted"; echo ok`, log)
	pkt := pk([]string{"x.txt"}, packet.Line{Shell: line, Expect: "reads `ok`", Kind: packet.Reads, Value: "ok"})
	st := w.stage(t, "x.txt")
	var caches []string
	for gate := 1; gate <= 2; gate++ {
		n := len(caches)
		Run(w.fx, st, pkt, nil, seen, Verify{Name: "auto"})
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatalf("gate %d ran no line: %v", gate, err)
		}
		caches = caches[:0]
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			f := strings.Fields(l)
			if f[0] == "seen" {
				t.Errorf("gate %d: a line read a file planted before it: %s", gate, l)
				continue
			}
			if len(f) != 5 || f[4] != "1" {
				t.Errorf("gate %d: a line's env %q: want GOCACHE, HOME, TMPDIR and PYTHONNOUSERSITE=1", gate, l)
				continue
			}
			home, tmp := f[2], f[3]
			if filepath.Dir(home) != filepath.Join(p.Home, "cache") || !strings.HasPrefix(filepath.Base(home), "scratch-") ||
				f[1] != filepath.Join(home, "go-build") {
				t.Errorf("gate %d: a line ran under %q, not a scratch home in %s", gate, l, filepath.Join(p.Home, "cache"))
			}
			// AR31 act 1: TMPDIR is <m>/t, <m> a fresh dir in the pin's
			// short temp dir, at most TmpMax bytes.
			if filepath.Base(tmp) != "t" || filepath.Dir(filepath.Dir(tmp)) != p.Tmp || len(tmp) > effects.TmpMax {
				t.Errorf("gate %d: a line's TMPDIR %q (%d bytes), not <m>/t in %s within %d bytes", gate, tmp, len(tmp), p.Tmp, effects.TmpMax)
			}
			for _, d := range []string{home, filepath.Dir(tmp)} {
				if _, err := os.Stat(d); !os.IsNotExist(err) {
					t.Errorf("gate %d: the scratch's %s outlived its gate: %v", gate, d, err)
				}
			}
			caches = append(caches, f[1])
		}
		if len(caches) <= n {
			t.Fatalf("gate %d ran no line of its own (%d lines, %d before)", gate, len(caches), n)
		}
	}
	distinct := map[string]bool{}
	for _, c := range caches {
		distinct[c] = true
	}
	if len(distinct) != len(caches) {
		t.Errorf("two lines shared a GOCACHE across gates: %q", caches)
	}
}
