package blade

import (
	"sort"
	"strings"
)

// RefKind is what a directive's string argument names.
type RefKind int

const (
	RefView    RefKind = iota // a view: `@include('partials.nav')`
	RefSection                // a section: `@section('title')`, `@yield('title')`
	RefStack                  // a stack: `@push('scripts')`, `@stack('scripts')`
)

// Ref is a name given as a string to a directive.
type Ref struct {
	Kind      RefKind
	Directive string // as written, without `@`
	Name      string
	Span      Span // the name, without its quotes
	// Declares is set for `@yield` and `@stack`, where a layout shows a
	// section or stack that other views fill.
	Declares bool
	// Optional is set where a missing view is fine, as in `@includeIf`.
	Optional bool
}

type argRole struct {
	kind     RefKind
	declares bool
	optional bool
}

// refArgs lists, by directive, which arguments name a view, section or stack.
var refArgs = map[string]map[int]argRole{
	"include":        {0: {kind: RefView}},
	"includeif":      {0: {kind: RefView, optional: true}},
	"includewhen":    {1: {kind: RefView}},
	"includeunless":  {1: {kind: RefView}},
	"each":           {0: {kind: RefView}, 3: {kind: RefView}},
	"extends":        {0: {kind: RefView}},
	"component":      {0: {kind: RefView}},
	"section":        {0: {kind: RefSection}},
	"hassection":     {0: {kind: RefSection}},
	"sectionmissing": {0: {kind: RefSection}},
	"yield":          {0: {kind: RefSection, declares: true}},
	"push":           {0: {kind: RefStack}},
	"pushonce":       {0: {kind: RefStack}},
	"prepend":        {0: {kind: RefStack}},
	"prependonce":    {0: {kind: RefStack}},
	"pushif":         {1: {kind: RefStack}},
	"stack":          {0: {kind: RefStack, declares: true}},
	"hasstack":       {0: {kind: RefStack}},
}

// Refs returns the names given as plain strings to directives, in order.
// Names built at runtime, such as `@include($view)` or `"rows.$type"`, are
// left out.
func Refs(src string, nodes []Node) []Ref {
	var refs []Ref
	for _, n := range nodes {
		if n.Kind != KindDirective || !n.HasArgs {
			continue
		}
		roles, ok := refArgs[strings.ToLower(n.Name)]
		if !ok {
			continue
		}
		for i, arg := range splitArgs(src, n.Args) {
			role, ok := roles[i]
			if !ok {
				continue
			}
			content, ok := stringLiteral(src, arg)
			if !ok {
				continue
			}
			refs = append(refs, Ref{
				Kind: role.kind, Directive: n.Name, Name: src[content.Start:content.End], Span: content,
				Declares: role.declares, Optional: role.optional,
			})
		}
	}
	return refs
}

// RefAt returns the ref whose name contains offset, its ends included.
func RefAt(refs []Ref, offset int) (Ref, bool) {
	for _, r := range refs {
		if r.Span.Start <= offset && offset <= r.Span.End {
			return r, true
		}
	}
	return Ref{}, false
}

// PartialRefAt reads the string a cursor is typing in a directive argument
// that names a view, section or stack, even before its quote and parenthesis
// are closed: `@include('partials.n|`. The ref's Name is what is typed up to
// offset, and its Span runs from the opening quote to the closing quote or
// the cursor.
func PartialRefAt(src string, offset int) (Ref, bool) {
	if offset < 0 || offset > len(src) {
		return Ref{}, false
	}
	lineStart := strings.LastIndexByte(src[:offset], '\n') + 1
	line := src[lineStart:offset]
	for at := strings.LastIndexByte(line, '@'); at >= 0; at = strings.LastIndexByte(line[:at], '@') {
		if at > 0 && isNameByte(line[at-1]) {
			continue
		}
		j := at + 1
		for j < len(line) && isNameByte(line[j]) {
			j++
		}
		name := line[at+1 : j]
		roles, ok := refArgs[strings.ToLower(name)]
		if !ok {
			continue
		}
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		if j >= len(line) || line[j] != '(' {
			continue
		}
		argIndex, quoteAt, ok := openStringArg(line, j+1)
		if !ok {
			return Ref{}, false
		}
		role, ok := roles[argIndex]
		if !ok {
			return Ref{}, false
		}
		start := lineStart + quoteAt + 1
		end := offset
		if k := strings.IndexByte(src[offset:], line[quoteAt]); k >= 0 && !strings.ContainsAny(src[offset:offset+k], "\n(),") {
			end = offset + k
		}
		return Ref{
			Kind: role.kind, Directive: name, Name: src[start:offset], Span: Span{start, end},
			Declares: role.declares, Optional: role.optional,
		}, true
	}
	return Ref{}, false
}

