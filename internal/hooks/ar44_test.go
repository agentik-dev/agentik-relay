package hooks

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestAR44_1_QuoteInSubstitution: the end of a `$(…)` is found as the shell
// finds it. Inside it the quoting starts fresh, so a `'…'` span, a `"…"`
// span, a backtick span and a comment each hide a `)`, and a `"` inside it
// nests in an outer double-quoted span; the command after the span the
// shell closes is read, and the command inside is still read.
func TestAR44_1_QuoteInSubstitution(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{"echo \"$(echo \"'\")\" ; git push origin main #'", "push"},
		{"echo \"$(echo \"${x:-\"'\"}\")\" ; git push origin main #'", "push"},
		{"echo \"$(echo ')')\" ; relay run", "nested-run"},
		{"echo \"$(echo \"a)b\")\" ; git push origin main", "push"},
		{"echo \"$(git push origin main)\"", "push"},
		// a substitution nests in a substitution
		{"echo \"$(echo \"$(echo \"'\")\")\" ; git push origin main #'", "push"},
		// a `${…}` inside hides a `)` in zsh and dash, and not in bash 3.2
		{"echo \"$(echo ${x:-)} ; git push origin main)\"", "push"},
		// a backtick span hides a `)`
		{"echo \"$(echo `echo )` `git push origin main`)\"", "push"},
		// a comment hides a `)` to the end of its line; after an operator it
		// is a comment in zsh and dash, and a word in bash 3.2
		{"echo \"$(echo a # )\ngit push origin main)\"", "push"},
		{"echo \"$(echo a;# )\ngit push origin main)\"", "push"},
		{"echo \"$(echo a;# )\" ; git push origin main", "push"},
		// a bracket pair reads a quote the same way, so its `<<` is a shift
		{"true ${x:=\"$(echo \"}\")\" 1<<EOF}\ngit push origin main\nEOF", "push"},
		{"true ${x:=\"${y:-\"}\"}\" 1<<EOF}\ngit push origin main\nEOF", "push"},
		{"echo \"$(date)\"", ""},
		{"echo \"$(echo \"it's\")\"", ""},
		{"echo \"$(echo \"a)b\")\"", ""},
		{"n=\"$(git log --oneline | wc -l)\"", ""},
		{"echo \"$(echo \\\")\" ; echo ok", ""},
		{"echo \"$(echo `date`)\" ; echo ok", ""},
		{"echo \"$(echo a#)\" ; echo ok", ""},
		// AR46: inside a substitution a comment, a `${…}` holding a `)` and
		// a quote inside `"${…}"` are where bash 3.2 reads otherwise, and
		// the tree holds one reading, so each is unreadable
		{"echo \"$(echo a # b\n)\" ; echo ok", "unreadable"},
		{"echo \"$(echo \"${x:-\"a)b\"}\")\" ; echo ok", "unreadable"},
		{"echo \"$(echo ${x:-)})\"", "unreadable"},
	} {
		if got := hookSays(t, tc.cmd, true); got != tc.want {
			t.Errorf("in a sitting, %q = %q, want %q", tc.cmd, got, tc.want)
		}
		if deny, _ := Verdict(tc.cmd, true); deny != (tc.want != "") {
			t.Errorf("Verdict(%q, true) = %v", tc.cmd, deny)
		}
	}
	// bash 3.2 ends the substitution at the `)` inside the `${…}`, and its
	// reading is kept beside zsh's and dash's: the push is a segment (the
	// line is denied `literal` first, for a word the other reading leaves)
	bash := "echo \"$(echo \"a)b\" ${x:-)\" ; git push origin main ; echo \"$(echo })\""
	if !slices.ContainsFunc(Segments(bash), func(seg []string) bool { return Normalise(seg) == "git push origin main" }) {
		t.Errorf("Segments(%q) has no push", bash)
	}
	if deny, _ := Verdict(bash, true); !deny {
		t.Errorf("Verdict(%q, true) = false", bash)
	}
	// what never closes panics nothing; AR46: the parser rejects it, so it
	// is unreadable
	for _, cmd := range []string{"echo \"$(", "echo \"$(echo \"", "echo \"$(echo ${", "echo \"$(echo `", "echo \"$(echo \\",
		"echo \"$(#", "echo $((", "echo ${", "echo $[", "echo \"$(echo '", "echo \"$(echo $'"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Verdict(%q, true) panicked: %v", cmd, r)
				}
			}()
			if got := hookSays(t, cmd, true); got != "unreadable" {
				t.Errorf("in a sitting, %q = %q, want unreadable", cmd, got)
			}
			Segments(cmd)
		}()
	}
}

