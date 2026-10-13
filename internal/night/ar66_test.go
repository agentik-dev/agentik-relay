package night_test

import (
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR66_1_OwnerWhilePaused (AR66 rulings 1, 2; usage-limit-resume r1
// §4 test 3): in TestAR65_3's world with one row, r1's refused five_hour
// event pauses the night, the step clock still. Then a `stop` stops it
// owner-stop at its next poll, the status never reading draining; `kill
// r1`, `kill *` and a SIGINT stop it owner-kill, with no `kill` line (a
// paused row has no child). Each: r1 failed usage-limit, its stage
// unlocked, its tree kept, one sitting line, `relay resume r1` in the stop
// line. Red on the pin: a stop or `kill r1` while paused was ignored.
func TestAR66_1_OwnerWhilePaused(t *testing.T) {
	for _, c := range []struct {
		name, line string // line "": a SIGINT on Signals
		want       relay.StopReason
	}{
		{"stop", "stop", relay.OwnerStop},
		{"kill r1", "kill r1", relay.OwnerKill},
		{"kill *", "kill *", relay.OwnerKill},
		{"SIGINT", "", relay.OwnerKill},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, q, o, sp, _ := ar65World(t, ar65Event("five_hour", ar65T.Add(time.Hour)), "r1")
			sig := make(chan os.Signal, 1)
			o.Signals = sig
			res := ar65Start(w, q, o, sp)
			ar65Until(t, "the pause", func() bool { return ar65Paused(o, "r1") })
			var drained atomic.Bool
			quit := make(chan struct{})
			watched := make(chan struct{})
			go func() { // the status, every millisecond, until the night ends
				defer close(watched)
				for {
					select {
					case <-quit:
						return
					default:
					}
					if night.ReadStatus(effects.OSFS{}, o.State, o.PlanOptions.Night).State == string(relay.Draining) {
						drained.Store(true)
					}
					time.Sleep(time.Millisecond)
				}
			}()
			if c.line == "" {
				sig <- syscall.SIGINT
			} else if err := night.WriteControl(effects.OSFS{}, night.ControlPath(o.State, o.PlanOptions.Night), c.line); err != nil {
				t.Fatal(err)
			}
			n := ar65End(t, res)
			close(quit)
			<-watched
			all := w.ledger.Written()
			stop := lastOf(all, "stop")
			wt := n.Plan.Rows[0].Worktree
			if n.State != relay.Stopped(c.want) || stop["reason"] != string(c.want) {
				t.Fatalf("night %s, the stop line's reason %v; want stopped:%s", n.State, stop["reason"], c.want)
			}
			if drained.Load() {
				t.Fatal("the paused night read draining on its way to the stop")
			}
			if s := n.Sittings["r1"]; s.State != relay.Failed || s.Reason != relay.UsageLimit {
				t.Fatalf("r1 %s %s, want failed usage-limit", s.State, s.Reason)
			}
			if night.Locked(wt) || len(rowLines(all, "sitting", "r1")) != 1 || fmt.Sprint(stop["resume"]) != "[relay resume r1]" {
				t.Fatalf("locked %v, sitting lines %d, the stop line's resume %v", night.Locked(wt), len(rowLines(all, "sitting", "r1")), stop["resume"])
			}
			if _, err := os.Stat(wt); err != nil {
				t.Fatalf("the tree is gone: %v", err)
			}
			if k := ar65Events(all, "kill"); len(k) != 0 {
				t.Fatalf("kill lines %v, want none: a paused row has no child", k)
			}
			if ol := ar65Events(all, "owner-stop"); (c.want == relay.OwnerStop) != (len(ol) == 1) || len(ol) > 1 {
				t.Fatalf("owner-stop lines %v", ol)
			}
			if sp.spawns() != 1 {
				t.Fatalf("%d spawns, want 1", sp.spawns())
			}
		})
	}
}
