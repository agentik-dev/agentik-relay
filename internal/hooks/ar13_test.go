package hooks

import (
	"reflect"
	"testing"
)

// TestAR13_6_GuardsCatchWrappers: every bypass ADV6-6, -11 and -19 list is
// denied: the wrappers that take arguments (timeout, nice -n, stdbuf,
// xargs), find -exec, git's -c alias.* and --no-verify, --amend by any
// prefix git takes, and an interpreter one-liner naming git; plain reads
// stay allowed.
func TestAR13_6_GuardsCatchWrappers(t *testing.T) {
	// In a row: the push guard is a sitting's only (AR20 act 6).
	env := func(k string) string { return map[string]string{"RELAY_DEVICE_VERBS": "devctl", "RELAY_ROW": "r"}[k] }
	gs := guards(env)
	for _, r := range []struct{ cmd, want string }{
		{"timeout 60 git push --no-verify origin HEAD", "push"},
		{"timeout -s KILL 60 git push origin", "push"},
		{"git -c alias.p=push p origin", "alias"},
		{"git -calias.p=push p origin", "alias"},
		{"git --config-env=alias.p=P p", "alias"},
		{"git config alias.p push", "alias"},
		{"git commit --amen -m x", "amend"},
		{"git commit --am -m x", "amend"},
		{"git commit -a --amend", "amend"},
		{"git commit --no-verify -m x", "no-verify"},
		{"git commit --no-veri -m x", "no-verify"},
		{"git commit -an -m x", "no-verify"},
		{"git merge --no-verify x", "no-verify"},
		{"xargs git push < /dev/null", "push"},
		{"echo origin | xargs -n 1 git push", "push"},
		{"xargs -I{} git push {}", "push"},
		{"find . -maxdepth 0 -exec git push \\;", "push"},
		{"find . -execdir git commit --amend {} +", "amend"},
		{"nice -n 10 git push", "push"},
		{"nice -10 git push", "push"},
		{"stdbuf -o0 git push", "push"},
		{"stdbuf -o 0 git push", "push"},
		{"sudo -u k git push", "push"},
		{"env -i PATH=/bin git push", "push"},
		{"env -u HOME git push", "push"},
		{"timeout 5 devctl status", "device"},
		{"sh -c 'timeout 9 git push'", "push"},
		{"bash -lc \"xargs git push\"", "push"},
		{"python3 -c \"import os; os.system('git push')\"", "interpreter"},
		{"python -c 'import subprocess; subprocess.run([\"git\",\"push\"])'", "interpreter"},
		{"perl -e 'system(\"git push\")'", "interpreter"},
		{"node -e \"require('child_process').execSync('git push')\"", "interpreter"},
		{"ruby -e '`git push`'", "interpreter"},
		{"git push", "push"},
		{"git status", ""},
		{"git commit -m 'no --amend here'", ""},
		{"git commit --author x -m y", ""},
		{"git -c user.name=x commit -m y", ""},
		{"timeout 60 go test ./internal/hooks", ""},
		{"find . -name '*.go' -exec grep -l push {} \\;", ""},
		{"python3 -c 'print(1)'", ""},
		{"python3 tools/digit.py", ""},
		{"xargs grep git", ""},
	} {
		g, ok := match(r.cmd, gs)
		if got := map[bool]string{true: g.Name}[ok]; got != r.want {
			t.Errorf("match(%q) = %q, want %q", r.cmd, got, r.want)
		}
	}
	// The exported splitter is the one the guards use (act 7 reads it).
	var lines []string
	for _, s := range Segments("timeout 60 git -C /x push && sh -c 'make check'") {
		lines = append(lines, Normalise(s))
	}
	if want := []string{"git push", "sh -c make_check", "make check"}; !reflect.DeepEqual(lines, want) {
		t.Errorf("Segments+Normalise = %q, want %q", lines, want)
	}
}
