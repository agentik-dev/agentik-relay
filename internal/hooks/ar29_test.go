package hooks

import (
	"strings"
	"testing"
)

// TestAR29_2_BackgroundTruthy is AR29 act 2 (ADV17-3): the background
// flag fails closed. In a row the helper line with run_in_background 1,
// "1", "yes", true, "true" or "True" is denied (helper); with false, nil,
// 0, "" or "false" it is allowed; ls -l with "yes" is allowed.
func TestAR29_2_BackgroundTruthy(t *testing.T) {
	_, row := ar27cFixture(t)
	for _, tc := range []struct {
		cmd  string
		bg   any
		deny bool
	}{
		{ar27bLine, 1, true},
		{ar27bLine, "1", true},
		{ar27bLine, "yes", true},
		{ar27bLine, true, true},
		{ar27bLine, "true", true},
		{ar27bLine, "True", true},
		{ar27bLine, false, false},
		{ar27bLine, nil, false},
		{ar27bLine, 0, false},
		{ar27bLine, "", false},
		{ar27bLine, "false", false},
		{"ls -l", "yes", false},
	} {
		deny, why := ar27cHook(t, tc.cmd, tc.bg, row)
		if deny != tc.deny || deny && !strings.HasPrefix(why, "relay: helper: ") {
			t.Errorf("%q with run_in_background %#v = %v, %q; want denied %v (helper)", tc.cmd, tc.bg, deny, why, tc.deny)
		}
	}
	for v, want := range map[any]bool{" FALSE ": false, "0": false, " 0 ": false, 0.0: false, 2.5: true, "no": true, "off": true} {
		if got := truthy(v); got != want {
			t.Errorf("truthy(%#v) = %v, want %v", v, got, want)
		}
	}
	for _, v := range []any{[]any{}, map[string]any{}} {
		if !truthy(v) {
			t.Errorf("truthy(%#v) = false, want true (fail closed)", v)
		}
	}
}
