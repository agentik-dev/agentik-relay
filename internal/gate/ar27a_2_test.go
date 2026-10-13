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

// TestAR27A_2_VerifyOwnScratch is AR27A act 2 (ADV16-7), mirroring
// TestAR25_1_RelayHomeBound: the verify line runs under a scratch home of
// its own, never the acceptance lines', so a file an acceptance line
// writes under its HOME, GOCACHE or GOMODCACHE is absent from the verify
// line's, and both scratch homes are gone when the gate ends. Amended at
// AR31 act 1: the verify line's TMPDIR is not the acceptance lines', and a
// file an acceptance line writes in its TMPDIR is absent from it.
func TestAR27A_2_VerifyOwnScratch(t *testing.T) {
	w := hermeticWorld(t)
	p, ok := effects.Current()
	if !ok {
		t.Fatal("no pin")
	}
	log := filepath.Join(t.TempDir(), "log")
	acc := fmt.Sprintf(`echo "acc $HOME $GOCACHE $GOMODCACHE $TMPDIR" >> %s; `+
		`mkdir -p "$GOMODCACHE"; touch "$HOME/ar27a" "$GOCACHE/ar27a" "$GOMODCACHE/ar27a" "$TMPDIR/ar27a"; echo ok`, log)
	ver := fmt.Sprintf(`echo "ver $HOME $GOCACHE $GOMODCACHE $TMPDIR" >> %[1]s; `+
		`for f in "$HOME/ar27a" "$GOCACHE/ar27a" "$GOMODCACHE/ar27a" "$TMPDIR/ar27a"; do test -e "$f" && echo "seen $f" >> %[1]s; done; true`, log)
	pkt := pk([]string{"x.txt"}, packet.Line{Shell: acc, Expect: "reads `ok`", Kind: packet.Reads, Value: "ok"})
	st := w.stage(t, "x.txt")
	_, _, g, _ := Run(w.fx, st, pkt, nil, seen, Verify{Name: ver})
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the gate ran no line: %v", err)
	}
	homes := map[string][]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Fields(l)
		if f[0] == "seen" {
			t.Errorf("the verify line read a file an acceptance line wrote: %s", f[1])
			continue
		}
		if len(f) != 5 {
			t.Fatalf("a line's env %q: want HOME, GOCACHE, GOMODCACHE and TMPDIR", l)
		}
		homes[f[0]] = f[1:]
		if filepath.Dir(f[1]) != filepath.Join(p.Home, "cache") || !strings.HasPrefix(filepath.Base(f[1]), "scratch-") ||
			f[2] != filepath.Join(f[1], "go-build") || f[3] != filepath.Join(f[1], "gomod") {
			t.Errorf("the %s line ran under %q, not a scratch home in %s", f[0], l, filepath.Join(p.Home, "cache"))
		}
		if filepath.Dir(filepath.Dir(f[4])) != p.Tmp {
			t.Errorf("the %s line's TMPDIR %s is not in the pin's temp dir %s", f[0], f[4], p.Tmp)
		}
		for _, d := range []string{f[1], filepath.Dir(f[4])} {
			if _, err := os.Stat(d); !os.IsNotExist(err) {
				t.Errorf("the %s line's scratch %s outlived its gate: %v", f[0], d, err)
			}
		}
	}
	if homes["acc"] == nil || homes["ver"] == nil {
		t.Fatalf("lines %q: want one acceptance and one verify line (verify %+v)", homes, g.Verify)
	}
	if homes["acc"][0] == homes["ver"][0] {
		t.Errorf("the verify line shared the acceptance lines' scratch home %s", homes["ver"][0])
	}
	if homes["acc"][3] == homes["ver"][3] {
		t.Errorf("the verify line shared the acceptance lines' TMPDIR %s", homes["ver"][3])
	}
}