// openStringArg reads directive arguments from i to the end of s, which must
// end inside a quoted string at the top level of the arguments. It returns the
// index of the argument and of its opening quote.
func openStringArg(s string, i int) (argIndex, quoteAt int, ok bool) {
	depth := 0
	quoteAt = -1
	for ; i < len(s); i++ {
		c := s[i]
		if quoteAt >= 0 {
			if c == '\\' {
				i++
			} else if c == s[quoteAt] {
				quoteAt = -1
			}
			continue
		}
		switch c {
		case '\'', '"':
			if depth != 0 {
				// A string nested in a call or array: skip it.
				q := c
				for i++; i < len(s) && s[i] != q; i++ {
					if s[i] == '\\' {
						i++
					}
				}
				continue
			}
			quoteAt = i
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return 0, 0, false
			}
			depth--
		case ',':
			if depth == 0 {
				argIndex++
			}
		}
	}
	return argIndex, quoteAt, quoteAt >= 0
}

// splitArgs splits directive arguments at their top-level commas.
func splitArgs(src string, sp Span) []Span {
	var args []Span
	start, depth := sp.Start, 0
	for i := sp.Start; i < sp.End; i++ {
		switch c := src[i]; c {
		case '\'', '"':
			for i++; i < sp.End && src[i] != c; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, Span{start, i})
				start = i + 1
			}
		}
	}
	if strings.TrimSpace(src[start:sp.End]) != "" || len(args) > 0 {
		args = append(args, Span{start, sp.End})
	}
	return args
}

// stringLiteral returns the content of an argument that is a single quoted
// string with nothing interpolated in it.
func stringLiteral(src string, sp Span) (Span, bool) {
	for sp.Start < sp.End && isSpaceByte(src[sp.Start]) {
		sp.Start++
	}
	for sp.End > sp.Start && isSpaceByte(src[sp.End-1]) {
		sp.End--
	}
	if sp.End-sp.Start < 2 {
		return Span{}, false
	}
	q := src[sp.Start]
	if (q != '\'' && q != '"') || src[sp.End-1] != q {
		return Span{}, false
	}
	content := Span{sp.Start + 1, sp.End - 1}
	body := src[content.Start:content.End]
	if strings.IndexByte(body, q) >= 0 || strings.IndexByte(body, '\\') >= 0 || (q == '"' && strings.IndexByte(body, '$') >= 0) {
		return Span{}, false
	}
	return content, true
}

// DirectivePrefixAt reports whether offset ends an `@` and a partial
// directive name in the template's text, where a directive can be typed. It
// returns the offset of the `@` and the name typed so far.
func DirectivePrefixAt(src string, nodes []Node, offset int) (at int, prefix string, ok bool) {
	if offset < 0 || offset > len(src) {
		return 0, "", false
	}
	j := offset
	for j > 0 && isNameByte(src[j-1]) {
		j--
	}
	at = j - 1
	if at < 0 || src[at] != '@' {
		return 0, "", false
	}
	// `name@example.com` and `@@if` aren't directives.
	if at > 0 && (isNameByte(src[at-1]) || src[at-1] == '@') {
		return 0, "", false
	}
	if InPHP(nodes, offset) {
		return 0, "", false
	}
	return at, src[j:offset], true
}

// InPHP reports whether offset is in PHP the template contains: an echo, a
// PHP block, a bound attribute or a directive's arguments.
func InPHP(nodes []Node, offset int) bool {
	i := sort.Search(len(nodes), func(i int) bool { return nodes[i].Span.Start > offset })
	// Nodes don't overlap, except the nodes inside a component tag, which come
	// after it; look back over the few that can contain offset.
	for k := i - 1; k >= 0 && k >= i-8; k-- {
		n := nodes[k]
		switch n.Kind {
		case KindEcho, KindPHP, KindBoundAttribute:
			if n.Expr.Start <= offset && offset <= n.Expr.End {
				return true
			}
		case KindDirective:
			if n.HasArgs && n.Args.Start <= offset && offset <= n.Args.End {
				return true
			}
		}
	}
	return false
}

// DirectiveAt returns the directive whose `@name` contains offset.
func DirectiveAt(nodes []Node, offset int) (Node, bool) {
	i := sort.Search(len(nodes), func(i int) bool { return nodes[i].Span.Start > offset }) - 1
	if i < 0 {
		return Node{}, false
	}
	n := nodes[i]
	if n.Kind != KindDirective || offset > n.Span.Start+1+len(n.Name) {
		return Node{}, false
	}
	return n, true
}

// Lines maps between byte offsets and 0-based lines and byte columns.
type Lines []int

// NewLines indexes the line starts of src.
func NewLines(src string) Lines { return lineStarts(src) }

// Position returns the line and column of a byte offset.
func (l Lines) Position(offset int) (line, col int) {
	line = sort.Search(len(l), func(i int) bool { return l[i] > offset }) - 1
	if line < 0 {
		return 0, offset
	}
	return line, offset - l[line]
}

// Offset returns the byte offset of a line and column, or -1 past the end.
func (l Lines) Offset(line, col int) int {
	if line < 0 || line >= len(l) || col < 0 {
		return -1
	}
	return l[line] + col
}
