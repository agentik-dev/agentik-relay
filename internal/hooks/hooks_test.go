package hooks

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/<event>/<case>.out")

const statePathT = "/state/agentik-relay/sessions/t/start.json"

var clock = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

func startJSON(head string, blocks int) []byte {
	return []byte(`{"head":"` + head + `","cwd":"/repo","started":"2026-09-22T11:00:00Z","packet":"","blocks":` + string(rune('0'+blocks)) + "}\n")
}

func TestRun(t *testing.T) {
	big := []byte(strings.Repeat("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde\n", 320))
	row := map[string]string{"RELAY_ROW": "r"}              // Stop acts only in a sitting (AR23)
	depth := map[string]string{"CLAUDE_SESSION_DEPTH": "1"} // and so does SessionStart's state
	table := []struct {
		event, name string
		env         map[string]string
		fx          Fake
		state       string // start.json after the hook; "" skips the check
	}{
		{"PreToolUse", "push", map[string]string{"RELAY_ROW": "r"}, Fake{}, ""},
		{"PreToolUse", "push_after_and", map[string]string{"RELAY_ROW": "r"}, Fake{}, ""},
		{"PreToolUse", "amend", nil, Fake{}, ""},
		{"PreToolUse", "rebase", nil, Fake{}, ""},
		{"PreToolUse", "ssh", nil, Fake{}, ""},
		{"PreToolUse", "device_verb", map[string]string{"RELAY_DEVICE_VERBS": "other, nasctl"}, Fake{}, ""},
		{"PreToolUse", "plain_commit", nil, Fake{}, ""},
		{"PreToolUse", "non_bash", nil, Fake{}, ""},
		{"PreToolUse", "garbage", nil, Fake{}, ""},

		{"Stop", "dirty", row, Fake{Porcelain: map[string]string{"/repo": "?? x\n M y\n"}, Files: map[string][]byte{statePathT: startJSON("abc", 0)}},
			`{"head":"abc","cwd":"/repo","started":"2026-09-22T11:00:00Z","packet":"","blocks":1}`},
		{"Stop", "clean_no_commit", row, Fake{Porcelain: map[string]string{"/repo": ""}, Heads: map[string]string{"/repo": "abc"}, Files: map[string][]byte{statePathT: startJSON("abc", 1)}},
			`{"head":"abc","cwd":"/repo","started":"2026-09-22T11:00:00Z","packet":"","blocks":2}`},
		{"Stop", "clean_commit", row, Fake{Porcelain: map[string]string{"/repo": ""}, Heads: map[string]string{"/repo": "def"}, Files: map[string][]byte{statePathT: startJSON("abc", 0)}},
			`{"head":"abc","cwd":"/repo","started":"2026-09-22T11:00:00Z","packet":"","blocks":0}`},
		{"Stop", "stop_hook_active", row, Fake{Porcelain: map[string]string{"/repo": "?? x\n"}, Files: map[string][]byte{statePathT: startJSON("abc", 0)}}, ""},
		{"Stop", "fourth_nudge", row, Fake{Porcelain: map[string]string{"/repo": "?? x\n"}, Files: map[string][]byte{statePathT: startJSON("abc", 3)}},
			`{"head":"abc","cwd":"/repo","started":"2026-09-22T11:00:00Z","packet":"","blocks":3}`},
		{"Stop", "no_start_json", row, Fake{Porcelain: map[string]string{"/repo": ""}, Heads: map[string]string{"/repo": "abc"}}, ""},
		{"Stop", "no_start_json_dirty", row, Fake{Porcelain: map[string]string{"/repo": "?? x\n"}}, ""},

		{"SessionStart", "with_packet", map[string]string{"CLAUDE_SESSION_DEPTH": "1", "RELAY_PACKET": "design/packets/p.md"},
			Fake{Heads: map[string]string{"/repo": "abc"}, Files: map[string][]byte{"/repo/design/packets/p.md": []byte("# Packet P\n\nBuild the thing.\n")}},
			`{"head":"abc","cwd":"/repo","started":"2026-09-22T12:00:00Z","packet":"design/packets/p.md","blocks":0}`},
		{"SessionStart", "without_packet", depth, Fake{Heads: map[string]string{"/repo": "abc"}},
			`{"head":"abc","cwd":"/repo","started":"2026-09-22T12:00:00Z","packet":"","blocks":0}`},
		{"SessionStart", "oversized", map[string]string{"CLAUDE_SESSION_DEPTH": "1", "RELAY_PACKET": "/abs/big.md"},
			Fake{Heads: map[string]string{"/repo": "abc"}, Files: map[string][]byte{"/abs/big.md": big}}, ""},
		{"SessionStart", "resume_keeps_head", depth, Fake{Heads: map[string]string{"/repo": "def"}, Files: map[string][]byte{statePathT: startJSON("abc", 2)}},
			`{"head":"abc","cwd":"/repo","started":"2026-09-22T11:00:00Z","packet":"","blocks":2}`},
		{"SessionStart", "unwritable", depth, Fake{Heads: map[string]string{"/repo": "abc"}, WriteErr: errors.New("read-only")}, ""},
	}
	for _, r := range table {
		t.Run(r.event+"/"+r.name, func(t *testing.T) {
			base := filepath.Join("testdata", r.event, r.name)
			in, err := os.ReadFile(base + ".json")
			if err != nil {
				t.Fatal(err)
			}
			env := map[string]string{"XDG_STATE_HOME": "/state"}
			for k, v := range r.env {
				env[k] = v
			}
			fx := r.fx
			fx.Clock = clock
			var out bytes.Buffer
			if code := Run(r.event, bytes.NewReader(in), &out, func(k string) string { return env[k] }, &fx); code != 0 {
				t.Fatalf("exit %d, want 0", code)
			}
			if *update {
				if err := os.WriteFile(base+".out", out.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(base + ".out")
			if err != nil {
				t.Fatal(err)
			}
			if out.String() != string(want) {
				t.Fatalf("stdout\n%s\nwant\n%s", out.String(), want)
			}
			if r.state != "" {
				if got := strings.TrimSpace(string(fx.Files[statePathT])); got != r.state {
					t.Fatalf("start.json %s, want %s", got, r.state)
				}
			}
		})
	}
}

func TestRunCannotDecide(t *testing.T) {
	env := func(string) string { return "" }
	for _, r := range []struct{ event, stdin string }{
		{"", `{"session_id":"t"}`},
		{"Unknown", `{"session_id":"t"}`},
		{"PreToolUse", ""},
		{"Stop", "   \n"},
		{"SessionStart", "{"},
		{"Stop", `{"session_id":"t","cwd":"/nowhere","stop_hook_active":false}`},
		{"SessionStart", `{"session_id":"../x","cwd":"/repo","source":"startup"}`},
	} {
		var out bytes.Buffer
		if code := Run(r.event, strings.NewReader(r.stdin), &out, env, &Fake{}); code != 0 || out.String() != "{}\n" {
			t.Errorf("Run(%q, %q) = %d, %q; want 0, {}", r.event, r.stdin, code, out.String())
		}
	}
}

func TestMatch(t *testing.T) {
	gs := guards(func(k string) string {
		switch k {
		case "RELAY_DEVICE_VERBS":
			return "nasctl"
		case "RELAY_ROW": // the push guard is a sitting's only (AR20 act 6)
			return "r"
		}
		return ""
	})
	for _, r := range []struct{ cmd, want string }{
		{"git push", "push"},
		{"git -C some/dir push --force", "push"},
		{"GIT_TRACE=1 git push", "push"},
		{"/usr/bin/git push", "push"},
		{"make check; git push", "push"},
		{"make check || git push", "push"},
		{"git status | cat & git push", "push"},
		{"echo $(git push)", "push"},
		{"echo \"$(git push)\"", "push"},
		{"echo `git push`", "push"},
		{"bash -c 'git push'", "push"},
		{"sh -ec \"git add . && git push\"", "push"},
		{"eval git push", "push"},
		{"(cd x && git push)", "push"},
		{"git commit --amend", "amend"},
		{"git commit -a --amend -m x", "amend"},
		{"git rebase main", "rebase"},
		{"git pull --rebase", "rebase"},
		{"ssh host", "device"},
		{"sudo ssh host", "device"},
		{"/usr/bin/scp a host:b", "device"},
		{"rsync -a a b", "device"},
		{"nasctl status", "device"},
		{"git commit -m 'push, amend and rebase'", ""},
		{"git commit -m \"note --amend\"", ""},
		{"git commit -m \"$(cat <<'EOF'\nguards: deny git push and ssh\nEOF\n)\"", ""},
		{"cat <<EOF > notes.md\ngit push\nssh host\nEOF\ngit status", ""},
		{"cat <<-EOF\n\tgit push\n\tEOF\necho ok", ""},
		{"git stash push -m x", ""},
		{"git log --grep=rebase", ""},
		{"echo ssh host", ""},
		{"grep -r 'git push' .", ""},
		{"make check # then git push", ""},
		{"git commit -m x", ""},
		{"git status", ""},
		{"", ""},
	} {
		g, ok := match(r.cmd, gs)
		if got := map[bool]string{true: g.Name}[ok]; got != r.want {
			t.Errorf("match(%q) = %q, want %q", r.cmd, got, r.want)
		}
	}
}

func TestGuardsWithoutDeviceVerbs(t *testing.T) {
	if _, ok := match("nasctl status", guards(func(string) string { return "" })); ok {
		t.Fatal("nasctl denied without RELAY_DEVICE_VERBS")
	}
}

func TestOS(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	if err := exec.Command("git", "init", "-q", dir).Run(); err != nil {
		t.Fatal(err)
	}
	var fx OS
	if p, err := fx.StatusPorcelain(dir); err != nil || p != "" {
		t.Fatalf("StatusPorcelain = %q, %v", p, err)
	}
	if _, err := fx.Head(dir); err == nil {
		t.Fatal("Head of an empty repo succeeded")
	}
	path := filepath.Join(dir, "state", "a", "start.json")
	if err := fx.WriteFile(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b, err := fx.ReadFile(path); err != nil || string(b) != "x" {
		t.Fatalf("ReadFile = %q, %v", b, err)
	}
	if p, _ := fx.StatusPorcelain(dir); countLines(p) != 1 {
		t.Fatalf("StatusPorcelain after a write = %q, want one path", p)
	}
	if fx.Now().IsZero() {
		t.Fatal("Now is zero")
	}
}
