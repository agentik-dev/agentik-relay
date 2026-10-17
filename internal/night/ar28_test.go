package night_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/hooks"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// streamOf writes a fixture of one init line, one top-level assistant
// message per context (its input tokens) and, when result, a result line.
func streamOf(t *testing.T, result bool, contexts ...int) string {
	t.Helper()
	lines := []string{`{"type":"system","subtype":"init","session_id":"SESSION","model":"claude-opus-5-5"}`}
	for i, c := range contexts {
		lines = append(lines, fmt.Sprintf(`{"type":"assistant","parent_tool_use_id":null,"session_id":"SESSION","message":{"id":"msg_%02d","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"toolu_%02d","name":"Bash","input":{"command":"go test ./internal/night"}}],"usage":{"input_tokens":%d,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":50}}}`, i+1, i+1, c))
	}
	if result {
		lines = append(lines, fmt.Sprintf(`{"type":"result","subtype":"success","is_error":false,"num_turns":%d,"duration_ms":900,"total_cost_usd":1.25,"permission_denials":[],"result":"done","session_id":"SESSION"}`, len(contexts)))
	}
	p := filepath.Join(t.TempDir(), "stream.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (w *world) envAt(fixture string, extra ...string) []string {
	return append(append(os.Environ(), "FAKECLAUDE_FIXTURE="+fixture, "FAKECLAUDE_TRANSCRIPT="+w.transcripts), extra...)
}

// softSeen is a live log that reads the soft request at each of its writes.
type softSeen struct {
	mu   sync.Mutex
	path string
	body []string
	mode []os.FileMode
}

func (s *softSeen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fi, err := os.Stat(s.path); err == nil {
		b, _ := os.ReadFile(s.path)
		s.body, s.mode = append(s.body, string(b)), append(s.mode, fi.Mode().Perm())
	}
	return len(p), nil
}

func linesOf(lines []map[string]any, event string) []map[string]any {
	var out []map[string]any
	for _, l := range lines {
		if l["event"] == event {
			out = append(out, l)
		}
	}
	return out
}

// TestAR28_1_SoftBeforeKill: a stream of 100k, 118k, 131k, 140k and 157k
// of 130k writes the soft request once, at 118k (file and line alike, the
// file 0600 while the child runs and gone after the exit), and kills at
// 157k alone: nothing kills at the whole.
func TestAR28_1_SoftBeforeKill(t *testing.T) {
	fakeClaude(t)
	w := newWorld(t)
	night.KillGrace = 300 * time.Millisecond
	st, rp := w.stage(t, "a1")
	log := &softSeen{path: hooks.SoftPath(os.Getenv, rp.SessionID)}
	s, _ := night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
		Context: 130000, Wall: time.Minute, Log: log, Transcripts: w.transcripts,
		Env: w.envAt(streamOf(t, true, 100000, 118000, 131000, 140000, 157000), "FAKECLAUDE_DELAY_MS=100")})
	if s.State != relay.Failed || s.Reason != relay.CeilingKill {
		t.Fatalf("sitting %+v", s)
	}
	exit := lastOf(w.ledger.Written(), "exit")
	if exit["turns"] != float64(5) || exit["context"] != float64(157000) {
		t.Fatalf("the kill before 157k: exit %v", exit)
	}
	softs := linesOf(w.ledger.Written(), "soft")
	if len(softs) != 1 {
		t.Fatalf("soft lines %v, want one", softs)
	}
	for k, v := range map[string]float64{"turns": 2, "context": 118000, "ceiling": 130000, "soft": 117000} {
		if softs[0][k] != v {
			t.Fatalf("soft line %v: %s want %v", softs[0], k, v)
		}
	}
	if len(log.body) == 0 || log.body[0] != `{"ceiling":130000,"context":118000,"soft":117000,"turns":2}`+"\n" || log.mode[0] != 0o600 {
		t.Fatalf("the soft request while the child ran: %q %v", log.body, log.mode)
	}
	if _, err := os.Stat(log.path); !os.IsNotExist(err) {
		t.Fatalf("the soft request outlived the exit: %v", err)
	}
}

// TestAR28_1_TuneOnExit: the exit line's tune. The request at turn 3, the
// head moved before turn 5, the exit at 140k: {3, 5, 140000, false}. 116k
// then 157k: the request and the kill on one message, no head move read,
// killed. The fake's commit waits for turn 4's head read (AR93): that
// order, not the machine's load, puts the move at turn 5.
func TestAR28_1_TuneOnExit(t *testing.T) { ar28TuneOnExit(t, 0) }

