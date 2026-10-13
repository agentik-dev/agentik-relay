package effects

import (
	"strings"
	"testing"
	"time"
)

// TestAR59_1_VerifySeconds is AR59 act 1: `verify_seconds` is read in any
// layer as every key is; set, VerifyLimit is it, unset 900 s; 0, -1, 3601
// and 15m are LoadConfig errors naming the line and the key; 3600 is
// taken; it is never Unknown.
func TestAR59_1_VerifySeconds(t *testing.T) {
	env := map[string]string{"HOME": "/h"}
	get := func(k string) string { return env[k] }
	fs := &FakeFS{}
	c, err := LoadConfig(fs, get, "/r")
	if err != nil || c.VerifySeconds != 0 || c.VerifyLimit() != 900*time.Second || c.VerifyLimit() != Timeout {
		t.Fatalf("unset: %+v, limit %s, %v", c, c.VerifyLimit(), err)
	}
	const repo = "/r/.claude/claude-code-session/config.yaml"
	fs.WriteFile(repo, []byte("agent-relay:\n  verify_seconds: 1800\n"), 0o600)
	if c, err = LoadConfig(fs, get, "/r"); err != nil || c.VerifySeconds != 1800 || c.VerifyLimit() != 30*time.Minute || len(c.Unknown) != 0 {
		t.Fatalf("1800: %+v, limit %s, unknown %q, %v", c, c.VerifyLimit(), c.Unknown, err)
	}
	fs.WriteFile("/h/.config/agent-relay/config.yaml", []byte("verify_seconds: 1200\n"), 0o600)
	if c, err = LoadConfig(fs, get, "/r"); err != nil || c.VerifySeconds != 1800 {
		t.Fatalf("the repo's layer over the user's: %+v, %v", c, err)
	}
	fs.Remove(repo)
	if c, err = LoadConfig(fs, get, "/r"); err != nil || c.VerifyLimit() != 20*time.Minute || len(c.Unknown) != 0 {
		t.Fatalf("the user's layer: %+v, %v", c, err)
	}
	fs.Remove("/h/.config/agent-relay/config.yaml")
	for _, bad := range []string{"0", "-1", "3601", "15m"} {
		fs.WriteFile(repo, []byte("agent-relay:\n  verify_seconds: "+bad+"\n"), 0o600)
		_, err := LoadConfig(fs, get, "/r")
		if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "verify_seconds") ||
			!strings.Contains(err.Error(), repo) {
			t.Errorf("verify_seconds: %s: %v, want an error naming %s, line 2 and the key", bad, err, repo)
		}
	}
	fs.WriteFile(repo, []byte("agent-relay:\n  verify_seconds: 3600\n"), 0o600)
	if c, err = LoadConfig(fs, get, "/r"); err != nil || c.VerifyLimit() != time.Hour || MaxVerifySeconds != 3600 {
		t.Fatalf("3600: %+v, %v", c, err)
	}
}

// TestAR59_3_Cut is AR59 act 3: a line is cut when it exits 124 at or past
// its limit; 124 sooner, or another exit at the limit, is not.
func TestAR59_3_Cut(t *testing.T) {
	l := 900 * time.Second
	if !Cut(124, l, l) || !Cut(CutExit, l+time.Second, l) {
		t.Fatal("exit 124 at the limit is not cut")
	}
	if Cut(124, l-time.Millisecond, l) || Cut(1, l, l) || Cut(0, 2*l, l) {
		t.Fatal("a line cut that exited 124 sooner, or another exit")
	}
}

// TestAR59_4_Suggest is AR59 act 4: twice the measured time, rounded up to
// a whole minute, at least 900 and at most 3600.
func TestAR59_4_Suggest(t *testing.T) {
	for _, c := range []struct {
		took time.Duration
		want int
	}{
		{13 * time.Minute, 1560},
		{100 * time.Second, 900},
		{7*time.Minute + 30*time.Second + 200*time.Millisecond, 960},
		{31 * time.Minute, 3600},
		{0, 900},
	} {
		if got := SuggestVerifySeconds(c.took); got != c.want {
			t.Errorf("SuggestVerifySeconds(%s) = %d, want %d", c.took, got, c.want)
		}
	}
}
