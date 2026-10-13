package hooks

// The tree reader (AR46, design/guard-parser-r1.md §2): segments(cmd, depth)
// read through mvdan.cc/sh/v3/syntax. The contract is Segments': a segment is
// one simple command as its words, a backgrounded line's segments carry
// bgMark first, and a line the reader cannot read whole carries the one-word
// segment unreadMark, with boundMark after it when the reader gave up on the
// line: at the depth bound, or on a panic it recovered.

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// maxLine is the longest string handed to the parser; a longer one is
// unreadable unparsed, so no input recurses the parser without bound.
const maxLine = 64 << 10

// dialects are the grammars a line is read under (§2, rule 1): the Bash
// tool's shell is zsh, and `sh -c` and `bash -c` are bash. Both are
// required: a line either rejects is unreadable (rule 2).
var dialects = []syntax.LangVariant{syntax.LangBash, syntax.LangZsh}

// walkTree, when set, stands in for the walk of one parsed tree: a test's
// seam, to put a panic or a node kind the walker does not know under the
// reader's own recover. Nil outside a test.
var walkTree func(w *walker, f *syntax.File)

// parseTree is the parser one dialect's reading goes through. A test swaps
// it, to put a panic in the parser under the reader's own recover.
var parseTree = func(cmd string, lang syntax.LangVariant) (*syntax.File, error) {
	return syntax.NewParser(syntax.Variant(lang), syntax.KeepComments(true)).Parse(strings.NewReader(cmd), "")
}

// treeSegments is segments(cmd, depth): the union of what each dialect
// reads (rule 1), and unreadMark when any of them could not read the line
// whole (rule 2), after what did parse, so a named guard answers first. A
// line no dialect parsed to its end is read loosely from where the parser
// stopped, before the mark: a guard that named such a line still names it.
// And whatever left a reading unread (a rejected line, rule 3, a node kind
// the walker does not know), the whole line is read loosely after the
// mark: in a row the mark has answered by then, and outside one, where
// nothing reads the mark, the guards every session meets still answer on
// what no tree holds. That reading is blind to quotes, so outside a row an
// unread line that only names a guarded command inside a quote is denied.
func treeSegments(cmd string, depth int) [][]string {
	if depth > maxDepth {
		// past the bound: unreadable, and no tree is read below it; the
		// loose reading of the whole line, above, still judges its text
		return [][]string{{unreadMark}, {boundMark}}
	}
	if len(cmd) > maxLine {
		// never parsed: read loosely, and given up on when its nesting,
		// counted loosely from this depth, passes the depth bound
		out := append(loose(cmd, depth), []string{unreadMark})
		if depth+looseNest(cmd) > maxDepth {
			out = append(out, []string{boundMark})
		}
		return out
	}
	var (
		readings [][][]string
		out      [][]string
		unread   bool
		bound    bool   // a reading gave up: the depth bound, or a panic
		parsed   bool   // a dialect parsed the line to its end
		rest     string // the longest tail a dialect left unparsed
	)
	for _, lang := range dialects {
		segs, mark, tail, ok := readTree(cmd, depth, lang)
		unread = unread || mark != ""
		bound = bound || mark == boundMark
		parsed = parsed || ok
		if len(tail) > len(rest) {
			rest = tail
		}
		dup := false
		for _, r := range readings {
			dup = dup || reflect.DeepEqual(r, segs)
		}
		if !dup {
			readings = append(readings, segs)
			out = append(out, segs...)
		}
	}
	if !unread {
		return out
	}
	if !parsed {
		out = append(out, loose(rest, depth)...)
	}
	out = append(out, []string{unreadMark})
	if bound {
		out = append(out, []string{boundMark})
	}
	if parsed || rest != cmd {
		out = append(out, loose(cmd, depth)...) // else it is read above
	}
	return out
}

// looseNest is the substitution nesting of a line too long to parse,
// counted over its bytes: each `$(` and each backquote opens a level, and
// nothing closes one, so the count is never under the nesting a parser
// would find. It stops one past the depth bound, and never recurses. A
// long line of over maxDepth substitutions side by side counts as past the
// bound too: an over-denial, taken so that no deep line is read as flat.
func looseNest(text string) int {
	n := 0
	for i := 0; i < len(text) && n <= maxDepth; i++ {
		if text[i] == '`' || text[i] == '$' && i+1 < len(text) && text[i+1] == '(' {
			n++
		}
	}
	return n
}

