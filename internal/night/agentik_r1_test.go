package night_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/hooks"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAgentikR1_3_PidDirNewElseOld (agentik-r1 §6.3 c2; ruling 3): the
// pid dir under the linked home is agentik-relay when it exists, else
// agent-relay when it exists, else agentik-relay. The package vars are set
// in-process and restored: no binary is built or run.
func TestAgentikR1_3_PidDirNewElseOld(t *testing.T) {
	oldPort, oldHome := night.LockPortLink, night.PidHomeLink
	t.Cleanup(func() { night.LockPortLink, night.PidHomeLink = oldPort, oldHome })
	home := t.TempDir()
	night.LockPortLink, night.PidHomeLink = "29872", home
	state := filepath.Join(home, ".local", "state")
	nw, old := filepath.Join(state, "agentik-relay"), filepath.Join(state, "agent-relay")
	for _, step := range []struct{ mk, want string }{{"", nw}, {old, old}, {nw, nw}} {
		if step.mk != "" {
			if err := os.MkdirAll(step.mk, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := night.PidDir(); err != nil || got != step.want {
			t.Errorf("after %q: PidDir = %q, %v; want %q", step.mk, got, err, step.want)
		}
	}
}

// TestAgentikR1_3_HooksAndRelayAgree (ruling 3): the soft request the relay
// writes (hooks.SoftPath) lies under the state dir it reads
// (effects.StateDir) whichever spelling exists.
func TestAgentikR1_3_HooksAndRelayAgree(t *testing.T) {
	base := t.TempDir()
	env := func(k string) string {
		if k == "XDG_STATE_HOME" {
			return base
		}
		return ""
	}
	for _, mk := range []string{"", "agent-relay", "agentik-relay"} {
		if mk != "" {
			if err := os.Mkdir(filepath.Join(base, mk), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		soft, dir := hooks.SoftPath(env, "s1"), effects.StateDir(env)
		if !strings.HasPrefix(soft, dir+string(filepath.Separator)) {
			t.Errorf("after %q: SoftPath %q is not under StateDir %q", mk, soft, dir)
		}
	}
	if got := effects.StateDir(env); got != filepath.Join(base, "agentik-relay") {
		t.Errorf("both: StateDir = %q", got)
	}
}

// TestAgentikR1_4_OneResumeLine (ruling 5): relay.ResumeLine writes the one
// resume line and relay.ResumeRow reads it, so no non-test Go file outside
// internal/relay spells it.
func TestAgentikR1_4_OneResumeLine(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") ||
				strings.HasPrefix(p, filepath.Join(root, "internal", "relay")+string(filepath.Separator)) {
				return err
			}
			b, err := os.ReadFile(p)
			if err == nil && strings.Contains(string(b), `relay resume "`) {
				t.Errorf("%s spells the resume line; use relay.ResumeLine or relay.ResumeRow", p)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestAgentikR1_4_ResumeLines (agentik-r1 §4.3; ruling 5): the relay prints
// `agentik relay resume <row>` and reads it and the old `relay resume
// <row>`: a merge clears a status's resume lines in either spelling, and a
// report's red sitting names the new line.
func TestAgentikR1_4_ResumeLines(t *testing.T) {
	if got := relay.ResumeLine("ar1"); got != "agentik relay resume ar1" {
		t.Errorf("ResumeLine = %q", got)
	}
	for _, l := range []string{"agentik relay resume ar1", "relay resume ar1"} {
		if row, ok := relay.ResumeRow(l); !ok || row != "ar1" {
			t.Errorf("ResumeRow(%q) = %q, %v; want ar1", l, row, ok)
		}
	}
	for _, l := range []string{"", "relay resume ", "relay resume: x", "relay resume a b", "usage: relay resume [<row>]"} {
		if row, ok := relay.ResumeRow(l); ok {
			t.Errorf("ResumeRow(%q) = %q; want none", l, row)
		}
	}
	s := night.Status{
		Rows:   []night.RowStatus{{ID: "a", Resume: "relay resume a"}, {ID: "b", Resume: "agentik relay resume b"}},
		Resume: []string{"relay resume a", "agentik relay resume b", "relay resume c"},
	}
	s.TakeMerges(map[string]string{"a": "sha1", "b": "sha2"})
	if !reflect.DeepEqual(s.Resume, []string{"relay resume c"}) || s.Rows[0].Resume != "" || s.Rows[1].Resume != "" {
		t.Errorf("TakeMerges left %q, rows %+v", s.Resume, s.Rows)
	}
	r := night.BuildReport([]map[string]any{
		{"night": "n1", "event": "sitting", "row": "ar1", "state": "red", "reason": "gate"},
	}, "n1")
	if !slices.Contains(r.Resume, "agentik relay resume ar1") {
		t.Errorf("BuildReport's red sitting: Resume = %q", r.Resume)
	}
}
