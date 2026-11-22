package main

// dr3-relay-doctor-folder: outside a git checkout the doctor notes what it
// did not check (R2), a note being the contract's fourth status (R3), and
// its text exit is 0 when no machine check fails (R4); inside one every
// check is as it was (R1).

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// dr3Answer is doctor --json's object.
type dr3Answer struct {
	Failed, Warned int
	Checks         []check
}

// dr3World fakes the machine's programs (all present, the claude probe
// passing), the env (a temp HOME, its state under it) and the pin, and
// makes dir the working folder; git is the real one, so toplevel, the
// worktree list and the root read dir as a person's shell would.
func dr3World(t *testing.T, dir string, env map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	m := map[string]string{"HOME": home, effects.MeasureDepthVar: "1", "XDG_STATE_HOME": filepath.Join(home, "state")}
	for k, v := range env {
		m[k] = strings.ReplaceAll(v, "$HOME", home)
	}
	oldEnv, oldPin, oldLook, oldProbe, oldWT, oldFx, oldTop := getenv, pinFor, lookPath, claudeProbe, gitWorktree, effectsFor, toplevel
	t.Cleanup(func() {
		getenv, pinFor, lookPath, claudeProbe, gitWorktree, effectsFor, toplevel = oldEnv, oldPin, oldLook, oldProbe, oldWT, oldFx, oldTop
	})
	getenv = func(k string) string { return m[k] }
	pinFor = func() (func(), error) { return func() {}, nil } // doctor takes the package's pin (TestMain)
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(repo string) error { return exec.Command("git", "-C", repo, "worktree", "list").Run() }
	effectsFor = func(string, string) effects.Effects { return effects.Effects{Git: effects.OSGit{}, FS: effects.OSFS{}} }
	toplevel = func() (string, error) {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		return strings.TrimSpace(string(out)), err
	}
	t.Chdir(dir)
}

// dr3Home is HOME as the world's getenv reads it.
func dr3Home() string { return getenv("HOME") }

// dr3Doctor runs doctor --json and plain doctor; it returns the object, the
// text and the text's exit, and logs the object for the pin's comparison.
func dr3Doctor(t *testing.T) (dr3Answer, string, int) {
	t.Helper()
	code, out, errs := ak8Text("doctor", "--json")
	var a dr3Answer
	if err := json.Unmarshal([]byte(out), &a); err != nil {
		t.Fatalf("doctor --json (exit %d): %v\n%s%s", code, err, out, errs)
	}
	t.Logf("json: %s", out)
	tcode, text, _ := ak8Text("doctor")
	return a, text, tcode
}

func dr3Check(t *testing.T, a dr3Answer, name string) check {
	t.Helper()
	for _, c := range a.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %s check in %+v", name, a.Checks)
	return check{}
}

// dr3Counts checks the counts against the statuses: a note adds to
// neither (R3), and its booleans read ok true, warn false.
func dr3Counts(t *testing.T, a dr3Answer) {
	t.Helper()
	failed, warned := 0, 0
	for _, c := range a.Checks {
		switch c.Status {
		case "fail":
			failed++
			if c.OK || c.Warn {
				t.Errorf("%s: fail with ok %v, warn %v", c.Name, c.OK, c.Warn)
			}
		case "warn":
			warned++
			if !c.OK || !c.Warn {
				t.Errorf("%s: warn with ok %v, warn %v", c.Name, c.OK, c.Warn)
			}
		case "ok", "note":
			if !c.OK || c.Warn {
				t.Errorf("%s: %s with ok %v, warn %v", c.Name, c.Status, c.OK, c.Warn)
			}
		default:
			t.Errorf("%s: status %q", c.Name, c.Status)
		}
	}
	if a.Failed != failed || a.Warned != warned {
		t.Errorf("failed %d, warned %d; the statuses count %d, %d", a.Failed, a.Warned, failed, warned)
	}
}

// dr3Folder is a temp folder that is no git checkout. A temp folder inside
// a checkout fails the test, never skips it, so the acceptance count never
// reads a skip as a pass (review 1, L7).
func dr3Folder(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	dir = filepath.Join(dir, "folder")
	os.MkdirAll(dir, 0o755)
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").CombinedOutput(); err == nil {
		t.Fatalf("the temp folder lies in a checkout, so no folder outside one can be made: %s", out)
	}
	return dir
}