// readTree is one dialect's reading of cmd. mark is "" for a line read
// whole; unreadMark when the parser rejected the line or the walker met
// what it cannot hold; and boundMark when the reader gave up, the walker at
// the depth bound or either on a panic. What was read before that is still
// returned. ok is whether the parser took the line to its end, and tail is
// what it left when not.
func readTree(cmd string, depth int, lang syntax.LangVariant) (segs [][]string, mark, tail string, ok bool) {
	w := &walker{src: cmd, depth: depth}
	defer func() {
		if recover() != nil {
			segs, mark, tail, ok = w.marked(), boundMark, cmd, false
		}
	}()
	f, err := parseTree(cmd, lang)
	if f != nil {
		if walkTree != nil {
			walkTree(w, f)
		} else {
			w.stmts(f.Stmts)
		}
		w.check(f)
	}
	if err != nil {
		w.unread = true
		return w.marked(), w.mark(), unparsed(cmd, f), false
	}
	return w.marked(), w.mark(), "", true
}

// unparsed is what follows the last statement the parser read whole.
func unparsed(cmd string, f *syntax.File) string {
	if f == nil || len(f.Stmts) == 0 {
		return cmd
	}
	end := f.Stmts[len(f.Stmts)-1].End()
	if !end.IsValid() || int(end.Offset()) > len(cmd) {
		return cmd
	}
	return cmd[end.Offset():]
}

// check is the library's own walk of the tree, for what the walker's does
// not show. Every simple command it finds must have been read, so a subtree
// the walker does not descend is never dropped unread. And inside a
// substitution, a comment or a `${…}` holding a `)` is unreadable: bash 3.2
// ends the substitution at that `)` (AR44 act 1), and the tree holds the
// other shells' reading alone.
func (w *walker) check(f *syntax.File) {
	var (
		n, subst int
		open     []bool // each node the walk is inside: is it a substitution
	)
	syntax.Walk(f, func(node syntax.Node) bool {
		if node == nil {
			if open[len(open)-1] {
				subst--
			}
			open = open[:len(open)-1]
			return true
		}
		is := false
		switch x := node.(type) {
		case *syntax.CallExpr, *syntax.DeclClause, *syntax.LetClause:
			n++
		case *syntax.CmdSubst, *syntax.ProcSubst:
			is = true
			subst++
		case *syntax.Comment:
			w.unread = w.unread || subst > 0
		case *syntax.ParamExp:
			if subst > 0 && !x.Short && strings.Contains(w.text(x), ")") {
				w.unread = true
			}
		}
		open = append(open, is)
		return true
	})
	if n != w.simple {
		w.unread = true
	}
}

