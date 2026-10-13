package hooks

import (
	"encoding/json"
	"strings"
	"testing"
)

// hookSays is the PreToolUse answer for cmd: the guard's name when it
// denies, "" when it allows; inRow sets RELAY_ROW as ChildEnv does.
func hookSays(t *testing.T, cmd string, inRow bool) string {
	t.Helper()
	env := func(k string) string {
		if k == "RELAY_ROW" && inRow {
			return "ar17b-guards-r10"
		}
		return ""
	}
	in, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": cmd}})
	b, _ := json.Marshal(preToolUse(in, env))
	s := string(b)
	if s == "null" {
		return ""
	}
	_, why, ok := strings.Cut(s, `"permissionDecisionReason":"relay: `)
	if !ok {
		t.Fatalf("preToolUse(%q) = %s", cmd, s)
	}
	name, _, _ := strings.Cut(why, ":")
	return name
}

// TestAR17B_0_PushDoorsDenied: every push spelling ADV10-3 landed on a bare
// remote from a child's env is denied in a row: git's other push doors, a
// command word that is not a literal word, the wrappers Segments unwraps,
// and a shell fed from stdin, a here-string, a heredoc or a pipe.
func TestAR17B_0_PushDoorsDenied(t *testing.T) {
	for _, cmd := range []string{
		"git send-pack /tmp/bare.git HEAD:refs/heads/main",
		"git http-push https://x/y.git main",
		"git subtree push --prefix=x origin main",
		"/usr/libexec/git-core/git-send-pack /tmp/bare.git main",
		"$(echo git) push --no-verify",
		"`echo git` push",
		"g=git; $g push",
		"\"$g\" push origin",
		"${g} push",
		"git $(echo push) origin",
		"git \"$(echo push)\" origin",
		"git push --no-verify file:///tmp/bare.git HEAD:main",
		"env -S 'git push' origin",
		"env -S'git push'",
		"env --split-string='git push'",
		"timeout 5 env -S 'git push'",
		"echo push | xargs git",
		"printf 'push\\n' | xargs -I{} git {} origin",
		"bash <<< 'git push'",
		"bash <<<'git push'",
		"sh <<EOF\ngit push\nEOF",
		"echo 'git push' | bash",
		"echo 'git push' | sh -s",
		"bash < /tmp/push.sh",
		"echo 'git push' | xargs sh -c",
		"g?t push",
		"{git,push}",
		"echo 'git push' | bash -euo pipefail",
		"if git push; then :; fi",
	} {
		if got := hookSays(t, cmd, true); got == "" {
			t.Errorf("in a row, %q is allowed", cmd)
		}
	}
	// git's other push doors are the push guard's (in a row only, AR20 act 6).
	for _, cmd := range []string{"git send-pack x", "git http-push x", "git subtree push --prefix=a o m"} {
		if got := hookSays(t, cmd, true); got != "push" {
			t.Errorf("in a row, %q = %q, want push", cmd, got)
		}
	}
}

// TestAR17B_0_LiteralWordsStillAllowed: a sitting's ordinary commands stay
// allowed in a row, and the by-shape rules are absent outside one.
func TestAR17B_0_LiteralWordsStillAllowed(t *testing.T) {
	for _, cmd := range []string{
		"git status",
		"go test ./internal/x -run T",
		"make fmt",
		"git diff --stat main...HEAD",
		"echo $HOME",
		"FOO=$(date) make fmt",
		"bash scripts/bounded make check-unlocked",
		"bash -c 'go vet ./...'",
		"git commit -m \"act 0: $(echo ok)\"",
		"ls *.go",
		"git log --format='%h %s' -3",
		"xargs grep git < /dev/null",
		"cat <<EOF > x\ngit push\nEOF",
		"[ -f go.mod ] && make fmt",
		"bash -euo pipefail scripts/x",
	} {
		if got := hookSays(t, cmd, true); got != "" {
			t.Errorf("in a row, %q denied by %s", cmd, got)
		}
	}
	for _, cmd := range []string{"$(echo ls)", "echo x | bash"} {
		if got := hookSays(t, cmd, false); got != "" {
			t.Errorf("outside a row, %q denied by %s", cmd, got)
		}
	}
}

// TestAR17B_0_VerdictIsTheEntry: Verdict and CommandWordLiteral read a
// command as the hook does.
func TestAR17B_0_VerdictIsTheEntry(t *testing.T) {
	if deny, why := Verdict("$(echo git) push", true); !deny || !strings.HasPrefix(why, "relay: literal: ") {
		t.Errorf("Verdict in a row = %v %q", deny, why)
	}
	if deny, _ := Verdict("$(echo git) push", false); deny {
		t.Error("Verdict outside a row denies a non-literal word")
	}
	if CommandWordLiteral("m=make; $m check") || !CommandWordLiteral("make fmt && git status") {
		t.Error("CommandWordLiteral")
	}
}

