package effects

import (
	"io"
	"testing"
)

// TestAR67_1_FakeOuts (AR67 ruling 1): the n-th Start gets Outs[n], its
// stream and its exit; past the last, Out and Exit as before.
func TestAR67_1_FakeOuts(t *testing.T) {
	s := &FakeSpawner{Out: []byte("after\n"), Exit: 7,
		Outs: []FakeRun{{Out: []byte("one\n"), Exit: 143}, {Out: []byte("two\n"), Exit: 0}}}
	for i, want := range []struct {
		out  string
		exit int
	}{{"one\n", 143}, {"two\n", 0}, {"after\n", 7}, {"after\n", 7}} {
		p, out, err := s.Start(Spawn{Argv: []string{"claude"}})
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(out)
		if exit, _ := p.Wait(); exit != want.exit || string(got) != want.out {
			t.Fatalf("start %d: out %q exit %d, want %q %d", i+1, got, exit, want.out, want.exit)
		}
	}
	if len(s.Spawns()) != 4 {
		t.Fatalf("spawns %d, want 4", len(s.Spawns()))
	}
}
