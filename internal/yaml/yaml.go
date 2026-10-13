// Package yaml parses the strict YAML subset of conventions §3 (D12): block
// mappings, scalars (plain, single- and double-quoted), flat lists (block
// `- x` and flow `[a, b]`), flow mappings one level deep (`{turns: 100}`),
// and comments. Anchors, aliases, tags, multi-line scalars, nested flow,
// tabs, document markers and duplicate keys are refused with a line number.
// Every scalar is kept as its string; the typed helpers convert on demand.
package yaml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind is a node's shape.
type Kind int

const (
	Scalar Kind = iota
	List
	Map
)

func (k Kind) String() string {
	return [...]string{"scalar", "list", "map"}[k]
}

// Node is one value of the tree. Map preserves the document's key order.
type Node struct {
	Kind Kind
	Str  string
	List []Node
	Map  []KV
	Line int
}

// KV is one mapping entry.
type KV struct {
	Key string
	Val Node
}

// Error is a refusal, naming the 1-based line it was found on.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

func refuse(n int, format string, a ...any) error {
	return &Error{Line: n, Msg: fmt.Sprintf(format, a...)}
}

// Get returns a mapping's value for key.
func (n Node) Get(key string) (Node, bool) {
	if n.Kind != Map {
		return Node{}, false
	}
	for _, kv := range n.Map {
		if kv.Key == key {
			return kv.Val, true
		}
	}
	return Node{}, false
}

// String returns a scalar's text.
func (n Node) String() (string, bool) {
	if n.Kind != Scalar {
		return "", false
	}
	return n.Str, true
}

// Int returns a scalar as a decimal integer.
func (n Node) Int() (int, bool) {
	if n.Kind != Scalar {
		return 0, false
	}
	v, err := strconv.Atoi(n.Str)
	return v, err == nil
}

// Bool returns a scalar that reads exactly true or false.
func (n Node) Bool() (bool, bool) {
	if n.Kind != Scalar || (n.Str != "true" && n.Str != "false") {
		return false, false
	}
	return n.Str == "true", true
}

// Strings returns a list of scalars.
func (n Node) Strings() ([]string, bool) {
	if n.Kind != List {
		return nil, false
	}
	out := make([]string, 0, len(n.List))
	for _, v := range n.List {
		if v.Kind != Scalar {
			return nil, false
		}
		out = append(out, v.Str)
	}
	return out, true
}

// line is one logical line: comments stripped, flow continuations joined.
type line struct {
	n, indent int
	text      string
}

// Parse parses src. An empty document is an empty mapping.
func Parse(src []byte) (Node, error) {
	lines, err := split(string(src))
	if err != nil {
		return Node{}, err
	}
	if len(lines) == 0 {
		return Node{Kind: Map, Line: 1}, nil
	}
	p := &parser{lines: lines}
	root, err := p.block(lines[0].indent)
	if err != nil {
		return Node{}, err
	}
	if p.i < len(lines) {
		return Node{}, refuse(lines[p.i].n, "unexpected indentation")
	}
	return root, nil
}

// split turns src into logical lines. A flow collection left open at the end
// of a line takes the following lines until it closes.
func split(src string) ([]line, error) {
	raw := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out []line
	depth := 0
	for i, r := range raw {
		n := i + 1
		text := stripComment(r)
		if strings.TrimSpace(text) == "" {
			continue
		}
		body := strings.TrimLeft(text, " ")
		if strings.HasPrefix(body, "\t") {
			return nil, refuse(n, "tab in indentation")
		}
		if strings.Contains(text, "\t") {
			return nil, refuse(n, "tab character")
		}
		body = strings.TrimRight(body, " ")
		if depth > 0 {
			last := &out[len(out)-1]
			last.text += " " + body
			if depth += flowDepth(body); depth < 0 {
				return nil, refuse(n, "closer without opener")
			}
			continue
		}
		switch {
		case body == "---" || body == "..." || strings.HasPrefix(body, "--- "):
			return nil, refuse(n, "document marker")
		case strings.HasPrefix(body, "%"):
			return nil, refuse(n, "directive")
		}
		out = append(out, line{n: n, indent: len(text) - len(strings.TrimLeft(text, " ")), text: body})
		if depth = flowDepth(body); depth < 0 {
			return nil, refuse(n, "closer without opener")
		}
	}
	if depth > 0 {
		return nil, refuse(out[len(out)-1].n, "unterminated flow collection")
	}
	return out, nil
}

// quoteOpens reports whether a quote at s[i] starts a quoted scalar: it must
// begin a token (line start, or after `:`, `-`, `[`, `{`, `,` and spaces).
func quoteOpens(s string, i int) bool {
	j := i - 1
	for j >= 0 && s[j] == ' ' {
		j--
	}
	return j < 0 || strings.IndexByte(":-[{,", s[j]) >= 0
}

