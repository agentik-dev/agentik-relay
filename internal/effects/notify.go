package effects

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// Notify tells the owner what the night did (§7, §9, D23). A failed Send is
// a log line, never a stop.
type Notify interface {
	Send(n Notice) error
}

// Notice is one event's notice: the event's fields only, never a prompt or a
// message (§5).
type Notice struct {
	Event, Night, Row, Title, Body string
	Priority                       int
}

// NoticeCap caps a notice body in bytes.
const NoticeCap = 200

// valueCap caps each string value in a body.
const valueCap = 48

// noticeFields are the fields each noticed event carries into its body, in
// order. The list is the guarantee no content leaves: a field not named here
// is never sent.
var noticeFields = map[string][]string{
	"night-open":  {"rows", "parallel", "until", "max_sittings", "max_usd", "no_tmux"},
	"sitting":     {"state", "reason", "gate", "turns", "context", "cost_usd"},
	"ceiling":     {"turns", "context", "ceiling"},
	"stop":        {"reason", "state", "sittings", "red", "cost_usd", "resume", "unmerged"},
	"night-close": {"state", "duration_ms"},
	// AR32A act 5 (orchestrator r1 §7): the count, the file and main's
	// sha12, never a question's text.
	"question": {"open", "file", "at"},
	// AR66 ruling 5 (usage-limit-resume r1 §2): the pause line's rows, kind,
	// reset, wake and count, never its source.
	"pause": {"rows", "kind", "resets_at", "wake", "count"},
}

// Notices maps a ledger event to its notice: night-open, sitting, ceiling,
// stop, night-close, question and pause (priority 4 for ceiling, stop and
// question, else 3, so quiet hours keep a pause's); every other event is
// refused. A red or failed sitting's body ends with its resume line; a
// paused sitting line (`limit: paused`) sends none, its pause line's
// notice serves (AR65, AR66).
func Notices(event string, fields map[string]any) (Notice, bool) {
	keys, ok := noticeFields[event]
	if !ok {
		return Notice{}, false
	}
	if l, _ := fields["limit"].(string); event == "sitting" && l == "paused" {
		return Notice{}, false // the pause line's notice serves (relay-r1 §1; AR65)
	}
	n := Notice{Event: event, Priority: 3}
	n.Night, _ = fields["night"].(string)
	n.Row, _ = fields["row"].(string)
	if event == "ceiling" || event == "stop" || event == "question" {
		n.Priority = 4
	}
	var parts []string
	for _, k := range keys {
		if v, ok := noticeValue(fields[k]); ok {
			parts = append(parts, k+"="+v)
		}
	}
	suffix := ""
	if st, _ := fields["state"].(string); event == "sitting" && (st == "red" || st == "failed") && n.Row != "" {
		suffix = "; " + relay.ResumeLine(n.Row)
	}
	suffix = capText(suffix, NoticeCap)
	n.Body = capText(strings.Join(parts, " "), NoticeCap-len(suffix)) + suffix
	n.Title = strings.Join(nonEmpty("relay", n.Night, event, n.Row), " ")
	return n, true
}

// noticeValue prints a scalar, a list of strings, or an object by its
// `result` string (the sitting's gate); anything else is left out.
func noticeValue(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		if x == "" {
			return "", false
		}
		return capText(x, valueCap), true
	case bool, int, int64, float64:
		return fmt.Sprint(x), true
	case []string:
		if len(x) == 0 {
			return "", false
		}
		return capText(strings.Join(x, ","), valueCap), true
	case []any:
		var s []string
		for _, e := range x {
			str, ok := e.(string)
			if !ok {
				return "", false
			}
			s = append(s, str)
		}
		return noticeValue(s)
	case map[string]any:
		r, ok := x["result"].(string)
		if !ok {
			return "", false
		}
		return noticeValue(r)
	}
	return "", false
}

func nonEmpty(s ...string) []string {
	var out []string
	for _, x := range s {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// NtfyTimeout bounds one post.
const NtfyTimeout = 5 * time.Second

// Ntfy posts the body to URL/Topic with the Title and Priority headers.
// Client nil: a client with NtfyTimeout.
type Ntfy struct {
	URL, Topic string
	Client     *http.Client
}

func (t Ntfy) Send(n Notice) error {
	c := t.Client
	if c == nil {
		c = &http.Client{Timeout: NtfyTimeout}
	}
	ctx, cancel := context.WithTimeout(context.Background(), NtfyTimeout)
	defer cancel()
	url := strings.TrimRight(t.URL, "/") + "/" + t.Topic
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(n.Body)))
	if err != nil {
		return err
	}
	req.Header.Set("Title", n.Title)
	req.Header.Set("Priority", strconv.Itoa(n.Priority))
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ntfy: %s", resp.Status)
	}
	return nil
}

// LogNotify writes each notice as one line.
type LogNotify struct{ W io.Writer }

func (l LogNotify) Send(n Notice) error {
	_, err := fmt.Fprintf(l.W, "notify %s: %s\n", n.Title, n.Body)
	return err
}

// Multi sends to every Notify and joins their errors.
func Multi(ns ...Notify) Notify { return multi(ns) }

type multi []Notify

func (m multi) Send(n Notice) error {
	var errs []error
	for _, x := range m {
		if x != nil {
			errs = append(errs, x.Send(n))
		}
	}
	return errors.Join(errs...)
}
