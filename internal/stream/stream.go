// Package stream reads claude's stream-json output (§7): system/init for the
// session and model, assistant messages for usage, tool_use blocks and
// parent_tool_use_id, and result for the sitting's end. Definitions are
// burn-report's: a turn is one deduplicated top-level assistant message;
// context is the last such message's input plus cache creation plus cache
// read. No message text is kept beyond a tool's short summary and the
// result's capped tail.
package stream

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// Caps on the free text an Event carries.
const (
	SummaryCap = 80  // a tool's command or path
	TailCap    = 400 // the result's tail (§5's exit line)
)

// Event is one stream line, the fields §7 names lifted out; Raw is the line.
// A Tally's Result is an Event too: every result event folded (AR83).
type Event struct {
	Type, Subtype    string
	SessionID, Model string
	Usage            *Usage
	Tools            []ToolUse
	ParentToolUseID  string
	NumTurns         int
	CostUSD          float64
	IsError          bool
	Denials          int
	DurationMS       int64
	ResultTail       string
	MessageID        string
	Raw              json.RawMessage
}

// Usage is one assistant message's token counts, or a stream's total.
type Usage struct {
	Input       int `json:"input"`
	CacheRead   int `json:"cache_read"`
	CacheCreate int `json:"cache_create"`
	Output      int `json:"output"`
}

// plus is u with v's counts added, sign times.
func (u Usage) plus(v Usage, sign int) Usage {
	return Usage{u.Input + sign*v.Input, u.CacheRead + sign*v.CacheRead, u.CacheCreate + sign*v.CacheCreate, u.Output + sign*v.Output}
}

// Context is §7's context: input plus cache creation plus cache read.
func (u Usage) Context() int { return u.Input + u.CacheCreate + u.CacheRead }

// ToolUse is one tool_use block: the tool's name and the first SummaryCap
// bytes of its command or path, never its prompt.
type ToolUse struct{ Name, Summary string }

// Tally is the running sum over a stream. Builders counts distinct messages
// per parent_tool_use_id; NonJSON counts lines that did not parse. Fresh is
// true when the last event Add saw was an assistant event with a message id
// not seen before: one message streamed as several events is fresh once.
// Usage totals every assistant message's tokens, the builders' too, each
// message id once at its last usage (AR28): what the sitting burned when
// no result event reads its cost. Once a result reads a usage, Usage.Output
// is the folded results' output plus the streamed output of the builders and
// of the main-thread messages first seen after the last such result, since an
// assistant event carries its message's opening count; input and the caches
// stay the stream's (AR86). A turn line carries Usage as it stands at its
// event: the streamed output before any result, after one the results' output
// plus what streamed since. Result is every result event folded into the
// first (AR83; Event.fold), CostUSD its cost; Results counts the result
// events read, so a record tells a background wait's later query from a
// sitting of one (AR90).
type Tally struct {
	Turns, Context int
	CostUSD        float64
	LastTool       string
	LastSummary    string
	Builders       map[string]int
	ResultSeen     bool
	Result         *Event
	Results        int
	SessionID      string
	Model          string
	NonJSON        int
	Fresh          bool
	Usage          Usage
	// Account is the first account stop an event named (Account, run 10
	// ruling 1), "" when none.
	Account relay.StopReason

	limit      limitFacts // a usage-limit stop's facts (AR63; Limit)
	seen       map[string]bool
	usage      map[string]Usage // by parent/message id, its last usage
	streamed   Usage            // every message's last usage, summed (AR28)
	builderOut int              // the builders' share of streamed.Output (AR86)
	after      map[string]bool  // main-thread ids first seen since the last result with a usage
	afterOut   int              // their share of streamed.Output (AR86)
}

// wireUsage is a usage object as the stream writes it: an assistant
// message's, or a result's at the top level (AR86).
type wireUsage struct {
	Input       int `json:"input_tokens"`
	CacheCreate int `json:"cache_creation_input_tokens"`
	CacheRead   int `json:"cache_read_input_tokens"`
	Output      int `json:"output_tokens"`
}

func (u wireUsage) usage() *Usage {
	return &Usage{Input: u.Input, CacheCreate: u.CacheCreate, CacheRead: u.CacheRead, Output: u.Output}
}

