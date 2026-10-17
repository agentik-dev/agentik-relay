package night

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// Report is the morning read (§9 `report`), built from the night's ledger
// lines only: what merged with its sha, what stopped the night and its
// `stop` line (as `key: value` lines in the markdown), the burn per sitting
// (builders included; nightBurn's, so a sitting in flight is counted), the
// branches waiting on a hand merge, the resume lines, and the `## Handover`
// section of every charter the rows committed on their branches.
type Report struct {
	Night     string     `json:"night"`
	State     string     `json:"state"`
	Merged    []Merged   `json:"merged"`
	Stopped   Stopped    `json:"stopped"`
	Sittings  []Burn     `json:"sittings"`
	Waiting   []string   `json:"waiting"`
	Resume    []string   `json:"resume"`
	Handovers []Handover `json:"handovers"`
	Band      BandHolds  `json:"band"`
}

// BandHolds is the night's band holds and their wait (AR81 ruling 5): a
// hold is a row's part with a `held` line; its wait is the first of its
// end line's, its `unchecked` line's or its `sitting` line's band wait_s
// (a sitting line with no band key: from the held line's ts to its own),
// and an open hold counts with no wait yet. Open counts the holds no line
// ended, OpenSince the earliest of their held lines' ts as the ledger wrote
// it (AR86): the report has no clock, so an open hold's wait is not counted
// to now. A held line with no ts (never the ledger's) is not counted open.
type BandHolds struct {
	Holds     int    `json:"holds"`
	WaitS     int    `json:"wait_s"`
	Open      int    `json:"open,omitempty"`
	OpenSince string `json:"open_since,omitempty"`
}

// NotStopped is a report's State when the night's lines hold a
// `night-open` line and no `stop` line (AR55): the JSON and the markdown
// title say the same words. It is not "running": the report reads the
// ledger only, and a night with no `stop` line may be live or may have died
// without writing one; `status` says which.
const NotStopped = "not stopped"

// NotOpened is the markdown title's state for a report with none (AR60 act
// 2, AR55's F4): a night with no `night-open` line whose status gave none
// either (TakeStatus), such as a plan whose relay is gone. Its JSON keeps
// `state` "".
const NotOpened = "not opened"

// TakeStatus gives a report with no state the night's status's (AR60 act
// 2): a night still planning reads `planning`, and a plan that opened no
// night reads its mark's end, `stopped:<reason>`, with that reason as the
// stop's, so the body agrees with the title (the review's N1). A report
// with a state (a `stop` line's, or NotStopped for an opened night) keeps
// it.
func (r *Report) TakeStatus(s Status) {
	if r.State == "" {
		r.State = s.State
		if r.Stopped.Reason == "" {
			r.Stopped.Reason = s.Reason
		}
	}
}

// Merged is one row merged into the root.
type Merged struct {
	Row string `json:"row"`
	SHA string `json:"sha"`
}

// Stopped is why the night ended, its `stop` line as written, and every row
// that did not end green.
type Stopped struct {
	Reason string       `json:"reason"`
	Line   string       `json:"line"`
	Rows   []StoppedRow `json:"rows"`
}