// scan walks s outside quoted scalars, calling f on each unquoted byte; f
// returning false stops the walk.
func scan(s string, f func(i int) bool) {
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q == '"' && c == '\\', q == '\'' && c == '\'' && i+1 < len(s) && s[i+1] == '\'':
			i++
		case q != 0 && c == q:
			q = 0
		case q != 0:
		case (c == '"' || c == '\'') && quoteOpens(s, i):
			q = c
		default:
			if !f(i) {
				return
			}
		}
	}
}

func stripComment(s string) string {
	cut := len(s)
	scan(s, func(i int) bool {
		if s[i] == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			cut = i
			return false
		}
		return true
	})
	return strings.TrimRight(s[:cut], " \t")
}

func flowDepth(s string) int {
	d := 0
	scan(s, func(i int) bool {
		switch s[i] {
		case '[', '{':
			d++
		case ']', '}':
			d--
		}
		return true
	})
	return d
}

type parser struct {
	lines []line
	i     int
}

func isItem(s string) bool { return s == "-" || strings.HasPrefix(s, "- ") }

func (p *parser) block(ind int) (Node, error) {
	if isItem(p.lines[p.i].text) {
		return p.list(ind)
	}
	return p.mapping(ind)
}

var keyRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// splitKey splits `key: rest` or `key:`.
func splitKey(s string) (key, rest string, ok bool) {
	idx := -1
	scan(s, func(i int) bool {
		if s[i] == ':' && (i+1 == len(s) || s[i+1] == ' ') {
			idx = i
			return false
		}
		return true
	})
	if idx < 0 {
		return "", "", false
	}
	return s[:idx], strings.TrimSpace(s[idx+1:]), true
}

func (p *parser) mapping(ind int) (Node, error) {
	node := Node{Kind: Map, Line: p.lines[p.i].n}
	seen := map[string]bool{}
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if l.indent < ind {
			break
		}
		if l.indent > ind {
			return Node{}, refuse(l.n, "unexpected indentation")
		}
		if isItem(l.text) {
			return Node{}, refuse(l.n, "list item where a key was expected")
		}
		key, rest, ok := splitKey(l.text)
		if !ok {
			return Node{}, refuse(l.n, "expected key: value")
		}
		if !keyRE.MatchString(key) {
			return Node{}, refuse(l.n, "unsupported key %q", key)
		}
		if seen[key] {
			return Node{}, refuse(l.n, "duplicate key %q", key)
		}
		seen[key] = true
		p.i++
		var val Node
		if rest == "" {
			val = Node{Kind: Scalar, Line: l.n}
			if p.i < len(p.lines) {
				next := p.lines[p.i]
				if next.indent > ind || (next.indent == ind && isItem(next.text)) {
					v, err := p.block(next.indent)
					if err != nil {
						return Node{}, err
					}
					val = v
				}
			}
		} else {
			v, err := inline(rest, l.n)
			if err != nil {
				return Node{}, err
			}
			if err := p.nothingDeeper(ind); err != nil {
				return Node{}, err
			}
			val = v
		}
		node.Map = append(node.Map, KV{Key: key, Val: val})
	}
	return node, nil
}

// nothingDeeper refuses a line indented under a value already complete.
func (p *parser) nothingDeeper(ind int) error {
	if p.i < len(p.lines) && p.lines[p.i].indent > ind {
		if isItem(p.lines[p.i].text) {
			return refuse(p.lines[p.i].n, "list item under a scalar")
		}
		return refuse(p.lines[p.i].n, "unexpected indentation (multi-line scalars are refused)")
	}
	return nil
}

func (p *parser) list(ind int) (Node, error) {
	node := Node{Kind: List, Line: p.lines[p.i].n}
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if l.indent < ind || (l.indent == ind && !isItem(l.text)) {
			break
		}
		if l.indent > ind {
			return Node{}, refuse(l.n, "unexpected indentation")
		}
		rest := strings.TrimLeft(l.text[1:], " ")
		switch {
		case rest == "":
			p.i++
			if p.i < len(p.lines) && p.lines[p.i].indent > ind {
				if isItem(p.lines[p.i].text) {
					return Node{}, refuse(p.lines[p.i].n, "nested list")
				}
				v, err := p.mapping(p.lines[p.i].indent)
				if err != nil {
					return Node{}, err
				}
				node.List = append(node.List, v)
			} else {
				node.List = append(node.List, Node{Kind: Scalar, Line: l.n})
			}
		case isItem(rest):
			return Node{}, refuse(l.n, "nested list")
		default:
			if k, _, ok := splitKey(rest); ok && keyRE.MatchString(k) {
				// `- key: value` opens a mapping at the column of its key.
				p.lines[p.i] = line{n: l.n, indent: l.indent + len(l.text) - len(rest), text: rest}
				v, err := p.mapping(p.lines[p.i].indent)
				if err != nil {
					return Node{}, err
				}
				node.List = append(node.List, v)
				continue
			}
			v, err := inline(rest, l.n)
			if err != nil {
				return Node{}, err
			}
			if v.Kind == List {
				return Node{}, refuse(l.n, "nested list")
			}
			p.i++
			if err := p.nothingDeeper(ind); err != nil {
				return Node{}, err
			}
			node.List = append(node.List, v)
		}
	}
	return node, nil
}

