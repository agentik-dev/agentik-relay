package night

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// greenShell answers every line green without running it.
type greenShell struct{}

func (greenShell) Run(dir, line string, timeout time.Duration) (int, string, error) {
	return 0, "ok", nil
}

// TestAR34Fix_StopDuringRootLockIsOwnerStop is ar34 review F2 (adversary
// 2): a stop said while a line waits on the root lock (another measure of
// the root holds it) cut the line as `budget`, since the stop marks the
// budget spent, so it counted in the plan's Budget and not in
// measure-stopped's unrun. The line reads owner-stop.
func TestAR34Fix_StopDuringRootLockIsOwnerStop(t *testing.T) {
	old := LockDir
	LockDir = func() string { return filepath.Join(t.TempDir(), "locks") }
	defer func() { LockDir = old }()
	root := t.TempDir()
	b := &boundShell{sh: greenShell{}, clock: effects.SystemClock{}, root: root, breaker: DefaultBreaker}
	b.ctx, b.cancel = context.WithCancel(context.Background())
	b.bound(10 * time.Minute)
	unlock, err := RootLock(root) // another measure of this root
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, _, err := b.Run(root, "echo ok", time.Minute); done <- err }()
	time.Sleep(100 * time.Millisecond) // Run parks on the root lock (were it not there yet, it reads the stop before the lock)
	b.stop("owner-stop", "stop")
	unlock()
	if err := <-done; err == nil || err.Error() != "owner-stop" {
		t.Fatalf("the line after the owner's stop: %v; want owner-stop", err)
	}
	if ran, _ := b.counts(); ran != 0 {
		t.Fatalf("%d lines ran after the stop", ran)
	}
}