// loose reads text no tree holds (the parser rejected it, or was never
// handed it), so that a guard that answered on such a line still does. It
// reads twice, and both readings count: with each closed quote a span of
// its word, and with every quote character dropped, so a command inside an
// unfinished quote or substitution is read too. Every operator ends a
// segment; a segment of the first reading is re-read as any other.
func loose(text string, depth int) [][]string {
	var segs [][]string
	for _, blind := range []bool{false, true} {
		var (
			at     = len(segs)
			words  []string
			word   strings.Builder
			inWord bool
			bg     bool
		)
		endWord := func() {
			if inWord {
				words = append(words, word.String())
			}
			word.Reset()
			inWord = false
		}
		endSeg := func() {
			endWord()
			// a command word that is no literal is what an unread
			// substitution left behind: the mark answers for it
			if c := commandWord(words); len(words) > 0 && (c == len(words) || literal(words[c])) {
				segs = append(segs, words)
				if !blind {
					sub := &walker{src: text, depth: depth}
					sub.reread(words)
					segs = append(segs, sub.segs...)
					if sub.bound {
						segs = append(segs, []string{unreadMark}, []string{boundMark})
					}
				}
			}
			words = nil
		}
		for i := 0; i < len(text); i++ {
			switch c := text[i]; {
			case c == '\\' && i+1 < len(text):
				i++
				if text[i] != '\n' {
					word.WriteByte(text[i])
					inWord = true
				}
			case c == '\'' || c == '"':
				inWord = true
				if j := strings.IndexByte(text[i+1:], c); !blind && j >= 0 {
					word.WriteString(text[i+1 : i+1+j])
					i += j + 1
				}
			case c == ' ' || c == '\t':
				endWord()
			case c == '$' && i+1 < len(text) && text[i+1] == '(':
				// the `(` ends the segment; the `$` is no word of its own
			case strings.IndexByte(";&|()\n`", c) >= 0:
				if c == '&' && !(i+1 < len(text) && strings.IndexByte("&>", text[i+1]) >= 0) &&
					!(i > 0 && strings.IndexByte("&><|;", text[i-1]) >= 0) {
					bg = true
				}
				endSeg()
			default:
				word.WriteByte(c)
				inWord = true
			}
		}
		endSeg()
		for k := at; bg && k < len(segs); k++ {
			segs[k] = append([]string{bgMark}, segs[k]...)
		}
		if blind && at > 0 && reflect.DeepEqual(segs[:at], segs[at:]) {
			segs = segs[:at] // no quote on the line: one reading
		}
	}
	return segs
}

// walker turns one tree into segments.
type walker struct {
	src    string
	depth  int
	segs   [][]string
	simple int  // the simple commands read
	unread bool // the tree holds what the walker cannot read
	bound  bool // and it is past the depth bound (boundMark)
	bg     bool // a command is backgrounded: every segment is marked (bgMark)
	quoted int  // how many double-quoted spans (or heredoc bodies) are open
}

// mark is what the walker's reading carries: nothing for a tree read whole,
// boundMark when the walk stopped at the depth bound, else unreadMark.
func (w *walker) mark() string {
	switch {
	case w.bound:
		return boundMark
	case w.unread:
		return unreadMark
	}
	return ""
}

// marked is the walker's segments, each with bgMark first when the line
// backgrounds a command.
func (w *walker) marked() [][]string {
	if !w.bg {
		return w.segs
	}
	out := make([][]string, len(w.segs))
	for k, s := range w.segs {
		out[k] = append([]string{bgMark}, s...)
	}
	return out
}

func (w *walker) stmts(list []*syntax.Stmt) {
	for _, s := range list {
		w.stmt(s, nil)
	}
}

func (w *walker) emit(words []string) {
	if len(words) > 0 {
		w.segs = append(w.segs, words)
	}
}

// text is the node's own bytes in the source.
func (w *walker) text(n syntax.Node) string {
	a, b := int(n.Pos().Offset()), int(n.End().Offset())
	if !n.Pos().IsValid() || !n.End().IsValid() || b > len(w.src) || a > b {
		w.unread = true
		return ""
	}
	return w.src[a:b]
}

