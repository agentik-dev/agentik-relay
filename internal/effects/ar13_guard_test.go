package effects

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAR13_6_PushurlLandsNothing: the braces under the hook's belt (ADV6-6).
// A real clone of a temp bare remote, run under ChildEnv's environment:
// `git push --no-verify` fails for every remote of the root, one with its
// own pushurl in the repo's config too, and the bare remote's refs are
// unchanged.
func TestAR13_6_PushurlLandsNothing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	bare, clone := filepath.Join(dir, "bare.git"), filepath.Join(dir, "clone")
	base := append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	run := func(env []string, wd string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = wd, env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	must := func(wd string, args ...string) string {
		out, err := run(base, wd, args...)
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return out
	}
	must(dir, "init", "-q", "--bare", "-b", "main", bare)
	must(dir, "clone", "-q", bare, clone)
	must(clone, "commit", "-q", "--allow-empty", "-m", "one")
	must(clone, "push", "-q", "origin", "HEAD:main")
	must(clone, "remote", "add", "mirror", "/nowhere")
	must(clone, "config", "remote.mirror.pushurl", bare)
	before := must(bare, "for-each-ref")
	must(clone, "commit", "-q", "--allow-empty", "-m", "two")

	env := ChildEnv(base, "r1", "", "", 80, []string{"origin", "mirror"}, "")
	for _, r := range []string{"origin", "mirror"} {
		if out, err := run(env, clone, "push", "--no-verify", r, "HEAD:main"); err == nil {
			t.Errorf("push --no-verify %s landed:\n%s", r, out)
		}
		if out, err := run(env, clone, "push", "--no-verify", r, "HEAD:refs/heads/side"); err == nil {
			t.Errorf("push --no-verify %s side landed:\n%s", r, out)
		}
	}
	if after := must(bare, "for-each-ref"); after != before {
		t.Fatalf("the bare remote moved:\n%s\nwas\n%s", after, before)
	}
}

// TestAR13_6_DeviceVerbsReachChild: RELAY_DEVICE_VERBS alone of the
// parent's RELAY_* reaches the child (ADV6-11), so its hook's device guard
// has the owner's verbs; a parent's own GIT_CONFIG_* entries stay, ours
// after them.
func TestAR13_6_DeviceVerbsReachChild(t *testing.T) {
	in := []string{"HOME=/h", "RELAY_DEVICE_VERBS=devctl,bmc", "RELAY_PLUGIN=x", "RELAY_ROW=old",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=user.name", "GIT_CONFIG_VALUE_0=k"}
	got := ChildEnv(in, "r1", "", "", 80, []string{"origin"}, "")
	want := []string{"HOME=/h", "RELAY_DEVICE_VERBS=devctl,bmc",
		"GIT_CONFIG_KEY_0=user.name", "GIT_CONFIG_VALUE_0=k",
		"RELAY_ROW=r1", MeasureDepthVar + "=1", SessionDepthVar + "=1", SessionTurnsVar + "=80",
		"GIT_CONFIG_KEY_1=remote.origin.pushurl", "GIT_CONFIG_VALUE_1=",
		"GIT_CONFIG_KEY_2=remote.origin.pushurl", "GIT_CONFIG_VALUE_2=" + DeniedPushURL,
		"GIT_CONFIG_COUNT=3"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ChildEnv =\n%q\nwant\n%q", got, want)
	}
}
