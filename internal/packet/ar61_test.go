package packet

import (
	"os"
	"path/filepath"
	"testing"
)

// ar61Example is the packet text the relay-nights analysis read (AR61 §1):
// it orders the verify line per act.
const ar61Example = "Each act: [...] its package tests, `make lint`, then the gate `make -k check-fast` in the foreground"

// ar61At is a packet with text at place: "wall", or "§N" for section N.
func ar61At(place, text string) *Packet {
	pk := &Packet{Wall: "this repo only"}
	if place == "wall" {
		pk.Wall = text
		return pk
	}
	pk.Sections[int(place[len(place)-1]-'1')] = text
	return pk
}

// TestAR61_4_OrdersPerAct (AR61 act 2, ruling 3): §1's example in §2 reads
// §2, in the wall wall, the wall before any section; §3, §5 and §6 are
// read; a code span holds the line's words as a run, `-` words dropped;
// each per-act spelling, case aside; an `e.g.` and a `;` inside a span end
// no clause; a quote not closed on its line is not dropped.
func TestAR61_4_OrdersPerAct(t *testing.T) {
	for _, c := range []struct {
		name, place, text, line, want string
	}{
		{"the example in §2", "§2", ar61Example, "make check-fast", "§2"},
		{"the example in the wall", "wall", ar61Example, "make check-fast", "wall"},
		{"§3", "§3", ar61Example, "make check-fast", "§3"},
		{"§5", "§5", ar61Example, "make check-fast", "§5"},
		{"§6", "§6", ar61Example, "make check-fast", "§6"},
		{"the example with -k check", "§2", "Each act: its package tests, then the gate `make -k check` in the foreground", "make check", "§2"},
		{"a flag and an argument", "§2", "Each act runs `make -k check CHECK_GATES=\"vet lint\"` at its end.", "make check", "§2"},
		{"every act", "§2", "Every act ends with `make check`.", "make check", "§2"},
		{"per act", "§2", "Run `make check` per act.", "make check", "§2"},
		{"after an act", "§2", "After an act, `make check` in the foreground.", "make check", "§2"},
		{"act gate", "§2", "The act gate is `make check`.", "make check", "§2"},
		{"acts and a number", "§2", "Acts 3 and 4 each end on `make check`.", "make check", "§2"},
		{"case aside", "§2", "EACH ACT ends on `make check`.", "make check", "§2"},
		{"e.g. ends no clause", "§2", "Each act runs its tests, e.g. `go test ./x`, then `make check`.", "make check", "§2"},
		{"a span's ; ends no clause", "§2", "Each act runs `cd x; make check` in its tree.", "make check", "§2"},
		{"an unclosed quote stays", "§2", "Each act runs \"the gate `make check` in the foreground", "make check", "§2"},
		{"a later line", "§2", "Intro.\n\nThe act gate: `make check` in the foreground.", "make check", "§2"},
	} {
		if got := ar61At(c.place, c.text).OrdersPerAct(c.line); got != c.want {
			t.Errorf("%s: OrdersPerAct(%q) = %q, want %q", c.name, c.line, got, c.want)
		}
	}
	pk := ar61At("wall", ar61Example)
	pk.Sections[1] = ar61Example
	if got := pk.OrdersPerAct("make check-fast"); got != "wall" {
		t.Errorf("the wall and §2: %q, want wall (read first)", got)
	}
}

// TestAR61_5_QuietGuards (AR61 act 2, ruling 3): each reads "": the
// example only in §1, §4 or in double quotes; with `never`; with `once,
// before the last commit`; the act and the span in two sentences; `make
// check-fast` for `make check`; an empty line; other guard words.
func TestAR61_5_QuietGuards(t *testing.T) {
	for _, c := range []struct {
		name, place, text, line string
	}{
		{"§1 quotes", "§1", ar61Example, "make check-fast"},
		{"§4 quotes", "§4", ar61Example, "make check-fast"},
		{"in double quotes", "§2", "The analysis read \"" + ar61Example + "\" in one packet.", "make check-fast"},
		{"never", "§2", "Each act: never run `make check` in the foreground.", "make check"},
		{"once, before the last commit", "§2", "Each act ends with `make check` once, before the last commit.", "make check"},
		{"two sentences", "§2", "Each act runs its package tests. The gate is `make check`.", "make check"},
		{"two clauses", "§2", "Each act runs its package tests; the gate is `make check`.", "make check"},
		{"make check-fast is not make check", "§2", ar61Example, "make check"},
		{"no line", "§2", ar61Example, ""},
		{"no", "§2", "Each act: no `make check` there.", "make check"},
		{"not", "§2", "Each act does not run `make check`.", "make check"},
		{"don't", "§2", "Each act: don't run `make check`.", "make check"},
		{"last", "§2", "Each act runs `make check` as its last step.", "make check"},
		{"final", "§2", "Per act the tests, the final commit after `make check`.", "make check"},
		{"another line", "§2", "Each act runs its tests\n`make check` closes the row.", "make check"},
		{"act alone", "§2", "Act 1 runs `make check`.", "make check"},
	} {
		if got := ar61At(c.place, c.text).OrdersPerAct(c.line); got != "" {
			t.Errorf("%s: OrdersPerAct(%q) = %q, want \"\"", c.name, c.line, got)
		}
	}
}

// TestAR61_6_CorpusQuiet (AR61 act 2, ruling 3): no packet in
// design/packets, read from a copy as TestAR20_5_SpentStems reads it,
// orders `make check` per act.
func TestAR61_6_CorpusQuiet(t *testing.T) {
	src, err := filepath.Glob(filepath.Join(copyIn(t, "../..", "design/packets"), "design", "packets", "*.md"))
	if err != nil || len(src) == 0 {
		t.Fatalf("no packets: %v", err)
	}
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
		if got := pk.OrdersPerAct("make check"); got != "" {
			t.Errorf("%s: orders make check per act at %s", filepath.Base(p), got)
		}
	}
}
