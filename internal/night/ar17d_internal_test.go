package night

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/queue"
)

// verifyRowFixture is the plan fixture with a4-schemas' packet carrying the
// verify row `row`.
func verifyRowFixture(t *testing.T, row string) (string, *queue.Queue) {
	t.Helper()
	repo, _ := fixture(t)
	path := filepath.Join(repo, "design", "packets", "a4-schemas.md")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "| budget |") })
	lines = slices.Insert(lines, i+1, "| verify | `"+row+"` |")
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	q, err := queue.Load(repo, "queue.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if q.Rows[1].Packet == nil || q.Rows[1].Packet.Verify != row {
		t.Fatalf("the packet's verify row did not parse: %+v", q.Rows[1].Packet)
	}
	return repo, q
}

// TestAR17D_1_VerifyRowMismatchRefused (run 10 ruling 6, ADV10-24): a
// packet whose verify row differs from the resolved gate line (`auto` is
// `make check`) is an invalid row: exit 2, the problem named, nothing
// measured, not even `make -n check`.
func TestAR17D_1_VerifyRowMismatchRefused(t *testing.T) {
	repo, q := verifyRowFixture(t, "go test ./...")
	fx, _, sh, _ := fakes(repo, false)
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a4-schemas"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "verify-row: packet verify row `go test ./...` differs from the gate line `make check`"
	if p.Valid || p.Exit() != 2 || len(sh.Calls()) != 0 {
		t.Fatalf("valid %v, exit %d, shell %v", p.Valid, p.Exit(), sh.Calls())
	}
	if rp := p.Rows[0]; rp.State != StateInvalid || !slices.Contains(rp.Problems, want) {
		t.Fatalf("a4 %s, problems %q", rp.State, rp.Problems)
	}
}

// TestAR17D_1_VerifyRowMatchPlans: a verify row that reads the gate line
// plans as before.
func TestAR17D_1_VerifyRowMatchPlans(t *testing.T) {
	repo, q := verifyRowFixture(t, "make check")
	fx, _, sh, _ := fakes(repo, false)
	p, err := BuildPlan(fx, q, Options{Repo: repo, State: "/state", Rows: []string{"a4-schemas"}})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Valid || p.Exit() != 0 || len(p.Rows[0].Problems) != 0 || len(sh.Calls()) == 0 {
		t.Fatalf("valid %v, exit %d, problems %q, shell %v", p.Valid, p.Exit(), p.Rows[0].Problems, sh.Calls())
	}
}

// TestAR17D_2_SignalNamedOnExit (run 10 ruling 7): a child killed by a
// signal reads `signal: killed` on its exit line, whole; an exit status
// stays `exit N`, the stderr after it never kept (§5).
func TestAR17D_2_SignalNamedOnExit(t *testing.T) {
	for _, c := range []struct{ script, want string }{
		{"kill -9 $$", "signal: killed"},
		{"echo the prompt >&2; exit 3", "exit 3"},
	} {
		p, out, err := effects.OSSpawner{}.Start(effects.Spawn{Dir: t.TempDir(), Argv: []string{"/bin/sh", "-c", c.script}})
		if err != nil {
			t.Fatal(err)
		}
		go io.Copy(io.Discard, out)
		_, werr := p.Wait()
		out.Close()
		if werr == nil || exitError(werr) != c.want {
			t.Errorf("%q: error %v, the exit line's %q; want %q", c.script, werr, exitErrorOf(werr), c.want)
		}
	}
}

func exitErrorOf(err error) string {
	if err == nil {
		return ""
	}
	return exitError(err)
}