// TestAR17B_1_UpdateRefDenied: in a row nothing but the gate moves a
// branch or re-points the hooks (ADV10-1's door, ADV10-11): update-ref, a
// forced branch move or delete, another tree named by -C, --git-dir or
// --work-tree or git's environment, and any core.hooksPath write.
func TestAR17B_1_UpdateRefDenied(t *testing.T) {
	for _, cmd := range []string{
		"git update-ref refs/heads/main HEAD",
		"git branch -f main HEAD",
		"git branch -M main",
		"git branch -D main",
		"git branch --force main HEAD",
		"git checkout -B main",
		"git switch -C main",
		"git fetch . HEAD:main",
		"git -C /srv/root merge ar17b",
		"git --git-dir=/r/.git commit -m x",
		"git --work-tree /r status",
		"GIT_DIR=/r/.git git commit -m x",
		"export GIT_WORK_TREE=/r",
		"env -u GIT_CONFIG_COUNT git commit -m x",
		"GIT_CONFIG_COUNT=0 git commit -m x",
		"unset GIT_CONFIG_COUNT",
		"git config core.hooksPath /tmp/none",
		"git config --worktree --unset core.hookspath",
		"git -c core.hooksPath=/dev/null commit -m x",
	} {
		if got := hookSays(t, cmd, true); got == "" {
			t.Errorf("in a row, %q is allowed", cmd)
		}
	}
	for _, cmd := range []string{"git branch -a", "git branch --show-current", "git checkout -b x", "git fetch origin",
		"git config user.name", "grep -n GIT_CONFIG_COUNT internal/effects/spawn.go"} {
		if got := hookSays(t, cmd, true); got != "" {
			t.Errorf("in a row, %q denied by %s", cmd, got)
		}
	}
}

// TestAR17B_2_GoRunRelayDenied: in a row the nested-run guard reads the
// argv, not the spelling (ADV10-6, -18): a cmd/relay path or a binary named
// relay with run, plan, gate or resume after it, however env was trimmed.
func TestAR17B_2_GoRunRelayDenied(t *testing.T) {
	for _, cmd := range []string{
		"env -u RELAY_ROW -u RELAY_MEASURE_DEPTH go run ./cmd/relay run --rows x",
		"go run example.com/x/cmd/relay plan",
		"go run ./cmd/relay/ --json run",
		"relay resume ar17b-guards-r10",
		"relay gate ar17b-guards-r10",
		"xargs relay run < rows",
		"./relay run",
		"env -u RELAY_MEASURE_DEPTH relay plan --rows x",
		"RELAY_ROW= relay plan --rows x",
		"env -u RELAY_ROW go run ./cmd/relay resume",
		"go build -o bin/relay ./cmd/relay", // AR20 act 4 (ADV11-9): the built binary runs under any name
	} {
		if got := hookSays(t, cmd, true); got != "nested-run" {
			t.Errorf("in a row, %q = %q, want nested-run", cmd, got)
		}
	}
	for _, cmd := range []string{
		"relay plan --rows a",
		"relay status",
		"go test ./cmd/relay -run TestAR17B_ -count=1",
		"go run ./cmd/relay status",
		"grep -n run cmd/relay/main.go",
		"echo relay run",
	} {
		if got := hookSays(t, cmd, true); got != "" {
			t.Errorf("in a row, %q denied by %s", cmd, got)
		}
	}
}

// TestAR17B_4_HistoryRewritesDenied: the three spellings ADV10-20 landed,
// and their kin, are denied by the amend and rebase guards in and out of a
// row; a reset that keeps history stays allowed.
func TestAR17B_4_HistoryRewritesDenied(t *testing.T) {
	for cmd, want := range map[string]string{
		"git reset --soft HEAD~1":                    "amend",
		"git reset -q --sof HEAD^":                   "amend",
		"git commit-tree HEAD^{tree} -p HEAD~2 -m x": "amend",
		"git -c pull.rebase=true pull":               "rebase",
		"git -c pull.rebase=interactive pull origin": "rebase",
		"git -c branch.main.rebase=true pull":        "rebase",
		"git config pull.rebase true":                "rebase",
		"git pull --rebase":                          "rebase",
	} {
		for _, inRow := range []bool{false, true} {
			if got := hookSays(t, cmd, inRow); got != want {
				t.Errorf("row %v: %q = %q, want %q", inRow, cmd, got, want)
			}
		}
	}
	for _, cmd := range []string{"git reset HEAD f", "git reset -q", "git pull --ff-only", "git -c color.ui=never log"} {
		if got := hookSays(t, cmd, true); got != "" {
			t.Errorf("%q denied by %s", cmd, got)
		}
	}
}