// stmt reads one statement: a simple command is a segment (its
// assignments, words and redirections in the order they are written, so a
// redirection before the command word reads as it did), and a compound one
// is walked for the simple commands it holds.
func (w *walker) stmt(s *syntax.Stmt, prefix []string) {
	if s == nil {
		return
	}
	if s.Background || s.Disown || s.Coprocess {
		w.bg = true
	}
	var (
		items  []item
		arrays [][]string
		simple bool
	)
	assign := func(a *syntax.Assign) {
		word, elems := w.assign(a)
		items = append(items, item{a.Pos(), []string{word}})
		if elems != nil {
			arrays = append(arrays, elems)
		}
	}
	switch c := s.Cmd.(type) {
	case nil:
	case *syntax.CallExpr:
		simple = true
		for _, a := range c.Assigns {
			assign(a)
		}
		for _, a := range c.Args {
			items = append(items, item{a.Pos(), []string{w.word(a)}})
		}
	case *syntax.DeclClause:
		simple = true
		if c.Variant != nil {
			items = append(items, item{c.Pos(), []string{c.Variant.Value}})
		}
		for _, a := range c.Args {
			assign(a)
		}
	case *syntax.LetClause:
		simple = true
		items = append(items, item{c.Pos(), []string{"let"}})
		for _, e := range c.Exprs {
			items = append(items, item{e.Pos(), []string{w.arithm(e)}})
		}
		if strings.Contains(w.text(c), "<<") {
			// the shells run let as a command, so its `<<` opens a
			// heredoc; the tree reads a shift, and what follows otherwise
			w.unread = true
		}
	case *syntax.TimeClause:
		w.stmt(c.Stmt, append(prefix, "time"))
	case *syntax.CoprocClause:
		w.bg = true
		if c.Name != nil {
			w.word(c.Name)
		}
		w.stmt(c.Stmt, prefix) // judged as the command it runs
	case *syntax.Subshell:
		w.stmts(c.Stmts)
	case *syntax.Block:
		w.stmts(c.Stmts)
	case *syntax.IfClause:
		for i := c; i != nil; i = i.Else {
			w.stmts(i.Cond)
			w.stmts(i.Then)
		}
	case *syntax.WhileClause:
		w.stmts(c.Cond)
		w.stmts(c.Do)
	case *syntax.ForClause:
		switch l := c.Loop.(type) {
		case *syntax.WordIter:
			head := []string{"for", "", "in"}
			if l.Name != nil {
				head[1] = l.Name.Value
			}
			for _, it := range l.Items {
				head = append(head, w.word(it))
			}
			w.emit(head)
		case *syntax.CStyleLoop:
			head := []string{"for", "(("}
			for _, e := range []syntax.ArithmExpr{l.Init, l.Cond, l.Post} {
				if e != nil {
					head = append(head, w.arithm(e))
				}
			}
			w.emit(append(head, "))"))
		default:
			w.unread = true // a loop kind the walker does not know
		}
		w.stmts(c.Do)
	case *syntax.CaseClause:
		head := []string{"case", w.word(c.Word), "in"}
		for _, it := range c.Items {
			for _, p := range it.Patterns {
				head = append(head, w.word(p))
			}
		}
		w.emit(head)
		for _, it := range c.Items {
			w.stmts(it.Stmts)
		}
	case *syntax.BinaryCmd:
		w.stmt(c.X, nil)
		w.stmt(c.Y, nil)
	case *syntax.FuncDecl:
		if c.Name != nil {
			w.emit([]string{c.Name.Value})
		}
		for _, n := range c.Names {
			w.emit([]string{n.Value})
		}
		w.stmt(c.Body, nil)
	case *syntax.ArithmCmd:
		w.emit([]string{"((", w.arithm(c.X), "))"})
	case *syntax.TestClause:
		head := []string{"[["}
		w.test(c.X, &head)
		w.emit(append(head, "]]"))
	default:
		w.unread = true // a command kind the walker does not know
	}
	for _, r := range s.Redirs {
		items = append(items, item{r.Pos(), w.redir(r)})
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].at.Offset() < items[b].at.Offset() })
	var words []string
	if simple {
		words = append(words, prefix...)
	}
	for _, it := range items {
		words = append(words, it.words...)
	}
	w.emit(words) // a compound command's redirections are a segment of their own
	for _, e := range arrays {
		// an array's elements are a segment, re-read as the old reader
		// re-read every segment: `a=(sh -c '…')` hands its string on
		w.emit(e)
		w.reread(e)
	}
	if simple {
		w.simple++
		w.reread(words)
	}
}

// item is one piece of a simple command, with where it is written.
type item struct {
	at    syntax.Pos
	words []string
}

// assign is an assignment as one word, NAME=value; an array's elements
// come back as a segment of their own, as the old reader gave them. A keyed
// element's key is no word of that segment, and is read for what it runs.
func (w *walker) assign(a *syntax.Assign) (string, []string) {
	name := ""
	if a.Name != nil {
		name = a.Name.Value
	}
	if a.Index != nil {
		name += "[" + w.arithm(a.Index) + "]"
	}
	switch {
	case a.Array != nil:
		elems := []string{}
		for _, e := range a.Array.Elems {
			if e.Index != nil {
				w.arithm(e.Index)
			}
			if e.Value != nil {
				elems = append(elems, w.word(e.Value))
			}
		}
		return name + "=", elems
	case a.Naked && a.Value != nil && a.Name == nil:
		return w.word(a.Value), nil
	case a.Naked:
		return name, nil
	case a.Value != nil:
		if a.Append {
			return name + "+=" + w.word(a.Value), nil
		}
		return name + "=" + w.word(a.Value), nil
	}
	return name + "=", nil
}

