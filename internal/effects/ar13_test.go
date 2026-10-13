package effects

import (
	"os"
	"sync"
	"testing"
	"time"
)

// TestAR13_2_GroupsKillEscalates (ADV6-2): Kill sends every live group
// SIGTERM, then SIGKILL after the grace to the ones still tracked; a group
// added after the Kill gets its SIGTERM at once; Watch sees every change.
func TestAR13_2_GroupsKillEscalates(t *testing.T) {
	var mu sync.Mutex
	got := map[int][]os.Signal{}
	sig := func(pgid int) func(os.Signal) error {
		return func(s os.Signal) error {
			mu.Lock()
			defer mu.Unlock()
			got[pgid] = append(got[pgid], s)
			return nil
		}
	}
	gs := &Groups{}
	var seen [][]Group
	gs.Watch(func(l []Group) { seen = append(seen, l) })
	gs.Add(Group{Pgid: 101, Row: "r1"}, sig(101))
	gs.Add(Group{Pgid: 102}, sig(102))
	gs.Kill(50 * time.Millisecond)
	gs.Done(101) // exited on its TERM
	gs.Add(Group{Pgid: 103}, sig(103))
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	want := map[int]string{101: "[terminated]", 102: "[terminated killed]", 103: "[terminated killed]"}
	for pgid, w := range want {
		if s := sigString(got[pgid]); s != w {
			t.Fatalf("group %d: %s, want %s", pgid, s, w)
		}
	}
	if len(seen) != 5 || len(seen[4]) != 2 || seen[1][0].Row != "r1" {
		t.Fatalf("watched %v", seen)
	}
}

func sigString(ss []os.Signal) string {
	out := "["
	for i, s := range ss {
		if i > 0 {
			out += " "
		}
		out += s.String()
	}
	return out + "]"
}

// TestAR13_2_StartedIsStable: OSProcs.Started reads this process's start
// time twice alike; a pid ps does not list is an error.
func TestAR13_2_StartedIsStable(t *testing.T) {
	a, err := OSProcs{}.Started(os.Getpid())
	if err != nil || a == "" {
		t.Fatalf("started %q: %v", a, err)
	}
	if b, _ := (OSProcs{}).Started(os.Getpid()); b != a {
		t.Fatalf("started %q then %q", a, b)
	}
	if _, err := (OSProcs{}).Started(1 << 30); err == nil {
		t.Fatal("a pid that is no process has a start time")
	}
}
