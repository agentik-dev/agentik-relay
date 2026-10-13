package stream

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // a reset's IANA zone resolves on any host (AR63)

	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// The kinds of a usage limit (AR63, usage-limit-resume-r1 §2): the 5-hour
// window, the weekly cap, one model's cap, or none the stream names.
const (
	KindWindow  = "window"
	KindWeekly  = "weekly"
	KindModel   = "model"
	KindUnknown = "unknown"
)

// resetBound is the farthest a reset may lie after the hit (§2): a later
// one, or one not after the hit, is no reset.
const resetBound = 8 * 24 * time.Hour

// Limit is what a stream names of its account stop (AR63): the reason, and
// for a usage limit its kind, its reset (zero with none), where the reset
// came from (`event`, `text`, "" with none) and a model limit's model word.
// Named is the reset the stop named before the bound (AR69 ruling 1): the
// event's resetsAt in the hit's zone when it sent one, else Reset; the
// exit line does not carry it.
type Limit struct {
	Reason relay.StopReason
	Kind   string
	Reset  time.Time
	Source string
	Scope  string
	Named  time.Time
}

// limitFacts is what a usage-limit stop's events hold: the first refused
// rate_limit_event's rateLimitType and resetsAt (Unix seconds, 0 none), and
// the first usage-limit result's text.
type limitFacts struct {
	typ  string
	at   int64
	text string
}

// add folds one usage-limit event into the facts, keeping the first of each.
func (f *limitFacts) add(m map[string]any) {
	switch m["type"] {
	case "rate_limit_event":
		info, _ := m["rate_limit_info"].(map[string]any)
		for _, src := range []map[string]any{m, info} {
			if s, ok := src["rateLimitType"].(string); ok && f.typ == "" {
				f.typ = s
			}
			if v, ok := src["resetsAt"].(float64); ok && f.at == 0 && v > 0 {
				f.at = int64(v)
			}
		}
	case "result":
		if s, _ := m["result"].(string); f.text == "" {
			f.text = s
		}
	}
}

// eventKinds maps a refused event's rateLimitType to its kind.
var eventKinds = map[string]string{"five_hour": KindWindow, "seven_day": KindWeekly}

// modelWords are the words `hit your <word> limit` and `reached your <word>
// limit` read as a model's limit (case-insensitive); any other word stops
// the night as usage-limit, its kind unknown (the fix round's ruling).
var modelWords = map[string]bool{"fable": true, "opus": true, "sonnet": true, "haiku": true}

// usageWords is the usage row's two worded needles, read on lowered text:
// `hit your <word> limit` and `reached your <word> limit`, whole words,
// `<word>` one run of letters, digits or `-`.
var usageWords = regexp.MustCompile(`(?:^|[^a-z0-9])(hit|reached) your ([a-z0-9-]+) limit(?:[^a-z0-9]|$)`)

// Limit is the stop's facts at hit, the instant the stream first named it
// (AR63): a usage limit's kind is its event's type, else its text's; its
// reset the event's resetsAt within resetBound (Source `event`), else the
// text's clause (ResetIn, Source `text`); Named the event's resetsAt
// unbounded, else Reset (AR69). Any other stop names its reason alone; no
// stop is the zero Limit.
func (t Tally) Limit(hit time.Time) Limit {
	l := Limit{Reason: t.Account}
	if t.Account != relay.UsageLimit {
		return l
	}
	f := t.limit
	if l.Kind = eventKinds[f.typ]; l.Kind == "" {
		l.Kind, l.Scope = textKind(f.text)
	}
	if f.at > 0 {
		l.Named = time.Unix(f.at, 0).In(hit.Location())
		if within(l.Named, hit) {
			l.Reset, l.Source = l.Named, "event"
		}
	}
	if l.Source == "" {
		if r, ok := ResetIn(f.text, hit); ok {
			l.Reset, l.Source = r, "text"
		}
	}
	if f.at == 0 {
		l.Named = l.Reset
	}
	return l
}

// scopeCap is the most of a scope the exit line carries, in bytes: its free
// text stays capped (relay-r1 §5).
const scopeCap = 32

// Line is the limit as the exit line's `limit` (AR63): the kind, the reset
// in RFC3339 in its own zone ("" with none), its source and the scope, at
// most scopeCap bytes.
func (l Limit) Line() map[string]any {
	at := ""
	if !l.Reset.IsZero() {
		at = l.Reset.Format(time.RFC3339)
	}
	return map[string]any{"kind": l.Kind, "resets_at": at, "source": l.Source, "scope": head(l.Scope, scopeCap)}
}

