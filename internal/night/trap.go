package night

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// Trap is the measuring verbs' trap (AR17C, ADV10-5): `run`, `plan`,
// `gate` and `resume` install it in main, before loadPlan or anything
// they measure. INT, TERM and HUP are heard once: the first sends every
// group in reg SIGTERM, then SIGKILL after KillGrace (Groups.Kill, so a
// group added later is signalled at once), and cancels the context with
// `signal:<name>` as its cause. A second signal is the default's: the
// process dies. The stop function ends the trap; after a signal it waits
// up to KillGrace and a second for reg to drain, so the verb exits 1 with
// nothing it started alive. night.Run keeps its own trap for the night's
// stop line; both hear the signal and both kill the same registry.
func Trap(ctx context.Context, reg *effects.Groups) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done, fired := make(chan struct{}), make(chan struct{})
	go func() {
		select {
		case sig := <-ch:
			signal.Stop(ch) // once: a second signal is the default's
			reg.Kill(KillGrace)
			cancel(errors.New("signal:" + signalName(sig)))
			close(fired)
		case <-done:
		}
	}()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			signal.Stop(ch)
			close(done)
			select {
			case <-fired:
				Drain(reg, KillGrace+time.Second)
			default:
			}
			cancel(nil)
		})
	}
}

// Signalled is the trapped signal's `signal:<name>`, "" when none came.
func Signalled(ctx context.Context) string {
	if ctx.Err() == nil {
		return ""
	}
	if c := context.Cause(ctx); c != nil && c != context.Canceled {
		return c.Error()
	}
	return ""
}

// Drain waits until reg tracks no group, or d.
func Drain(reg *effects.Groups, d time.Duration) {
	for end := time.Now().Add(d); len(reg.Live()) > 0 && time.Now().Before(end); {
		time.Sleep(20 * time.Millisecond)
	}
}

// WatchGroups keeps reg in <state>/nights/<night>/groups.json from now on
// (ADV6-2, AR17C): the file names this relay (pid, start time), the root
// it measures ("" for none; AR37 fix, read by status and stop before the
// night's first status.json) and every live group, and is written only
// while a group is live, so a measurement that is never killed leaves no
// groups file behind, and under plan no night directory (under run the
// directory holds the night's planning mark, AR60). The stop function ends
// the watch and removes the file when no group is left (a survivor's record
// stays for the next start's Reap).
func WatchGroups(fx effects.Effects, reg *effects.Groups, state, night, root string) func() {
	if reg == nil || state == "" || night == "" {
		return func() {}
	}
	dir := NightDir(state, night)
	file, self := filepath.Join(dir, GroupsFile), groupsDoc{PID: os.Getpid(), Started: selfStarted(fx.Procs), Root: root}
	wrote := false
	reg.Watch(func(gs []effects.Group) {
		if len(gs) == 0 && !wrote {
			return
		}
		if err := fx.FS.MkdirAll(dir, 0o700); err != nil {
			return
		}
		doc := self
		doc.Groups = gs
		b, _ := json.Marshal(doc)
		if fx.FS.WriteFile(file, append(b, '\n'), 0o600) == nil {
			wrote = true
		}
	})
	return func() {
		reg.Watch(nil)
		if len(reg.Live()) == 0 {
			os.Remove(file)
			os.Remove(dir) // only when empty: a night of its own keeps its files
		}
	}
}
