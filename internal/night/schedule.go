package night

import (
	"path/filepath"

	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// PickOptions carry what Pick's signature leaves out. PathOf is a row's
// worktree path relative to the root (nil: .claude/worktrees/<id>, the
// plan's). Satisfied is, per row, the edges the plan found satisfied (a green
// line on an earlier night, or a branch already in the base).
type PickOptions struct {
	PathOf    func(id string) string
	Satisfied map[string][]string
	// Refused is the rows refused at plan (AR58): skipped, never started,
	// yet doomed behind a failed dependency as main's waiting row was
	// before its stage refusal, so blocked reads as it did on main.
	Refused map[string]bool
	// Hold is the rows a model's pause holds (AR71 ruling 2; nil: none):
	// never picked and counted as waiting, as a paused row is, so a row
	// on one waits, never doomed nor blocked.
	Hold func(id string) bool
}

// Pick is §8's scheduler over the plan's defaults: PickIn with no satisfied
// edges and the plan's worktree paths; an Order() error picks nothing.
func Pick(q *queue.Queue, s map[string]*relay.Sitting, running []string, parallel int) (next []string, blocked bool) {
	next, blocked, _ = PickIn(q, s, running, parallel, PickOptions{})
	return next, blocked
}

// PickIn returns the rows to start now, in Order(): a row tonight (in s),
// ready or waiting, whose every dependency and base is green tonight or was
// satisfied at plan, that is not running, and that shares no worktree path
// with a running or picked row. At most parallel rows run at once (default
// 1); two may run together only when neither is the other's base or
// dependency, directly or through another row. A paused row (the
// account's limit, AR65) is never picked and counts as waiting: a row on
// it waits, never doomed; so is a row o.Hold holds (AR71). blocked is
// true when nothing runs, nothing can start, and rows still wait: every
// one of them descends from a red, failed or skipped row (§1's
// gate-failed-dependency). err is q.Order()'s: nothing can start, and the
// night stops no-progress.
func PickIn(q *queue.Queue, s map[string]*relay.Sitting, running []string, parallel int, o PickOptions) (next []string, blocked bool, err error) {
	if parallel < 1 {
		parallel = 1
	}
	pathOf := o.PathOf
	if pathOf == nil {
		pathOf = func(id string) string { return filepath.Join(".claude", "worktrees", id) }
	}
	order, err := q.Order()
	if err != nil {
		return nil, false, err
	}
	rows := map[string]queue.Row{}
	for _, r := range q.Rows {
		rows[r.ID] = r
	}
	busy := map[string]bool{}
	for _, id := range running {
		busy[id] = true
	}
	doomed := map[string]bool{}
	waiting := 0
	for _, id := range order {
		st := s[id]
		if st == nil || busy[id] || relay.Terminal(st.State) && !o.Refused[id] {
			continue
		}
		if st.State == relay.Paused || o.Hold != nil && o.Hold(id) { // held for its wake, never picked; its dependents wait (AR65, AR71)
			waiting++
			continue
		}
		ok := true
		for _, d := range edgesOf(rows[id]) {
			if contains(o.Satisfied[id], d) {
				continue
			}
			ds := s[d]
			switch {
			case ds != nil && ds.State == relay.Green:
			case ds == nil, ds.State == relay.Red, ds.State == relay.Failed, ds.State == relay.Skipped, doomed[d]:
				ok, doomed[id] = false, true
			default:
				ok = false
			}
		}
		if doomed[id] || o.Refused[id] { // a refused row never waits nor starts (AR58)
			continue
		}
		waiting++
		if !ok || len(running)+len(next) >= parallel {
			continue
		}
		free := true
		for _, other := range append(append([]string{}, running...), next...) {
			if pathOf(other) == pathOf(id) || related(rows, id, other) || related(rows, other, id) {
				free = false
			}
		}
		if free {
			next = append(next, id)
		}
	}
	return next, len(next) == 0 && len(running) == 0 && waiting == 0 && len(doomed) > 0, nil
}

// related reports whether b is a's base or dependency, directly or through
// another row of the queue.
func related(rows map[string]queue.Row, a, b string) bool {
	seen := map[string]bool{}
	var walk func(id string) bool
	walk = func(id string) bool {
		for _, d := range edgesOf(rows[id]) {
			if d == b {
				return true
			}
			if !seen[d] {
				seen[d] = true
				if walk(d) {
					return true
				}
			}
		}
		return false
	}
	return walk(a)
}
