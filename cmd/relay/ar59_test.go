package main

import (
	"bytes"
	"encoding/json"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// TestAR59_4_DoctorSuggests is AR59 act 4, on TestAR25_3_DoctorVerify's
// fixture: `relay doctor --verify` prints the repo's limit and, for a line
// that exits 0, a suggested verify_seconds (a quick line: 900); with
// `verify_seconds: 1800` in the repo's config the limit is 1800; a red
// line suggests nothing and says why; exit codes are unchanged.
func TestAR59_4_DoctorSuggests(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	getenv = fakeEnv(home)
	t.Cleanup(func() { getenv = os.Getenv })
	measuring(t)
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
	mk := func(body string) {
		os.WriteFile(filepath.Join(repo, "Makefile"), []byte("check:\n\t@"+body+"\n"), 0o644)
		g("add", "Makefile")
		g("commit", "-q", "-m", "check")
	}
	g("init", "-q", "-b", "main")
	mk(`echo fine`)
	do := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := run(append([]string{"doctor", "--verify", repo}, args...), strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	report := func() verifyReport {
		t.Helper()
		code, out, errs := do("--json")
		var rep verifyReport
		if err := json.Unmarshal([]byte(out), &rep); err != nil || code != exitOK || !rep.OK {
			t.Fatalf("doctor --verify --json = %d %q %q (%v)", code, out, errs, err)
		}
		if !strings.Contains(out, `"limit":`) || !strings.Contains(out, `"suggest":`) {
			t.Fatalf("--json lacks limit or suggest: %s", out)
		}
		return rep
	}
	if rep := report(); rep.Limit != 900 || rep.Suggest != 900 {
		t.Fatalf("unset: limit %d, suggest %d; want 900, 900", rep.Limit, rep.Suggest)
	}
	code, out, _ := do()
	if code != exitOK || !strings.Contains(out, "; limit: 900 s (unset); ") ||
		!strings.Contains(out, "suggest: verify_seconds: 900 in "+effects.RepoConfigFile+", section relay\n") {
		t.Fatalf("plain = %d %q", code, out)
	}
	cfg := filepath.Join(repo, effects.RepoConfigFile)
	os.MkdirAll(filepath.Dir(cfg), 0o755)
	if err := os.WriteFile(cfg, []byte("relay:\n  verify_seconds: 1800\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rep := report(); rep.Limit != 1800 || rep.Suggest != 900 {
		t.Fatalf("verify_seconds 1800: limit %d, suggest %d; want 1800, 900", rep.Limit, rep.Suggest)
	}
	if code, out, _ = do(); code != exitOK || !strings.Contains(out, "; limit: 1800 s (verify_seconds); ") {
		t.Fatalf("plain, verify_seconds 1800 = %d %q", code, out)
	}
	mk(`echo boom; exit 3`)
	code, out, _ = do()
	if code != exitRed || !strings.Contains(out, "suggest: none: the line exited ") || strings.Contains(out, "suggest: verify_seconds") {
		t.Fatalf("a red line = %d %q, want no suggestion and why", code, out)
	}
	code, out, _ = do("--json")
	var rep verifyReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil || code != exitRed || rep.Suggest != 0 || rep.Limit != 1800 {
		t.Fatalf("a red line --json = %d %q (%v), want suggest 0, limit 1800", code, out, err)
	}
	// The fix round: --line suggests nothing; a config that does not load
	// is measured anyway, its limit unknown (0 in --json).
	mk(`echo fine`)
	if code, out, _ = do("--line", "echo probe"); code != exitOK || !strings.Contains(out, "suggest: none: --line is not the verify line\n") {
		t.Fatalf("--line = %d %q, want no suggestion", code, out)
	}
	if err := os.WriteFile(cfg, []byte("relay:\n  caffeinate: maybe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = do("--json")
	rep = verifyReport{}
	if err := json.Unmarshal([]byte(out), &rep); err != nil || code != exitOK || !rep.OK || rep.Limit != 0 || rep.Suggest != 900 {
		t.Fatalf("a bad config --json = %d %q (%v), want measured, limit 0, suggest 900", code, out, err)
	}
	if code, out, _ = do(); code != exitOK || !strings.Contains(out, "; limit: unknown (") || !strings.Contains(out, "caffeinate") {
		t.Fatalf("a bad config = %d %q, want the limit unknown, naming the error", code, out)
	}
}

// TestAR59_4_Gofmt is AR59 act 4: every .go file under cmd and internal
// (testdata skipped) is as go/format prints it; gofmt is off the
// acceptance allowlist, so the check is a test.
func TestAR59_4_Gofmt(t *testing.T) {
	root := copyIn(t, "../..", "cmd", "internal")
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		n++
		got, err := format.Source(src)
		rel, _ := filepath.Rel(root, p)
		if err != nil {
			t.Errorf("%s: %v", rel, err)
		} else if !bytes.Equal(got, src) {
			t.Errorf("%s: not gofmt'd", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no .go file was read")
	}
}
