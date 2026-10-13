package night

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/agentik-dev/agentik-relay/internal/stream"
)

// The pause's bounds (usage-limit-resume r1 §2; AR65 act 1): a wake lies
// WakeSlack past the reset; a limit with no reset waits FallbackWait; with
// no --until, a wake over PauseReach away stops the night instead.
const (
	WakeSlack    = 2 * time.Minute
	FallbackWait = 30 * time.Minute
	PauseReach   = 6 * time.Hour
)

// MaxPauses is the most pauses a night takes (AR69 ruling 2): a hit after
// the twelfth stops the night.
const MaxPauses = 12

// pauseIn is what pauseFor decides on: the limited part's Limit, the
// clock at the decision, the night's --until (zero: none), whether the
// owner's stop is draining the night, the night's pauses before this hit
// (Count) and the reset the night last woke from (Waited; zero: not
// woken, or woken from a fallback) (AR69 ruling 2); ProbePassed, a
// sitting spawned at or after the last fallback wake ended without a
// usage-limit refusal, which disarms the probe (the fix round's F1).
type pauseIn struct {
	Limit       stream.Limit
	Now         time.Time
	Until       time.Time
	Draining    bool
	Count       int
	Waited      time.Time
	ProbePassed bool
}

// pause is pauseFor's answer: Stop the night at once (usage-limit, the
// stop line's detail Detail), else pause it until Wake. Source is where
// the wake came from: the Limit's (`event`, `text`) or `fallback`. Scope
// is the rows it pauses (AR71 ruling 1): a model's word, the Limit's, on
// a model's limit; "" every row.
type pause struct {
	Stop   bool
	Detail string
	Source string
	Wake   time.Time
	Scope  string
}

// ofModel reports whether a row on model is in a pause's scope (AR71
// ruling 1): every row when scope is ""; else when the model, lowered and
// cut at every rune not a letter or digit, holds scope as one piece
// (claude-fable-5-1 holds fable; claude-opus-5-5 and fablex-1 do not).
func ofModel(model, scope string) bool {
	if scope == "" {
		return true
	}
	for _, piece := range strings.FieldsFunc(strings.ToLower(model), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if piece == scope {
			return true
		}
	}
	return false
}

// pauseFor decides at the hit (usage-limit-resume r1 §2; AR65 ruling 1),
// pure. The wake: with no reset, Now plus FallbackWait, source fallback;
// else the reset plus WakeSlack, the Limit's source. It stops, naming why,
// while draining; on a weekly limit; on a model's limit with no reset;
// then the guards (AR69 ruling 2): after a pause, with no Waited, no
// reset and the probe not passed (the probe refused again); with a Named
// reset not after Waited;
// at Count MaxPauses; then when the wake is at or past Until; with no
// Until, when the wake is over PauseReach after Now. Else it pauses (a
// model's limit with a reset too). Each stop but the model's keeps its
// wake and source. A model's pause carries the Limit's Scope; any other,
// "" (AR71 ruling 1).
func pauseFor(in pauseIn) pause {
	l := in.Limit
	var p pause
	if l.Reset.IsZero() {
		p.Wake, p.Source = in.Now.Add(FallbackWait), "fallback"
	} else {
		p.Wake, p.Source = l.Reset.Add(WakeSlack), l.Source
	}
	stop := func(why string) pause { p.Stop, p.Detail = true, why; return p }
	switch {
	case in.Draining:
		return stop("the night is draining")
	case l.Kind == stream.KindWeekly:
		return stop("a weekly limit")
	case l.Kind == stream.KindModel && l.Reset.IsZero():
		p.Wake, p.Source = time.Time{}, ""
		return stop(fmt.Sprintf("the %s limit names no reset", l.Scope))
	case in.Count > 0 && in.Waited.IsZero() && !in.ProbePassed && l.Reset.IsZero():
		return stop("refused again after the 30-minute wait, with no reset")
	case !in.Waited.IsZero() && !l.Named.IsZero() && !l.Named.After(in.Waited):
		return stop(fmt.Sprintf("the reset %s is not after the one waited out, %s", l.Named.Format(time.RFC3339), in.Waited.Format(time.RFC3339)))
	case in.Count >= MaxPauses:
		return stop(fmt.Sprintf("%d pauses tonight, the most a night takes", MaxPauses))
	case !in.Until.IsZero() && !p.Wake.Before(in.Until):
		return stop(fmt.Sprintf("the wake %s is at or past --until %s", p.Wake.Format(time.RFC3339), in.Until.Format(time.RFC3339)))
	case in.Until.IsZero() && p.Wake.Sub(in.Now) > PauseReach:
		return stop(fmt.Sprintf("the wake %s is over 6h away, and no --until", p.Wake.Format(time.RFC3339)))
	}
	if l.Kind == stream.KindModel {
		p.Scope = l.Scope
	}
	return p
}

// heldLine is a part's `sitting` line held for the night (AR65 ruling 3):
// a part ending failed usage-limit, before its spawn too, writes no line
// itself; RunRow fills Limit (zero for a row another's stop killed) and
// write, and the night writes the line once it has decided, `limit:
// paused` when it pauses on the part. The line's invariants are read
// first: one broken fails the part invariant, the line has no `limit`
// and the night does not pause (the fix round's F1).
type heldLine struct {
	Limit stream.Limit
	write func(paused bool)
}