// redir is a redirection as words: the operator, then its target; `<` and
// `<<<` stand alone before their word, and `<<` stands alone, its
// delimiter no word (the old reader's shape). Every redirection that feeds
// the command is the `<` word, `<&` and `<>` too, so a shell fed by one
// reads as fed from stdin (stdinShell). A heredoc's body is read for the
// substitutions it runs.
func (w *walker) redir(r *syntax.Redirect) []string {
	n := ""
	if r.N != nil {
		n = r.N.Value
	}
	switch r.Op {
	case syntax.Hdoc, syntax.DashHdoc:
		w.word(r.Word)
		if r.Hdoc != nil {
			w.quoted++
			w.word(r.Hdoc)
			w.quoted--
		}
		return []string{"<<"}
	case syntax.WordHdoc:
		return []string{"<<<", w.word(r.Word)}
	case syntax.RdrIn, syntax.RdrInOut, syntax.DplIn:
		return []string{n + "<", w.word(r.Word)}
	case syntax.RdrAll, syntax.RdrAllClob:
		return []string{">", w.word(r.Word)}
	case syntax.AppAll, syntax.AppAllClob:
		return []string{">>", w.word(r.Word)}
	}
	return []string{n + r.Op.String(), w.word(r.Word)}
}

// word is a word as one string: quotes removed where static, an expansion
// kept as its source text (so it holds a `$` and is never literal), a
// substitution as `$()` with its commands read as segments of their own.
func (w *walker) word(x *syntax.Word) string {
	if x == nil {
		return ""
	}
	var b strings.Builder
	for _, p := range x.Parts {
		w.part(&b, p)
	}
	return b.String()
}

func (w *walker) part(b *strings.Builder, p syntax.WordPart) {
	switch p := p.(type) {
	case *syntax.Lit:
		b.WriteString(unescapeLit(p.Value, w.quoted > 0))
	case *syntax.SglQuoted:
		if p.Dollar {
			ansiC(p.Value+"'", 0, b)
		} else {
			b.WriteString(p.Value)
		}
	case *syntax.DblQuoted:
		w.quoted++
		for _, q := range p.Parts {
			w.part(b, q)
		}
		w.quoted--
	case *syntax.ParamExp:
		src := w.text(p)
		if w.quoted > 0 && !p.Short && strings.ContainsAny(src, `'"`) {
			// rule 3: the shells differ on a quote inside `"${…}"`, and
			// the tree holds one reading for every dialect
			w.unread = true
		}
		w.param(p)
		b.WriteString(src)
	case *syntax.CmdSubst:
		if p.TempFile || p.ReplyVar {
			w.unread = true // `${ cmd; }` and `${|cmd;}`: no shell on the box runs them
		}
		w.subst(p.Stmts)
		b.WriteString("$()")
	case *syntax.ArithmExp:
		w.arithm(p.X)
		b.WriteString(w.text(p))
	case *syntax.ProcSubst:
		w.subst(p.Stmts)
		// the old reader's shape: `<(…)` is the word `<`, so a shell fed
		// from one reads as fed from stdin
		b.WriteString(strings.TrimSuffix(p.Op.String(), "("))
	default:
		w.unread = true // an extended glob, and any part kind added later
	}
}

// param reads the words inside an expansion for the substitutions they run.
func (w *walker) param(p *syntax.ParamExp) {
	if p.NestedParam != nil {
		var b strings.Builder
		w.part(&b, p.NestedParam)
	}
	if p.Index != nil {
		w.arithm(p.Index)
	}
	if p.Slice != nil {
		for _, e := range []syntax.ArithmExpr{p.Slice.Offset, p.Slice.Length} {
			if e != nil {
				w.arithm(e)
			}
		}
	}
	if p.Repl != nil {
		w.word(p.Repl.Orig)
		w.word(p.Repl.With)
	}
	if p.Exp != nil {
		w.word(p.Exp.Word)
	}
}

