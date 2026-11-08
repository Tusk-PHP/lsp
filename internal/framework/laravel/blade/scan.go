// Package blade reads Laravel Blade templates: it splits a template into its
// parts (HTML, echoes, directives, PHP blocks, component tags) and assembles
// the PHP in it into a virtual PHP document with a map back to the template, so
// PHP features can run on Blade.
package blade

import "strings"

// Kind is the kind of a part of a template.
type Kind int

const (
	// KindEcho is `{{ expr }}` (Raw false) or `{!! expr !!}` (Raw true). Expr is
	// the expression between the delimiters.
	KindEcho Kind = iota
	// KindDirective is a known `@name`, with its arguments between the
	// parentheses in Args when HasArgs is set.
	KindDirective
	// KindPHP is PHP code: the body of `@php … @endphp` or `<?php … ?>`, or the
	// expression of `<?= … ?>` (Raw true).
	KindPHP
	// KindBoundAttribute is the PHP expression of a bound attribute on a
	// component tag: `:title="$post->title"` or the `$user` of `:$user`.
	KindBoundAttribute
	// KindComponent is a component tag's name: `alert` in `<x-alert>`,
	// `livewire:counter` in `<livewire:counter>`.
	KindComponent
)

// Span is a byte range [Start, End) of the template.
type Span struct{ Start, End int }

// Node is one part of a template. Text between nodes is HTML, comments or
// anything else Blade leaves alone.
type Node struct {
	Kind    Kind
	Span    Span   // the whole construct, delimiters included
	Name    string // directive name without `@`; component name
	Expr    Span   // the PHP inside: echo expression, PHP body, bound attribute
	Args    Span   // a directive's arguments, without the parentheses
	HasArgs bool
	Raw     bool
}

// Parse splits a template into its nodes, in order.
func Parse(src string) []Node {
	s := scanner{src: src}
	s.scan(0, len(src))
	return s.nodes
}

type scanner struct {
	src   string
	nodes []Node
}

func (s *scanner) add(n Node) { s.nodes = append(s.nodes, n) }

// scan reads src[start:end] for Blade constructs.
func (s *scanner) scan(start, end int) {
	src := s.src
	i := start
	for i < end {
		c := src[i]
		switch {
		case c == '{' && strings.HasPrefix(src[i:end], "{{--"):
			i = indexFrom(src, "--}}", i+4, end, 4)
		case c == '{' && strings.HasPrefix(src[i:end], "{!!"):
			i = s.echo(i, end, "{!!", "!!}", true)
		case c == '{' && strings.HasPrefix(src[i:end], "{{{"):
			i = s.echo(i, end, "{{{", "}}}", false)
		case c == '{' && strings.HasPrefix(src[i:end], "{{"):
			i = s.echo(i, end, "{{", "}}", false)
		case c == '<' && strings.HasPrefix(src[i:end], "<?php"):
			i = s.rawPHP(i, end, 5, false)
		case c == '<' && strings.HasPrefix(src[i:end], "<?="):
			i = s.rawPHP(i, end, 3, true)
		case c == '<' && isComponentTagStart(src[i:end]):
			i = s.component(i, end)
		case c == '@':
			i = s.at(i, end)
		default:
			i++
		}
	}
}

func (s *scanner) echo(i, end int, open, close string, raw bool) int {
	exprStart := i + len(open)
	closeAt := strings.Index(s.src[exprStart:end], close)
	if closeAt < 0 {
		// An unclosed echo: Blade leaves it as text.
		return exprStart
	}
	exprEnd := exprStart + closeAt
	s.add(Node{Kind: KindEcho, Span: Span{i, exprEnd + len(close)}, Expr: Span{exprStart, exprEnd}, Raw: raw})
	return exprEnd + len(close)
}

func (s *scanner) rawPHP(i, end, openLen int, short bool) int {
	bodyStart := i + openLen
	bodyEnd := end
	next := end
	if k := strings.Index(s.src[bodyStart:end], "?>"); k >= 0 {
		bodyEnd = bodyStart + k
		next = bodyEnd + 2
	}
	s.add(Node{Kind: KindPHP, Span: Span{i, next}, Expr: Span{bodyStart, bodyEnd}, Raw: short})
	return next
}

// at reads what follows an `@`: an escape, a directive, or text.
func (s *scanner) at(i, end int) int {
	src := s.src
	// `name@example.com`: an @ right after a word character isn't Blade.
	if i > 0 && isNameByte(src[i-1]) {
		return i + 1
	}
	rest := src[i+1 : end]
	switch {
	case strings.HasPrefix(rest, "{{"):
		// `@{{ x }}` prints `{{ x }}` as it is.
		return indexFrom(src, "}}", i+3, end, 2)
	case strings.HasPrefix(rest, "{!!"):
		return indexFrom(src, "!!}", i+4, end, 3)
	case strings.HasPrefix(rest, "@"):
		// `@@if` prints `@if`.
		j := i + 2
		for j < end && isNameByte(src[j]) {
			j++
		}
		return j
	}
	j := i + 1
	for j < end && isNameByte(src[j]) {
		j++
	}
	name := src[i+1 : j]
	if name == "" || !isDirective(name) {
		return j
	}
	n := Node{Kind: KindDirective, Span: Span{i, j}, Name: name}
	// Blade takes arguments only when `(` follows, after spaces or tabs.
	k := j
	for k < end && (src[k] == ' ' || src[k] == '\t') {
		k++
	}
	if k < end && src[k] == '(' {
		if close := matchParen(src, k, end); close >= 0 {
			n.HasArgs = true
			n.Args = Span{k + 1, close}
			n.Span.End = close + 1
		}
	}
	switch name {
	case "php":
		if !n.HasArgs {
			return s.block(n, end, "@endphp", KindPHP)
		}
		s.add(Node{Kind: KindPHP, Span: n.Span, Expr: n.Args, Name: "php"})
		return n.Span.End
	case "verbatim":
		return indexFrom(src, "@endverbatim", n.Span.End, end, len("@endverbatim"))
	}
	s.add(n)
	return n.Span.End
}