type wire struct {
	Type            string  `json:"type"`
	Subtype         string  `json:"subtype"`
	SessionID       string  `json:"session_id"`
	Model           string  `json:"model"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
	Message         *struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Usage *wireUsage `json:"usage"`
	} `json:"message"`
	Usage      *wireUsage        `json:"usage"` // a result's (AR86)
	NumTurns   int               `json:"num_turns"`
	CostUSD    float64           `json:"total_cost_usd"`
	IsError    bool              `json:"is_error"`
	Denials    []json.RawMessage `json:"permission_denials"`
	DurationMS int64             `json:"duration_ms"`
	Result     *string           `json:"result"`
}

// Decode lifts one line into an Event.
func Decode(line []byte) (Event, error) {
	var w wire
	if err := json.Unmarshal(line, &w); err != nil {
		return Event{}, err
	}
	if w.Type == "" {
		return Event{}, errors.New("stream: a line without a type")
	}
	ev := Event{Type: w.Type, Subtype: w.Subtype, SessionID: w.SessionID, Model: w.Model,
		NumTurns: w.NumTurns, CostUSD: w.CostUSD, IsError: w.IsError, Denials: len(w.Denials),
		DurationMS: w.DurationMS, Raw: append(json.RawMessage(nil), line...)}
	if w.ParentToolUseID != nil {
		ev.ParentToolUseID = *w.ParentToolUseID
	}
	if w.Result != nil {
		ev.ResultTail = tail(*w.Result, TailCap)
	}
	if w.Usage != nil && w.Type == "result" {
		ev.Usage = w.Usage.usage()
	}
	if m := w.Message; m != nil && w.Type == "assistant" {
		ev.MessageID = m.ID
		if ev.Model == "" {
			ev.Model = m.Model
		}
		if m.Usage != nil {
			ev.Usage = m.Usage.usage()
		}
		for _, c := range m.Content {
			if c.Type == "tool_use" {
				ev.Tools = append(ev.Tools, ToolUse{Name: c.Name, Summary: summary(c.Input)})
			}
		}
	}
	return ev, nil
}

// summary is the first SummaryCap bytes of a tool input's command or path;
// any other key (a prompt, a description, a file's content) is never read.
func summary(input json.RawMessage) string {
	var in map[string]json.RawMessage
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	for _, k := range []string{"command", "file_path", "notebook_path", "path", "pattern"} {
		var s string
		if json.Unmarshal(in[k], &s) == nil && s != "" {
			return head(s, SummaryCap)
		}
	}
	return ""
}

// Add folds ev into the tally.
func (t *Tally) Add(ev Event) {
	if t.seen == nil {
		t.seen = map[string]bool{}
	}
	if t.Builders == nil {
		t.Builders = map[string]int{}
	}
	t.Fresh = false
	if (t.Account == "" || t.Account == relay.UsageLimit) && len(ev.Raw) > 0 && (ev.Type == "rate_limit_event" || ev.Type == "result" && ev.IsError) {
		var m map[string]any
		if json.Unmarshal(ev.Raw, &m) == nil {
			a := Account(m)
			if t.Account == "" {
				t.Account = a // the first stop is the reason
			}
			if a == relay.UsageLimit && t.Account == relay.UsageLimit {
				t.limit.add(m) // later usage-limit events fill what the first lacks (AR63)
			}
		}
	}
	switch ev.Type {
	case "system":
		if ev.Subtype == "init" {
			t.SessionID, t.Model = ev.SessionID, ev.Model
		}
	case "assistant":
		key := ev.ParentToolUseID + "/" + ev.MessageID
		fresh := ev.MessageID == "" || !t.seen[key]
		t.seen[key] = true
		t.Fresh = fresh
		if ev.Usage != nil {
			var prev Usage
			if ev.MessageID != "" {
				if t.usage == nil {
					t.usage = map[string]Usage{}
				}
				prev = t.usage[key]
				t.usage[key] = *ev.Usage
			}
			t.streamed = t.streamed.plus(prev, -1).plus(*ev.Usage, 1)
			if ev.ParentToolUseID != "" {
				t.builderOut += ev.Usage.Output - prev.Output
			} else {
				if fresh {
					if t.after == nil {
						t.after = map[string]bool{}
					}
					t.after[key] = true
				}
				if t.after[key] {
					t.afterOut += ev.Usage.Output - prev.Output
				}
			}
			t.Usage = t.total()
		}
		if ev.ParentToolUseID != "" {
			if fresh {
				t.Builders[ev.ParentToolUseID]++
			}
			return
		}
		if fresh {
			t.Turns++
		}
		if ev.Usage != nil {
			t.Context = ev.Usage.Context()
		}
		if n := len(ev.Tools); n > 0 {
			t.LastTool, t.LastSummary = ev.Tools[n-1].Name, ev.Tools[n-1].Summary
		}
	case "result":
		t.Results++
		r := ev
		r.Raw = nil
		if t.Result != nil { // a later query's result (AR83): fold it into the first
			f := *t.Result
			f.fold(r)
			r = f
		}
		t.ResultSeen, t.Result, t.CostUSD = true, &r, r.CostUSD
		if ev.Usage != nil { // its output now holds the main thread's messages so far
			t.after, t.afterOut = nil, 0
		}
		t.Usage = t.total()
	}
}

// total is the stream's usage, its output the folded results' plus the
// builders' streamed output and the main thread's streamed since the last
// result with a usage, once a result reads one (AR86).
func (t *Tally) total() Usage {
	u := t.streamed
	if t.Result != nil && t.Result.Usage != nil {
		u.Output = t.Result.Usage.Output + t.builderOut + t.afterOut
	}
	return u
}

// fold folds ev, a later result event, into r (AR83): a child that takes
// one more turn after a background command ends writes a result per query.
// num_turns, duration_ms, the denials and the usage are per query and sum;
// the cost is the process's running total, so the highest any result read,
// never a sum.
// The end is the last result's (its tail and session), an error kept:
// IsError when any result erred, Subtype the last erring result's, else the
// last's.
func (r *Event) fold(ev Event) {
	r.NumTurns += ev.NumTurns
	r.DurationMS += ev.DurationMS
	r.Denials += ev.Denials
	if ev.Usage != nil { // a fresh sum: the first result's Usage is never written through
		var u Usage
		if r.Usage != nil {
			u = *r.Usage
		}
		u = u.plus(*ev.Usage, 1)
		r.Usage = &u
	}
	r.CostUSD = max(r.CostUSD, ev.CostUSD)
	if ev.IsError || !r.IsError {
		r.Subtype = ev.Subtype
	}
	r.IsError = r.IsError || ev.IsError
	r.ResultTail, r.SessionID = ev.ResultTail, ev.SessionID
}

// MaxLine is the longest stream line Parse holds, in bytes (AR17C,
// ADV10-23): a longer one is counted in NonJSON, its excess read past and
// never held, and the stream goes on at the next line.
const MaxLine = 1 << 20

// Parse reads r line by line, calling onEvent (when not nil) after each
// event is folded in. A line that is not a JSON event, or is longer than
// MaxLine, is counted in NonJSON, never fatal; the error is only the
// reader's.
func Parse(r io.Reader, onEvent func(Event, Tally)) (Tally, error) {
	var t Tally
	t.Builders = map[string]int{}
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, long, err := readLine(br)
		if long {
			t.NonJSON++
		} else if len(trim(line)) > 0 {
			if ev, derr := Decode(trim(line)); derr != nil {
				t.NonJSON++
			} else {
				t.Add(ev)
				if onEvent != nil {
					onEvent(ev, t)
				}
			}
		}
		if err == io.EOF {
			return t, nil
		}
		if err != nil {
			return t, err
		}
	}
}

// readLine is br's next line, newline and all, when it is at most MaxLine
// bytes; past MaxLine it reads on to the newline holding none of it, and
// long is true (AR17C, ADV10-23). bufio.Scanner is not used: it ends the
// stream at a token over its cap.
func readLine(br *bufio.Reader) (line []byte, long bool, err error) {
	for {
		frag, ferr := br.ReadSlice('\n')
		if !long {
			if len(line)+len(frag) > MaxLine {
				long, line = true, nil
			} else {
				line = append(line, frag...)
			}
		}
		if ferr != bufio.ErrBufferFull {
			return line, long, ferr
		}
	}
}

// Line is the live line: `turn N: <tool> "<summary>"; context 71k/130k;
// $1.20`, the tool part `-` before the first tool, the cost `unread` until
// a result event reads it (AR28: never $0.00). The summary is cut, `...`
// marking the cut, so the line stays under LineCap columns.
func (t Tally) Line(ceiling int) string {
	cost := "unread"
	if t.Result != nil {
		cost = fmt.Sprintf("$%.2f", t.CostUSD)
	}
	rest := fmt.Sprintf("; context %dk/%dk; %s", t.Context/1000, ceiling/1000, cost)
	lead := fmt.Sprintf("turn %d: ", t.Turns)
	if t.LastTool == "" {
		return lead + "-" + rest
	}
	sum := t.LastSummary
	for {
		l := fmt.Sprintf("%s%s %q%s", lead, t.LastTool, sum, rest)
		over := utf8.RuneCountInString(l) - (LineCap - 1)
		if over <= 0 || sum == "" {
			return l
		}
		cut := strings.TrimSuffix(sum, "...")
		n := utf8.RuneCountInString(cut) - over - 3
		if n <= 0 {
			sum = ""
			continue
		}
		sum = string([]rune(cut)[:n]) + "..."
	}
}

// LineCap is the live line's width in columns: every line is under it.
const LineCap = 100

func trim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

// head is s's first n bytes on a rune boundary.
func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// tail is s's last n bytes on a rune boundary.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := len(s) - n
	for i < len(s) && s[i]&0xC0 == 0x80 {
		i++
	}
	return s[i:]
}