// subst reads a substitution's commands one level down; past the bound the
// line is unreadable and nothing below is judged.
func (w *walker) subst(list []*syntax.Stmt) {
	if w.depth+1 > maxDepth {
		w.unread, w.bound = true, true
		return
	}
	w.depth++
	q := w.quoted
	w.quoted = 0 // the quoting starts fresh inside a substitution
	w.stmts(list)
	w.quoted = q
	w.depth--
}

// arithm is an arithmetic expression as its source text, its words read
// for substitutions.
func (w *walker) arithm(e syntax.ArithmExpr) string {
	switch e := e.(type) {
	case *syntax.Word:
		return w.word(e)
	case *syntax.BinaryArithm:
		return w.arithm(e.X) + e.Op.String() + w.arithm(e.Y)
	case *syntax.UnaryArithm:
		if e.Post {
			return w.arithm(e.X) + e.Op.String()
		}
		return e.Op.String() + w.arithm(e.X)
	case *syntax.ParenArithm:
		return "(" + w.arithm(e.X) + ")"
	default:
		w.unread = true // zsh's subscript flags, and any kind added later
	}
	return ""
}

// test gathers the words of a `[[ … ]]` in order.
func (w *walker) test(e syntax.TestExpr, out *[]string) {
	switch e := e.(type) {
	case *syntax.Word:
		*out = append(*out, w.word(e))
	case *syntax.BinaryTest:
		w.test(e.X, out)
		w.test(e.Y, out)
	case *syntax.UnaryTest:
		*out = append(*out, e.Op.String())
		w.test(e.X, out)
	case *syntax.ParenTest:
		w.test(e.X, out)
	default:
		w.unread = true
	}
}

// reread reads the strings a simple command hands to another reader, one
// level deeper: the argument of `sh -c` (and kin), eval's words, the string
// `env -S` splits, and each command find runs by -exec (and kin).
func (w *walker) reread(words []string) {
	i := commandWord(words)
	if s, ok := splitString(words[:i]); ok {
		// env reads the split words as its own: options, then the command
		w.segs = append(w.segs, treeSegments("env "+s+" "+strings.Join(words[i:], " "), w.depth+1)...)
		return
	}
	if i == len(words) {
		return
	}
	switch filepath.Base(words[i]) {
	case "find":
		for j := i + 1; j < len(words); j++ {
			switch words[j] {
			case "-exec", "-execdir", "-ok", "-okdir":
				k := j + 1
				for k < len(words) && words[k] != ";" && words[k] != "+" {
					k++
				}
				if run := words[j+1 : k]; len(run) > 0 {
					w.emit(run)
					if w.depth+1 > maxDepth {
						// what the command would run in turn is past the
						// bound, so unreadable
						_, runs := splitString(run)
						if c := commandWord(run); !runs && c < len(run) {
							base := filepath.Base(run[c])
							runs = shells[base] || base == "find" || base == "eval"
						}
						w.unread, w.bound = w.unread || runs, w.bound || runs
					} else {
						sub := &walker{src: w.src, depth: w.depth + 1}
						sub.reread(run)
						w.segs = append(w.segs, sub.segs...)
						w.unread, w.bound = w.unread || sub.unread, w.bound || sub.bound
					}
				}
				j = k
			}
		}
	case "eval":
		w.segs = append(w.segs, treeSegments(strings.Join(words[i+1:], " "), w.depth+1)...)
	case "sh", "bash", "zsh", "dash", "ksh":
		for j := i + 1; j+1 < len(words); j++ {
			a := words[j]
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") {
				w.segs = append(w.segs, treeSegments(words[j+1], w.depth+1)...)
				return
			}
		}
	}
}

// unescapeLit removes a literal's backslashes as the shell does: outside
// quotes each takes the next byte; inside double quotes only before one of
// $ ` " \ and a newline.
func unescapeLit(s string, quoted bool) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			n := s[i+1]
			if !quoted || strings.IndexByte("$`\"\\\n", n) >= 0 {
				i++
				if n != '\n' {
					b.WriteByte(n)
				}
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
