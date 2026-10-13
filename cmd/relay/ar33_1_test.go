package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
)

// watchWorld is a temp state dir with the watch's sleep, hold and
// PidStarted replaced: each sleep is recorded, then runs tick with its
// count (from 1); PidStarted answers started for the pids in live and
// counts its calls.
type watchWorld struct {
	state string
	slept []time.Duration
	held  int
	ps    int
	live  map[int]string
	tick  func(n int)
}

func newWatchWorld(t *testing.T) *watchWorld {
	t.Helper()
	home := t.TempDir()
	oldEnv, oldSleep, oldHold, oldPS := getenv, watchSleep, watchHold, night.PidStarted
	t.Cleanup(func() { getenv, watchSleep, watchHold, night.PidStarted = oldEnv, oldSleep, oldHold, oldPS })
	getenv = fakeEnv(home)
	w := &watchWorld{state: effects.StateDir(getenv), live: map[int]string{}, tick: func(int) {}}
	watchSleep = func(d time.Duration) {
		w.slept = append(w.slept, d)
		w.tick(len(w.slept))
	}
	watchHold = func() { w.held++ }
	night.PidStarted = func(pid int) (string, error) {
		w.ps++
		if s, ok := w.live[pid]; ok {
			return s, nil
		}
		return "", os.ErrNotExist
	}
	return w
}

// write puts the night's status.json (state, pid, started) or, with name
// groups.json, its groups doc.
func (w *watchWorld) write(t *testing.T, id, name string, doc map[string]any) {
	t.Helper()
	dir := night.NightDir(w.state, id)
	os.MkdirAll(dir, 0o700)
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (w *watchWorld) status(t *testing.T, id, state string, pid int, started string) {
	w.write(t, id, "status.json", map[string]any{"night": id, "state": state, "rows": []any{}, "resume": []any{}, "pid": pid, "started": started})
}

// frames runs `relay <args>` and splits its output at each redraw.
func (w *watchWorld) frames(t *testing.T, args ...string) (int, []string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errb)
	if errb.Len() > 0 {
		t.Logf("stderr: %s", errb.String())
	}
	return code, strings.Split(out.String(), night.Clear)[1:]
}

// TestAR33_1_WatchStopsWhenNightEnds (AR33 act 1): `status --night D
// --watch 3` redraws D, not the latest night, every 3 s in-process, and
// when D stops prints it once more and exits 0; --hold then waits. The
// relay's start time is read once (PidStarted, a ps), not per tick.
func TestAR33_1_WatchStopsWhenNightEnds(t *testing.T) {
	w := newWatchWorld(t)
	const id, later = "2026-09-29-3", "2026-09-29-4"
	me := os.Getpid()
	w.live[me] = "S0"
	w.status(t, id, "running", me, "S0")
	w.status(t, later, "running", me, "S0")
	w.tick = func(n int) {
		if n == 2 {
			w.status(t, id, "stopped:queue-empty", me, "S0")
		}
	}
	code, fr := w.frames(t, "status", "--night", id, "--watch", "3")
	if code != exitOK || len(fr) != 3 || len(w.slept) != 2 || w.slept[0] != 3*time.Second || w.held != 0 {
		t.Fatalf("watch = %d, %d frames, slept %v, held %d", code, len(fr), w.slept, w.held)
	}
	if !strings.HasPrefix(fr[0], "night "+id+": running") || !strings.HasPrefix(fr[2], "night "+id+": stopped:queue-empty") {
		t.Fatalf("frames %q", fr)
	}
	if strings.Contains(strings.Join(fr, ""), later) {
		t.Fatalf("the watch showed the latest night, not its own: %q", fr)
	}
	if w.ps != 1 {
		t.Fatalf("PidStarted ran %d times over 3 ticks, want 1", w.ps)
	}
	t.Logf("last frame: %q", fr[2])

	code, fr = w.frames(t, "status", "--night", id, "--watch", "5", "--hold")
	if code != exitOK || len(fr) != 1 || w.held != 1 || len(w.slept) != 2 {
		t.Fatalf("--hold = %d, %d frames, held %d, slept %v", code, len(fr), w.held, w.slept)
	}
	for _, bad := range [][]string{{"--watch", "0"}, {"--watch", "x"}, {"--watch", "86401"}, {"--watch", "9223372037"}, {"--hold"}, {"--watch", "5", "--json"}} {
		if code, _ := w.frames(t, append([]string{"status"}, bad...)...); code != exitUsage {
			t.Fatalf("status %q = %d, want %d", bad, code, exitUsage)
		}
	}
}

// TestAR33_1_WatchStopsWhenRelayGone (AR33 act 1): `status --watch` (the
// latest night) stops, with the dashboard once more, when the night's
// relay is live under another start time, when it dies mid-watch, and when
// a night with no status yet loses its groups.json (the relay's record
// before its first status).
func TestAR33_1_WatchStopsWhenRelayGone(t *testing.T) {
	w := newWatchWorld(t)
	const id = "2026-09-29-5"
	me := os.Getpid()
	w.live[me] = "S-now"
	w.status(t, id, "running", me, "S-then")
	code, fr := w.frames(t, "status", "--watch", "5")
	if code != exitOK || len(fr) != 1 || len(w.slept) != 0 || !strings.HasPrefix(fr[0], "night "+id+": running") {
		t.Fatalf("another start time: %d, %q, slept %v", code, fr, w.slept)
	}

	w = newWatchWorld(t)
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Process.Kill(); child.Wait() })
	w.live[child.Process.Pid] = "S0"
	w.status(t, id, "running", child.Process.Pid, "S0")
	w.tick = func(n int) {
		if n == 2 {
			child.Process.Kill()
			child.Wait()
		}
	}
	code, fr = w.frames(t, "status", "--watch", "5")
	if code != exitOK || len(fr) != 3 || len(w.slept) != 2 || w.slept[0] != 5*time.Second || w.ps != 1 {
		t.Fatalf("relay died: %d, %d frames, slept %v, ps %d", code, len(fr), w.slept, w.ps)
	}

	w = newWatchWorld(t)
	w.live[me] = "S0"
	w.status(t, "2026-09-29-1", "running", me, "S0")
	w.write(t, id, night.GroupsFile, map[string]any{"pid": me, "started": "S0", "groups": []any{}})
	w.tick = func(int) { os.Remove(filepath.Join(night.NightDir(w.state, id), night.GroupsFile)) }
	code, fr = w.frames(t, "status", "--watch", "5")
	if code != exitOK || len(fr) != 2 || len(w.slept) != 1 || !strings.HasPrefix(fr[1], "night "+id) {
		t.Fatalf("groups.json: %d, %q, slept %v", code, fr, w.slept)
	}
	t.Logf("last frame: %q", fr[1])
}