// panicFx is an Effects whose reads panic, counting each.
type panicFx struct {
	*Fake
	n *int
}

func (p panicFx) StatusPorcelain(string) (string, error) { *p.n++; panic("boom") }
func (p panicFx) ReadFile(string) ([]byte, error)        { *p.n++; panic("boom") }

// TestAR44_2_PanicDenies: a panic while the guards read a Bash command is a
// deny (`unreadable`) when a guard is in play, a sitting's or the helper
// guard for a subagent's call; with neither, and for every other tool and
// event, a panic is still the empty answer.
func TestAR44_2_PanicDenies(t *testing.T) {
	base, pidDir, calls := Guards, PidDir, 0
	t.Cleanup(func() { Guards, PidDir = base, pidDir })
	Guards = append(base[:len(base):len(base)], Guard{Name: "boom", Shape: func([]string) bool { calls++; panic("boom") }})
	PidDir = func() (string, error) { calls++; panic("boom") }
	row := func(k string) string { return map[string]string{"RELAY_ROW": "ar44", "HOME": "/h"}[k] }
	none := func(string) string { return "" }
	fx := panicFx{&Fake{}, &calls}
	// say is answer's JSON, and whether a panic was met on the way
	say := func(event string, in map[string]any, env func(string) string) (string, bool) {
		t.Helper()
		calls = 0
		b, _ := json.Marshal(in)
		out, _ := json.Marshal(answer(event, bytes.NewReader(b), env, fx))
		return string(out), calls > 0
	}
	bash := map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": "echo hi"}}
	helper := map[string]any{"tool_name": "Bash", "agent_id": "a1", "tool_input": map[string]string{"command": "echo hi"}}
	const deny = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"relay: unreadable: `
	for _, tc := range []struct {
		name string
		in   map[string]any
		env  func(string) string
	}{{"a sitting", bash, row}, {"a subagent's call", helper, none}, {"a subagent's call in a sitting", helper, row}} {
		got, met := say("PreToolUse", tc.in, tc.env)
		if !met || !strings.HasPrefix(got, deny) || !strings.Contains(got, "more simply") {
			t.Errorf("%s: answer = %s (panic met: %v), want the unreadable deny", tc.name, got, met)
		}
	}
	if got := hookSays(t, "echo hi", true); got != "unreadable" {
		t.Errorf("in a sitting, hookSays = %q, want unreadable", got)
	}
	// no guard in play, another tool, another event: the empty answer
	for _, tc := range []struct {
		name, event string
		in          map[string]any
		env         func(string) string
	}{
		{"no sitting and no subagent", "PreToolUse", bash, none},
		{"Write in a sitting", "PreToolUse", map[string]any{"tool_name": "Write", "tool_input": map[string]string{"file_path": "/nowhere/x"}}, row},
		{"Stop in a sitting", "Stop", map[string]any{"session_id": "s", "cwd": "/nowhere"}, row},
		{"PostToolUse in a sitting", "PostToolUse", map[string]any{"session_id": "s"}, row},
	} {
		if got, met := say(tc.event, tc.in, tc.env); got != "{}" || !met {
			t.Errorf("%s: answer = %s (panic met: %v), want {} after a panic", tc.name, got, met)
		}
	}
}

