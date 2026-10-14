package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// TestAR56_4 (AR56 ruling 2): a night's red row r1 the owner merged by hand
// --no-ff after it, with no `merge` line: `status --json` (its status.json
// as the night wrote it) and `report --json` (the ledger) both read r1
// merged at git's sha, and neither prints `agentik relay resume r1`.
func TestAR56_4(t *testing.T) {
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_GLOBAL": os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}
	getenv = fakeEnv(t.TempDir())
	t.Cleanup(func() { getenv = os.Getenv })
	root := filepath.Join(t.TempDir(), "root")
	g := func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g(filepath.Dir(root), "init", "-q", "-b", "main", root)
	g(root, "commit", "-q", "--allow-empty", "-m", "init")
	g(root, "checkout", "-q", "-b", "r1")
	g(root, "commit", "-q", "--allow-empty", "-m", "r1 work")
	g(root, "checkout", "-q", "main")
	g(root, "merge", "-q", "--no-ff", "-m", "merge r1", "r1")
	sha := g(root, "rev-parse", "HEAD")

	const n = "2026-10-03-1"
	state := effects.StateDir(getenv)
	var day bytes.Buffer
	for _, l := range []map[string]any{
		{"event": "night-open", "night": n, "root": root},
		{"event": "stage", "night": n, "row": "r1", "branch": "r1", "base": "main"},
		{"event": "sitting", "night": n, "row": "r1", "state": "red", "reason": "acceptance"},
	} {
		b, _ := json.Marshal(l)
		day.Write(append(b, '\n'))
	}
	st := night.Status{Night: n, State: "stopped", Reason: "queue-empty", Root: root, Resume: []string{"agentik relay resume r1"},
		Rows: []night.RowStatus{{ID: "r1", State: "red", Reason: "acceptance", Resume: "agentik relay resume r1", Acts: []night.Act{}}}}
	sj, _ := json.Marshal(st)
	for path, body := range map[string][]byte{
		filepath.Join(effects.LedgerDir(state), "2026-10-03.jsonl"): day.Bytes(),
		filepath.Join(night.NightDir(state, n), "status.json"):      sj,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	do := func(verb string) string {
		t.Helper()
		var out, errb bytes.Buffer
		if code := run([]string{verb, "--json", "--night", n}, strings.NewReader(""), &out, &errb); code != exitOK {
			t.Fatalf("%s --json = %d: %s", verb, code, errb.String())
		}
		if strings.Contains(out.String(), "agentik relay resume r1") {
			t.Fatalf("%s --json names agentik relay resume r1: %s", verb, out.String())
		}
		return out.String()
	}
	var gotSt night.Status
	if out := do("status"); json.Unmarshal([]byte(out), &gotSt) != nil || len(gotSt.Rows) != 1 ||
		gotSt.Rows[0].MergeSHA != sha || gotSt.Rows[0].State != "red" || len(gotSt.Resume) != 0 {
		t.Fatalf("status --json, want r1 red at %s: %s", sha, out)
	}
	var rep night.Report
	if out := do("report"); json.Unmarshal([]byte(out), &rep) != nil ||
		!reflect.DeepEqual(rep.Merged, []night.Merged{{Row: "r1", SHA: sha}}) || len(rep.Resume) != 0 {
		t.Fatalf("report --json, want r1 merged at %s: %s", sha, out)
	}
}
