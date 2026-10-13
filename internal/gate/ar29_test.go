package gate

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR29_3_PacketMatchesSource is AR29 act 3 (ADV17-2): with a packets
// source (Verify.Source), the gate compares design/packets/<id>.md at the
// head with the source's bytes: equal is green; a byte changed, or the
// copy absent at the head, is red (PacketDiffers). Unset, or a row that
// declared its copy-in skipped, reads nothing (green over a missing copy
// and a source that would be unrun); no source file for the row is
// skipped; two dated sources are unrun.
func TestAR29_3_PacketMatchesSource(t *testing.T) {
	const packetCopy = "design/packets/a1.md"
	body := "a1 " + packetCopy + "\n" // what stage writes
	src := func(files map[string]string) string {
		dir := t.TempDir()
		for name, b := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	check := func(w *world, from int) (string, bool) {
		for _, l := range w.ledger.Written()[from:] {
			if l["check"] == "packet-source" {
				s, _ := l["status"].(string)
				return s, true
			}
		}
		return "", false
	}
	twoDated := src(map[string]string{"2026-09-24-a1.md": body, "2026-09-25-a1.md": body})
	w := newWorld(t)
	st := w.stage(t, packetCopy)
	p := pk([]string{"design/packets/"})
	for _, c := range []struct {
		name   string
		v      Verify
		res    relay.Result
		differ bool
		status string // the packet-source ledger check; "" is none
	}{
		{"equal", Verify{Source: src(map[string]string{"a1.md": body})}, relay.GreenResult, false, relay.CheckGreen},
		{"equal, dated", Verify{Source: src(map[string]string{"2026-09-25-a1.md": body})}, relay.GreenResult, false, relay.CheckGreen},
		{"a byte changed", Verify{Source: src(map[string]string{"a1.md": body + "\n"})}, relay.RedResult, true, relay.CheckRed},
		{"no source file", Verify{Source: src(map[string]string{"a2.md": body})}, relay.GreenResult, false, relay.CheckSkipped},
		{"two dated", Verify{Source: twoDated}, relay.Unrun, false, ""},
		{"unreadable", Verify{Source: filepath.Join(t.TempDir(), "absent")}, relay.Unrun, false, ""},
	} {
		from := len(w.ledger.Written())
		res, _, g, _ := Run(w.fx, st, p, nil, seen, c.v)
		status, _ := check(w, from)
		if res != c.res || g.PacketDiffers != c.differ || status != c.status {
			t.Errorf("%s: %s, packet_differs %v, packet-source %q; want %s, %v, %q", c.name, res, g.PacketDiffers, status, c.res, c.differ, c.status)
		}
	}
	w = newWorld(t)
	st = w.stage(t, "a.txt")
	for _, c := range []struct {
		name   string
		v      Verify
		res    relay.Result
		status string
	}{
		{"the copy deleted", Verify{Source: src(map[string]string{"a1.md": body})}, relay.RedResult, relay.CheckRed},
		{"unset", Verify{}, relay.GreenResult, ""},
		{"skipped", Verify{Source: twoDated, CopyInSkipped: true}, relay.GreenResult, relay.CheckSkipped},
	} {
		from := len(w.ledger.Written())
		res, _, g, _ := Run(w.fx, st, pk([]string{"a.txt"}), nil, seen, c.v)
		status, _ := check(w, from)
		if res != c.res || g.PacketDiffers != (c.res == relay.RedResult) || status != c.status {
			t.Errorf("%s: %s, packet_differs %v, packet-source %q; want %s, %q", c.name, res, g.PacketDiffers, status, c.res, c.status)
		}
		if skipped := slices.Contains(g.Skipped, "packet-source"); skipped != (c.status == relay.CheckSkipped) {
			t.Errorf("%s: skipped %q", c.name, g.Skipped)
		}
	}
	if _, err := PacketSource("rel/packets", "a1"); err == nil {
		t.Error("PacketSource over a relative dir: no error")
	}
	both := src(map[string]string{"a1.md": body, "2026-09-25-a1.md": body, "2026-09-25-xa1.md": body})
	if got, err := PacketSource(both, "a1"); err != nil || got != filepath.Join(both, "a1.md") {
		t.Errorf("PacketSource(<id>.md beside a dated one) = %q, %v; want the undated", got, err)
	}
}