// TestAR44_3_PastDepthDenies: past the depth bound the reader drops nothing
// unread: it marks the line unreadable, and a row denies the mark. span
// stops at the same bound, so no input recurses without limit.
func TestAR44_3_PastDepthDenies(t *testing.T) {
	deep := func(n int, inner string) string {
		return "echo " + strings.Repeat("$(echo ", n-1) + "$(" + inner + strings.Repeat(")", n)
	}
	for _, tc := range []struct{ cmd, want string }{
		{deep(5, "git push origin main"), "unreadable"},
		{deep(5, "relay run"), "unreadable"},
		{deep(6, "git push origin main"), "unreadable"},
		{deep(5, "date"), "unreadable"},
		{"echo \"" + deep(5, "date")[5:] + "\"", "unreadable"},
		// what sh -c would run past the bound is unreadable too
		{deep(4, "sh -c 'git push origin main'"), "unreadable"},
		{strings.Repeat("$(", 200000), "unreadable"},
		{strings.Repeat("\"$(", 200000), "unreadable"},
		{deep(1, "git push origin main"), "push"},
		{deep(2, "git push origin main"), "push"},
		{deep(3, "git push origin main"), "push"},
		{deep(4, "git push origin main"), "push"},
		{deep(3, "sh -c 'git push origin main'"), "push"},
		{deep(2, "date"), ""},
		{deep(4, "date"), ""},
		{deep(4, "find . -exec echo {} +"), ""},
	} {
		if got := hookSays(t, tc.cmd, true); got != tc.want {
			t.Errorf("in a sitting, %.80q = %q, want %q", tc.cmd, got, tc.want)
		}
		if deny, _ := Verdict(tc.cmd, true); deny != (tc.want != "") {
			t.Errorf("Verdict(%.80q, true) = %v", tc.cmd, deny)
		}
	}
	// outside a row no guard reads the mark: the owner's session is not denied
	if got := hookSays(t, deep(5, "date"), false); got != "" {
		t.Errorf("outside a sitting, five deep = %q, want no deny", got)
	}
	for _, n := range []int{200000, 6000000} {
		if len(Segments(strings.Repeat("$(", n))) == 0 {
			t.Errorf("Segments of %d open substitutions is empty, want the mark", n)
		}
	}
}

// TestAR44_4_CaseInSubstitution: a `case` in command position inside a
// `$(…)` or a backtick span is denied. Before AR46 it was unreadable, since
// its pattern's `)` ended the substitution for the reader and not for the
// shell; the tree reader holds the case grammar, so its body is read.
func TestAR44_4_CaseInSubstitution(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		deny bool
	}{
		{"echo \"$(case a in a) git push origin main;; esac)\"", true},
		{"echo $(case a in a) git push origin main;; esac)", true},
		{"echo \"$(true; case a in a) relay run;; esac)\"", true},
		{"echo \"`case a in a) git push origin main;; esac`\"", true},
		{"echo `case a in a) git push origin main;; esac`", true},
		{"echo \"$(echo case)\"", false},
		{"echo \"$(echo a) case\"", false},
		{"echo `echo case`", false},
	} {
		if got := hookSays(t, tc.cmd, true); (got != "") != tc.deny {
			t.Errorf("in a sitting, %q = %q, want denied %v", tc.cmd, got, tc.deny)
		}
		if deny, _ := Verdict(tc.cmd, true); deny != tc.deny {
			t.Errorf("Verdict(%q, true) = %v", tc.cmd, deny)
		}
	}
	if got := hookSays(t, "echo \"$(case a in a) git push origin main;; esac)\"", true); got != "push" {
		t.Errorf("the case line = %q, want push", got)
	}
	// at the top level the pattern's `)` ends a segment, so the body is read
	if got := hookSays(t, "case a in a) git push origin main;; esac", true); got != "push" {
		t.Errorf("a top-level case = %q, want push", got)
	}
}
