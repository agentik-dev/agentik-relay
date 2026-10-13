package hygiene

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The private list in these tests holds invented terms only.
const privateList = "# a comment line: plugh\tnever loads\n" +
	"\n" +
	"   \n" +
	"invented\tzorbla[x]\n" +
	"home\tfrobnico[t]\n"

type run struct {
	code           int
	stdout, stderr string
}

// scan runs scripts/hygiene over a fresh tree holding files, under env (the
// private-list variables are cleared first, so the account's list is never read).
func scan(t *testing.T, files map[string]string, env map[string]string, args ...string) run {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "hygiene"))
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(py, append([]string{"-B", script, "--repo", repo}, args...)...)
	for _, kv := range os.Environ() {
		k := kv[:strings.IndexByte(kv, '=')+1]
		if k == "AGENTIK_HYGIENE_PRIVATE=" || k == "XDG_CONFIG_HOME=" || k == "HOME=" {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	if _, ok := env["HOME"]; !ok {
		cmd.Env = append(cmd.Env, "HOME="+t.TempDir())
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	code := 0
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return run{code, stdout.String(), stderr.String()}
}

func writeList(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "hygiene-private.tsv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPublicR1_1_LoadsPrivateList: the list's groups load from
// $AGENTIK_HYGIENE_PRIVATE, a new group and one that adds to a generic group;
// a hit prints as a path, a group and a count, never the regex.
func TestPublicR1_1_LoadsPrivateList(t *testing.T) {
	list := writeList(t, t.TempDir(), privateList)
	r := scan(t, map[string]string{"a.txt": "one zorblax here\n", "b.txt": "a frobnicot\n"},
		map[string]string{"AGENTIK_HYGIENE_PRIVATE": list}, "--require-private")
	if r.code != 2 {
		t.Fatalf("exit %d, want 2; stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"a.txt  invented  1", "b.txt  home  1", "hits outside allowlist: 2"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
		}
	}
	for _, never := range []string{"zorbla", "frobnico"} {
		if strings.Contains(r.stdout+r.stderr, never) {
			t.Errorf("output prints a private pattern %q:\n%s%s", never, r.stdout, r.stderr)
		}
	}
	if r.stderr != "" {
		t.Errorf("stderr %q, want none", r.stderr)
	}
}

// TestPublicR1_2_CommentsAndBlanksSkipped: a # line and blank lines load
// nothing, so the comment's group is no group and its word no hit.
func TestPublicR1_2_CommentsAndBlanksSkipped(t *testing.T) {
	list := writeList(t, t.TempDir(), privateList)
	r := scan(t, map[string]string{"a.txt": "plugh and never loads\n"},
		map[string]string{"AGENTIK_HYGIENE_PRIVATE": list}, "--require-private")
	if r.code != 0 || !strings.Contains(r.stdout, "hits outside allowlist: 0") {
		t.Fatalf("exit %d, want 0:\n%s%s", r.code, r.stdout, r.stderr)
	}
	if strings.Contains(r.stdout, "plugh") || strings.Contains(r.stdout, "comment") {
		t.Errorf("a comment line loaded as a group:\n%s", r.stdout)
	}
}

// TestPublicR1_3_AbsentList: no list prints one stderr line, the scan runs on
// the generic groups, and its exit follows them.
func TestPublicR1_3_AbsentList(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.tsv")
	env := map[string]string{"AGENTIK_HYGIENE_PRIVATE": missing}
	r := scan(t, map[string]string{"a.txt": "zorblax\n"}, env)
	want := "hygiene: no private list at " + missing + "; generic groups only\n"
	if r.code != 0 || r.stderr != want {
		t.Fatalf("exit %d stderr %q, want 0 and %q", r.code, r.stderr, want)
	}
	// The address is built here, so this file holds no LAN address of its own.
	r = scan(t, map[string]string{"a.txt": "a LAN host " + "192." + "168.0.9\n"}, env)
	if r.code != 2 || !strings.Contains(r.stdout, "a.txt  lan  1") {
		t.Fatalf("a generic hit without the list: exit %d, want 2:\n%s", r.code, r.stdout)
	}
}

// TestPublicR1_4_RequirePrivate: --require-private exits 2 on an absent or
// an unreadable list.
func TestPublicR1_4_RequirePrivate(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.tsv")
	r := scan(t, map[string]string{"a.txt": "clean\n"},
		map[string]string{"AGENTIK_HYGIENE_PRIVATE": missing}, "--require-private")
	if r.code != 2 {
		t.Errorf("absent list: exit %d, want 2; stderr %q", r.code, r.stderr)
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 file")
	}
	list := writeList(t, t.TempDir(), privateList)
	if err := os.Chmod(list, 0); err != nil {
		t.Fatal(err)
	}
	r = scan(t, map[string]string{"a.txt": "clean\n"},
		map[string]string{"AGENTIK_HYGIENE_PRIVATE": list}, "--require-private")
	if r.code != 2 || strings.Contains(r.stderr, "zorbla") {
		t.Errorf("unreadable list: exit %d, want 2; stderr %q", r.code, r.stderr)
	}
}

// TestPublicR1_5_DefaultPath: without the variable the list is
// $XDG_CONFIG_HOME/agentik/hygiene-private.tsv, else ~/.config's.
func TestPublicR1_5_DefaultPath(t *testing.T) {
	xdg := t.TempDir()
	writeList(t, filepath.Join(xdg, "agentik"), privateList)
	r := scan(t, map[string]string{"a.txt": "zorblax\n"},
		map[string]string{"XDG_CONFIG_HOME": xdg}, "--require-private")
	if r.code != 2 || !strings.Contains(r.stdout, "a.txt  invented  1") {
		t.Errorf("XDG list: exit %d, want 2:\n%s%s", r.code, r.stdout, r.stderr)
	}
	home := t.TempDir()
	writeList(t, filepath.Join(home, ".config", "agentik"), privateList)
	r = scan(t, map[string]string{"a.txt": "zorblax\n"},
		map[string]string{"HOME": home}, "--require-private")
	if r.code != 2 || !strings.Contains(r.stdout, "a.txt  invented  1") {
		t.Errorf("HOME list: exit %d, want 2:\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestPublicR1_6_AllowAndBadLine: an allowlist entry may name a private
// group whether or not the list is present; a malformed line is a usage error
// that names its number and never its text.
func TestPublicR1_6_AllowAndBadLine(t *testing.T) {
	files := map[string]string{"a.txt": "zorblax\n", ".hygiene-allow": "a.txt: invented\n"}
	list := writeList(t, t.TempDir(), privateList)
	if r := scan(t, files, map[string]string{"AGENTIK_HYGIENE_PRIVATE": list}); r.code != 0 {
		t.Errorf("allowed private hit: exit %d, want 0:\n%s%s", r.code, r.stdout, r.stderr)
	}
	missing := filepath.Join(t.TempDir(), "none.tsv")
	if r := scan(t, files, map[string]string{"AGENTIK_HYGIENE_PRIVATE": missing}); r.code != 0 {
		t.Errorf("allow names an absent group: exit %d, want 0:\n%s%s", r.code, r.stdout, r.stderr)
	}
	bad := writeList(t, t.TempDir(), "invented zorblax-no-tab\n")
	r := scan(t, map[string]string{"a.txt": "clean\n"}, map[string]string{"AGENTIK_HYGIENE_PRIVATE": bad})
	if r.code != 1 || !strings.Contains(r.stderr, ":1: bad line") || strings.Contains(r.stderr, "zorblax") {
		t.Errorf("bad line: exit %d, want 1 naming the line only; stderr %q", r.code, r.stderr)
	}
}
