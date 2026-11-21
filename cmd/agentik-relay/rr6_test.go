package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// rr6Queue is a queue of one row, ok-minimal, whose packet sits beside it.
const rr6Queue = "version: 1\ndefaults: {model: m, turns: 10, context: 20000, usd: 5, wall: 1h, merge: merge-on-green, device: never}\nwords: \"\"\nrows:\n" +
	"  - {id: ok-minimal, packet: design/packets/ok-minimal.md}\n"

// rr6Git runs a test's own git in dir: no system config, the TestMain's
// temp HOME for the global file, committing as t.
func rr6Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// rr6Repo is planFakes' top, with its queue and packet, made a real git
// repo with one commit on main (the effects stay planFakes', so the plan
// measures nothing); git is what R1 reads, through the hermetic runner.
// It returns the top and the fake home holding it and the state root.
func rr6Repo(t *testing.T) (top, home string) {
	t.Helper()
	planFakes(t, "main")
	top, _ = toplevel()
	home = getenv("HOME")
	pkt, err := os.ReadFile(filepath.Join(top, "internal", "packet", "testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	for p, text := range map[string]string{"relay/queue.yaml": rr6Queue, "design/packets/ok-minimal.md": string(pkt)} {
		os.MkdirAll(filepath.Dir(filepath.Join(top, p)), 0o755)
		if err := os.WriteFile(filepath.Join(top, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rr6Git(t, top, "init", "-q", "-b", "main")
	rr6Git(t, top, "add", "relay", "design")
	rr6Git(t, top, "commit", "-q", "-m", "the queue")
	return top, home
}

// rr6Plan is `plan --json` in the current top: its exit, stdout and stderr.
func rr6Plan(t *testing.T) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run([]string{"plan", "--json"}, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

// rr6Listing is every path under dirs with its mode and, for a file, its
// bytes' sha256, for a link its target: what "byte-identical" compares.
func rr6Listing(t *testing.T, dirs ...string) string {
	t.Helper()
	var rows []string
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := e.Info()
			if err != nil {
				return err
			}
			row := fmt.Sprintf("%s %v", p, info.Mode())
			switch {
			case info.Mode()&fs.ModeSymlink != 0:
				target, _ := os.Readlink(p)
				row += " -> " + target
			case info.Mode().IsRegular():
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				row += fmt.Sprintf(" %x", sha256.Sum256(b))
			}
			rows = append(rows, row)
			return nil
		})
		if errors.Is(err, fs.ErrNotExist) {
			rows = append(rows, d+" absent")
		} else if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// rr6Refused runs plan, run, gate and resume outside a measurement (so
// each would take the pid file, the lock and the ledger) and wants R2's
// line alone on stderr, nothing on stdout, exit 2, and the fake home (the
// repo, its .git, the state root) and the pid dir as they were.
func rr6Refused(t *testing.T, why string, home string) {
	t.Helper()
	measuring(t)
	pid, err := night.PidDir()
	if err != nil {
		t.Fatal(err)
	}
	state := effects.StateDir(getenv)
	if !strings.HasPrefix(state, home) {
		t.Fatalf("the state root %s is not under the fake home %s", state, home)
	}
	for _, args := range [][]string{{"plan"}, {"plan", "--json"}, {"run"}, {"run", "--dry"}, {"gate", "ok-minimal"}, {"resume", "ok-minimal"}, {"resume"}} {
		before := rr6Listing(t, home, pid)
		var out, errb bytes.Buffer
		code := run(args, strings.NewReader(""), &out, &errb)
		if code != exitUsage || errb.String() != recordsRefusal+"\n" || out.Len() != 0 {
			t.Errorf("%s: %v = %d, stdout %q, stderr %q; want %d and R2's line", why, args, code, out.String(), errb.String(), exitUsage)
		}
		if after := rr6Listing(t, home, pid); after != before {
			t.Errorf("%s: %v wrote:\nbefore\n%s\nafter\n%s", why, args, before, after)
		}
	}
}

// TestRR6_0_RefusalLine pins R2's words.
func TestRR6_0_RefusalLine(t *testing.T) {
	if want := "relay: this repo keeps its records outside its clone (git config agentik.records); the relay does not run in such a repo yet"; recordsRefusal != want {
		t.Errorf("recordsRefusal = %q; want %q", recordsRefusal, want)
	}
}

// TestRR6_1_OwnPlansAsAtThePin (acceptance 1): a repo with no key and no
// declaration plans its small queue exactly as the same tree did before it
// was a git repo (the pin's reading: R1 never asked), valid, exit 0.
func TestRR6_1_OwnPlansAsAtThePin(t *testing.T) {
	planFakes(t, "main")
	top, _ := toplevel()
	pkt, err := os.ReadFile(filepath.Join(top, "internal", "packet", "testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	for p, text := range map[string]string{"relay/queue.yaml": rr6Queue, "design/packets/ok-minimal.md": string(pkt)} {
		os.MkdirAll(filepath.Dir(filepath.Join(top, p)), 0o755)
		os.WriteFile(filepath.Join(top, p), []byte(text), 0o644)
	}
	code0, out0, err0 := rr6Plan(t)
	rr6Git(t, top, "init", "-q", "-b", "main")
	rr6Git(t, top, "add", "relay", "design")
	rr6Git(t, top, "commit", "-q", "-m", "the queue")
	code, out, errs := rr6Plan(t)
	var p night.Plan
	if err := json.Unmarshal([]byte(out), &p); err != nil || code != exitOK || !p.Valid || len(p.Rows) != 1 {
		t.Fatalf("own: plan --json = %d (%v), valid %v, %d rows: %s %s", code, err, p.Valid, len(p.Rows), out, errs)
	}
	if code != code0 || out != out0 || errs != err0 {
		t.Errorf("own: plan --json differs from the tree's before git:\n%d %s %s\n%d %s %s", code0, out0, err0, code, out, errs)
	}
}

// TestRR6_2_KeyRefusesEveryVerb (acceptance 2): the key set to a folder
// that resolves (absolute, existing, outside the clone, named as the main
// checkout is): plan, run, gate and resume each refuse, writing nothing.
func TestRR6_2_KeyRefusesEveryVerb(t *testing.T) {
	top, home := rr6Repo(t)
	records := filepath.Join(t.TempDir(), "top")
	os.MkdirAll(records, 0o755)
	rr6Git(t, top, "config", "--local", effects.RecordsKey, records)
	rr6Refused(t, "a folder that resolves", home)
}

// TestRR6_3_BrokenKeyRefuses (acceptance 3): a key whose path does not
// exist, a relative one and an empty one refuse the same.
func TestRR6_3_BrokenKeyRefuses(t *testing.T) {
	for _, v := range []string{filepath.Join(os.TempDir(), "rr6-gone", "top"), "records/top", ""} {
		t.Run(fmt.Sprintf("%q", v), func(t *testing.T) {
			top, home := rr6Repo(t)
			rr6Git(t, top, "config", "--local", effects.RecordsKey, v)
			rr6Refused(t, "the key at "+v, home)
		})
	}
}

// TestRR6_4_Declaration (acceptance 4): `records: out` committed in
// .hygiene-allow refuses; the line only in the working tree does not; a
// row with trailing white space declares (commons' `_declares` strips
// it), a trailing word or a longer line does not.
func TestRR6_4_Declaration(t *testing.T) {
	commit := func(t *testing.T, text string) (string, string) {
		top, home := rr6Repo(t)
		os.WriteFile(filepath.Join(top, effects.DeclaredFile), []byte(text), 0o644)
		rr6Git(t, top, "add", effects.DeclaredFile)
		rr6Git(t, top, "commit", "-q", "-m", "allow")
		return top, home
	}
	for _, text := range []string{"# rows\nrecords: out\n", "records: out \t\n"} {
		t.Run(fmt.Sprintf("refused %q", text), func(t *testing.T) {
			_, home := commit(t, text)
			rr6Refused(t, fmt.Sprintf("%q committed", text), home)
		})
	}
	for _, text := range []string{"records: out now\n", "# records: out\n", "allow: records: out\n"} {
		t.Run(fmt.Sprintf("own %q", text), func(t *testing.T) {
			commit(t, text)
			if code, out, errs := rr6Plan(t); code != exitOK || strings.Contains(errs, recordsRefusal) || !strings.HasPrefix(out, "{") {
				t.Errorf("%q committed: plan = %d: %s %s", text, code, out, errs)
			}
		})
	}
	t.Run("uncommitted", func(t *testing.T) {
		top, _ := commit(t, "# rows\n")
		os.WriteFile(filepath.Join(top, effects.DeclaredFile), []byte("# rows\nrecords: out\n"), 0o644)
		if code, out, errs := rr6Plan(t); code != exitOK || strings.Contains(errs, recordsRefusal) || !strings.HasPrefix(out, "{") {
			t.Errorf("the line in the working tree only: plan = %d: %s %s", code, out, errs)
		}
	})
}

// TestRR6_5_WorktreeAndEnv (acceptance 5): from a worktree of the clone
// the clone's key is seen and refuses; a value given only through
// GIT_CONFIG_COUNT or the global file, which a plain git reads, is not.
func TestRR6_5_WorktreeAndEnv(t *testing.T) {
	t.Run("worktree", func(t *testing.T) {
		top, home := rr6Repo(t)
		wt := filepath.Join(home, "wt")
		rr6Git(t, top, "worktree", "add", "-q", "-b", "side", wt)
		rr6Git(t, top, "config", "--local", effects.RecordsKey, filepath.Join(t.TempDir(), "top"))
		fakeTop(t, wt)
		rr6Refused(t, "from a worktree", home)
	})
	t.Run("env and global", func(t *testing.T) {
		top, _ := rr6Repo(t)
		t.Setenv("GIT_CONFIG_COUNT", "1")
		t.Setenv("GIT_CONFIG_KEY_0", effects.RecordsKey)
		t.Setenv("GIT_CONFIG_VALUE_0", "/from/the/env")
		global := filepath.Join(t.TempDir(), "gitconfig")
		os.WriteFile(global, []byte("[agentik]\n\trecords = /from/the/global/file\n"), 0o644)
		t.Setenv("GIT_CONFIG_GLOBAL", global)
		if got := rr6Git(t, top, "config", "--get-all", effects.RecordsKey); got != "/from/the/global/file\n/from/the/env\n" {
			t.Fatalf("the premise: a plain git reads %q", got)
		}
		if code, out, errs := rr6Plan(t); code != exitOK || strings.Contains(errs, recordsRefusal) || !strings.HasPrefix(out, "{") {
			t.Errorf("a value only in the env and the global file: plan = %d: %s %s", code, out, errs)
		}
	})
}

// TestRR6_6_Doctor (acceptance 6, R3): doctor's `records` row, after
// `root`, in both states, in text and JSON: ok "in the repo", or warn
// "outside the clone: ...". Doctor still exits 0 with the row warning.
func TestRR6_6_Doctor(t *testing.T) {
	oldLook, oldProbe, oldWT, oldBand, oldVersion := lookPath, claudeProbe, gitWorktree, bandFor, bandVersion
	t.Cleanup(func() {
		lookPath, claudeProbe, gitWorktree, bandFor, bandVersion = oldLook, oldProbe, oldWT, oldBand, oldVersion
	})
	bandFor = func(string, func(string) string) (effects.Band, string) { return nil, "/fake/agentik-session" }
	bandVersion = func(string) (string, error) { return "agentik-session 0.0.0", nil }
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	for _, c := range []struct {
		name, status, detail, text string
		key                        bool
	}{
		{"own", "ok", "in the repo", "ok   records      in the repo\n", false},
		{"out", "warn", "outside the clone: the relay does not run here (git config agentik.records)",
			"warn records      outside the clone: the relay does not run here (git config agentik.records)\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			top, _ := rr6Repo(t)
			if c.key {
				rr6Git(t, top, "config", "--local", effects.RecordsKey, filepath.Join(t.TempDir(), "top"))
			}
			var out, errb bytes.Buffer
			code := run([]string{"doctor", "--json"}, strings.NewReader(""), &out, &errb)
			var rep struct {
				Warned int
				Checks []check
			}
			if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
				t.Fatal(err)
			}
			var names []string
			var row *check
			for i, ch := range rep.Checks {
				names = append(names, ch.Name)
				if ch.Name == "records" {
					row = &rep.Checks[i]
				}
			}
			if code != exitOK || row == nil || row.Status != c.status || row.Detail != c.detail || !row.OK || row.Warn != c.key || row.Remedy != nil {
				t.Fatalf("doctor --json = %d, records %+v: %s", code, row, out.String())
			}
			if at := strings.Index(strings.Join(names, " "), "root records breaker"); at < 0 {
				t.Errorf("the records row is not after root: %v", names)
			}
			out.Reset()
			if code := run([]string{"doctor"}, strings.NewReader(""), &out, &errb); code != exitOK || !strings.Contains(out.String(), "\n"+c.text) {
				t.Errorf("doctor = %d; want the line %q in:\n%s", code, c.text, out.String())
			}
		})
	}
}
