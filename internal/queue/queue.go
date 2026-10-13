// Package queue loads and structurally validates `relay/queue.yaml`
// (design/relay-r1.md §3, D4). Dependency satisfaction (the ledger, ancestry)
// is not judged here: an id absent from the queue is external, never invalid.
package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/packet"
	"github.com/agentik-dev/agentik-relay/internal/yaml"
)

// Queue is one loaded queue file.
type Queue struct {
	Version  int
	Defaults Row
	Words    string
	Rows     []Row
	Hash     string // sha256 of the file, hex
}

// Row is one queue row, the defaults already applied.
type Row struct {
	ID, PacketPath      string
	Packet              *packet.Packet
	DependsOn           []string
	Base, Model         string
	Turns, Context, USD int
	Wall                time.Duration
	Merge, Device       string
	Parked              string
	Verify              string // defaults only (§3's `verify: auto`)
	// MeasureSeconds is the measure's budget on main, defaults only (§3,
	// §15; AR38 act 1): 60 to 14400 seconds, 0 unset. `--measure-seconds`
	// outranks it; unset, night.DefaultMeasureSeconds.
	MeasureSeconds int `yaml:"measure_seconds"`
	// Harness is the verify line's harness as the queue names it (AR20
	// act 2, ADV11-4): repo-relative paths the gate and the merge lay over
	// the branch's tree as they are on main, and guard in the touch set;
	// nil is unset. HarnessOf resolves it.
	Harness []string
	// Kind is build (the default) or orchestrator (AR32A, orchestrator r1
	// §2). An orchestrator row has no packet: Prompt is its standing brief
	// at main, Sections the brief's sections its body line names, MayTouch
	// the guarded paths it may also change.
	Kind, Prompt string
	Sections     []string
	MayTouch     []string

	packetErr error
}

// The row kinds (orchestrator r1 §2).
const (
	KindBuild        = "build"
	KindOrchestrator = "orchestrator"
)

// IsOrchestrator reports whether r is the queue's standing row.
func (r Row) IsOrchestrator() bool { return r.Kind == KindOrchestrator }

// SameRow reports whether a and b are the same row as the queue file
// writes it: every field but the parsed packet and its error (AR32A act
// 3), so a gate compares a row at main with one at a branch's head.
func SameRow(a, b Row) bool {
	a.Packet, a.packetErr, b.Packet, b.packetErr = nil, nil, nil, nil
	return reflect.DeepEqual(a, b)
}

// Orchestrator is the queue's orchestrator row and true, or false.
func (q *Queue) Orchestrator() (Row, bool) {
	for _, r := range q.Rows {
		if r.IsOrchestrator() {
			return r, true
		}
	}
	return Row{}, false
}

// Problem is one validation finding; Row is empty for the queue as a whole.
type Problem struct {
	Row string `json:"row"`
	Msg string `json:"msg"`
}

// The bounds of the defaults' measure_seconds (§3, §15; AR38 act 1).
const (
	MinMeasureSeconds = 60
	MaxMeasureSeconds = 14400
)

// The merge policies (§3); merge-on-green needs the row's base to be main.
const (
	BranchOnly   = "branch-only"
	MergeOnGreen = "merge-on-green"
)

var (
	topKeys      = []string{"version", "defaults", "words", "rows"}
	defaultsKeys = []string{"model", "turns", "context", "usd", "wall", "merge", "device", "verify", "harness", "measure_seconds"}
	rowKeys      = []string{"id", "packet", "depends_on", "base", "model", "budget", "merge", "device", "parked", "harness",
		"kind", "prompt", "sections", "may_touch"}
	budgetKeys = []string{"turns", "context", "usd", "wall"}
)

