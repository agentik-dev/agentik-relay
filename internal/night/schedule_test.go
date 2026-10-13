package night

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/queue"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// q3 is a, b based on a, c independent.
func q3() *queue.Queue {
	return &queue.Queue{Rows: []queue.Row{{ID: "a"}, {ID: "b", Base: "a"}, {ID: "c"}}}
}

func sittings(states map[string]relay.State) map[string]*relay.Sitting {
	out := map[string]*relay.Sitting{}
	for id, st := range states {
		out[id] = &relay.Sitting{ID: id, State: st}
	}
	return out
}

// TestPick: one two-row example per branch of Pick.
func TestPick(t *testing.T) {
	for _, tc := range []struct {
		name     string
		states   map[string]relay.State
		running  []string
		parallel int
		next     []string
		blocked  bool
	}{
		{"first ready in order", map[string]relay.State{"a": relay.Ready, "b": relay.Waiting, "c": relay.Ready}, nil, 1, []string{"a"}, false},
		{"slot full", map[string]relay.State{"a": relay.Running, "b": relay.Waiting, "c": relay.Ready}, []string{"a"}, 1, nil, false},
		{"base green tonight", map[string]relay.State{"a": relay.Green, "b": relay.Waiting}, nil, 1, []string{"b"}, false},
		{"parallel: unrelated rows together", map[string]relay.State{"a": relay.Ready, "b": relay.Waiting, "c": relay.Ready}, nil, 2, []string{"a", "c"}, false},
		{"parallel: never a row with its base", map[string]relay.State{"a": relay.Running, "b": relay.Waiting}, []string{"a"}, 2, nil, false},
		{"red base: blocked", map[string]relay.State{"a": relay.Red, "b": relay.Waiting}, nil, 1, nil, true},
		{"red row, independent row runs", map[string]relay.State{"a": relay.Red, "b": relay.Waiting, "c": relay.Ready}, nil, 1, []string{"c"}, false},
		{"all terminal: empty, not blocked", map[string]relay.State{"a": relay.Green, "b": relay.Green}, nil, 1, nil, false},
	} {
		next, blocked := Pick(q3(), sittings(tc.states), tc.running, tc.parallel)
		if !reflect.DeepEqual(next, tc.next) || blocked != tc.blocked {
			t.Errorf("%s: got %v %v, want %v %v", tc.name, next, blocked, tc.next, tc.blocked)
		}
	}
	// A base satisfied at plan (merged on an earlier night) is no wait.
	next, _, _ := PickIn(q3(), sittings(map[string]relay.State{"b": relay.Waiting}), nil, 1,
		PickOptions{Satisfied: map[string][]string{"b": {"a"}}})
	if !reflect.DeepEqual(next, []string{"b"}) {
		t.Errorf("satisfied at plan: got %v", next)
	}
}

// TestOneSittingPerTree: Pick never yields two rows with one path, even
// unrelated with slots free, and Stage's lock refuses a second sitting in
// a staged tree.
func TestOneSittingPerTree(t *testing.T) {
	q := &queue.Queue{Rows: []queue.Row{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	one := PickOptions{PathOf: func(id string) string {
		if id == "c" {
			return "c"
		}
		return "shared"
	}}
	all := map[string]relay.State{"a": relay.Ready, "b": relay.Ready, "c": relay.Ready}
	next, _, _ := PickIn(q, sittings(all), nil, 3, one)
	if !reflect.DeepEqual(next, []string{"a", "c"}) {
		t.Fatalf("picked %v, want [a c]", next)
	}
	all["a"] = relay.Running
	if next, _, _ = PickIn(q, sittings(all), []string{"a"}, 3, one); !reflect.DeepEqual(next, []string{"c"}) {
		t.Fatalf("with a running, picked %v, want [c]", next)
	}
	fx, g, _, rp := stageFakes(t)
	st, err := Stage(fx, rp)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Unlock()
	g.WTs, g.Existing = nil, nil // the tree and branch gone: only the lock refuses
	if _, err := Stage(fx, rp); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("a second stage of one tree: %v", err)
	}
}
