package effects

import (
	"os"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Group is one live process group the relay started (ADV6-2): a spawned
// child's or a measured line's. Started is its leader's start time as
// Procs.Started reads it, so a later relay never kills a reused pid; Row
// is the row it belongs to ("" for a line outside any row).
type Group struct {
	Pgid    int    `json:"pgid"`
	Started string `json:"started"`
	Row     string `json:"row"`
}

// Groups is the set of the relay's live process groups (relay-r1 §7, the
// dead relay): every spawn and every line is added when it starts and done
// when it ends, so a signal to the relay reaches each group, and the Watch
// function keeps the night's groups.json for the next start to reap. A nil
// *Groups tracks nothing. After Kill, a group added is signalled at once:
// nothing the relay starts outlives it.
type Groups struct {
	mu     sync.Mutex
	change func([]Group)
	live   map[int]*member
	killed bool
	grace  time.Duration
}

type member struct {
	g      Group
	signal func(os.Signal) error
}

// Add tracks the group pgid; signal reaches the whole group.
func (gs *Groups) Add(g Group, signal func(os.Signal) error) {
	if gs == nil {
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.live == nil {
		gs.live = map[int]*member{}
	}
	m := &member{g: g, signal: signal}
	gs.live[g.Pgid] = m
	gs.changed()
	if gs.killed {
		gs.term(m, gs.grace)
	}
}

// Done ends the tracking of pgid.
func (gs *Groups) Done(pgid int) {
	if gs == nil {
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if _, ok := gs.live[pgid]; ok {
		delete(gs.live, pgid)
		gs.changed()
	}
}

// Watch calls f with the live set now and after every Add and Done, under
// the lock (so the writes it makes are in order); nil stops it.
func (gs *Groups) Watch(f func([]Group)) {
	if gs == nil {
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.change = f
	gs.changed()
}

// Live is every tracked group, by pgid.
func (gs *Groups) Live() []Group {
	if gs == nil {
		return nil
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.list()
}

// Kill sends every live group SIGTERM, then SIGKILL after grace to each
// still tracked (§7's escalation), and does the same to every group added
// from now on.
func (gs *Groups) Kill(grace time.Duration) {
	if gs == nil {
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.killed, gs.grace = true, grace
	for _, m := range gs.live {
		gs.term(m, grace)
	}
}

// term is SIGTERM now and SIGKILL after grace unless m is done; gs.mu is
// held.
func (gs *Groups) term(m *member, grace time.Duration) {
	m.signal(syscall.SIGTERM)
	time.AfterFunc(grace, func() {
		gs.mu.Lock()
		still := gs.live[m.g.Pgid] == m
		gs.mu.Unlock()
		if still {
			m.signal(syscall.SIGKILL)
		}
	})
}

func (gs *Groups) list() []Group {
	out := make([]Group, 0, len(gs.live))
	for _, m := range gs.live {
		out = append(out, m.g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pgid < out[j].Pgid })
	return out
}

func (gs *Groups) changed() {
	if gs.change != nil {
		gs.change(gs.list())
	}
}

// groupSignal is a signal to the whole group pgid.
func groupSignal(pgid int) func(os.Signal) error {
	return func(sig os.Signal) error {
		s, ok := sig.(syscall.Signal)
		if !ok || pgid <= 1 {
			return nil
		}
		return syscall.Kill(-pgid, s)
	}
}