// StoppedRow is a row red, failed or skipped, and its resume line.
type StoppedRow struct {
	Row    string `json:"row"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	Resume string `json:"resume,omitempty"`
	Cause  string `json:"cause"` // the `sitting` line's gate.cause (AR62 act 2)
	// Shelved is a row the `stop` line's `shelved` names: its model's
	// limit held it to the night's end (AR92 ruling 5).
	Shelved bool `json:"shelved,omitempty"`
}

// Handover is a charter's `## Handover` section read from the row's branch.
type Handover struct {
	Row  string `json:"row"`
	Path string `json:"path"`
	Text string `json:"text"`
}

// GitShow is `git show <ref>:<path>` in the root; a variable so a test can
// fake it.
var GitShow = func(root, ref, path string) ([]byte, error) {
	return exec.Command("git", "-C", root, "show", ref+":"+path).Output()
}

// GitChanged is the paths under dir that branch changed since base; a
// variable so a test can fake it.
var GitChanged = func(root, base, branch, dir string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", "--no-renames", base+"..."+branch, "--", dir).Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// BuildReport is night's report from the ledger's lines (every night's; the
// night's are picked by their `night` field). With a git (AR56 ruling 1) a
// row merged by hand after the night (HandMerges) is in Merged at git's
// sha: a green one no longer waits, a red or failed one loses its resume
// line and keeps its Stopped entry, the night's verdict. Without one, the
// ledger alone. A row's verdict is its last `sitting` line's, as `status`
// reads it (AR74 ruling 2): each one first takes back what an earlier one
// of its row put in Stopped.Rows, Resume and the green rows, and so does a
// `spawn` line, the row's next part in flight, which `status` reads
// running; a `skip` line is untouched, and the burn keeps every sitting.
// Its handovers come once, at its stage, and none while its last sitting
// is in flight (ruling 3). A `sitting` line the night paused on (`limit:
// paused`) is no verdict (AR77 ruling 3, as `redSittings`): it takes back
// and holds its row, with no entry, until the row's next `spawn` or
// `sitting`; the night's `stop` line halts each row still held, failed
// usage-limit, with its resume line only when that line's `resume` names
// it, so a sitting the wake halted has none, as `status`, and Shelved when
// its `shelved` does (AR92 ruling 5).
func BuildReport(lines []map[string]any, night string, g ...effects.Git) Report {
	r := Report{Night: night, Merged: []Merged{}, Sittings: []Burn{}, Waiting: []string{}, Resume: []string{},
		Handovers: []Handover{}, Stopped: Stopped{Rows: []StoppedRow{}}}
	if night == "" {
		return r
	}
	var root string
	opened := false // a night-open line: only an opened night is NotStopped (AR60, AR55's F3)
	type staged struct{ branch, base string }
	stages := map[string]staged{}
	merged := map[string]bool{}
	green := []string{}
	var sat []string // the rows with a sitting line, first seen first
	type stop struct {
		StoppedRow
		sitting bool // a `sitting` line's, so a later one of its row takes it back (AR74)
	}
	var stops []stop
	// held is the rows whose last line is a paused sitting line, in its
	// order, and heldCause that line's gate.cause (AR77 ruling 3).
	var held []string
	heldCause := map[string]string{}
	takeBack := func(row string) { // an earlier sitting's verdict, its row's next line here (AR74)
		green = slices.DeleteFunc(green, func(id string) bool { return id == row })
		r.Resume = slices.DeleteFunc(r.Resume, func(x string) bool { id, ok := relay.ResumeRow(x); return ok && id == row })
		stops = slices.DeleteFunc(stops, func(s stop) bool { return s.sitting && s.Row == row })
		held = slices.DeleteFunc(held, func(id string) bool { return id == row })
	}
	type part struct {
		row string
		n   int
	}
	holding := map[part]string{} // the open band holds, each its held line's ts (AR81 ruling 5)
	// waited closes k's hold: wait when its line gives one, else the time
	// from the held line to this line's ts, as a part that never asked (a
	// queued row a stop or kill cut) has no band key (the fix round's
	// finding 2).
	waited := func(k part, wait any, ts string) {
		since, ok := holding[k]
		if !ok {
			return
		}
		delete(holding, k)
		if wait != nil {
			r.Band.WaitS += int(number(wait))
			return
		}
		from, err1 := time.Parse(time.RFC3339, since)
		to, err2 := time.Parse(time.RFC3339, ts)
		if err1 == nil && err2 == nil && to.After(from) {
			r.Band.WaitS += int(to.Sub(from).Seconds())
		}
	}
	for _, l := range lines {
		if l["night"] != night {
			continue
		}
		row, _ := l["row"].(string)
		str := func(k string) string { s, _ := l[k].(string); return s }
		k := part{row, max(int(number(l["part"])), 1)}
		switch l["event"] {
		case "night-open":
			root, opened = str("root"), true
		case "band":
			if str("state") == "held" {
				if _, ok := holding[k]; !ok {
					holding[k] = str("ts")
					r.Band.Holds++
				}
			} else { // admitted, reset or unchecked
				waited(k, l["wait_s"], str("ts"))
			}
		case "stage":
			stages[row] = staged{str("branch"), str("base")}
		case "spawn": // a row's next part in flight; a row with no sitting has nothing to take back
			takeBack(row)
		case "merge":
			if str("result") == "merged" {
				r.Merged = append(r.Merged, Merged{row, str("sha")})
				merged[row] = true
			}
		case "sitting": // its burn is nightBurn's, below
			var wait any // a hold a stop, a kill or a limit ended
			if b, ok := l["band"].(map[string]any); ok {
				wait = b["wait_s"]
			}
			waited(k, wait, str("ts"))
			if !slices.Contains(sat, row) {
				sat = append(sat, row)
			}
			gate, _ := l["gate"].(map[string]any)
			cause, _ := gate["cause"].(string)
			takeBack(row)
			if str("limit") == "paused" { // no verdict: the night paused on it (AR77 ruling 3)
				held, heldCause[row] = append(held, row), cause
				break
			}
			resume := relay.ResumeLine(row)
			switch state, reason := str("state"), str("reason"); state {
			case "green":
				green = append(green, row)
			case "red", "failed":
				r.Resume = append(r.Resume, resume)
				stops = append(stops, stop{StoppedRow{Row: row, State: state, Reason: reason, Resume: resume, Cause: cause}, true})
			default:
				stops = append(stops, stop{StoppedRow{Row: row, State: state, Reason: reason, Cause: cause}, true})
			}
		case "skip":
			stops = append(stops, stop{StoppedRow: StoppedRow{Row: row, State: "skipped", Reason: str("reason")}})
		case "stop":
			r.Stopped.Reason = str("reason")
			r.State = str("state")
			b, _ := json.Marshal(l)
			r.Stopped.Line = string(b)
			named := map[string]bool{} // the halt's resume lines (AR77 ruling 3)
			for _, x := range stringsOf(l["resume"]) {
				if id, ok := relay.ResumeRow(x); ok {
					named[id] = true
				}
			}
			shelved := stringsOf(l["shelved"])
			for _, id := range held {
				sr := StoppedRow{Row: id, State: string(relay.Failed), Reason: string(relay.UsageLimit), Cause: heldCause[id],
					Shelved: slices.Contains(shelved, id)}
				if named[id] {
					sr.Resume = relay.ResumeLine(id)
					r.Resume = append(r.Resume, sr.Resume)
				}
				stops = append(stops, stop{sr, true})
			}
			held = nil
		}
	}
	for _, since := range holding { // the holds no line ended (AR86): RFC3339 UTC, so the least string is the earliest
		if since == "" {
			continue
		}
		r.Band.Open++
		if r.Band.OpenSince == "" || since < r.Band.OpenSince {
			r.Band.OpenSince = since
		}
	}
	for _, s := range stops {
		r.Stopped.Rows = append(r.Stopped.Rows, s.StoppedRow)
	}
	if r.State == "" && opened { // AR55 ruling 1: no `stop` line, so no stop state; no night-open, none (AR60)
		r.State = NotStopped
	}
	if len(g) > 0 && g[0] != nil {
		hand := HandMerges(g[0], lines, night)
		for _, id := range sat {
			sha, ok := hand[id]
			if !ok {
				continue
			}
			r.Merged = append(r.Merged, Merged{id, sha})
			merged[id] = true
			r.Resume = slices.DeleteFunc(r.Resume, func(l string) bool { row, ok := relay.ResumeRow(l); return ok && row == id })
			for i := range r.Stopped.Rows {
				if r.Stopped.Rows[i].Row == id {
					r.Stopped.Rows[i].Resume = ""
				}
			}
		}
	}
	// AR51 ruling 2: the burn is nightBurn's, the count status.json reads
	// too, so a sitting in flight (spawned, no `sitting` line yet) is here.
	r.Sittings, _ = nightBurn(lines, night)
	for _, id := range green {
		if !merged[id] {
			st := stages[id]
			r.Waiting = append(r.Waiting, fmt.Sprintf("%s (off %s)", or(st.branch, id), or(st.base, "main")))
		}
	}
	date := night
	if i := strings.LastIndex(night, "-"); i > 0 {
		date = night[:i]
	}
	var rows []string           // the rows of the burn, first seen first (AR74 ruling 3)
	last := map[string]string{} // and each one's last sitting's state
	for _, b := range r.Sittings {
		if _, ok := last[b.Row]; !ok {
			rows = append(rows, b.Row)
		}
		last[b.Row] = b.State
	}
	for _, id := range rows {
		st, ok := stages[id]
		if !ok || root == "" || last[id] == inFlight { // a handover is a finished sitting's, read once per row
			continue
		}
		paths, err := GitChanged(root, or(st.base, "main"), st.branch, "history/sittings")
		if err != nil {
			continue
		}
		sort.Strings(paths)
		for _, p := range paths {
			if !strings.HasPrefix(filepath.Base(p), date+"-") || filepath.Ext(p) != ".md" {
				continue
			}
			src, err := GitShow(root, st.branch, p)
			if err != nil {
				continue
			}
			if text := section(string(src), "## Handover"); text != "" {
				r.Handovers = append(r.Handovers, Handover{id, p, text})
			}
		}
	}
	return r
}

// HandMerges is the rows of night merged by hand after it (AR56 ruling 1),
// row to merge sha, read from git as §3 reads a landing, never from a line:
// a row whose last `sitting` line this night is green, red or failed, that
// no `merge` line says merged, whose `stage` branch exists in the
// night-open root and has landed (`landed`; the stage's base only narrows
// it, so it only reds). The sha is the newest merge on main's first-parent
// line with the branch's tip as a later parent. A git error leaves the row
// out; a fast-forward never lands, so a row main fast-forwarded to is not
// here. Ruling 6 (the review's F1, F2): a row whose tip is still its
// `stage` line's head_before made no commit, so the tip is the night's cut
// and a merge that took the cut is not the row's; and the last `sitting`
// line's head_after, when present, must be the tip or an ancestor of it
// (g.IsAncestor), else the branch name was reused and its tip is another
// night's work.
func HandMerges(g effects.Git, lines []map[string]any, night string) map[string]string {
	out := map[string]string{}
	if g == nil || night == "" {
		return out
	}
	var root string
	type staged struct{ branch, base, before string }
	stages := map[string]staged{}
	last, after := map[string]string{}, map[string]string{}
	merged := map[string]bool{}
	var rows []string
	for _, l := range lines {
		if l["night"] != night {
			continue
		}
		row, _ := l["row"].(string)
		str := func(k string) string { s, _ := l[k].(string); return s }
		switch l["event"] {
		case "night-open":
			root = str("root")
		case "stage":
			stages[row] = staged{str("branch"), str("base"), str("head_before")}
		case "merge":
			if str("result") == "merged" {
				merged[row] = true
			}
		case "sitting":
			if _, seen := last[row]; !seen {
				rows = append(rows, row)
			}
			last[row], after[row] = str("state"), str("head_after")
		}
	}
	if root == "" {
		return out
	}
	var log []string // main's first-parent merges, newest first: "<sha> <parents>"
	for _, id := range rows {
		switch last[id] {
		case "green", "red", "failed":
		default:
			continue
		}
		st, ok := stages[id]
		if merged[id] || !ok || st.branch == "" {
			continue
		}
		if ok, err := g.BranchExists(root, st.branch); err != nil || !ok {
			continue
		}
		tip, err := g.Tip(root, st.branch)
		if err != nil || tip == "" || tip == st.before {
			continue
		}
		if a := after[id]; a != "" {
			if ok, err := g.IsAncestor(root, a, tip); err != nil || !ok {
				continue
			}
		}
		if !landed(g, root, or(st.base, "main"), st.branch, "") {
			continue
		}
		if log == nil {
			text, err := g.Hermetic(root, "log", "--first-parent", "--merges", "--format=%H %P", "main")
			if err != nil {
				return out
			}
			log = strings.Split(strings.TrimSpace(text), "\n")
		}
		for _, l := range log {
			if f := strings.Fields(l); len(f) > 2 && slices.Contains(f[2:], tip) {
				out[id] = f[0]
				break
			}
		}
	}
	return out
}

// stopLines is the `stop` ledger line for a person: one `key: value` line
// per key, keys sorted, `event` dropped, a list joined by `, `, anything
// else nested written as compact JSON. A line that is not JSON is itself.
func stopLines(line string) []string {
	d := json.NewDecoder(strings.NewReader(line))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return []string{line}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "event" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+": "+stopValue(m[k]))
	}
	return out
}

