package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// sockFixture is the socket fixture's test (AR31 act 3): it binds a unix
// socket in os.MkdirTemp("", "sy") at a name that ends 40 bytes past
// TMPDIR/ (TmpRoom), and fails naming the path's length and the error.
const sockFixture = `package sy

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSock(t *testing.T) {
	d, err := os.MkdirTemp("", "sy")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)
	p := filepath.Join(d, strings.Repeat("s", len(os.TempDir())+1+40-len(d)-1))
	l, err := net.Listen("unix", p)
	if err != nil {
		t.Fatalf("len %d: %v", len(p), err)
	}
	l.Close()
}
`

// TestAR31_3_SocketFixture is AR31 act 3, the owner's acceptance 2: a
// fixture repo whose test binds a unix socket in os.MkdirTemp("", "sy"),
// at a name 40 bytes past TMPDIR/, passes `agentik-relay doctor --verify` (auto:
// make check, so go test under the pin): exit 0, ok, no log, and no file
// in the test's own Logs. The same fixture under a TMPDIR padded past
// TmpMax is red, and its kept log names the failed test and the bind
// error, so the green is the short TMPDIR's and never the fixture's.
// Skipped without a go the pin reaches.
func TestAR31_3_SocketFixture(t *testing.T) {
	if p, _ := effects.Current(); p.Tools["go"] == "" {
		t.Skip("no go the pin reaches")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = os.Getenv })
	measuring(t)
	logs := ownLogs(t)
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	for name, body := range map[string]string{
		"go.mod":     "module sy\n\ngo 1.22\n",
		"Makefile":   "check:\n\tgo test -count=1 ./...\n",
		"sy_test.go": sockFixture,
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g("init", "-q", "-b", "main")
	g("add", ".")
	g("commit", "-q", "-m", "fixture")
	do := func(args ...string) (int, verifyReport, string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(append([]string{"doctor", "--verify", repo, "--json"}, args...), strings.NewReader(""), &out, &errb)
		var rep verifyReport
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("doctor --verify --json: %v: %q %q", err, out.String(), errb.String())
		}
		return code, rep, errb.String()
	}

	code, rep, errs := do()
	if code != exitOK || !rep.OK || rep.Exit != 0 || rep.Line != "make check" || rep.Log != "" || !strings.HasPrefix(rep.Last, "ok") {
		t.Fatalf("the socket fixture = %d %+v %q; want make check green with no log", code, rep, errs)
	}
	if names := logsIn(t, logs); len(names) != 0 {
		t.Errorf("a green fixture kept logs: %v", names)
	}

	long := "$HOME/" + strings.Repeat("l", effects.TmpMax)
	code, rep, errs = do("--line", `mkdir -p "`+long+`" && TMPDIR="`+long+`" make check`)
	if code != exitRed || rep.OK || rep.Log == "" {
		t.Fatalf("the fixture under a TMPDIR past %d bytes = %d %+v %q; want red with its log", effects.TmpMax, code, rep, errs)
	}
	b, err := os.ReadFile(rep.Log)
	if err != nil || filepath.Dir(rep.Log) != logs || !strings.Contains(string(b), "--- FAIL: TestSock") || !strings.Contains(string(b), "bind: invalid argument") {
		t.Errorf("the red fixture's log %s (%v) holds:\n%s\nwant the failed test and the bind error", rep.Log, err, b)
	}
}
