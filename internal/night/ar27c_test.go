package night

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// TestAR27C_2_ClaudeByRealFile (review-adv16 test 3, the breaker;
// ADV16-3): the pin by PinAt with a fixture claude first on PATH; a
// claude's comm is argv[0], so a fake Procs gives comm zz and ./c with
// Exes the real file, its symlink, or a copy (the size, then the bound
// sum): three trip helper-storm, two do not; a same-size file with
// another sum is none, nor is a comm with no Exe.
func TestAR27C_2_ClaudeByRealFile(t *testing.T) {
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, t.TempDir())
	}
	dir := t.TempDir()
	for _, d := range []string{"versions", "bin", "cwd", "other"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(dir, "versions", "2.1.282")
	write := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(real, "#!/bin/sh\nexit 0 # a claude\n")
	write(filepath.Join(dir, "other", "copy"), "#!/bin/sh\nexit 0 # a claude\n")
	write(filepath.Join(dir, "other", "same-size"), "#!/bin/sh\nexit 1 # a claude\n")
	for _, l := range []string{filepath.Join(dir, "bin", "claude"), filepath.Join(dir, "cwd", "c")} {
		if err := os.Symlink(real, l); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	old, _ := effects.Current()
	tools := effects.PinTools
	t.Cleanup(func() { effects.Use(old); effects.PinTools = tools })
	effects.PinTools = []string{"claude"}
	p, err := effects.PinAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Release() })

	storm := func(comms, exes map[int]string) relay.StopReason {
		t.Helper()
		procs := &effects.FakeProcs{Mtime: time.Now(), Comms: comms, Exes: exes,
			Desc:   map[int][]int{4242: {4243, 4244, 4245, 4246}},
			Starts: map[int]string{4243: "t1", 4244: "t2", 4245: "t3", 4246: "t4"}}
		wd := Watchdog{Git: &effects.FakeGit{Heads: map[string]string{"/wt": "abc"}}, Procs: procs,
			Clock: effects.FakeClock{T: time.Now()}, Dir: "/wt", Pgid: 4242, Helpers: MaxHelpers}
		done := make(chan struct{})
		go func() { time.Sleep(50 * time.Millisecond); close(done) }()
		_, why := wd.Run(done)
		return why
	}
	comms := map[int]string{4243: "zz", 4244: "./c", 4245: "zz", 4246: "sh"}
	three := map[int]string{4243: real, 4244: filepath.Join(dir, "cwd", "c"), 4245: filepath.Join(dir, "other", "copy"), 4246: "/bin/sh"}
	if why := storm(comms, three); why != relay.HelperStorm {
		t.Fatalf("zz, ./c and a copy by executable: Run %q; want helper-storm", why)
	}
	two := map[int]string{4243: real, 4244: filepath.Join(dir, "cwd", "c"), 4245: filepath.Join(dir, "other", "same-size"), 4246: "/bin/sh"}
	if why := storm(comms, two); why != "" {
		t.Fatalf("two claudes and a same-size file with another sum: Run %q; want no storm", why)
	}
	delete(three, 4245)
	if why := storm(comms, three); why != "" {
		t.Fatalf("a comm with no executable read: Run %q; want no storm", why)
	}
	if effects.IsClaude("zz", filepath.Join(dir, "other", "same-size")) || !effects.IsClaude("zz", filepath.Join(dir, "other", "copy")) {
		t.Fatal("IsClaude keys a copy by its size and sum")
	}
}