// TestDR3_1_OutsideACheckoutNotes is acceptance line 1: from a folder that
// is no checkout, git worktree, root, queue and records are notes that say
// so, nothing fails where the machine's fakes pass, and the text exits 0
// with each note's mark `note`.
func TestDR3_1_OutsideACheckoutNotes(t *testing.T) {
	dir := dr3Folder(t)
	dr3World(t, dir, nil)
	a, text, code := dr3Doctor(t)
	dr3Counts(t, a)
	for _, name := range []string{"git worktree", "root", "queue", "records"} {
		c := dr3Check(t, a, name)
		if c.Status != "note" || !strings.Contains(c.Detail, "no git checkout") || strings.Contains(c.Detail, "exit status") {
			t.Errorf("%s = %s %q, want a note saying the folder is no git checkout", name, c.Status, c.Detail)
		}
		if !strings.Contains(text, "note "+name) {
			t.Errorf("the text form marks no note on %s:\n%s", name, text)
		}
	}
	if a.Failed != 0 || code != exitOK {
		t.Errorf("failed %d, text exit %d; want 0 and 0:\n%s", a.Failed, code, text)
	}
	for _, name := range []string{"claude", "git", "caffeinate", "state", "template", "config"} {
		if c := dr3Check(t, a, name); c.Status != "ok" {
			t.Errorf("machine check %s = %s %q", name, c.Status, c.Detail)
		}
	}
}

// TestDR3_2_InsideACheckoutAsBefore is acceptance line 2: a temp checkout
// clean on main, one dirty, one on another branch, each read from a
// folder under its root: the repo's checks and the counts as at the pin.
func TestDR3_2_InsideACheckoutAsBefore(t *testing.T) {
	for _, tc := range []struct {
		name, root, rootDetail string
		failed, warnedOver     int
	}{
		{"clean", "ok", "%s clean, on main", 0, 0},
		{"dirty", "fail", "%s is dirty (1 paths): the baseline and the merge need it clean", 1, 0},
		{"branch", "warn", `%s is clean but on "other", not main: the baseline will be unrun`, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			base, _ := filepath.EvalSymlinks(t.TempDir())
			root := filepath.Join(base, "site")
			ar32bGit(t, base, "init", "-q", "-b", "main", root)
			ar32bGit(t, root, "commit", "-q", "--allow-empty", "-m", "init")
			switch tc.name {
			case "dirty":
				os.WriteFile(filepath.Join(root, "loose.txt"), []byte("x\n"), 0o644)
			case "branch":
				ar32bGit(t, root, "checkout", "-q", "-b", "other")
			}
			sub := filepath.Join(root, "sub")
			os.MkdirAll(sub, 0o755)
			dr3World(t, sub, nil)
			a, text, code := dr3Doctor(t)
			dr3Counts(t, a)
			want := map[string][2]string{
				"git worktree": {"ok", "git worktree list runs in " + root},
				"queue":        {"ok", "none at " + filepath.Join(root, "relay", "queue.yaml")},
				"root":         {tc.root, strings.ReplaceAll(tc.rootDetail, "%s", root)},
				"records":      {"ok", "in the repo"},
				"config":       {"ok", "0 files read, no unknown keys"},
			}
			for name, w := range want {
				if c := dr3Check(t, a, name); c.Status != w[0] || c.Detail != w[1] {
					t.Errorf("%s = %s %q, want %s %q", name, c.Status, c.Detail, w[0], w[1])
				}
			}
			machineWarned := 0
			for _, c := range a.Checks {
				if c.Status == "note" {
					t.Errorf("a note inside a checkout: %+v", c)
				}
				if _, repoCheck := want[c.Name]; !repoCheck && c.Status == "warn" {
					machineWarned++
				}
			}
			if a.Failed != tc.failed || a.Warned != machineWarned+tc.warnedOver {
				t.Errorf("failed %d, warned %d; want %d, %d", a.Failed, a.Warned, tc.failed, machineWarned+tc.warnedOver)
			}
			if wantCode := map[bool]int{true: exitUnwell, false: exitOK}[tc.failed > 0]; code != wantCode {
				t.Errorf("text exit %d, want %d:\n%s", code, wantCode, text)
			}
		})
	}
}

// TestDR3_3_ConfigOutsideReadsTheUserLayer is acceptance line 3: outside a
// checkout a session's resolved file for another checkout warns nothing; a
// user layer with an unknown key warns and one that does not parse fails,
// each as at the pin.
func TestDR3_3_ConfigOutsideReadsTheUserLayer(t *testing.T) {
	const id = "sess-1"
	resolved := "$HOME/state/agentik-session/sessions/" + id + "/resolved.yaml"
	userFile := "$HOME/.config/agentik/relay/config.yaml"
	for _, tc := range []struct {
		name, user, status, detail string
	}{
		{"another checkout's resolved file", "", "ok", "1 files read, no unknown keys"},
		{"an unknown user key", "colour: blue\n", "warn", "unknown keys or values: " + userFile + ": colour"},
		{"a user file that does not parse", "ntfy: [\n", "fail", "config " + userFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := dr3Folder(t)
			dr3World(t, dir, map[string]string{"CLAUDE_CODE_SESSION_ID": id})
			home := dr3Home()
			other := filepath.Join(home, "elsewhere", "site", ".claude", "agentik", "config.yaml")
			ak8Write(t, strings.ReplaceAll(resolved, "$HOME", home), "provider:\n  inputs:\n    - path: "+other+"\n      sha256: null\n    - path: "+filepath.Join(filepath.Dir(other), "config.local.yaml")+"\n      sha256: null\nrelay:\n  tmux: auto\n")
			if tc.user == "" {
				ak8Write(t, strings.ReplaceAll(userFile, "$HOME", home), "tmux: auto\n")
			} else {
				ak8Write(t, strings.ReplaceAll(userFile, "$HOME", home), tc.user)
			}
			a, _, _ := dr3Doctor(t)
			dr3Counts(t, a)
			c := dr3Check(t, a, "config")
			detail := strings.ReplaceAll(tc.detail, "$HOME", home)
			if c.Status != tc.status || !strings.HasPrefix(c.Detail, detail) || strings.Contains(c.Detail, "another checkout") {
				t.Errorf("config = %s %q, want %s from %q", c.Status, c.Detail, tc.status, detail)
			}
		})
	}
}

