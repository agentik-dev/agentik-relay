package relay

import "testing"

// AR2-8: a line or a verify check passes only on a pass status; lost, unrun
// or no status at all is never green.
func TestAR2_8_UnrunChecksNeverPass(t *testing.T) {
	ok := Gate{ExitOK: true, ResultSeen: true, Commit: true, Clean: true, OriginSame: true, Gathered: true}
	for _, s := range []string{CheckUnrun, CheckLost, CheckRed, ""} {
		g := ok
		g.Lines = []Check{{Status: s}}
		if res, _ := Decide(g); res == GreenResult {
			t.Errorf("a line %q: Decide = green", s)
		}
		g = ok
		g.Verify = &Check{Status: s}
		if res, _ := Decide(g); res == GreenResult {
			t.Errorf("verify %q: Decide = green", s)
		}
	}
	for _, s := range []string{CheckGreen, CheckSameAsMain, CheckSkipped} {
		g := ok
		g.Lines, g.Verify = []Check{{Status: s}}, &Check{Status: s}
		if res, _ := Decide(g); res != GreenResult {
			t.Errorf("%q: Decide = %s", s, res)
		}
	}
}
