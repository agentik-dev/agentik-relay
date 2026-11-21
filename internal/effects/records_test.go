package effects

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRR6_1_Declares (packet rr6-relay-records, R1): the declaration is
// the whole line `records: out`, read as agentik-commons' `_declares`
// reads it: a row whose trailing white space is stripped equals the line.
// A trailing word, a longer line or a leading blank declares nothing.
func TestRR6_1_Declares(t *testing.T) {
	for text, want := range map[string]bool{
		"records: out":                        true,
		"records: out\n":                      true,
		"# allow rows\nrecords: out\nx: y\n":  true,
		"records: out \t\n":                   true,
		"records: out\r\n":                    true,
		"a\rrecords: out\rb":                  true,
		"records: out now\n":                  false,
		"# records: out\n":                    false,
		" records: out\n":                     false,
		"records:out\n":                       false,
		"Records: out\n":                      false,
		"":                                    false,
		"records: outside\n":                  false,
		"allow: records: out, and more\n":     false,
		"records: out\u00a0\n":                true,
		"records: out\x1f\n":                  true,
		"records: out\x1c":                    true,
		"records: out\u3000\n":                true,
		"records: out\u200b\n":                false,
		"records: out\u2028next line follows": true,
	} {
		if got := Declares(text); got != want {
			t.Errorf("Declares(%q) = %v; want %v", text, got, want)
		}
	}
}

// rr6Git runs git in dir as a test's own git: no system or global config.
func rr6Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// TestRR6_2_RecordsOut (R1): one question through the hermetic runner, in
// real temp repos. No key and no declaration: own. The key set, to any
// value (a folder, a missing path, a relative one, the empty string):
// out. `records: out` committed in `.hygiene-allow`: out; the same line
// only in the working tree: own. A folder in no repo: own (a git failure
// is the repo's own). A worktree of the clone sees the clone's key. A
// value only in GIT_CONFIG_COUNT or the global file: not seen.
func TestRR6_2_RecordsOut(t *testing.T) {
	fresh := func() string {
		dir := filepath.Join(t.TempDir(), "top")
		os.MkdirAll(dir, 0o755)
		rr6Git(t, dir, "init", "-q", "-b", "main")
		os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644)
		rr6Git(t, dir, "add", "README.md")
		rr6Git(t, dir, "commit", "-q", "-m", "one")
		return dir
	}
	if dir := fresh(); RecordsOut(dir) {
		t.Errorf("no key, no declaration: out; want own")
	}
	for _, v := range []string{t.TempDir(), "/nonexistent-rr6/top", "records/top", ""} {
		dir := fresh()
		rr6Git(t, dir, "config", "--local", RecordsKey, v)
		if !RecordsOut(dir) {
			t.Errorf("the key at %q: own; want out", v)
		}
	}
	dir := fresh()
	os.WriteFile(filepath.Join(dir, DeclaredFile), []byte("# rows\n"+DeclaredLine+"\n"), 0o644)
	if RecordsOut(dir) {
		t.Errorf("the declaration untracked: out; want own")
	}
	rr6Git(t, dir, "add", DeclaredFile)
	if RecordsOut(dir) {
		t.Errorf("the declaration staged, not committed: out; want own")
	}
	rr6Git(t, dir, "commit", "-q", "-m", "declare")
	if !RecordsOut(dir) {
		t.Errorf("the declaration committed: own; want out")
	}
	os.WriteFile(filepath.Join(dir, DeclaredFile), []byte("# rows\n"), 0o644)
	if !RecordsOut(dir) {
		t.Errorf("the declaration committed, removed in the working tree only: own; want out")
	}
	if RecordsOut(t.TempDir()) {
		t.Errorf("a folder in no repo: out; want own")
	}

	dir = fresh()
	wt := filepath.Join(t.TempDir(), "wt")
	rr6Git(t, dir, "worktree", "add", "-q", "-b", "side", wt)
	rr6Git(t, dir, "config", "--local", RecordsKey, t.TempDir())
	if !RecordsOut(wt) {
		t.Errorf("a worktree of a clone with the key: own; want out")
	}

	dir = fresh()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", RecordsKey)
	t.Setenv("GIT_CONFIG_VALUE_0", "/from/the/env")
	global := filepath.Join(t.TempDir(), "gitconfig")
	os.WriteFile(global, []byte("[agentik]\n\trecords = /from/the/global/file\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	if got := rr6Git(t, dir, "config", "--get-all", RecordsKey); got != "/from/the/global/file\n/from/the/env\n" {
		t.Fatalf("the premise: a plain git reads %q", got)
	}
	if RecordsOut(dir) {
		t.Errorf("a value only in the env and the global file: out; want own")
	}
}

// TestRR6_3_KeyPathCannotExpand (the coordinator's ruling, 2026-11-20): a
// key that is set but that `git config --path` cannot expand (`~nouser/x`,
// git exit 128) is out like any set key, since commons reads it broken,
// never the repo's own. A git failure that is not about the key (a clone
// whose config git cannot parse) stays the repo's own, as R1 has it.
func TestRR6_3_KeyPathCannotExpand(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "top")
	os.MkdirAll(dir, 0o755)
	rr6Git(t, dir, "init", "-q", "-b", "main")
	rr6Git(t, dir, "config", "--local", RecordsKey, "~nouser/x")
	if err := exec.Command("git", "-C", dir, "config", "--local", "--path", "--get", RecordsKey).Run(); err == nil {
		t.Fatalf("the premise: git config --path expands ~nouser/x")
	}
	if !RecordsOut(dir) {
		t.Errorf("the key at ~nouser/x: own; want out")
	}
	f, err := os.OpenFile(filepath.Join(dir, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("[broken\n")
	f.Close()
	if RecordsOut(dir) {
		t.Errorf("a config git cannot parse, the key in it: out; want own")
	}
}
