package packet

import (
	"strings"
	"testing"
)

// TestAR97_2_ColonSlashRefused (AR97 ruling 2): a git operand that names a
// commit by `:/<text>`, alone, on either side of `..` or `...`, after `^`,
// after `--` or as a separate option's value, is refused with the loud
// form `HEAD^{/<text>}`. Amendment 1 (the refuter's F1, F2): a brace the
// shell expands, `--stdin` and `--ancestry-path=:/x` carry the search
// too, each refused with its own reason, and `HEAD...:/x` pins the `...`
// split.
func TestAR97_2_ColonSlashRefused(t *testing.T) {
	for _, l := range []string{
		"git log -1 ':/a b'", "git diff :/a HEAD", "git log HEAD..:/x",
		"git log :/x...HEAD", "git log HEAD...:/x", "git log ^:/x", "git log -- :/x",
		"git log --grep :/x",
	} {
		if why := Unbounded(l); !strings.Contains(why, "HEAD^{/") {
			t.Errorf("Unbounded(%q) = %q; want a refusal naming HEAD^{/", l, why)
		}
	}
	for l, want := range map[string]string{
		"git log -1 {:/x,HEAD}":                   "has a brace the shell may expand",
		"git log -1 ^{:/x,HEAD}":                  "has a brace the shell may expand",
		"git log -1 HEAD^{9..:}":                  "has a brace the shell may expand",
		"echo ':/x' | git log -1 --stdin":         "reads its revisions from the input",
		"git log -1 --ancestry-path=:/x HEAD":     "names its commit by",
		"git log -1 --ancestry-path={a,:/x} HEAD": "names its commit by",
		"git log -1 --ancestry-path :/x HEAD":     "HEAD^{/",
	} {
		if why := Unbounded(l); !strings.Contains(why, want) {
			t.Errorf("Unbounded(%q) = %q; want a refusal with %q", l, why, want)
		}
	}
}

// TestAR97_3_LoudFormPasses (AR97 ruling 2): `HEAD^{/<text>}` and a
// `--grep=` value pass; a packet with a `:/` line does not parse.
func TestAR97_3_LoudFormPasses(t *testing.T) {
	for _, l := range []string{"git log -1 'HEAD^{/a b}'", "git log --grep=:/x",
		"git log 'HEAD^{/x}..HEAD'", "git show HEAD^{commit}", "git log --ancestry-path=HEAD~1 HEAD"} {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q, want \"\"", l, why)
		}
	}
	if err := withLine(t, "ar97-x", "git log -1 ':/qzvq' --oneline"); err == nil || !strings.Contains(err.Error(), "HEAD^{/") {
		t.Errorf("Parse of a `:/` line = %v; want a refusal naming HEAD^{/", err)
	}
}

// TestAR97_4_GoTestC (AR97 ruling 3): `go test -C <dir>` passes as the
// first flag with a relative dir and no `..`; any other place, a dir that
// leaves the worktree or a variable is refused.
func TestAR97_4_GoTestC(t *testing.T) {
	for _, l := range []string{
		"go test -C sub ./internal/packet -run TestX",
		"go test --C=a/b ./internal/packet -run TestX",
		"go test -C . ./internal/packet -run TestX",
	} {
		if why := Unbounded(l); why != "" {
			t.Errorf("Unbounded(%q) = %q, want \"\"", l, why)
		}
	}
	for l, want := range map[string]string{
		"go test -C ../x ./internal/packet -run TestX":    "leaves the worktree",
		"go test -C a/../.. ./internal/packet -run TestX": "leaves the worktree",
		"go test -C /abs ./internal/packet -run TestX":    "leaves the worktree",
		"go test -C '$D' ./internal/packet -run TestX":    "names a variable",
		"go test -C a -C b ./internal/packet -run TestX":  "must be the first flag",
		"go test ./internal/packet -C sub -run TestX":     "must be the first flag",
		"go test ./internal/packet -run TestX -C=sub":     "must be the first flag",
		"go test -C": "wants a value",
	} {
		if why := Unbounded(l); !strings.Contains(why, want) {
			t.Errorf("Unbounded(%q) = %q; want a refusal with %q", l, why, want)
		}
	}
}
