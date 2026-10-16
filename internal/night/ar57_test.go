package night

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// boundedCanonical is the sha256 of the one canonical scripts/bounded,
// agentik-commons', byte-identical in every repo that carries a copy (AR57).
const boundedCanonical = "188adff23545f99159a189174baf3771f7e790e5f1256958b0e3e3ac4ed11306"

// TestAR57_1_BoundedIsCanonical (AR57, ruling 1, 5): scripts/bounded is the
// canonical copy byte for byte, and keeps its executable mode.
func TestAR57_1_BoundedIsCanonical(t *testing.T) {
	path := filepath.Join(copyIn(t, "../..", "scripts/bounded"), "scripts", "bounded")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != boundedCanonical {
		t.Errorf("sha256 of scripts/bounded = %s, want the canonical %s", got, boundedCanonical)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 != 0o111 {
		t.Errorf("scripts/bounded's mode = %v, want executable", fi.Mode().Perm())
	}
}

// TestAR57_2_NestedStop (AR57, ADV11-2): a bounded started at depth 1 that
// finds its lock held writes `bounded: nested stop: lock` as its last
// stderr line, exits 2, and never runs the command.
func TestAR57_2_NestedStop(t *testing.T) {
	py := boundedPython(t)
	dir := t.TempDir()
	lock := filepath.Join(dir, "held.lock")
	f, err := os.OpenFile(lock, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(f, "%d %d\n", os.Getpid(), time.Now().Unix())
	ran := filepath.Join(dir, "ran")
	code, stderr := runBoundedEnv(t, py, []string{"BOUNDED_DEPTH=1"}, "--lock", lock, "--wall", "5", "--", "touch", ran)
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	_, statErr := os.Stat(ran)
	if code != 2 || lines[len(lines)-1] != "bounded: nested stop: lock" || statErr == nil {
		t.Fatalf("BOUNDED_DEPTH=1, the lock held: exit %d, ran %v, stderr %q; want exit 2, unrun, the last line `bounded: nested stop: lock`", code, statErr == nil, stderr)
	}
}

// TestAR57_3_GuardianTakesB (AR57, AR52's line): the guardian, the
// command's parent, is started with -B, so it writes no bytecode beside
// scripts/bounded.
func TestAR57_3_GuardianTakesB(t *testing.T) {
	py := boundedPython(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "guardian-args")
	code, stderr := runBounded(t, py, "--wall", "5", "--", "/bin/sh", "-c", `ps -o args= -p $PPID > "$1"`, "sh", out)
	b, err := os.ReadFile(out)
	if code != 0 || err != nil {
		t.Fatalf("the probe: exit %d, %v, stderr %q", code, err, stderr)
	}
	fields := strings.Fields(string(b))
	guard, flag := -1, -1
	for i, f := range fields {
		if f == "--guard" && guard < 0 {
			guard = i
		}
		if f == "-B" && flag < 0 {
			flag = i
		}
	}
	if guard < 0 || flag < 0 || flag > guard {
		t.Fatalf("the guardian's argv %q; want -B before --guard", strings.TrimSpace(string(b)))
	}
}