// Load reads the queue at path (relative paths are under repo) and parses
// each row's packet from repo. A queue that does not parse, or holds a key
// or a value of the wrong shape, is an error; a row's own faults wait for
// Validate.
func Load(repo, path string) (*Queue, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, path)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(src)
	q := &Queue{Hash: hex.EncodeToString(sum[:])}
	root, err := yaml.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("queue %s: %w", path, err)
	}
	if err := q.fill(root); err != nil {
		return nil, fmt.Errorf("queue %s: %w", path, err)
	}
	for i := range q.Rows {
		r := &q.Rows[i]
		if r.PacketPath == "" || r.Parked != "" {
			continue
		}
		if !filepath.IsLocal(r.PacketPath) { // absolute, or climbs out: never read
			r.packetErr = errors.New("packet path leaves the repo")
			continue
		}
		p := filepath.Join(repo, r.PacketPath)
		src, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			r.packetErr = fmt.Errorf("packet %s: missing", r.PacketPath)
			continue
		} else if err != nil {
			r.packetErr = err
			continue
		}
		r.Packet, r.packetErr = packet.Parse(p, src)
	}
	return q, nil
}

func refuse(n yaml.Node, format string, a ...any) error {
	return fmt.Errorf("line %d: %s", n.Line, fmt.Sprintf(format, a...))
}

func only(n yaml.Node, what string, keys []string) error {
	if n.Kind != yaml.Map {
		return refuse(n, "%s: want a mapping", what)
	}
	for _, kv := range n.Map {
		ok := false
		for _, k := range keys {
			ok = ok || k == kv.Key
		}
		if !ok {
			return refuse(kv.Val, "%s: unknown key %q", what, kv.Key)
		}
	}
	return nil
}

func (q *Queue) fill(root yaml.Node) error {
	if err := only(root, "queue", topKeys); err != nil {
		return err
	}
	v, ok := root.Get("version")
	if !ok {
		return refuse(root, "version: missing")
	}
	if q.Version, ok = v.Int(); !ok {
		return refuse(v, "version: want an integer")
	}
	if d, ok := root.Get("defaults"); ok {
		if err := only(d, "defaults", defaultsKeys); err != nil {
			return err
		}
		if err := apply(&q.Defaults, d, "defaults"); err != nil {
			return err
		}
	}
	if w, ok := root.Get("words"); ok {
		if q.Words, ok = w.String(); !ok {
			return refuse(w, "words: want a string")
		}
	}
	rows, ok := root.Get("rows")
	if !ok || rows.Kind != yaml.List {
		return refuse(root, "rows: want a list")
	}
	for i, n := range rows.List {
		what := fmt.Sprintf("row %d", i+1)
		if err := only(n, what, rowKeys); err != nil {
			return err
		}
		r := q.Defaults
		r.DependsOn = nil
		if err := apply(&r, n, what); err != nil {
			return err
		}
		if r.Kind == "" {
			r.Kind = KindBuild
		}
		if _, named := n.Get("merge"); r.Kind == KindOrchestrator && !named {
			r.Merge = MergeOnGreen // ar32b act 4: its own merge, over branch-only defaults
		}
		if b, ok := n.Get("budget"); ok {
			if err := only(b, what+" budget", budgetKeys); err != nil {
				return err
			}
			if err := apply(&r, b, what+" budget"); err != nil {
				return err
			}
		}
		q.Rows = append(q.Rows, r)
	}
	return nil
}

