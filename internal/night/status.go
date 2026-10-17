package night

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// Status is the night's dashboard (§9 `status`), written as
// <state>/nights/<night>/status.json after every event. Next is the next
// stop condition and its time; Resume is `agentik-relay resume <row>` for every red
// or failed row.
type Status struct {
	Night  string      `json:"night"`
	State  string      `json:"state"`
	Reason string      `json:"reason"`
	Rows   []RowStatus `json:"rows"`
	Burn   Burn        `json:"burn"`
	Next   string      `json:"next"`
	Resume []string    `json:"resume"`
	// PID and Started are the relay running the night (AR33 act 1), which
	// `status --watch` reads to stop when that relay is gone.
	PID     int    `json:"pid"`
	Started string `json:"started"`
	// Root is the plan's root, the repo the night runs in (AR37 act 1): the
	// account holds one relay and every repo's nights (§8), so the dashboard
	// names it. A status.json from before AR37 has none.
	Root string `json:"root"`
}

// RowStatus is one row tonight: its sitting's state and reason, the merge,
// the burn so far (turns, context against the ceiling, cost), the time since
// it started, the last tool, the builders, the acts as commits land (AR30),
// and the resume line when it is red or failed.
type RowStatus struct {
	ID       string  `json:"id"`
	Worktree string  `json:"worktree"`
	State    string  `json:"state"`
	Reason   string  `json:"reason"`
	MergeSHA string  `json:"merge_sha"`
	Turns    int     `json:"turns"`
	Context  int     `json:"context"`
	Ceiling  int     `json:"ceiling"`
	CostUSD  float64 `json:"cost_usd"`
	Unread   bool    `json:"unread"` // no result event read the cost (AR28)
	Elapsed  string  `json:"elapsed"`
	LastTool string  `json:"last_tool"`
	Builders int     `json:"builders"`
	Acts     []Act   `json:"acts"` // [] when none
	Resume   string  `json:"resume"`
	// Cause is the gate's cause when the row ended red or failed (AR62
	// act 2), gate.Cause through RedCause; "" else.
	Cause string `json:"cause"`
	// Wait is `verify behind <row> since <hh:mm:ss>Z` (UTC) while the row's
	// verify waits for its root's gate lock, from GateLock's waiting mark
	// until it returns; "" else (verify-cut-r1 §3; AR79 rulings 1, 2).
	Wait string `json:"wait"`
	// Band is the row's band hold while it holds (band-pacing-r1 §3; AR81
	// ruling 1): State reads `held` and Reason the band's text then; nil
	// when the row is not held.
	Band *RowBand `json:"band"`
	// Shelved is the row's wake (RFC 3339) once its model's limit shelved
	// it to the night's end (AR92 ruling 5): State reads `paused` and
	// Reason `usage-limit` until the night's end halts it; "" when not.
	Shelved string `json:"shelved"`
}

// RowBand is a held row's band: the last verdict and its peak (null for
// `queued`), the hold's start (RFC 3339, the clock's zone) and the asks in
// it, 0 while queued (AR81 rulings 1, 2).
type RowBand struct {
	Verdict string   `json:"verdict"`
	Peak    *float64 `json:"peak"`
	Since   string   `json:"since"`
	Asks    int      `json:"asks"`
}

