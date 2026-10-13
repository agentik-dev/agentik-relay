package effects

import (
	"bytes"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

// TestEnvStripped: the child's environment has no CLAUDE* variable but the
// session budget and no RELAY_* but RELAY_ROW and RELAY_MEASURE_DEPTH (§7,
// §15, R25-91 (3)), in
// ChildEnv and in a real child.
func TestEnvStripped(t *testing.T) {
	in := []string{"HOME=/h", "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDE_PLUGIN_ROOT=/p",
		"RELAY_ROW=old", "RELAY_PLUGIN=x", "PATH=/usr/bin:/bin", "MYCLAUDE=kept"}
	env := ChildEnv(in, "a7", "", "", 80, nil, "")
	want := []string{"HOME=/h", "PATH=/usr/bin:/bin", "MYCLAUDE=kept", "RELAY_ROW=a7", "RELAY_MEASURE_DEPTH=1", "CLAUDE_SESSION_DEPTH=1", "CLAUDE_SESSION_MAX_TURNS=80"}
	if strings.Join(env, " ") != strings.Join(want, " ") {
		t.Fatalf("ChildEnv = %v, want %v", env, want)
	}
	var copied bytes.Buffer
	p, out, err := OSSpawner{}.Start(Spawn{Dir: t.TempDir(), Argv: []string{"/usr/bin/env"},
		Env: ChildEnv(append(os.Environ(), "CLAUDECODE=1"), "a7", "", "", 80, nil, ""), Stdout: &copied})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(out)
	if exit, err := p.Wait(); exit != 0 || err != nil {
		t.Fatalf("env: exit %d %v", exit, err)
	}
	for _, line := range strings.Split(string(got), "\n") {
		if (strings.HasPrefix(line, "CLAUDE") && line != "CLAUDE_SESSION_DEPTH=1" && line != "CLAUDE_SESSION_MAX_TURNS=80") || (strings.HasPrefix(line, "RELAY_") && line != "RELAY_ROW=a7" && line != "RELAY_MEASURE_DEPTH=1") {
			t.Fatalf("the child saw %q", line)
		}
	}
	if !strings.Contains(string(got), "RELAY_ROW=a7\n") || copied.String() != string(got) {
		t.Fatalf("RELAY_ROW missing or the copy differs:\n%s", got)
	}
}

// TestSpawnStdinAndExit: the prompt reaches stdin; a nonzero exit is the
// code with stderr's tail; a signal reaches the group and reads 128+n.
func TestSpawnStdinAndExit(t *testing.T) {
	p, out, err := OSSpawner{}.Start(Spawn{Dir: t.TempDir(), Argv: []string{"/bin/sh", "-c", "cat; echo bad >&2; exit 3"},
		Stdin: []byte("the prompt\n"), Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(out)
	exit, err := p.Wait()
	if string(got) != "the prompt\n" || exit != 3 || err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("out %q exit %d err %v", got, exit, err)
	}
	p, out, err = OSSpawner{}.Start(Spawn{Dir: t.TempDir(), Argv: []string{"/bin/sh", "-c", "sleep 30 & wait"}, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := p.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	io.ReadAll(out) // EOF only once the grandchild sleep is gone too
	if exit, _ := p.Wait(); exit != 128+int(syscall.SIGTERM) || time.Since(start) > 10*time.Second {
		t.Fatalf("exit %d after %v", exit, time.Since(start))
	}
}

func TestFakeSpawner(t *testing.T) {
	s := &FakeSpawner{Out: []byte("line\n"), Exit: 1}
	p, out, err := s.Start(Spawn{Argv: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(out)
	p.Signal(syscall.SIGTERM)
	if exit, _ := p.Wait(); exit != 1 || string(got) != "line\n" || len(s.Spawns()) != 1 || len(s.Signals()) != 1 {
		t.Fatalf("exit %d out %q spawns %d signals %d", exit, got, len(s.Spawns()), len(s.Signals()))
	}
}

// AR2-23: 401 bytes of two-byte runes and one ASCII byte; the last StderrCap
// bytes begin mid-rune, and the tail must start on a rune.
func TestAR6_StderrTailStartsOnRune(t *testing.T) {
	var b tailBuffer
	b.Write([]byte(strings.Repeat("é", 200) + "a"))
	if s := b.String(); !utf8.ValidString(s) || !strings.HasSuffix(s, "a") {
		t.Fatalf("tail %q: valid %v", s, utf8.ValidString(s))
	}
}
