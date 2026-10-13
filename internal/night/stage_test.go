package night

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// stageFakes is a root at /r with origin one ref, a HEAD for the row's
// worktree, the locks in a temp dir and no live claude.
func stageFakes(t *testing.T) (effects.Effects, *effects.FakeGit, *effects.FakeLedger, *RowPlan) {
	t.Helper()
	locks := t.TempDir()
	oldDir, oldLive := LockDir, LiveClaude
	LockDir = func() string { return locks }
	LiveClaude = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() { LockDir, LiveClaude = oldDir, oldLive })
	wt := "/r/.claude/worktrees/a6"
	g := &effects.FakeGit{Heads: map[string]string{wt: "h0"}, Tips: map[string]string{"a5": "b5", "main": "m5"},
		Remote: map[string]string{"refs/remotes/origin/main": "m0"}}
	clock := effects.FakeClock{T: time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)}
	l := &effects.FakeLedger{Clock: clock, Night: "2026-09-22-1"}
	fx := effects.Effects{Git: g, Shell: &effects.FakeShell{}, Clock: clock, FS: &effects.FakeFS{}, Ledger: l}
	return fx, g, l, &RowPlan{ID: "a6", Worktree: wt, Branch: "a6", Base: "a5"}
}

func TestStage(t *testing.T) {
	fx, g, l, rp := stageFakes(t)
	st, err := Stage(fx, rp)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"BranchExists /r a6", "Exclude /r .claude/worktrees/", "WorktreeAdd /r " + rp.Worktree + " a6 a5",
		"InstallPrePush " + rp.Worktree, "RemoteRefs /r", "Remotes /r", "Head " + rp.Worktree,
		"RevParseCommit /r refs/heads/main^{commit}", "RevParseCommit /r refs/heads/a5^{commit}", "RemoteURLs /r origin", "LsRemote /r origin"}
	if !reflect.DeepEqual(g.Calls(), want) {
		t.Fatalf("calls\n got %q\nwant %q", g.Calls(), want)
	}
	if st.HeadBefore != "h0" || st.Origin["refs/remotes/origin/main"] != "m0" || st.Base != "a5" {
		t.Fatalf("staged = %+v", st)
	}
	lines := l.Written()
	if len(lines) != 1 || lines[0]["event"] != "stage" || lines[0]["head_before"] != "h0" {
		t.Fatalf("ledger = %v", lines)
	}
	if back := StagedFromLedger(lines, "a6"); back == nil || back.HeadBefore != "h0" || !reflect.DeepEqual(back.Origin, st.Origin) {
		t.Fatalf("StagedFromLedger = %+v", back)
	}
	if StagedFromLedger(lines, "a7") != nil {
		t.Fatal("StagedFromLedger found a stage line for a row never staged")
	}

	// The lock is held: a second stage of the same path is refused, even
	// with the worktree and branch gone.
	g.WTs, g.Existing = nil, nil
	if _, err := Stage(fx, rp); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("second Stage under a held lock = %v", err)
	}
	if err := st.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err := st.Unlock(); err != nil {
		t.Fatalf("second Unlock = %v", err)
	}
	if _, err := os.Stat(lockPath(rp.Worktree)); !os.IsNotExist(err) {
		t.Fatalf("the lock survived Unlock: %v", err)
	}
}

func TestStageRefuses(t *testing.T) {
	cases := map[string]func(fx effects.Effects, g *effects.FakeGit){
		"worktree": func(fx effects.Effects, g *effects.FakeGit) {
			fx.FS.MkdirAll("/r/.claude/worktrees/a6", 0o755)
		},
		"branch": func(fx effects.Effects, g *effects.FakeGit) { g.Existing = map[string]bool{"a6": true} },
		"live claude": func(effects.Effects, *effects.FakeGit) {
			LiveClaude = func(string) (bool, error) { return true, nil }
		},
		"claude unknown": func(effects.Effects, *effects.FakeGit) {
			LiveClaude = func(string) (bool, error) { return false, errors.New("no lsof") }
		},
		"hook": func(fx effects.Effects, g *effects.FakeGit) {
			g.Err = map[string]error{"InstallPrePush": errors.New("no")}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			fx, g, l, rp := stageFakes(t)
			setup(fx, g)
			if st, err := Stage(fx, rp); err == nil {
				t.Fatalf("Stage = %+v, want a refusal", st)
			}
			if len(l.Written()) != 0 {
				t.Fatalf("a refused stage wrote %v", l.Written())
			}
			if _, err := os.Stat(lockPath(rp.Worktree)); !os.IsNotExist(err) {
				t.Fatalf("a refused stage kept its lock: %v", err)
			}
			if name == "hook" && g.Calls()[len(g.Calls())-1] != "WorktreeRemove /r "+rp.Worktree {
				t.Fatalf("a worktree without its hook was left: %q", g.Calls())
			}
		})
	}
	if _, err := rootOf("/r/elsewhere/a6"); err == nil {
		t.Fatal("rootOf accepted a path outside .claude/worktrees")
	}
}

// TestAR7_LiveClaudeLsofError: lsof's exit 1 with stderr text is an lsof
// error naming the text; exit 1 with both streams empty is no claude.
func TestAR7_LiveClaudeLsofError(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s' \"$LSOF_ERR\" >&2\nexit 1\n"
	if err := os.WriteFile(bin+"/lsof", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	t.Setenv("LSOF_ERR", "lsof: status error on /x: Permission denied")
	live, err := LiveClaude("/r")
	if err == nil || live || !strings.Contains(err.Error(), "Permission denied") || !strings.HasPrefix(err.Error(), "lsof: ") {
		t.Fatalf("stderr text: live=%v err=%v, want lsof: <text>", live, err)
	}

	t.Setenv("LSOF_ERR", "")
	if live, err := LiveClaude("/r"); err != nil || live {
		t.Fatalf("both empty: live=%v err=%v, want no claude", live, err)
	}
}