// apply copies n's known keys onto r; the budget mapping is applied apart.
func apply(r *Row, n yaml.Node, what string) error {
	strs := map[string]*string{"id": &r.ID, "packet": &r.PacketPath, "base": &r.Base, "model": &r.Model,
		"merge": &r.Merge, "device": &r.Device, "parked": &r.Parked, "verify": &r.Verify,
		"kind": &r.Kind, "prompt": &r.Prompt}
	ints := map[string]*int{"turns": &r.Turns, "context": &r.Context, "usd": &r.USD}
	for _, kv := range n.Map {
		switch {
		case strs[kv.Key] != nil:
			s, ok := kv.Val.String()
			if !ok {
				return refuse(kv.Val, "%s: %s: want a string", what, kv.Key)
			}
			*strs[kv.Key] = s
		case ints[kv.Key] != nil:
			v, ok := kv.Val.Int()
			if !ok {
				return refuse(kv.Val, "%s: %s: want an integer", what, kv.Key)
			}
			*ints[kv.Key] = v
		case kv.Key == "measure_seconds": // defaults only: no row key names it
			v, ok := kv.Val.Int()
			if !ok || v < MinMeasureSeconds || v > MaxMeasureSeconds {
				return refuse(kv.Val, "%s: measure_seconds: want an integer from %d to %d", what, MinMeasureSeconds, MaxMeasureSeconds)
			}
			r.MeasureSeconds = v
		case kv.Key == "wall":
			s, _ := kv.Val.String()
			d, err := time.ParseDuration(s)
			if err != nil {
				return refuse(kv.Val, "%s: wall: want a duration such as 4h", what)
			}
			r.Wall = d
		case kv.Key == "depends_on":
			ids, ok := kv.Val.Strings()
			if !ok {
				return refuse(kv.Val, "%s: depends_on: want a list of ids", what)
			}
			r.DependsOn = ids
		case kv.Key == "sections":
			secs, ok := kv.Val.Strings()
			if !ok {
				return refuse(kv.Val, "%s: sections: want a list of the brief's section names", what)
			}
			r.Sections = secs
		case kv.Key == "harness" || kv.Key == "may_touch":
			paths, ok := kv.Val.Strings()
			if !ok {
				return refuse(kv.Val, "%s: %s: want a list of repo-relative paths", what, kv.Key)
			}
			for _, p := range paths {
				if c := filepath.Clean(p); p == "" || c == "." || !filepath.IsLocal(p) {
					return refuse(kv.Val, "%s: %s: %q is absolute or leaves the repo", what, kv.Key, p)
				}
			}
			if kv.Key == "harness" {
				r.Harness = paths
			} else {
				r.MayTouch = paths
			}
		}
	}
	return nil
}

// MakeSpellings are the names make reads a makefile by, in its order: every
// one is always in the harness, so a branch cannot outrank main's Makefile
// with its own GNUmakefile (ADV11-4).
var MakeSpellings = []string{"GNUmakefile", "makefile", "Makefile"}

// DefaultHarness is the harness of a queue that names none.
var DefaultHarness = []string{"Makefile", "scripts"}

