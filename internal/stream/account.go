package stream

import (
	"regexp"
	"strings"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// accountPatterns is the one table of the account's stops (run 10 ruling
// 1; ADV11-12): an is_error result whose text holds a needle of a row as a
// whole word, lowercased and the first row winning, stops the night with
// that row's reason. Digits are read only in an `API Error: NNN` prefix
// (apiStatus), never in a needle, so a request id holds none; `limit
// reached` counts only after usage, 5-hour, weekly or session. The usage
// row's Words are `hit your <word> limit` and `reached your <word> limit`
// (AR63, usageWords in limit.go), whole words as the needles. The cells
// are internal/stream/testdata/account-cells.tsv, the one table a re-sync copies.
var accountPatterns = []struct {
	Reason  relay.StopReason
	Status  string // the `API Error: NNN` status that names this reason
	Needles []string
	Words   *regexp.Regexp // the row's worded needles, nil with none
}{
	{relay.UsageLimit, "429", []string{"usage limit", "rate limit", "rate_limit", "hit your limit",
		"5-hour limit reached", "weekly limit reached", "session limit reached"}, usageWords},
	{relay.Auth, "401", []string{"/login", "log in", "login", "expired token", "token expired", "token has expired",
		"oauth token", "invalid api key", "authentication", "unauthorized"}, nil},
}

// apiStatus is the status of a line's `API Error: NNN` prefix.
var apiStatus = regexp.MustCompile(`(?m)^\s*api error: ([0-9]{3})([^0-9]|$)`)

// Account is the account stop one raw stream event names, "" when neither
// (run 10 ruling 1): a rate_limit_event whose status is not an allowed one
// (`allowed`, `allowed_warning`) is usage-limit; an is_error result is read
// against accountPatterns, unless its subtype is the sitting's own cap
// (`error_max_turns`, `error_max_budget_usd`). A rate_limit_event without a status names none.
func Account(ev map[string]any) relay.StopReason {
	switch ev["type"] {
	case "rate_limit_event":
		status, _ := ev["status"].(string)
		if info, ok := ev["rate_limit_info"].(map[string]any); ok {
			if s, ok := info["status"].(string); ok {
				status = s
			}
		}
		if status != "" && !strings.HasPrefix(status, "allowed") {
			return relay.UsageLimit
		}
	case "result":
		sub, _ := ev["subtype"].(string)
		if isErr, _ := ev["is_error"].(bool); !isErr || strings.HasPrefix(sub, "error_max") {
			return "" // the sitting's own turn or budget cap is never the account's
		}
		text, _ := ev["result"].(string)
		text = strings.ToLower(text)
		for _, m := range apiStatus.FindAllStringSubmatch(text, -1) {
			for _, p := range accountPatterns {
				if m[1] == p.Status {
					return p.Reason // the API's own status outranks any needle
				}
			}
		}
		for _, p := range accountPatterns {
			for _, n := range p.Needles {
				if holds(text, n) {
					return p.Reason
				}
			}
			if p.Words != nil && p.Words.MatchString(text) {
				return p.Reason
			}
		}
	}
	return ""
}

// holds reports whether text holds needle as a whole word: neither end
// of a match touches a letter or a digit (an underscore is a boundary, so
// rate_limit holds in rate_limit_error).
func holds(text, needle string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], needle)
		if j < 0 {
			return false
		}
		a, b := i+j, i+j+len(needle)
		if (a == 0 || !wordByte(text[a-1]) || !wordByte(needle[0])) &&
			(b == len(text) || !wordByte(text[b]) || !wordByte(needle[len(needle)-1])) {
			return true
		}
		i = a + 1
	}
}

// wordByte is a letter or a digit; a byte of a multibyte rune is neither.
func wordByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
