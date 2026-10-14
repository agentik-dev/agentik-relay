package xdg

import "testing"

// TestAR76_5_StateHome (AR76 ruling 1): the state home with HOME set and
// empty, `~` and a leading `~/` under HOME, every other value as written.
func TestAR76_5_StateHome(t *testing.T) {
	for _, tc := range []struct{ home, state, want string }{
		{"/h", "", "/h/.local/state"},
		{"/h", "~/x", "/h/x"},
		{"/h", "~", "/h"},
		{"/h", "~/", "/h"},
		{"/h", "~/x/../y", "/h/y"},
		{"/h", "/s", "/s"},
		{"/h", "~x", "~x"},
		{"/h", "rel", "rel"},
		{"", "", ""},
		{"", "~/x", ""},
		{"", "~", ""},
		{"", "/s", "/s"},
	} {
		env := map[string]string{"HOME": tc.home, "XDG_STATE_HOME": tc.state}
		if got := StateHome(func(k string) string { return env[k] }); got != tc.want {
			t.Errorf("HOME=%q XDG_STATE_HOME=%q: StateHome = %q, want %q", tc.home, tc.state, got, tc.want)
		}
	}
}