// textKind is a usage-limit text's kind and scope: `session limit` or
// `5-hour limit` the window; `weekly limit` the weekly cap; `reached your
// <word> limit` or `hit your <word> limit` with a model's word, that
// model's (scope the word, lowered); else unknown.
func textKind(text string) (kind, scope string) {
	low := strings.ToLower(text)
	switch {
	case holds(low, "session limit") || holds(low, "5-hour limit"):
		return KindWindow, ""
	case holds(low, "weekly limit"):
		return KindWeekly, ""
	}
	for _, m := range usageWords.FindAllStringSubmatch(low, -1) {
		if modelWords[m[2]] {
			return KindModel, m[2]
		}
	}
	return KindUnknown, ""
}

// resetClause is the measured reset forms (§2): `resets H[:MM]am|pm` or
// `resets Mon D at H[:MM]am|pm`, then an optional ` (<zone>)`.
var resetClause = regexp.MustCompile(`\bresets (?:([A-Za-z]{3}) ([0-9]{1,2}) at )?([0-9]{1,2})(?::([0-9]{2}))?([AaPp][Mm])\b(?: \(([^()]*)\))?`)

var months = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

// ResetIn is the reset text's first measured clause names after hit (AR63,
// §2), false with none. The zone is the clause's IANA name as written,
// else hit's own location; an empty zone, `Local` or one LoadLocation does
// not know is none. A time is the first instant after hit its wall clock
// reads (onDay: the fall-back day's second pass counts; a spring-forward
// gap reads the next day's); a date is that day in hit's year, else the
// next year's, none when that day never reads it. A weekday, any other
// form, or a reset not after hit or past hit plus resetBound is none.
func ResetIn(text string, hit time.Time) (time.Time, bool) {
	m := resetClause.FindStringSubmatchIndex(text)
	if m == nil {
		return time.Time{}, false
	}
	group := func(i int) string {
		if m[2*i] < 0 {
			return ""
		}
		return text[m[2*i]:m[2*i+1]]
	}
	loc := hit.Location()
	if m[12] >= 0 {
		name := group(6)
		if name == "" || name == "Local" {
			return time.Time{}, false
		}
		var err error
		if loc, err = time.LoadLocation(name); err != nil {
			return time.Time{}, false
		}
	}
	h, _ := strconv.Atoi(group(3))
	mm := 0
	if s := group(4); s != "" {
		mm, _ = strconv.Atoi(s)
	}
	if h < 1 || h > 12 || mm > 59 {
		return time.Time{}, false
	}
	h %= 12
	if strings.EqualFold(group(5), "pm") {
		h += 12
	}
	at := hit.In(loc)
	var (
		r  time.Time
		ok bool
	)
	if mon := group(1); mon != "" {
		month, known := months[strings.ToLower(mon)]
		day, _ := strconv.Atoi(group(2))
		if !known {
			return time.Time{}, false
		}
		if r, ok = onDay(at.Year(), month, day, h, mm, loc, hit); !ok {
			r, ok = onDay(at.Year()+1, month, day, h, mm, loc, hit)
		}
	} else {
		for i := 0; i < 3 && !ok; i++ { // today, else the next day the wall clock reads it
			d := time.Date(at.Year(), at.Month(), at.Day()+i, 12, 0, 0, 0, loc)
			r, ok = onDay(d.Year(), d.Month(), d.Day(), h, mm, loc, hit)
		}
	}
	if !ok || !within(r, hit) {
		return time.Time{}, false
	}
	return r, true
}

// onDay is the first instant after hit whose wall clock in loc reads y-m-d
// h:mm (round one's F1), false with none: on a fall-back day the hour comes
// twice and its second pass may be the first after hit; on a spring-forward
// day, or a date the month lacks, no instant reads it.
func onDay(y int, m time.Month, d, h, mm int, loc *time.Location, hit time.Time) (time.Time, bool) {
	r := time.Date(y, m, d, h, mm, 0, 0, loc)
	for _, c := range []time.Time{r.Add(-time.Hour), r, r.Add(time.Hour)} {
		w := c.In(loc)
		if w.Year() == y && w.Month() == m && w.Day() == d && w.Hour() == h && w.Minute() == mm && c.After(hit) {
			return c.In(loc), true
		}
	}
	return time.Time{}, false
}

// within reports whether r is after hit and at most resetBound on.
func within(r, hit time.Time) bool {
	return r.After(hit) && !r.After(hit.Add(resetBound))
}