// HarnessOf is the harness a row's gate and merge use: the make spellings,
// then paths (DefaultHarness when nil), cleaned, each once.
func HarnessOf(paths []string) []string {
	if paths == nil {
		paths = DefaultHarness
	}
	out := append([]string(nil), MakeSpellings...)
	for _, p := range paths {
		if c := filepath.Clean(p); !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// Has reports whether id is a row of this queue.
func (q *Queue) Has(id string) bool {
	for _, r := range q.Rows {
		if r.ID == id {
			return true
		}
	}
	return false
}

// edges are a row's prerequisites: its depends_on and its base (unless main).
func edges(r Row) []string {
	out := append([]string(nil), r.DependsOn...)
	if r.Base != "" && r.Base != "main" {
		out = append(out, r.Base)
	}
	return out
}

// ExternalOf returns the ids r names that are not rows of this queue.
func (q *Queue) ExternalOf(r Row) []string {
	var out []string
	for _, id := range edges(r) {
		if !q.Has(id) && !contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// External returns the ids named by any row and absent from the queue, in
// the order first named.
func (q *Queue) External() []string {
	var out []string
	for _, r := range q.Rows {
		for _, id := range q.ExternalOf(r) {
			if !contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// Order returns the rows' ids so that every row follows its in-queue
// depends_on and base; among ready rows the queue's order wins. A cycle is
// an error naming the rows on it.
func (q *Queue) Order() ([]string, error) {
	var ids []string
	pre := map[string][]string{}
	for _, r := range q.Rows {
		if contains(ids, r.ID) {
			continue
		}
		ids = append(ids, r.ID)
		for _, e := range edges(r) {
			if q.Has(e) {
				pre[r.ID] = append(pre[r.ID], e)
			}
		}
	}
	placed := map[string]bool{}
	var out []string
	for len(out) < len(ids) {
		next := ""
		for _, id := range ids {
			if placed[id] {
				continue
			}
			ready := true
			for _, p := range pre[id] {
				ready = ready && placed[p]
			}
			if ready {
				next = id
				break
			}
		}
		if next == "" {
			return out, fmt.Errorf("cycle: %s", strings.Join(cycle(ids, pre, placed), " -> "))
		}
		placed[next] = true
		out = append(out, next)
	}
	return out, nil
}

// cycle walks prerequisites among the unplaced rows until one repeats, and
// returns the loop, starting and ending on that row.
func cycle(ids []string, pre map[string][]string, placed map[string]bool) []string {
	var start string
	for _, id := range ids {
		if !placed[id] {
			start = id
			break
		}
	}
	var path []string
	at := map[string]int{}
	for id := start; ; {
		if i, seen := at[id]; seen {
			return append(path[i:], id)
		}
		at[id] = len(path)
		path = append(path, id)
		for _, p := range pre[id] {
			if !placed[p] {
				id = p
				break
			}
		}
	}
}

// idShape is an id: it names a branch, a worktree and a file stem, so it is
// never `.` or `..` and never starts with `-` or `.`.
var idShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// refProblem is why a well-shaped id is no branch of its own (ADV10-19):
// what `git check-ref-format --branch` refuses beyond idShape (`..`, a
// trailing `.`, a `.lock` suffix, HEAD), or a base (`main`, or the row's
// own). HEAD, main and .lock fold case, as this Mac's filesystem does.
func refProblem(id, base string) string {
	lower := strings.ToLower(id)
	switch {
	case strings.Contains(id, ".."), strings.HasSuffix(id, "."), strings.HasSuffix(lower, ".lock"), lower == "head":
		return fmt.Sprintf("id %q: not a branch name (git check-ref-format --branch)", id)
	case lower == "main" || strings.EqualFold(id, base):
		return fmt.Sprintf("id %q: a base, never a row's branch", id)
	}
	return ""
}

// Validate returns every structural problem (§3); none means valid. Parked
// rows are listed, never validated, beyond a unique, well-shaped id. Ids are
// unique case-folded: this Mac's filesystem folds case, so `A` beside `a`
// would share a worktree.
func (q *Queue) Validate() []Problem {
	var ps []Problem
	if q.Version != 1 {
		ps = append(ps, Problem{"", fmt.Sprintf("version %d; want 1", q.Version)})
	}
	seen := map[string]bool{}
	for i, r := range q.Rows {
		name, folded := r.ID, strings.ToLower(r.ID)
		switch {
		case name == "":
			name = fmt.Sprintf("#%d", i+1)
			ps = append(ps, Problem{name, "no id"})
		case !idShape.MatchString(r.ID):
			ps = append(ps, Problem{name, fmt.Sprintf("id %q: not [A-Za-z0-9][A-Za-z0-9._-]*", r.ID)})
		case refProblem(r.ID, r.Base) != "":
			ps = append(ps, Problem{name, refProblem(r.ID, r.Base)})
		case seen[folded]:
			ps = append(ps, Problem{name, "duplicate id"})
		}
		seen[folded] = true
		if r.Parked != "" {
			continue
		}
		add := func(format string, a ...any) { ps = append(ps, Problem{name, fmt.Sprintf(format, a...)}) }
		switch r.Kind {
		case KindOrchestrator:
			orchestratorProblems(r, add)
			continue
		case KindBuild, "":
			if r.Prompt != "" || r.Sections != nil || r.MayTouch != nil {
				add("prompt, sections and may_touch belong to an orchestrator row; a build row is today's row")
			}
		default:
			add("kind %q; want %s or %s", r.Kind, KindBuild, KindOrchestrator)
		}
		switch stem := strings.TrimSuffix(filepath.Base(r.PacketPath), filepath.Ext(r.PacketPath)); {
		case r.PacketPath == "":
			add("no packet")
		case r.packetErr != nil:
			add("%v", r.packetErr)
		case r.ID != "" && stem != r.ID:
			add("id is not the packet's file stem %s", stem)
		}
		switch {
		case r.Merge != BranchOnly && r.Merge != MergeOnGreen:
			add("merge %q; want %s or %s", r.Merge, BranchOnly, MergeOnGreen)
		case r.Merge == MergeOnGreen && r.Base != "" && r.Base != "main":
			add("merge-on-green needs base main, not %s", r.Base)
		}
		if strings.TrimSpace(r.Device) == "" {
			add("device is empty; want never or the owner's words")
		}
		if r.Model == "" {
			add("no model")
		}
		if r.Turns <= 0 || r.Context <= 0 || r.USD <= 0 || r.Wall <= 0 {
			add("budget turns %d, context %d, usd %d, wall %s; each must be above zero (§7's ceilings)", r.Turns, r.Context, r.USD, r.Wall)
		}
	}
	ps = append(ps, q.orchestratorQueueProblems()...)
	if _, err := q.Order(); err != nil {
		loop := strings.Split(strings.TrimPrefix(err.Error(), "cycle: "), " -> ")
		for _, row := range loop[:len(loop)-1] {
			ps = append(ps, Problem{row, err.Error()})
		}
	}
	return ps
}

// orchestratorProblems holds orchestrator r1 §2 for the standing row: no
// packet, depends_on or base; merge-on-green; device never; a local
// prompt and the sections its body line names; a model and a budget.
func orchestratorProblems(r Row, add func(string, ...any)) {
	if r.PacketPath != "" || len(r.DependsOn) > 0 || r.Base != "" {
		add("an orchestrator row has no packet, depends_on or base")
	}
	if r.Merge != MergeOnGreen {
		add("an orchestrator row merges %s, not %q", MergeOnGreen, r.Merge)
	}
	if r.Device != "never" {
		add("an orchestrator row's device is never, not %q", r.Device)
	}
	if c := filepath.Clean(r.Prompt); r.Prompt == "" || c == "." || !filepath.IsLocal(r.Prompt) {
		add("prompt %q: want the standing brief's repo-relative path", r.Prompt)
	}
	if len(r.Sections) == 0 {
		add("an orchestrator row names the brief's sections its body line reads")
	}
	if r.Model == "" {
		add("no model")
	}
	if r.Turns <= 0 || r.Context <= 0 || r.USD <= 0 || r.Wall <= 0 {
		add("budget turns %d, context %d, usd %d, wall %s; each must be above zero (§7's ceilings)", r.Turns, r.Context, r.USD, r.Wall)
	}
}

// orchestratorQueueProblems holds the queue's side of §2: at most one
// orchestrator row, no row depends on it, and no row takes the id of one
// of its sittings (<id>.<n>, its branch and ledger key, §4).
func (q *Queue) orchestratorQueueProblems() []Problem {
	var ps []Problem
	var orch string
	for _, r := range q.Rows {
		if !r.IsOrchestrator() {
			continue
		}
		if orch != "" {
			ps = append(ps, Problem{r.ID, fmt.Sprintf("a second orchestrator row; %s is the queue's one", orch)})
			continue
		}
		orch = r.ID
	}
	if orch == "" {
		return nil
	}
	sitting := regexp.MustCompile(`^(?i)` + regexp.QuoteMeta(orch) + `\.[0-9]+$`)
	for _, r := range q.Rows {
		if r.ID == orch {
			continue
		}
		if sitting.MatchString(r.ID) {
			ps = append(ps, Problem{r.ID, fmt.Sprintf("id %q: the name of a sitting of %s", r.ID, orch)})
		}
		if r.Parked == "" && (contains(r.DependsOn, orch) || r.Base == orch) {
			ps = append(ps, Problem{r.ID, fmt.Sprintf("depends on the orchestrator row %s; no row does", orch)})
		}
	}
	return ps
}