// stopValue is one value of the stop line as stopLines writes it.
func stopValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "none"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = stopValue(e)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		b, _ := json.Marshal(x)
		return string(b)
	}
	return fmt.Sprint(v)
}

// stringsOf is a line's list of strings: []any read back from the
// ledger's JSON, []string a line still in memory; anything else none.
func stringsOf(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func or(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// section is the body under heading up to the next `## ` heading, trimmed.
func section(src, heading string) string {
	var out []string
	in := false
	for _, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(l, "## ") {
			if in {
				break
			}
			in = strings.TrimSpace(l) == heading
			continue
		}
		if in {
			out = append(out, l)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Markdown is the report as plain markdown, its headings in the morning's
// order: merged, stopped, waiting on a hand merge, resume lines, burn,
// handovers. The resume lines are bare, one per line in a fenced block, so
// each pastes whole. Paths in it are relative: root is written `.` and home
// `~`, so the file is hygiene-safe.
func Markdown(r Report, root, home string) []byte {
	var b bytes.Buffer
	if r.Night == "" {
		b.WriteString("# No night yet\n")
		return b.Bytes()
	}
	var total float64
	unread := 0
	for _, s := range r.Sittings {
		total, unread = total+s.CostUSD, unread+s.Unread
	}
	fmt.Fprintf(&b, "# Night %s: %s\n\n", r.Night, or(r.State, NotOpened)) // a stop state, NotStopped, a status's (TakeStatus) or NotOpened
	// AR51 ruling 3: a live night's report says what is in flight.
	sittings, flight := plural(len(r.Sittings), "sitting"), 0
	for _, s := range r.Sittings {
		if s.State == inFlight {
			flight++
		}
	}
	if flight > 0 {
		sittings += fmt.Sprintf(" (%d in flight)", flight)
	}
	fmt.Fprintf(&b, "%s, %s: %d merged, %d stopped short, %d waiting on a hand merge.\n\n",
		sittings, costOf(total, unread, false), len(r.Merged), len(r.Stopped.Rows), len(r.Waiting))
	b.WriteString("## Merged\n\n")
	for _, m := range r.Merged {
		fmt.Fprintf(&b, "- %s at %s\n", m.Row, m.SHA)
	}
	if len(r.Merged) == 0 {
		b.WriteString("Nothing merged.\n")
	}
	if r.State == "" { // NotOpened's title (AR60)
		b.WriteString("\n## Stopped\n\nThe night has not opened.\n\n")
	} else {
		fmt.Fprintf(&b, "\n## Stopped\n\nThe night stopped: %s.\n\n", or(r.Stopped.Reason, "not yet"))
	}
	if r.Stopped.Line != "" {
		b.WriteString(strings.Join(stopLines(r.Stopped.Line), "\n") + "\n\n")
	}
	for _, s := range r.Stopped.Rows {
		line := "- " + s.Row + ": " + s.State
		if s.Reason != "" {
			line += ", " + s.Reason
		}
		if s.Shelved { // AR92 ruling 5
			line += ", shelved"
		}
		if s.Cause != "" {
			// AR62: the cause alone is clipped, the line under 100 columns,
			// after the root and home rewrite below, so no clip cuts either
			// (fix round, F1); a line with no cause is written whole.
			room := 99 - len([]rune(relHome(line, root, home))) - len(" ()")
			line += " (" + clip(relHome(s.Cause, root, home), max(room, 20)) + ")"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n## Waiting on a hand merge\n\n")
	for _, w := range r.Waiting {
		fmt.Fprintf(&b, "- %s\n", w)
	}
	if len(r.Waiting) == 0 {
		b.WriteString("None.\n")
	}
	b.WriteString("\n## Resume\n\n")
	if len(r.Resume) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("```\n" + strings.Join(r.Resume, "\n") + "\n```\n")
	}
	b.WriteString("\n## Burn\n\n")
	cells := [][]string{{"row", "state", "turns", "context", "cost", "builders", "builder output"}}
	var turns, builders, output int
	for _, s := range r.Sittings {
		turns, builders, output = turns+s.Turns, builders+s.Builders, output+s.BuilderOutput
		cells = append(cells, []string{s.Row, s.State, fmt.Sprint(s.Turns), fmt.Sprintf("%dk", s.Context/1000),
			costOf(s.CostUSD, s.Unread, true), fmt.Sprint(s.Builders), fmt.Sprint(s.BuilderOutput)})
	}
	cells = append(cells, []string{"total", "", fmt.Sprint(turns), "", costOf(total, unread, false), fmt.Sprint(builders), fmt.Sprint(output)})
	mdTable(&b, cells, 2, 3, 4, 5, 6)
	if r.Band.Holds > 0 { // AR81 ruling 5: after the sittings
		fmt.Fprintf(&b, "\nband: %s, %s waited", plural(r.Band.Holds, "hold"), time.Duration(r.Band.WaitS)*time.Second)
		if r.Band.Open > 0 { // AR86: the holds no line ended yet
			fmt.Fprintf(&b, ", %d open since %s", r.Band.Open, r.Band.OpenSince)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Handovers\n")
	for _, h := range r.Handovers {
		fmt.Fprintf(&b, "\n### %s: %s\n\n%s\n", h.Row, h.Path, h.Text)
	}
	if len(r.Handovers) == 0 {
		b.WriteString("\nNone.\n")
	}
	return []byte(relHome(b.String(), root, home))
}

// relHome is s with root written `.` and then home `~`: Markdown's rewrite,
// which a clip must follow (AR62 fix round).
func relHome(s, root, home string) string {
	if root != "" {
		s = strings.ReplaceAll(s, root, ".")
	}
	if home != "" {
		s = strings.ReplaceAll(s, home, "~")
	}
	return s
}

// mdTable is cells as a markdown table padded so the raw text lines up too;
// the columns named in right are right-aligned (numbers). The first row is
// the header.
func mdTable(b *bytes.Buffer, cells [][]string, right ...int) {
	width := map[int]int{}
	for _, row := range cells {
		for i, c := range row {
			if n := len([]rune(c)); n > width[i] {
				width[i] = n
			}
		}
	}
	isRight := map[int]bool{}
	for _, i := range right {
		isRight[i] = true
	}
	for i, n := range width {
		width[i] = max(n, 3)
	}
	line := func(row []string) {
		for i, c := range row {
			pad := strings.Repeat(" ", width[i]-len([]rune(c)))
			if isRight[i] {
				c = pad + c
			} else {
				c += pad
			}
			fmt.Fprintf(b, "| %s ", c)
		}
		b.WriteString("|\n")
	}
	line(cells[0])
	sep := make([]string, len(cells[0]))
	for i := range sep {
		sep[i] = strings.Repeat("-", width[i])
		if isRight[i] {
			sep[i] = strings.Repeat("-", width[i]-1) + ":"
		}
	}
	for _, s := range sep {
		fmt.Fprintf(b, "| %s ", s)
	}
	b.WriteString("|\n")
	for _, row := range cells[1:] {
		line(row)
	}
}

// RootOf is the root the night's night-open line names, "" when none.
func RootOf(lines []map[string]any, night string) string {
	for _, l := range lines {
		if l["night"] == night && l["event"] == "night-open" {
			s, _ := l["root"].(string)
			return s
		}
	}
	return ""
}

// WriteReport writes md at rel under repo (D18: the only write into a
// repo); rel must stay inside it.
func WriteReport(fs effects.FS, repo, rel string, md []byte) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("--write %s: want a path relative to the repo", rel)
	}
	p := filepath.Join(repo, rel)
	if r, err := filepath.Rel(repo, p); err != nil || r == ".." || strings.HasPrefix(r, "../") {
		return "", fmt.Errorf("--write %s: outside the repo", rel)
	}
	if err := fs.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, fs.WriteFile(p, md, 0o644)
}
