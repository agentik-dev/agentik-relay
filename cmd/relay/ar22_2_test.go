package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/night"
)

// lockPortEnv hands this test binary's lock port to the children it
// re-executes (pidHolder): the tests' plumbing alone, the relay reads no
// port from the environment.
const lockPortEnv = "AR22_2_RELAY_LOCK_PORT"

// testLockPort is this test binary's lock port (TestMain).
var testLockPort int

// handed is every port freeLockPort has handed out: a built binary's
// never repeats another's (the e2e cases run in parallel).
var handed sync.Map

// freeLockPort is a free 127.0.0.1 port, never one handed out before; 0
// (an invalid linked port: the binary refuses, never the account's) when
// none is found.
func freeLockPort() int {
	for range 16 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if _, dup := handed.LoadOrStore(port, true); !dup {
			return port
		}
	}
	return 0
}

// useTestLockPort (TestMain) points night.LockPort at a free port, or at
// the parent test binary's: no test binds the account's.
func useTestLockPort() error {
	port, err := strconv.Atoi(os.Getenv(lockPortEnv))
	if err != nil || port <= 0 {
		if port = freeLockPort(); port == 0 {
			return errors.New("no free lock port")
		}
		os.Setenv(lockPortEnv, strconv.Itoa(port))
	}
	testLockPort = port
	night.LockPort = func() (int, error) { return port, nil }
	return nil
}

// TestAR22_2_HandGateHoldsLock (ADV13-14): relay gate takes the account's
// lock. While a live relay holds it, the gate refuses, exit 2, naming it,
// and never reads the root; while the gate runs it is the holder; once it
// returns the lock is free.
func TestAR22_2_HandGateHoldsLock(t *testing.T) {
	home := t.TempDir()
	getenv = fakeEnv(home)
	oldTop := toplevel
	t.Cleanup(func() { getenv, toplevel = os.Getenv, oldTop })
	dir, err := night.PidDir()
	if err != nil {
		t.Fatal(err)
	}
	var during error
	reached := false
	toplevel = func() (string, error) {
		reached = true
		_, during = night.PidFile(dir) // the gate holds it: refused, naming this process
		return t.TempDir(), nil
	}
	gateRun := func() (int, string) {
		var out, errb bytes.Buffer
		code := run([]string{"gate", "r1"}, strings.NewReader(""), &out, &errb)
		return code, errb.String()
	}

	release, err := night.PidFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	code, stderr := gateRun()
	release()
	if code != exitUsage || !strings.Contains(stderr, fmt.Sprintf("relay gate: nested: relay %d is live", os.Getpid())) || reached {
		t.Fatalf("relay gate under a live holder: exit %d, reached the root %v, stderr %q", code, reached, stderr)
	}

	gateRun()
	var ne night.NestedError
	if !reached || !errors.As(during, &ne) || ne.Pid != os.Getpid() {
		t.Fatalf("while relay gate ran, a second taker: reached %v, %v", reached, during)
	}
	again, err := night.PidFile(dir)
	if err != nil {
		t.Fatalf("after relay gate returned: %v", err)
	}
	again()
}
