package relay

import "strings"

// The resume line's prefixes: the one written, and the one a ledger or a
// status.json from before agentik-r1 holds (§4.3, §6.3).
const (
	resumePrefix    = "agentik relay resume "
	oldResumePrefix = "relay resume "
)

// ResumeLine is the one line a stop, a report, a status or a notice prints
// to resume row: `agentik relay resume <row>` (agentik-r1 §4.3).
func ResumeLine(row string) string { return resumePrefix + row }

// ResumeRow reads a resume line, ResumeLine's or the old `relay resume
// <row>`: the row and true, else "" and false (a row is one word).
func ResumeRow(line string) (row string, ok bool) {
	for _, p := range []string{resumePrefix, oldResumePrefix} {
		if r, ok := strings.CutPrefix(line, p); ok && r != "" && !strings.ContainsAny(r, " \t\r\n") {
			return r, true
		}
	}
	return "", false
}
