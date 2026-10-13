package packet

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestAR20_5_AcceptanceAllowlist (ADV11-10, the session's cells): an
// acceptance line passes only `go test` of named packages with a -run that
// does not match every test, and the read-only verbs; every cell §1 quotes
// and every shape the review ran is refused, and this packet's own lines
// pass.
func TestAR20_5_AcceptanceAllowlist(t *testing.T) {
	refused := []string{
		// the session's cells (§1)
		"caffeinate -i go test ./internal/packet -run TestX",
		"stdbuf -oL go test ./internal/packet -run TestX",
		"arch -arm64 go test ./internal/packet -run TestX",
		"echo ./internal/packet | xargs go test -run TestX",
		"go test -run . ./...", "go test all", "go test -run TestX all",
		"make readme", "make install",
		// the review's list (ADV11-10)
		"scripts/bounded -- go test ./...",
		"go test -run '^' ./internal/gate ./internal/night",
		"go test -run '.+' ./internal/gate ./internal/night",
		"make lint", "go test -tags e2e ./cmd/relay -run TestE2E_0_",
		"relay gate ar20-x", "relay resume", "python3 -c 'import os'",
		"git -c alias.x='!make check' x",
		// -run that matches every test, or none given
		"go test ./internal/packet", "go test ./internal/packet -run",
		"go test ./internal/packet -run ''", "go test ./internal/packet -run=",
		"go test ./internal/packet -run '.*'", "go test ./internal/packet -run '^Test'",
		"go test ./internal/packet -run 'TestX|.'", "go test ./internal/packet -run Test",
		"go test ./internal/packet -run '(' ", "go test -run TestX",
		// packages that are not named
		"go test std -run TestX", "go test ./internal/$p -run TestX",
		"go test ./internal/... -run TestX",
		// flags off the list
		"go test ./internal/packet -run TestX -exec sh",
		"go test ./internal/packet -run TestX -args -test.run=.",
		"go test ./internal/packet -run TestX -o bin/x",
		// env prefixes and wrappers
		"FOO=1 go test ./internal/packet -run TestX",
		"GOFLAGS=-run=. go test ./internal/packet -run TestX",
		"env go test ./internal/packet -run TestX", "timeout 60 go test ./internal/packet -run TestX",
		// every pipeline and list segment is checked
		"go test ./internal/packet -run TestX 2>&1 | sh", "grep -c x y | xargs make",
		"echo ok; make check", "echo ok && make check", "echo ok || make check",
		"(make check)", "echo ok & make check",
		// substitutions anywhere
		"grep -c \"$(make check)\" f", "grep -c `make check` f", "echo $(date)",
		"cat <(make check)",
		// writes
		"sed -i s/a/b/ f", "sed 3p f", "sed -n '1w out' f", "sed -n 's/a/b/e' f",
		"echo x > out", "cat a >> b", "tail -f log", "git diff --output=f",
		"git log --ext-diff", "tee out",
		// verbs off the list
		"git push", "git commit -m x", "git checkout main", "git -C . log", "git",
		"sleep 45", "shasum -a 256 f", "go vet ./internal/packet", "go run ./cmd/relay plan --rows x",
		"claude -p hi", "bash -c 'echo hi'", "sh -c 'echo hi'", "/usr/bin/grep x f", "awk 1 f",
		"for p in a; do go test ./internal/$p -run TestX; done",
		"if true; then echo ok; fi", "xmllint --noout f", "ls x",
	}
	for _, l := range refused {
		if Unbounded(l) == "" {
			t.Errorf("Unbounded(%q) passes it", l)
		}
	}
	passed := []string{
		// this packet's own lines (its several-package lines refused since
		// AR21 act 4, TestAR21_4_OnePackage; the stem is spent)
		"go test ./cmd/relay -run TestAR20_ -count=1 2>&1 | grep -c FAIL",
		"grep -rhc '^func TestAR20_' internal cmd | paste -sd+ - | bc",
		"grep -c 'TestAR20_' design/relay-r1.md",
		"test -e history/sessions/NEXT; echo $?",
		// neighbours
		"go test ./internal/packet -run 'TestAR13_7_' -count=1; echo $?",
		"go test ./internal/effects -run 'TestX|TestY' -v 2>&1 | grep -c '^--- PASS'",
		"go test ./internal/packet -run=TestAR20_ -count=1 >/dev/null 2>&1; echo $?",
		"go test -race -run TestAR20_5_ -timeout 5m ./internal/packet",
		"grep -c 'relay plan' README.md", "sed -n 3p README.md", "sed -n '/a/,/b/p' f",
		"sed -n -E '2,$p' f", "wc -l < f", "git show HEAD:Makefile | grep -c x",
		"git log --oneline -1", "git rev-parse HEAD", "git diff --stat main",
		"cat a b | grep -c x", "tail -1 f", "head -3 f | cut -c1-2", "echo $HOME",
		"[ -f x ] && echo ok", "test -e x || echo no",
	}
	for _, l := range passed {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q, want \"\"", l, why)
		}
	}
	if err := withLine(t, "ar20-x", "make readme; echo $?"); err == nil || !strings.Contains(err.Error(), "unbounded") {
		t.Errorf("Parse of a `make readme` line = %v; want an unbounded refusal", err)
	}
}

// TestAR20_5_SpentStems: every packet in design/packets (read from a copy)
// still parses; the stems the allowlist refuses are exactly the spent ones,
// parsed as written and never rewritten; a new stem is bound.
func TestAR20_5_SpentStems(t *testing.T) {
	src, err := filepath.Glob(filepath.Join(copyIn(t, "../..", "design/packets"), "design", "packets", "*.md"))
	if err != nil || len(src) == 0 {
		t.Fatalf("no packets: %v", err)
	}
	var refusing []string
	for _, p := range src {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		pk, err := Parse(p, b)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		if legacy[pk.ID] {
			continue
		}
		for _, l := range pk.Acceptance {
			if Unbounded(l.Shell) != "" {
				refusing = append(refusing, pk.ID)
				break
			}
		}
	}
	var spentIDs []string
	for id := range spent {
		spentIDs = append(spentIDs, id)
	}
	sort.Strings(refusing)
	sort.Strings(spentIDs)
	if strings.Join(refusing, " ") != strings.Join(spentIDs, " ") {
		t.Errorf("stems the rule refuses:\n  %v\nspent:\n  %v", refusing, spentIDs)
	}
	for _, id := range []string{"ar10-bounded-night", "ar13-fixes-r6", "ar15-readme-r7", "ar17a-gate-r10",
		"ar17b-guards-r10", "ar17c-bounds-r10", "ar17d-rulings-r10", "ar18-resync", "ar20-fixes-adv11"} {
		if !spent[id] {
			t.Errorf("%s is not spent", id)
		}
	}
	for _, id := range []string{"ar21-fixes-adv12", "ar21-x", "ar10-x"} {
		if err := withLine(t, id, "make readme; echo $?"); err == nil {
			t.Errorf("%s is grandfathered", id)
		}
	}
}