// Burn is what sittings spent: one row's (Row set) or the night's (Sittings
// counted). BuilderOutput is the builders' output tokens.
type Burn struct {
	Row           string  `json:"row,omitempty"`
	State         string  `json:"state,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	Sittings      int     `json:"sittings"`
	Turns         int     `json:"turns"`
	Context       int     `json:"context"`
	CostUSD       float64 `json:"cost_usd"`
	Unread        int     `json:"unread"` // sittings whose cost no result read (AR28)
	Builders      int     `json:"builders"`
	BuilderOutput int     `json:"builder_output_tokens"`
}

// unreadOf counts the rows whose cost is unread.
func unreadOf(rows map[string]*RowStatus) int {
	n := 0
	for _, r := range rows {
		if r.Unread {
			n++
		}
	}
	return n
}

// unreadOne is 1 for an unread cost, else 0.
func unreadOne(unread bool) int {
	if unread {
		return 1
	}
	return 0
}

// costOf is a cost cell: `$X.XX`, `unread` for none read, and a total's
// `$X.XX + N unread`.
func costOf(usd float64, unread int, one bool) string {
	switch {
	case one && unread > 0:
		return "unread"
	case unread > 0:
		return fmt.Sprintf("$%.2f + %d unread", usd, unread)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// number is a ledger field as a float: JSON's numbers read back as float64,
// a line still in memory holds ints.
func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

// inFlight is the state of a Burn between its `spawn` line and its
// `sitting` line (AR51): the child is running, or its gate is. A `sitting`
// line never says it, so it marks the sittings the report calls in flight.
const inFlight = string(relay.Running)

// nightBurn is a night's burn from its ledger lines (AR51 ruling 2), the one
// count `agentik-relay report` and status.json both read: BuildReport calls it over
// the ledger, the night's tap over the same lines as it appends them. A
// sitting counts from its `spawn` line: per row the `sitting` line once
// written, else the `exit` line (its turns, context and cost), else the
// `spawn` line with no cost and the last `turn` line's turns and context.
// A pick that never spawned (it could not stage) is no sitting. The rows
// come back in the order their `sitting` lines were written, then the ones
// in flight in spawn order; total sums them (its context is the largest).
// night "" takes every line. A `sitting` line the night paused on (`limit:
// paused`) is its part's history, `paused` (AR77 ruling 4); the night's
// `stop` line turns one whose row spawned no part after it `failed`, the
// halt.
func nightBurn(lines []map[string]any, night string) (rows []Burn, total Burn) {
	rows = []Burn{}
	open := map[string]*Burn{}
	paused := map[string]int{} // each row's paused entry in rows, until its next part
	var order []string
	for _, l := range lines {
		if night != "" && l["night"] != night {
			continue
		}
		row, _ := l["row"].(string)
		b := open[row]
		switch l["event"] {
		case "spawn":
			if b == nil {
				order = append(order, row)
			}
			open[row] = &Burn{Row: row, State: inFlight, Sittings: 1}
			delete(paused, row)
		case "stop":
			for _, i := range paused {
				rows[i].State = string(relay.Failed)
			}
			clear(paused)
		case "turn":
			if b != nil {
				b.Turns, b.Context = int(number(l["turns"])), int(number(l["context"]))
			}
		case "exit":
			if b == nil {
				break
			}
			b.Turns = int(number(l["turns"]))
			if c := int(number(l["context"])); c > 0 {
				b.Context = c
			}
			if b.Unread = unreadOne(l["cost_read"] == false); b.Unread == 0 {
				b.CostUSD = number(l["cost_usd"])
			}
		case "sitting":
			delete(open, row)
			s, _ := l["state"].(string)
			why, _ := l["reason"].(string)
			b := Burn{Row: row, State: s, Reason: why, Sittings: 1,
				Turns: int(number(l["turns"])), Context: int(number(l["context"])), CostUSD: number(l["cost_usd"]),
				Unread: unreadOne(l["cost_read"] == false)}
			if bs, ok := l["builders"].([]any); ok { // a line read back
				for _, x := range bs {
					if m, ok := x.(map[string]any); ok {
						b.Builders++
						b.BuilderOutput += int(number(m["output_tokens"]))
					}
				}
			} else if bs, ok := l["builders"].([]map[string]any); ok { // a line still in memory
				for _, m := range bs {
					b.Builders++
					b.BuilderOutput += int(number(m["output_tokens"]))
				}
			}
			delete(paused, row)
			if lim, _ := l["limit"].(string); lim == "paused" {
				b.State, paused[row] = string(relay.Paused), len(rows)
			}
			rows = append(rows, b)
		}
	}
	for _, row := range order {
		if b := open[row]; b != nil {
			rows = append(rows, *b)
			delete(open, row)
		}
	}
	for _, b := range rows {
		total.Sittings, total.Turns, total.CostUSD, total.Unread = total.Sittings+b.Sittings, total.Turns+b.Turns, total.CostUSD+b.CostUSD, total.Unread+b.Unread
		total.Builders, total.BuilderOutput = total.Builders+b.Builders, total.BuilderOutput+b.BuilderOutput
		total.Context = max(total.Context, b.Context)
	}
	return rows, total
}

// buildStatus is the night's status: burn is nightBurn's total over the
// lines the night wrote (AR51); usd is the read costs and held the unread
// sittings' spend (each at its row's usd), which --max-usd counts too; count
// is the rows that staged (AR58 ruling 3), which --max-sittings compares.
// While a row is shelved (AR92 ruling 5) the last stop is the usage-limit
// end, not queue-empty: the night stops so once nothing else is left.
func buildStatus(n *Night, ids []string, rows map[string]*RowStatus, burn Burn, usd, held float64, count int, o NightOptions, until time.Time) Status {
	st := Status{Night: n.ID, State: string(n.State), Reason: string(n.Reason), Rows: []RowStatus{}, Resume: []string{}}
	if n.Plan != nil {
		st.Root = n.Plan.Root
	}
	for _, id := range ids {
		rs := *rows[id]
		if rs.Acts == nil {
			rs.Acts = []Act{}
		}
		st.Rows = append(st.Rows, rs)
		if rs.Resume != "" {
			st.Resume = append(st.Resume, rs.Resume)
		}
	}
	st.Burn = burn
	var next []string
	if c := bandClause(st.Rows); c != "" { // AR81 ruling 3: the band's hold leads
		next = append(next, c)
	}
	if !until.IsZero() {
		next = append(next, "until-reached at "+until.Format("15:04"))
	}
	if o.MaxSittings > 0 { // rows staged, while the burn counts parts (AR77 ruling 2)
		next = append(next, fmt.Sprintf("count-reached after %d rows staged (%d so far)", o.MaxSittings, count))
	}
	if o.MaxUSD > 0 {
		next = append(next, fmt.Sprintf("budget-reached at $%.2f ($%.2f so far)", o.MaxUSD, usd+held))
	}
	var shelved []string
	for _, r := range st.Rows {
		if r.Shelved != "" && r.State == string(relay.Paused) {
			shelved = append(shelved, r.ID)
		}
	}
	if len(shelved) > 0 {
		next = append(next, "usage-limit once only the shelved rows are left ("+strings.Join(shelved, " ")+")")
	} else {
		next = append(next, "queue-empty")
	}
	if relay.NightTerminal(n.State) {
		next = nil
	} else if n.State == relay.Draining {
		next = []string{"owner-stop when the running sittings finish"}
	}
	st.Next = strings.Join(next, "; ")
	return st
}

// bandClause is `the band holds <rows> since HH:MM`: the held rows in
// order, the earliest since; "" when none is held (AR81 ruling 3). Rows
// that do not fit under 100 columns read `+N`.
func bandClause(rows []RowStatus) string {
	var held []string
	var since time.Time
	for _, r := range rows {
		if r.Band == nil {
			continue
		}
		held = append(held, r.ID)
		if t, err := time.Parse(time.RFC3339, r.Band.Since); err == nil && (since.IsZero() || t.Before(since)) {
			since = t
		}
	}
	if len(held) == 0 {
		return ""
	}
	tail := ""
	if !since.IsZero() {
		tail = " since " + since.Format("15:04")
	}
	// The fix round's finding 3: the clause fits PrintStatus's `next stop: `
	// line under 100 columns, naming the first rows that fit and `+N`.
	const room = 99 - len("next stop: ")
	for k := len(held); k >= 1; k-- {
		more := ""
		if k < len(held) {
			more = fmt.Sprintf(" +%d", len(held)-k)
		}
		if c := "the band holds " + strings.Join(held[:k], " ") + more + tail; len([]rune(c)) <= room {
			return c
		}
	}
	more := ""
	if len(held) > 1 {
		more = fmt.Sprintf(" +%d", len(held)-1)
	}
	return clip("the band holds "+held[0], room-len(more)-len(tail)) + more + tail
}

// PrintStatus is the dashboard once (§9's `Print(w, Status)`; the name
// Print is plan's). The first line is the night, its root (AR37 act 1;
// ShowRoot) and its state with the rows counted by state; then the rows, numbers right-aligned,
// a row whose verify waits for its root's gate lock one cell past merge,
// `verify waits behind <row> <m>m` (AR79); each row's acts
// (AR30), one line each under 100 columns; each red or failed row's
// `cause of <row>: <cause>`, clipped under 100 columns (AR62); each shelved
// row's `<row> shelved to the night's end, its wake <wake>` (AR92); the burn; the next
// stop; and last, every red or failed row's `agentik-relay resume <row>` on a line
// of its own. No colour, no cursor movement: it is read over ssh.
func PrintStatus(w io.Writer, s Status) {
	if s.Night == "" {
		fmt.Fprintln(w, "no night yet")
		return
	}
	var order []string
	by := map[string]int{}
	for _, r := range s.Rows {
		if by[r.State] == 0 {
			order = append(order, r.State)
		}
		by[r.State]++
	}
	var counts []string
	for _, st := range order {
		counts = append(counts, fmt.Sprintf("%d %s", by[st], st))
	}
	fmt.Fprintf(w, "night %s%s: %s; %s", s.Night, inRoot(s.Root), s.State, plural(len(s.Rows), "row"))
	if len(counts) > 0 {
		fmt.Fprintf(w, ": %s", strings.Join(counts, ", "))
	}
	fmt.Fprintln(w)
	// waitCell is a row's wait as the line reads it (AR79 ruling 3): `verify
	// waits behind <row> <m>m`, the whole minutes from the wait's clock to
	// statusNow in UTC (a clock later than now is the day before); a wait
	// that does not parse prints as it is.
	waitCell := func(wait string) string {
		rest, ok := strings.CutPrefix(wait, "verify behind ")
		i := strings.LastIndex(rest, " since ")
		if !ok || i <= 0 {
			return wait
		}
		clock, ok := strings.CutSuffix(rest[i+len(" since "):], "Z")
		at, err := time.Parse("15:04:05", clock)
		if !ok || err != nil {
			return wait
		}
		now := statusNow().UTC()
		since := time.Date(now.Year(), now.Month(), now.Day(), at.Hour(), at.Minute(), at.Second(), 0, time.UTC)
		if since.After(now) {
			since = since.Add(-24 * time.Hour)
		}
		return fmt.Sprintf("verify waits behind %s %dm", rest[:i], int(now.Sub(since)/time.Minute))
	}
	cells := [][]string{{"row", "state", "turns", "context", "cost", "time", "last tool", "builders", "acts", "merge"}}
	for _, r := range s.Rows {
		ctx := fmt.Sprintf("%dk", r.Context/1000)
		if r.Ceiling > 0 {
			ctx += fmt.Sprintf("/%dk", r.Ceiling/1000)
		}
		state := r.State
		if r.Reason != "" {
			state += ": " + r.Reason
		}
		cells = append(cells, []string{r.ID, state, strconv.Itoa(r.Turns), ctx, costOf(r.CostUSD, unreadOne(r.Unread), true),
			dash(r.Elapsed), dash(clip(r.LastTool, 12)), strconv.Itoa(r.Builders), strconv.Itoa(len(r.Acts)), dash(short(r.MergeSHA))})
		if r.Wait != "" { // one cell past merge, under no header (AR79 ruling 3)
			cells[len(cells)-1] = append(cells[len(cells)-1], waitCell(r.Wait))
		}
	}
	// AR81 ruling 4: a held row's state cell, the band's text with it, is
	// clipped so its line stays under 100 columns.
	for i, r := range s.Rows {
		if r.Band == nil {
			continue
		}
		room := 99
		for j := range cells[i+1] {
			if j != 1 {
				room -= colWidth(cells, j) + 2
			}
		}
		cells[i+1][1] = clip(cells[i+1][1], max(room, len("held")+3))
	}
	table(w, "", cells, 2, 3, 4, 5, 7, 8)
	for _, r := range s.Rows {
		if len(r.Acts) > 0 {
			fmt.Fprintf(w, "acts of %s:\n", r.ID)
			for _, a := range r.Acts {
				fmt.Fprintln(w, actsLine(a))
			}
		}
	}
	for _, r := range s.Rows { // AR62 act 2: --json carries it whole
		if r.Cause != "" {
			fmt.Fprintln(w, clip("cause of "+r.ID+": "+r.Cause, 99))
		}
	}
	for _, r := range s.Rows { // AR92 ruling 5: paused usage-limit, shelved
		if r.Shelved != "" && r.State == string(relay.Paused) {
			fmt.Fprintln(w, clip(r.ID+" shelved to the night's end, its wake "+r.Shelved, 99))
		}
	}
	fmt.Fprintf(w, "burn: %s, %s, %s, %s\n", plural(s.Burn.Sittings, "sitting"), plural(s.Burn.Turns, "turn"),
		costOf(s.Burn.CostUSD, s.Burn.Unread, false), plural(s.Burn.Builders, "builder"))
	for i, n := range strings.Split(s.Next, "; ") {
		switch {
		case n == "":
		case i == 0:
			fmt.Fprintf(w, "next stop: %s\n", n)
		default:
			fmt.Fprintf(w, "       or: %s\n", n)
		}
	}
	if len(s.Resume) > 0 {
		fmt.Fprintln(w, "to resume the red or failed rows:")
		for _, r := range s.Resume {
			fmt.Fprintln(w, r)
		}
	}
}

// statusNow is the clock PrintStatus reads a wait's minutes from (AR79
// ruling 3); only the internal test sets it.
var statusNow = time.Now

// ShowRoot is a night's root as the owner reads it (AR37): the home
// directory, read from HOME and cleaned (a trailing slash is the same
// home), written `~`, and a root that is the home itself is `~` (AR37 fix).
func ShowRoot(root string) string {
	home := os.Getenv("HOME")
	if home == "" {
		return TildeQuote([]string{root}, "")
	}
	home = filepath.Clean(home)
	if filepath.Clean(root) == home {
		return "~"
	}
	return TildeQuote([]string{root}, home)
}

// inRoot is ` in <root>` after a night's id, empty for a status with no
// root (one written before AR37), so it prints as it did.
func inRoot(root string) string {
	if root == "" {
		return ""
	}
	return " in " + ShowRoot(root)
}

// table writes cells in columns two spaces apart, each line led by indent;
// the columns named in right are right-aligned (numbers), the rest left.
func table(w io.Writer, indent string, cells [][]string, right ...int) {
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
	for _, row := range cells {
		var b strings.Builder
		b.WriteString(indent)
		for i, c := range row {
			pad := strings.Repeat(" ", width[i]-len([]rune(c)))
			if i > 0 {
				b.WriteString("  ")
			}
			if isRight[i] {
				b.WriteString(pad + c)
			} else {
				b.WriteString(c + pad)
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}

// colWidth is column j's width in cells, in runes.
func colWidth(cells [][]string, j int) int {
	n := 0
	for _, row := range cells {
		if j < len(row) {
			n = max(n, len([]rune(row[j])))
		}
	}
	return n
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// datedNight is a night's own name, <YYYY-MM-DD>-<n>; the gate's `hand`
// dir and any other name under nights/ is not one (AR39).
var datedNight = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]+$`)

// Nights lists the dated nights under the state directory, oldest first (by
// date, then by number: 2026-09-22-10 follows 2026-09-22-9). A name that is
// not <YYYY-MM-DD>-<n>, the gate's `hand` among them, is left out (AR39),
// so it is never the latest.
func Nights(fs effects.FS, state string) []string {
	all, _ := fs.List(filepath.Join(state, "nights"))
	var names []string
	for _, n := range all {
		if datedNight.MatchString(n) {
			names = append(names, n)
		}
	}
	key := func(s string) (string, int) {
		i := strings.LastIndex(s, "-")
		if i < 0 {
			return s, 0
		}
		n, _ := strconv.Atoi(s[i+1:])
		return s[:i], n
	}
	sort.Slice(names, func(i, j int) bool {
		a, an := key(names[i])
		b, bn := key(names[j])
		if a != b {
			return a < b
		}
		return an < bn
	})
	return names
}

// Latest is the night a verb reads when given no --night: the newest dated
// night that is planning or that opened, "" when there is none; never
// `hand` (AR39). The fix round of AR60, the observer's ruling: a night
// whose status ended before its `night-open` line (a run refused at its
// plan) and a night with no status that the ledger names with no
// `night-open` line (a dead plan Reap cleared) are skipped (neverOpened),
// so a refused run never hides the last real night; `--night` still reads
// either. A night with no status that the ledger does not name (a measure
// before AR60's mark, or a bare directory) is taken, as before.
func Latest(fs effects.FS, state string) string {
	ns := Nights(fs, state)
	for i := len(ns) - 1; i >= 0; i-- {
		if !neverOpened(fs, state, ns[i]) {
			return ns[i]
		}
	}
	return ""
}

// neverOpened is a night Latest skips: no `night-open` line for it in the
// ledger, and either a terminal status or no status while the ledger names
// it. A status planning, running or draining, or one that does not parse
// (an open night's half write, AR37 fix), is taken without the ledger.
func neverOpened(fs effects.FS, state, night string) bool {
	b, err := fs.ReadFile(filepath.Join(NightDir(state, night), "status.json"))
	has := err == nil
	if has {
		var st Status
		if json.Unmarshal(b, &st) != nil || !relay.NightTerminal(relay.NightState(st.State)) {
			return false
		}
	}
	opened, named := ledgerOpen(fs, state, night)
	return !opened && (has || named)
}

// ledgerOpen reads the ledger's day files as ledgerRoot does: opened when a
// `night-open` line names the night, named when any line does.
func ledgerOpen(fs effects.FS, state, night string) (opened, named bool) {
	dir, from := effects.LedgerDir(state), ledgerFrom(night)
	names, _ := fs.List(dir)
	sort.Strings(names)
	key := []byte(`"` + night + `"`)
	for _, name := range names {
		day, ok := strings.CutSuffix(name, ".jsonl")
		if !ok || day < from {
			continue
		}
		b, err := fs.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, ln := range bytes.Split(b, []byte("\n")) {
			var l map[string]any
			if !bytes.Contains(ln, key) || json.Unmarshal(ln, &l) != nil || l["night"] != night {
				continue
			}
			named = true
			if l["event"] == "night-open" {
				return true, true
			}
		}
	}
	return false, named
}

// ledgerFrom is the first ledger day a night's lines can be on: the day
// before its date, which covers a ledger day in another zone.
func ledgerFrom(night string) string {
	if len(night) >= 10 {
		if t, err := time.Parse("2006-01-02", night[:10]); err == nil {
			return t.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	return night
}

// TakeMerges folds in the rows merged by hand after the night (AR56 ruling
// 2, HandMerges' map): a row in m with no merge sha takes m's sha and no
// resume line, and its line leaves Resume. Its state and reason stay the
// night's: only the night gated it.
func (s *Status) TakeMerges(m map[string]string) {
	for i := range s.Rows {
		r := &s.Rows[i]
		sha, ok := m[r.ID]
		if !ok || sha == "" || r.MergeSHA != "" {
			continue
		}
		was := r.Resume
		r.MergeSHA, r.Resume = sha, ""
		s.Resume = slices.DeleteFunc(s.Resume, func(l string) bool { // either spelling (relay.ResumeRow)
			row, ok := relay.ResumeRow(l)
			return l != "" && (ok && row == r.ID || l == was)
		})
	}
}

// writeStatus writes st as dir's status.json whole or not at all (AR37
// fix): a temp file renamed over it, so a reader never sees half a status.
// Run's every write and the plan's mark (MarkPlanning) go through it.
func writeStatus(fs effects.FS, dir string, st Status) {
	b, _ := json.MarshalIndent(st, "", "  ")
	if tmp, err := fs.WriteTemp(dir, "status.json.*.tmp", append(b, '\n')); err == nil {
		if fs.Rename(tmp, filepath.Join(dir, "status.json")) != nil {
			fs.Remove(tmp)
		}
	}
}

// MarkPlanning is the night's status while `agentik-relay run` plans it (AR60 act
// 1): it makes the night's directory and writes its status.json, state
// `planning` under this relay's pid and start (Run's own pair), with its
// root and no rows, so a reader tells a planning night from no night. Run's
// first status replaces it once the night opens. end is for a plan that
// opens no night: a status still `planning` under this pid is rewritten
// `stopped:<r>` with reason r, its pid and start kept; any other status (one
// the night rewrote, or one already ended) is left alone. A night or state
// of "", or a directory that cannot be made, marks nothing.
func MarkPlanning(fx effects.Effects, state, night, root string) (end func(relay.StopReason)) {
	none := func(relay.StopReason) {}
	if state == "" || night == "" {
		return none
	}
	dir := NightDir(state, night)
	if fx.FS.MkdirAll(dir, 0o700) != nil {
		return none
	}
	pid := os.Getpid()
	writeStatus(fx.FS, dir, Status{Night: night, State: string(relay.Planning), Rows: []RowStatus{}, Resume: []string{},
		PID: pid, Started: selfStarted(fx.Procs), Root: root})
	return func(r relay.StopReason) {
		st := ReadStatus(fx.FS, state, night)
		if st.State != string(relay.Planning) || st.PID != pid {
			return
		}
		st.State, st.Reason = string(relay.Stopped(r)), string(r)
		writeStatus(fx.FS, dir, st)
	}
}

// ReadStatus reads a night's status.json; a night with none, or one that
// does not parse (or no night, "") is an empty status with its name. A
// status with no root takes the night's root from its groups.json, else
// from the ledger's night-open line (AR37 fix), so a night still measuring
// names its repo; one with neither prints as before AR37.
func ReadStatus(fs effects.FS, state, night string) Status {
	s := Status{Night: night, Rows: []RowStatus{}, Resume: []string{}}
	if night == "" {
		return s
	}
	if b, err := fs.ReadFile(filepath.Join(NightDir(state, night), "status.json")); err == nil {
		var read Status
		if json.Unmarshal(b, &read) == nil {
			s = read
		}
	}
	if s.Root == "" {
		s.Root = groupsRoot(fs, state, night)
	}
	if s.Root == "" {
		s.Root = ledgerRoot(fs, state, night)
	}
	return s
}

// groupsRoot is the root the night's groups.json names, written from the
// measure on (WatchGroups, then Run's own); "" when none.
func groupsRoot(fs effects.FS, state, night string) string {
	b, err := fs.ReadFile(filepath.Join(NightDir(state, night), GroupsFile))
	if err != nil {
		return ""
	}
	var g groupsDoc
	if json.Unmarshal(b, &g) != nil {
		return ""
	}
	return g.Root
}

// ledgerRoot is the root the night's night-open line names in the ledger
// under state (RootOf), "" when none. Only the day files from the day
// before the night's date on are read: the line is written as the night
// opens, and the day before covers a ledger day in another zone.
func ledgerRoot(fs effects.FS, state, night string) string {
	from := ledgerFrom(night)
	dir := effects.LedgerDir(state)
	names, _ := fs.List(dir)
	sort.Strings(names)
	for _, name := range names {
		day, ok := strings.CutSuffix(name, ".jsonl")
		if !ok || day < from {
			continue
		}
		b, err := fs.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var lines []map[string]any
		for _, ln := range bytes.Split(b, []byte("\n")) {
			var l map[string]any
			if bytes.Contains(ln, []byte(`"night-open"`)) && json.Unmarshal(ln, &l) == nil {
				lines = append(lines, l)
			}
		}
		if r := RootOf(lines, night); r != "" {
			return r
		}
	}
	return ""
}
