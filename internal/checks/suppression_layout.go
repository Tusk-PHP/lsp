package checks

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

type Span struct{ Start, End int }

type CommentStyle int

const (
	PHPComment CommentStyle = iota
	BladeComment
)

var commentDelimiters = map[CommentStyle][2]string{
	PHPComment:   {"// ", ""},
	BladeComment: {"{{-- ", " --}}"},
}

func (c CommentStyle) Wrap(text string) string {
	d := commentDelimiters[c]
	return d[0] + text + d[1]
}

type Layout struct {
	source     string
	lineStarts []int
	comments   []Span
	skipFrom   map[int]int
	skipTo     map[int]int
	php        []Span
	opaque     []Span
	blade      bool
}

func PHPLayout(source string) Layout {
	tokens := parser.Tokenize(source)
	l := newLayout(source, false)
	regionStart := -1
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if strings.Contains(tok.Value, "\n") {
			l.opaque = append(l.opaque, Span{tok.Offset, tok.Offset + len(tok.Value)})
		}
		switch {
		case tok.Kind == parser.TokenPHP:
			regionStart = tok.Offset + len(tok.Value)
		case tok.Value == "?>" && regionStart >= 0:
			l.php = append(l.php, Span{regionStart, tok.Offset})
			regionStart = -1
		case tok.Kind == parser.TokenComment || tok.Kind == parser.TokenDocComment:
			l.addComment(Span{tok.Offset, tok.Offset + len(tok.Value)})
		case tok.Kind == parser.TokenHash && tokens[i+1].Kind == parser.TokenOpenBracket:
			end := attributeEnd(tokens, i+1)
			l.addSkipped(Span{tok.Offset, tokens[end].Offset + len(tokens[end].Value)})
			i = end
		}
	}
	if regionStart >= 0 {
		l.php = append(l.php, Span{regionStart, len(source)})
	}
	return l
}

func BladeLayout(source string) Layout {
	l := newLayout(source, true)
	for _, c := range blade.Comments(source) {
		l.addComment(Span{c.Start, c.End})
	}
	for _, n := range blade.Parse(source) {
		if n.Kind != blade.KindPHP || n.Raw {
			continue
		}
		l.php = append(l.php, Span{n.Expr.Start, n.Expr.End})
		body := PHPLayout("<?php " + source[n.Expr.Start:n.Expr.End])
		shift := n.Expr.Start - len("<?php ")
		for _, c := range body.comments {
			l.addComment(Span{c.Start + shift, c.End + shift})
		}
		for _, o := range body.opaque {
			l.opaque = append(l.opaque, Span{o.Start + shift, o.End + shift})
		}
	}
	sort.Slice(l.comments, func(i, j int) bool { return l.comments[i].Start < l.comments[j].Start })
	return l
}

func newLayout(source string, isBlade bool) Layout {
	starts := []int{0}
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return Layout{source: source, lineStarts: starts, skipFrom: map[int]int{}, skipTo: map[int]int{}, blade: isBlade}
}

func attributeEnd(tokens []parser.Token, open int) int {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenBracket:
			depth++
		case parser.TokenCloseBracket:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(tokens) - 1
}

func (l *Layout) addComment(s Span) {
	s.End = s.Start + len(strings.TrimRight(l.source[s.Start:s.End], " \t\r\n"))
	l.comments = append(l.comments, s)
	l.addSkipped(s)
}

func (l *Layout) addSkipped(s Span) {
	l.skipFrom[s.Start] = s.End
	l.skipTo[s.End] = s.Start
}

func (l Layout) Line(offset int) int {
	return sort.SearchInts(l.lineStarts, offset+1) - 1
}

func (l Layout) LineStart(line int) (int, bool) {
	if line < 0 || line >= len(l.lineStarts) {
		return 0, false
	}
	return l.lineStarts[line], true
}

func (l Layout) nextCodeLine(offset int) int {
	for {
		offset += len(l.source[offset:]) - len(strings.TrimLeft(l.source[offset:], " \t\r\n"))
		end, ok := l.skipFrom[offset]
		if !ok {
			return l.Line(offset)
		}
		offset = end
	}
}

func (l Layout) MarkerLine(line int) int {
	start, _ := l.LineStart(line)
	offset := start + len(l.source[start:]) - len(strings.TrimLeft(l.source[start:], " \t"))
	marker := offset
	for {
		begin, ok := l.skipTo[len(strings.TrimRight(l.source[:offset], " \t\r\n"))]
		if !ok {
			return l.Line(marker)
		}
		offset = begin
		if l.startsLine(begin) {
			marker = begin
		}
	}
}

func (l Layout) startsLine(offset int) bool {
	start := l.lineStarts[l.Line(offset)]
	return strings.TrimLeft(l.source[start:offset], " \t") == ""
}

func (l Layout) StyleAt(offset int) (CommentStyle, bool) {
	if o, ok := spanBefore(l.opaque, offset); ok && offset < o.End {
		return PHPComment, false
	}
	if region, ok := spanBefore(l.php, offset+1); ok && offset <= region.End {
		return PHPComment, true
	}
	return BladeComment, l.blade
}

func spanBefore(spans []Span, offset int) (Span, bool) {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].Start >= offset })
	if i == 0 {
		return Span{}, false
	}
	return spans[i-1], true
}

func (l Layout) LineEnding() string {
	if strings.Contains(l.source, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func (l Layout) Indent(line int) string {
	start, _ := l.LineStart(line)
	rest := l.source[start:]
	return rest[:len(rest)-len(strings.TrimLeft(rest, " \t"))]
}