// block reads a body up to closer, such as `@php … @endphp`.
func (s *scanner) block(open Node, end int, closer string, kind Kind) int {
	bodyStart := open.Span.End
	bodyEnd, next := end, end
	if k := strings.Index(s.src[bodyStart:end], closer); k >= 0 {
		bodyEnd = bodyStart + k
		next = bodyEnd + len(closer)
	}
	s.add(Node{Kind: kind, Span: Span{open.Span.Start, next}, Expr: Span{bodyStart, bodyEnd}, Name: open.Name})
	return next
}

// component reads a component tag: its name, its bound attributes as PHP, and
// any echoes or directives in the rest of it.
func (s *scanner) component(i, end int) int {
	src := s.src
	nameStart := i + 1
	if strings.HasPrefix(src[nameStart:end], "x-") {
		nameStart += 2
	}
	j := nameStart
	for j < end && isTagNameByte(src[j]) {
		j++
	}
	s.add(Node{Kind: KindComponent, Span: Span{i, j}, Name: src[nameStart:j], Expr: Span{nameStart, j}})
	tagEnd := tagClose(src, j, end)
	k := j
	textStart := j
	for k < tagEnd {
		c := src[k]
		if c == '{' && strings.HasPrefix(src[k:tagEnd], "{{") {
			// An echo in the tag, such as `{{ $attributes }}`.
			k = indexFrom(src, "}}", k+2, tagEnd, 2)
			continue
		}
		if c == '"' || c == '\'' {
			// A quoted value outside a bound attribute is text with echoes.
			if close := strings.IndexByte(src[k+1:tagEnd], c); close >= 0 {
				k += close + 2
				continue
			}
			break
		}
		// `::name` is passed through to Alpine, `:name` is PHP. The colon must
		// start an attribute, not sit inside a name like `wire:model`.
		if c != ':' || !isSpaceByte(src[k-1]) || (k+1 < tagEnd && src[k+1] == ':') {
			k++
			continue
		}
		if k+1 < tagEnd && src[k+1] == '$' {
			// `:$user` is `:user="$user"`.
			e := k + 2
			for e < tagEnd && isNameByte(src[e]) {
				e++
			}
			s.scan(textStart, k)
			s.add(Node{Kind: KindBoundAttribute, Span: Span{k, e}, Expr: Span{k + 1, e}})
			k, textStart = e, e
			continue
		}
		e := k + 1
		for e < tagEnd && isAttrNameByte(src[e]) {
			e++
		}
		if e+1 < tagEnd && src[e] == '=' && (src[e+1] == '"' || src[e+1] == '\'') {
			q := src[e+1]
			if close := strings.IndexByte(src[e+2:tagEnd], q); close >= 0 {
				valEnd := e + 2 + close
				s.scan(textStart, k)
				s.add(Node{Kind: KindBoundAttribute, Span: Span{k, valEnd + 1}, Name: src[k+1 : e], Expr: Span{e + 2, valEnd}})
				k, textStart = valEnd+1, valEnd+1
				continue
			}
		}
		k = e
	}
	s.scan(textStart, tagEnd)
	return tagEnd
}

// isComponentTagStart reports whether s starts a tag whose bound attributes
// are PHP: `<x-name`, `<livewire:name` or `<flux:name`. `<x-slot` is one too.
func isComponentTagStart(s string) bool {
	for _, p := range []string{"<x-", "<livewire:", "<flux:"} {
		if strings.HasPrefix(s, p) && len(s) > len(p) && isTagNameByte(s[len(p)]) {
			return true
		}
	}
	return false
}

// tagClose returns the end of a tag starting before i: just after its `>`,
// skipping quoted values and `{{ }}` echoes, which may contain `>`.
func tagClose(src string, i, end int) int {
	for i < end {
		switch c := src[i]; {
		case c == '"' || c == '\'':
			if k := strings.IndexByte(src[i+1:end], c); k >= 0 {
				i += k + 2
				continue
			}
			return end
		case c == '{' && strings.HasPrefix(src[i:end], "{{"):
			if k := strings.Index(src[i+2:end], "}}"); k >= 0 {
				i += k + 4
				continue
			}
			return end
		case c == '>':
			return i + 1
		}
		i++
	}
	return end
}

// matchParen returns the index of the `)` that closes the `(` at open, skipping
// PHP strings and comments, or -1 when it isn't closed before end.
func matchParen(src string, open, end int) int {
	depth := 0
	for i := open; i < end; i++ {
		switch c := src[i]; c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		case '\'', '"':
			for i++; i < end && src[i] != c; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '/':
			if i+1 < end && src[i+1] == '*' {
				if k := strings.Index(src[i+2:end], "*/"); k >= 0 {
					i += k + 3
				}
			}
		}
	}
	return -1
}

// indexFrom returns the index just after the first sep at or after from, or
// end when there is none.
func indexFrom(src, sep string, from, end, sepLen int) int {
	if from > end {
		return end
	}
	if k := strings.Index(src[from:end], sep); k >= 0 {
		return from + k + sepLen
	}
	return end
}

func isNameByte(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c >= 0x80
}

func isTagNameByte(c byte) bool {
	return isNameByte(c) || c == '-' || c == '.' || c == ':'
}

func isAttrNameByte(c byte) bool {
	return isNameByte(c) || c == '-' || c == '.' || c == ':'
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
