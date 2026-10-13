package packet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, name string) (*Packet, error) {
	t.Helper()
	p := filepath.Join("testdata", name)
	src, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return Parse(p, src)
}

// TestRepoPackets parses this repo's six packets, copied as fixtures.
func TestRepoPackets(t *testing.T) {
	files, _ := filepath.Glob("testdata/a[0-9]-*.md")
	if len(files) != 6 {
		t.Fatalf("%d repo packets in testdata; want 6", len(files))
	}
	for _, f := range files {
		pk, err := load(t, filepath.Base(f))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if len(pk.MayTouch) == 0 || len(pk.Acceptance) == 0 || pk.Turns == 0 || pk.Context == 0 {
			t.Errorf("%s: %d may-touch, %d acceptance, %d turns, %d context", f, len(pk.MayTouch), len(pk.Acceptance), pk.Turns, pk.Context)
		}
		for k, s := range pk.Sections {
			if s == "" {
				t.Errorf("%s: section %d empty", f, k+1)
			}
		}
	}
}

func TestA3(t *testing.T) {
	pk, err := load(t, "a3-machine.md")
	if err != nil {
		t.Fatal(err)
	}
	if pk.ID != "a3-machine" || pk.Turns != 100 || pk.Context != 110000 || pk.USD != 0 {
		t.Errorf("id %s, %d turns, %d context, $%d", pk.ID, pk.Turns, pk.Context, pk.USD)
	}
	if len(pk.MayTouch) != 13 || len(pk.Acceptance) != 8 {
		t.Errorf("%d may-touch, %d acceptance; want 13, 8", len(pk.MayTouch), len(pk.Acceptance))
	}
	for _, g := range pk.MayTouch {
		if g.Raw == "internal/hooks/" || g.Raw == "-race" {
			t.Errorf("may touch took %q, which lies in prose or after Nothing else", g.Raw)
		}
	}
	first := pk.Acceptance[0]
	if first.Shell != "make check; echo $?" || first.Kind != Reads || first.Value != "0" {
		t.Errorf("item 1 = %+v", first)
	}
	// The fifth item's `\|` is the table's escape: the shell line holds a pipe.
	fifth := pk.Acceptance[4]
	want := "go test ./internal/relay -run 'TestStepMatrix|TestNightMatrix|TestEveryStopHasReason|TestActions|TestDecide' -v 2>&1 | grep -c '^--- PASS'"
	if fifth.Shell != want || fifth.Kind != Reads || fifth.N != 5 {
		t.Errorf("item 5 = %+v\nwant shell %s", fifth, want)
	}
	if last := pk.Acceptance[7]; last.Shell != "git status --short | wc -l" || last.Value != "0" {
		t.Errorf("item 8 = %+v", last)
	}
}

func TestA4MayTouch(t *testing.T) {
	pk, err := load(t, "a4-schemas.md")
	if err != nil {
		t.Fatal(err)
	}
	var raws []string
	for _, g := range pk.MayTouch {
		raws = append(raws, g.Raw)
	}
	want := "internal/yaml/ internal/queue/ internal/packet/ relay/queue.yaml cmd/relay/main.go cmd/relay/main_test.go .hygiene-allow"
	if strings.Join(raws, " ") != want {
		t.Errorf("may touch = %v\nwant %s", raws, want)
	}
	if n := len(pk.Acceptance); n != 9 {
		t.Errorf("%d acceptance items; want 9", n)
	}
	orMore := pk.Acceptance[6]
	if orMore.Kind != ReadsOrMore || orMore.N != 8 {
		t.Errorf("item 7 = %+v; want reads 8 or more", orMore)
	}
}

func TestMinimal(t *testing.T) {
	pk, err := load(t, "ok-minimal.md")
	if err != nil {
		t.Fatal(err)
	}
	if pk.Title != "a fixture that breaks one rule" || pk.Pin != "branch `main`" || pk.Wall != "this repo only" {
		t.Errorf("title %q, pin %q, wall %q", pk.Title, pk.Pin, pk.Wall)
	}
	if pk.Turns != 10 || pk.Context != 20000 || pk.USD != 5 {
		t.Errorf("%d turns, %d context, $%d", pk.Turns, pk.Context, pk.USD)
	}
	if len(pk.MayTouch) != 2 || !pk.MayTouch[0].Dir || pk.MayTouch[1].Dir {
		t.Errorf("may touch = %+v", pk.MayTouch)
	}
	if pk.Sections[5] != "A table." {
		t.Errorf("section 6 = %q", pk.Sections[5])
	}
}