// ar28TuneOnExit is TestAR28_1_TuneOnExit's table, each head read once
// the soft line is written slow late, the fake's commit after it (AR93).
func ar28TuneOnExit(t *testing.T, slow time.Duration) {
	fakeClaude(t)
	for _, c := range []struct {
		name     string
		contexts []int
		extra    []string
		reason   relay.StopReason
		tune     map[string]any
	}{
		{"commit", []int{100000, 110000, 118000, 120000, 125000, 140000}, []string{"FAKECLAUDE_COMMIT_AT=5"}, "",
			map[string]any{"soft_turn": 3.0, "commit_turn": 5.0, "context_exit": 140000.0, "killed": false}},
		{"runaway", []int{116000, 157000}, nil, relay.CeilingKill,
			map[string]any{"soft_turn": 2.0, "commit_turn": 0.0, "context_exit": 157000.0, "killed": true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t)
			done := filepath.Join(t.TempDir(), "read")
			w.fx.Git = &ar93SlowHead{w: w, slow: slow, done: done}
			night.KillGrace = 300 * time.Millisecond
			st, rp := w.stage(t, "a1")
			s, _ := night.RunRow(w.fx, effects.OSSpawner{}, st, rp, nil, night.RunOptions{
				Context: 130000, Wall: time.Minute, Transcripts: w.transcripts,
				Env: w.envAt(streamOf(t, true, c.contexts...), append(c.extra, "FAKECLAUDE_DELAY_MS=150", "FAKECLAUDE_COMMIT_AFTER="+done)...)})
			if s.Reason != c.reason && !(c.reason == "" && s.Reason == relay.GateUnrunReason) {
				t.Fatalf("sitting %+v", s)
			}
			exit := lastOf(w.ledger.Written(), "exit")
			tune, _ := exit["tune"].(map[string]any)
			if fmt.Sprint(tune) != fmt.Sprint(c.tune) {
				t.Fatalf("tune %v, want %v", exit["tune"], c.tune)
			}
			if n := len(linesOf(w.ledger.Written(), "soft")); n != 1 {
				t.Fatalf("soft lines %d", n)
			}
		})
	}
}

// TestAR28_3_CostUnread: a1 is killed before its result: its exit and
// sitting lines carry usage and cost_read false (the sitting's cost_usd
// stays 0); status and report read `unread` and `$0.00 + 1 unread`; the
// night's spend (the stop line's spent, what --max-usd compares) counts it
// at its row's usd, 15. The fake holds its result a minute (AR35): the
// kill lands before it however loaded the machine, where 150 ms raced it.
func TestAR28_3_CostUnread(t *testing.T) {
	w, q, by := nightWorld(t, spec{id: "a1", echo: "ok", fixture: "ok.jsonl"})
	night.KillGrace = 300 * time.Millisecond
	s := by["a1"]
	s.env = []string{"FAKECLAUDE_FIXTURE=" + streamOf(t, true, 116000, 157000), "FAKECLAUDE_DELAY_MS=150", "FAKECLAUDE_RESULT_DELAY_MS=60000"}
	by["a1"] = s
	o := w.opts(t, by)
	o.MaxUSD = 100
	n := runNight(t, w, q, o)
	if st := states(n); st != "a1=failed" || n.Reason != relay.CeilingKill {
		t.Fatalf("states %s, reason %s", st, n.Reason)
	}
	lines := w.ledger.Written()
	for _, ev := range []string{"exit", "sitting"} {
		l := lastOf(lines, ev)
		u, _ := l["usage"].(map[string]any)
		if l["cost_read"] != false || u["input"] != float64(273000) || u["output"] != float64(100) || l["cost_usd"] != nil && l["cost_usd"] != float64(0) {
			t.Fatalf("%s line %v", ev, l)
		}
	}
	var out strings.Builder
	night.PrintStatus(&out, night.ReadStatus(effects.OSFS{}, o.State, n.ID))
	if !strings.Contains(out.String(), " unread ") || !strings.Contains(out.String(), "$0.00 + 1 unread") {
		t.Fatalf("status:\n%s", out.String())
	}
	md := string(night.Markdown(night.BuildReport(lines, n.ID), w.root, os.Getenv("HOME")))
	if !strings.Contains(md, " unread |") || !strings.Contains(md, "$0.00 + 1 unread") {
		t.Fatalf("report:\n%s", md)
	}
	if stop := lastOf(lines, "stop"); stop["unread"] != float64(1) || stop["spent"] != float64(15) || stop["cost_usd"] != float64(0) {
		t.Fatalf("stop line %v", stop)
	}
}
