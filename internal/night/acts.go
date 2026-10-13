package night

import (
	"strings"
	"unicode"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// Act is one commit on a row's branch since its staged base (AR30 act 3):
// the live view lists the acts as commits land.
type Act struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// ReadActs is the branch's commits from base, oldest first, by `git log
// --reverse --format=%H%x09%s <base>..refs/heads/<branch>` in root. base is
// the staged base's sha, never main's name, so a merge into main loses no
// act. A subject keeps its tabs; no commit is an empty list, never nil.
func ReadActs(g effects.Git, root, base, branch string) ([]Act, error) {
	out, err := g.Hermetic(root, "log", "--reverse", "--format=%H%x09%s", base+"..refs/heads/"+branch)
	if err != nil {
		return nil, err
	}
	acts := []Act{}
	for _, l := range strings.Split(out, "\n") {
		if l == "" {
			continue
		}
		sha, subject, _ := strings.Cut(l, "\t")
		acts = append(acts, Act{SHA: sha, Subject: subject})
	}
	return acts, nil
}

// actView is a running row's acts as the status writes read them: the
// branch, the base sha they are read from, and the tip last read.
type actView struct{ branch, base, tip string }

// viewOf is the acts view of a staged row: its base bound at stage, or for
// a stage read back from the ledger its claim, a display, never a binding.
func viewOf(st *Staged) *actView {
	base := st.BaseSha
	if base == "" {
		base = st.BaseClaim
	}
	return &actView{branch: st.Branch, base: base}
}

// read reads the acts when the branch tip moved since the last read: nil
// and false when it did not, or on any read error, so the acts already
// shown stay; the view never stops a night.
func (v *actView) read(g effects.Git, root string) ([]Act, bool) {
	if v == nil || v.base == "" {
		return nil, false
	}
	tip, err := g.Tip(root, v.branch)
	if err != nil || tip == v.tip {
		return nil, false
	}
	acts, err := ReadActs(g, root, v.base, v.branch)
	if err != nil {
		return nil, false
	}
	v.tip = tip
	return acts, true
}

// actsLine is one act as `relay status` prints it: two spaces, the sha's
// twelve, two spaces, the subject with control bytes as spaces, clipped so
// the line stays under 100 columns.
func actsLine(a Act) string {
	subject := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, a.Subject)
	return "  " + short(a.SHA) + "  " + clip(subject, 99-16)
}
