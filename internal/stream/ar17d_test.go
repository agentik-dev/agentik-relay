package stream

import (
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR17D_0_AccountTable: the one table names usage-limit and auth from a
// refused rate_limit_event or an is_error result's text, nothing from an
// allowed limit, a clean result, the sitting's own caps, or a number that
// only holds 401; and the tally keeps the first.
func TestAR17D_0_AccountTable(t *testing.T) {
	for _, c := range []struct {
		ev   map[string]any
		want relay.StopReason
	}{
		{map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"status": "rejected"}}, relay.UsageLimit},
		{map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"status": "allowed_warning"}}, ""},
		{map[string]any{"type": "rate_limit_event", "status": "allowed"}, ""},
		{map[string]any{"type": "rate_limit_event"}, ""},
		{map[string]any{"type": "result", "is_error": true, "result": "Claude AI usage limit reached|1758000000"}, relay.UsageLimit},
		{map[string]any{"type": "result", "is_error": true, "result": "API Error: 429 rate_limit_error"}, relay.UsageLimit},
		{map[string]any{"type": "result", "is_error": true, "result": "Invalid API key · Please run /login"}, relay.Auth},
		{map[string]any{"type": "result", "is_error": true, "result": "API Error: 401 OAuth token has expired"}, relay.Auth},
		{map[string]any{"type": "result", "is_error": true, "result": "wrote 4012 lines"}, ""},
		{map[string]any{"type": "result", "is_error": false, "result": "fixed the login page's rate limit"}, ""},
		{map[string]any{"type": "result", "subtype": "error_max_turns", "is_error": true, "result": "the login form"}, ""},
		{map[string]any{"type": "assistant", "is_error": true, "result": "usage limit"}, ""},
	} {
		if got := Account(c.ev); got != c.want {
			t.Errorf("Account(%v) = %q, want %q", c.ev, got, c.want)
		}
	}
	src := `{"type":"system","subtype":"init","session_id":"s"}
{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}
{"type":"rate_limit_event","rate_limit_info":{"status":"rejected"}}
{"type":"result","subtype":"success","is_error":true,"result":"Please run /login"}
`
	tl, err := Parse(strings.NewReader(src), nil)
	if err != nil || tl.Account != relay.UsageLimit {
		t.Fatalf("tally account %q, %v; want the first, usage-limit", tl.Account, err)
	}
}
