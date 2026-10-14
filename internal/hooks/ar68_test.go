package hooks

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// ar68Hook answers one event of session s68 in /repo on fx, as source.
func ar68Hook(t *testing.T, event, source string, env map[string]string, fx *Fake) string {
	t.Helper()
	in, err := json.Marshal(map[string]any{"session_id": "s68", "cwd": "/repo", "source": source, "stop_hook_active": false})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	Run(event, bytes.NewReader(in), &out, func(k string) string { return env[k] }, fx)
	return strings.TrimSpace(out.String())
}

// ar68Head is the head start.json holds for s68, "" when none.
func ar68Head(t *testing.T, fx *Fake) string {
	t.Helper()
	var st state
	b, ok := fx.Files["/state/agentik-relay/sessions/s68/start.json"]
	if !ok {
		return ""
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return st.Head
}

// TestAR68_2_StartHookCut (AR68 ruling 2): in a row, HEAD at sha1, a
// startup start with RELAY_HEAD_BEFORE at sha0 records sha0, so a
// continued part with nothing left to commit stops on its clean tree
// unblocked; without RELAY_HEAD_BEFORE it records sha1 and Stop blocks
// no-commit, as before. A resume with an old head keeps it; one without
// records RELAY_HEAD_BEFORE as a startup does.
func TestAR68_2_StartHookCut(t *testing.T) {
	world := func() *Fake {
		return &Fake{Porcelain: map[string]string{"/repo": ""}, Heads: map[string]string{"/repo": "sha1"}, Clock: clock}
	}
	row := map[string]string{"XDG_STATE_HOME": "/state", "RELAY_ROW": "r1", "RELAY_HEAD_BEFORE": "sha0"}

	fx := world()
	ar68Hook(t, "SessionStart", "startup", row, fx)
	if h := ar68Head(t, fx); h != "sha0" {
		t.Fatalf("startup with RELAY_HEAD_BEFORE=sha0 recorded %q, want sha0", h)
	}
	if got := ar68Hook(t, "Stop", "", row, fx); got != "{}" {
		t.Fatalf("Stop on the clean tree after the cut = %s, want {}", got)
	}

	fx = world()
	bare := map[string]string{"XDG_STATE_HOME": "/state", "RELAY_ROW": "r1"}
	ar68Hook(t, "SessionStart", "startup", bare, fx)
	if h := ar68Head(t, fx); h != "sha1" {
		t.Fatalf("startup without RELAY_HEAD_BEFORE recorded %q, want sha1", h)
	}
	if got := ar68Hook(t, "Stop", "", bare, fx); !strings.Contains(got, `"decision":"block"`) || !strings.Contains(got, "no-commit: nothing committed since sha1") {
		t.Fatalf("Stop on the clean tree at its start = %s, want a no-commit block", got)
	}

	fx = world()
	fx.Files = map[string][]byte{"/state/agentik-relay/sessions/s68/start.json": []byte(`{"head":"old","cwd":"/repo","started":"2026-09-22T11:00:00Z","packet":"","blocks":1}`)}
	ar68Hook(t, "SessionStart", "resume", row, fx)
	if h := ar68Head(t, fx); h != "old" {
		t.Fatalf("resume over an old head recorded %q, want old", h)
	}
	fx = world()
	ar68Hook(t, "SessionStart", "resume", row, fx)
	if h := ar68Head(t, fx); h != "sha0" {
		t.Fatalf("resume with no old head recorded %q, want sha0", h)
	}
}