// TestDR3_4_ACheckoutGitRefusesAsAtThePin is brief 2's R2: only git's own
// "not a git repository" answer puts the doctor outside a checkout. A
// checkout git refuses (another owner, a corrupt config, a .git file that
// points nowhere), git off PATH inside a checkout, a bare repo and a folder
// inside .git each answer as at the pin `7bd9623`: the folder's checks, the
// failed count and the text exit, the repo's config layer read (its
// `tmux: required` fails tmux where tmux is missing), and no note.
func TestDR3_4_ACheckoutGitRefusesAsAtThePin(t *testing.T) {
	repoLayer := filepath.Join(".claude", "agentik", "config.yaml")
	checkout := func(t *testing.T, base string) string {
		root := filepath.Join(base, "site")
		ar32bGit(t, base, "init", "-q", "-b", "main", root)
		ar32bGit(t, root, "commit", "-q", "--allow-empty", "-m", "init")
		return root
	}
	breakConfig := func(t *testing.T, root string) {
		f, err := os.OpenFile(filepath.Join(root, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("[core\n")
		f.Close()
	}
	for _, tc := range []struct {
		name     string
		setup    func(t *testing.T, base string) string // the working folder
		missing  string                                 // a program lookPath does not find
		noPath   bool                                   // git off the process's PATH too
		statuses string                                 // git, git worktree, tmux, queue, root, records, config
		failed   int
		code     int
	}{
		{"another owner", func(t *testing.T, base string) string {
			root := checkout(t, base)
			ak8Write(t, filepath.Join(root, repoLayer), "relay:\n  tmux: required\n")
			ak8Write(t, filepath.Join(root, "loose"), "x\n")
			t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
			return root
		}, "tmux", false, "ok fail fail ok fail ok ok", 3, exitUnwell},
		{"a corrupt config", func(t *testing.T, base string) string {
			root := checkout(t, base)
			ak8Write(t, filepath.Join(root, repoLayer), "relay:\n  tmux: required\n")
			breakConfig(t, root)
			return root
		}, "tmux", false, "ok fail fail ok fail ok ok", 3, exitUnwell},
		{"a .git file that points nowhere", func(t *testing.T, base string) string {
			d := filepath.Join(base, "site")
			ak8Write(t, filepath.Join(d, ".git"), "gitdir: "+filepath.Join(base, "gone")+"\n")
			return d
		}, "", false, "ok fail ok ok fail ok ok", 2, exitUnwell},
		{"git off PATH inside a checkout", func(t *testing.T, base string) string {
			return checkout(t, base)
		}, "git", true, "fail fail ok ok ok ok ok", 2, exitUnwell},
		{"a bare repo", func(t *testing.T, base string) string {
			b := filepath.Join(base, "b.git")
			ar32bGit(t, base, "init", "-q", "--bare", "-b", "main", b)
			return b
		}, "", false, "ok ok ok ok fail ok ok", 1, exitUnwell},
		{"a folder inside .git", func(t *testing.T, base string) string {
			return filepath.Join(checkout(t, base), ".git")
		}, "", false, "ok ok ok ok ok ok ok", 0, exitOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			base, _ := filepath.EvalSymlinks(t.TempDir())
			dir := tc.setup(t, base)
			dr3World(t, dir, nil)
			lookPath = func(name string) (string, error) {
				if name == tc.missing {
					return "", exec.ErrNotFound
				}
				return "/fake/" + name, nil
			}
			if tc.noPath {
				t.Setenv("PATH", t.TempDir())
			}
			a, text, code := dr3Doctor(t)
			dr3Counts(t, a)
			var got []string
			for _, name := range []string{"git", "git worktree", "tmux", "queue", "root", "records", "config"} {
				got = append(got, dr3Check(t, a, name).Status)
			}
			if s := strings.Join(got, " "); s != tc.statuses {
				t.Errorf("statuses %q, the pin's %q:\n%s", s, tc.statuses, text)
			}
			for _, c := range a.Checks {
				if c.Status == "note" || strings.Contains(c.Detail, "no git checkout") {
					t.Errorf("%s reads as outside a checkout: %s %q", c.Name, c.Status, c.Detail)
				}
			}
			if a.Failed != tc.failed || code != tc.code {
				t.Errorf("failed %d, text exit %d; the pin's %d, %d:\n%s", a.Failed, code, tc.failed, tc.code, text)
			}
		})
	}
}