func TestRefusals(t *testing.T) {
	for _, r := range []struct{ file, want string }{
		{"refuse-oversize.md", "over the 9216-byte ceiling"},
		{"refuse-missing-row.md", "row budget: missing header row"},
		{"refuse-extra-row.md", "row owner: extra header row"},
		{"refuse-row-order.md", "row wall: out of order"},
		{"refuse-section-order.md", "section 4. Forbidden reads: out of order"},
		{"refuse-missing-section.md", "section 5. Forbidden reads: missing"},
		{"refuse-no-expectation.md", "row acceptance: item 1 (`make check; echo $?`): no expectation"},
		{"refuse-double-star.md", "row may touch: span \"internal/**/*.go\" holds **"},
		{"refuse-raw-pipe.md", "row acceptance: 3 cells, want 2: an unescaped |"},
		{"refuse-title-id.md", "title: id B2 does not name the file stem"},
		{"refuse-budget.md", "row budget: want"},
	} {
		t.Run(r.file, func(t *testing.T) {
			_, err := load(t, r.file)
			if err == nil || !strings.Contains(err.Error(), r.want) {
				t.Fatalf("Parse = %v; want %q", err, r.want)
			}
			if stem := strings.TrimSuffix(r.file, ".md"); !strings.HasPrefix(err.Error(), "packet "+stem+": ") {
				t.Errorf("error %q does not name the packet", err)
			}
		})
	}
}

func TestGlobMatch(t *testing.T) {
	g := func(raw string) Glob {
		pk := Glob{Raw: raw, Dir: strings.HasSuffix(raw, "/")}
		pk.Segs = strings.Split(strings.TrimSuffix(raw, "/"), "/")
		return pk
	}
	for _, r := range []struct {
		glob, path string
		want       bool
	}{
		{"internal/yaml/", "internal/yaml/yaml.go", true},
		{"internal/yaml/", "internal/yaml/testdata/accept/a.yaml", true},
		{"internal/yaml/", "internal/yamlx/yaml.go", false},
		{"internal/yaml/", "internal/yaml", false},
		{"cmd/relay/*.go", "cmd/relay/main.go", true},
		{"cmd/relay/*.go", "cmd/relay/sub/main.go", false},
		{"internal/*/testdata/*.md", "internal/packet/testdata/a3-machine.md", true},
		{".hygiene-allow", ".hygiene-allow", true},
		{".hygiene-allow", "x/.hygiene-allow", false},
		{"relay/queue.yaml", "./relay/queue.yaml", true},
		{"relay/queue.y?ml", "relay/queue.yaml", true},
	} {
		if got := g(r.glob).Match(r.path); got != r.want {
			t.Errorf("%s matches %s = %v; want %v", r.glob, r.path, got, r.want)
		}
	}
}

// AR2-17: the row budget is whole dollars; `$15.50` is refused, not read as 15.
func TestAR6_BudgetRefusesCents(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "ok-minimal.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		usd  string
		want int
		err  bool
	}{{"$15", 15, false}, {"$15.50", 0, true}} {
		s := strings.Replace(string(src), "| budget | 10 turns, 20k context, $5 |", "| budget | 10 turns, 20k context, "+c.usd+" |", 1)
		if s == string(src) {
			t.Fatal("fixture budget row not found")
		}
		pk, err := Parse("testdata/ok-minimal.md", []byte(s))
		if c.err {
			if err == nil || !strings.Contains(err.Error(), "row budget: want") {
				t.Errorf("%s: err %v, want the row budget refusal", c.usd, err)
			}
			continue
		}
		if err != nil || pk.USD != c.want {
			t.Errorf("%s: USD %v, err %v; want %d", c.usd, pk, err, c.want)
		}
	}
}