// inline parses a value written on its key's (or item's) line.
func inline(s string, n int) (Node, error) {
	switch s[0] {
	case '&':
		return Node{}, refuse(n, "anchor")
	case '*':
		return Node{}, refuse(n, "alias")
	case '!':
		return Node{}, refuse(n, "tag")
	case '|', '>':
		return Node{}, refuse(n, "multi-line scalar")
	case '@', '`':
		return Node{}, refuse(n, "reserved indicator %q", s[0])
	case '[', '{':
		return flow(s, n)
	case '"', '\'':
		v, rest, err := quoted(s, n)
		if err != nil {
			return Node{}, err
		}
		if strings.TrimSpace(rest) != "" {
			return Node{}, refuse(n, "text after a quoted scalar")
		}
		return Node{Kind: Scalar, Str: v, Line: n}, nil
	}
	if _, _, ok := splitKey(s); ok {
		return Node{}, refuse(n, "mapping value on the key's line (nested mapping)")
	}
	return Node{Kind: Scalar, Str: s, Line: n}, nil
}

// quoted reads a quoted scalar at the start of s and returns what follows it.
func quoted(s string, n int) (string, string, error) {
	q := s[0]
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case q == '\'' && c == '\'' && i+1 < len(s) && s[i+1] == '\'':
			b.WriteByte('\'')
			i++
		case c == q:
			return b.String(), s[i+1:], nil
		case q == '"' && c == '\\':
			if i+1 == len(s) {
				return "", "", refuse(n, "unterminated quote")
			}
			i++
			e, ok := map[byte]string{'\\': `\`, '"': `"`, '/': "/", 'n': "\n", 't': "\t"}[s[i]]
			if !ok {
				return "", "", refuse(n, "unsupported escape \\%c", s[i])
			}
			b.WriteString(e)
		default:
			b.WriteByte(c)
		}
	}
	return "", "", refuse(n, "unterminated quote")
}

// flow parses `[a, b]` or `{k: v}`, one level deep, filling the whole of s.
func flow(s string, n int) (Node, error) {
	open := s[0]
	closer := map[byte]byte{'[': ']', '{': '}'}[open]
	end := -1
	var nested bool
	scan(s, func(i int) bool {
		if i == 0 {
			return true
		}
		switch s[i] {
		case '[', '{':
			nested = true
			return false
		case ']', '}':
			end = i
			return false
		}
		return true
	})
	if nested {
		return Node{}, refuse(n, "nested flow collection")
	}
	if end < 0 || s[end] != closer {
		return Node{}, refuse(n, "unterminated flow collection")
	}
	if strings.TrimSpace(s[end+1:]) != "" {
		return Node{}, refuse(n, "text after a flow collection")
	}
	var items []string
	start := 1
	scan(s[:end], func(i int) bool {
		if s[i] == ',' {
			items = append(items, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
		return true
	})
	if last := strings.TrimSpace(s[start:end]); last != "" || len(items) > 0 {
		items = append(items, last)
	}
	if open == '[' {
		node := Node{Kind: List, Line: n}
		for _, it := range items {
			if _, _, ok := splitKey(it); ok {
				return Node{}, refuse(n, "mapping inside a flow list")
			}
			v, err := flowScalar(it, n)
			if err != nil {
				return Node{}, err
			}
			node.List = append(node.List, v)
		}
		return node, nil
	}
	node := Node{Kind: Map, Line: n}
	seen := map[string]bool{}
	for _, it := range items {
		k, v, ok := splitKey(it)
		if !ok || !keyRE.MatchString(k) {
			return Node{}, refuse(n, "expected key: value in a flow mapping")
		}
		if seen[k] {
			return Node{}, refuse(n, "duplicate key %q", k)
		}
		seen[k] = true
		val, err := flowScalar(v, n)
		if err != nil {
			return Node{}, err
		}
		node.Map = append(node.Map, KV{Key: k, Val: val})
	}
	return node, nil
}

func flowScalar(s string, n int) (Node, error) {
	if s == "" {
		return Node{}, refuse(n, "empty flow item")
	}
	switch s[0] {
	case '&', '*', '!', '|', '>', '@', '`':
		return inline(s, n)
	case '"', '\'':
		return inline(s, n)
	}
	if _, _, ok := splitKey(s); ok {
		return Node{}, refuse(n, "mapping value on the key's line (nested mapping)")
	}
	return Node{Kind: Scalar, Str: s, Line: n}, nil
}
