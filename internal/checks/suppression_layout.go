package checks

import (
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

type Span struct{ Start, End int }

type CommentStyle int

const (
	PHPComment CommentStyle = iota
	PHPBlockComment
	BladeComment
)

const maxBlankLinesInReach = 1

var commentDelimiters = map[CommentStyle][2]string{
	PHPComment:      {"// ", ""},
	PHPBlockComment: {"/* ", " */"},
	BladeComment:    {"{{-- ", " --}}"},
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
	attributes []Span
	php        []Span
	opaque     []Span
	constructs []Span
	blade      bool
}

func PHPLayout(source string) Layout {
	return phpLayout(source, parser.Tokenize(source))
}

func phpLayout(source string, tokens []parser.Token) Layout {
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
		case tok.Kind == parser.TokenHash && i+1 < len(tokens) && tokens[i+1].Kind == parser.TokenOpenBracket:
			end := parser.MatchingClose(tokens, i+1)
			if end < 0 {
				end = len(tokens) - 1
			}
			l.addAttribute(Span{tok.Offset, tokens[end].Offset + len(tokens[end].Value)})
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
		if (n.Kind == blade.KindDirective || n.Kind == blade.KindEcho) && strings.Contains(source[n.Span.Start:n.Span.End], "\n") {
			l.constructs = append(l.constructs, Span{n.Span.Start, n.Span.End})
		}
		if n.Kind != blade.KindPHP || n.Raw {
			continue
		}
		l.php = append(l.php, Span{n.Expr.Start, n.Expr.End})
		body := PHPLayout("<?php " + source[n.Expr.Start:n.Expr.End])
		shift := n.Expr.Start - len("<?php ")
		for _, c := range body.comments {
			l.addComment(Span{c.Start + shift, c.End + shift})
		}
		for _, a := range body.attributes {
			l.addAttribute(Span{a.Start + shift, a.End + shift})
		}
		for _, o := range body.opaque {
			l.opaque = append(l.opaque, Span{o.Start + shift, o.End + shift})
		}
	}
	sort.Slice(l.comments, func(i, j int) bool { return l.comments[i].Start < l.comments[j].Start })
	sort.Slice(l.constructs, func(i, j int) bool { return l.constructs[i].Start < l.constructs[j].Start })
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

func (l *Layout) addComment(s Span) {
	s.End = s.Start + len(strings.TrimRight(l.source[s.Start:s.End], " \t\r\n"))
	l.comments = append(l.comments, s)
	l.addSkipped(s)
}

func (l *Layout) addAttribute(s Span) {
	l.attributes = append(l.attributes, s)
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

func (l Layout) reach(offset int) (int, bool) {
	blanks := 0
	for {
		gap := leadingSpace(l.source[offset:], " \t\r\n")
		blanks += blankLines(gap)
		if blanks > maxBlankLinesInReach {
			return 0, false
		}
		offset += len(gap)
		end, ok := l.skipFrom[offset]
		if !ok {
			return l.Line(offset), true
		}
		offset = end
	}
}

func (l Layout) statementStart(line int) int {
	for {
		start := l.lineStarts[line]
		offset := start + len(leadingSpace(l.source[start:], " \t"))
		c, ok := spanBefore(l.constructs, offset)
		if !ok || offset >= c.End {
			return line
		}
		line = l.Line(c.Start)
	}
}

func (l Layout) statementEnd(line int) int {
	for {
		next := len(l.source)
		if line+1 < len(l.lineStarts) {
			next = l.lineStarts[line+1]
		}
		c, ok := spanBefore(l.constructs, next)
		if !ok || l.Line(c.End-1) <= line {
			return line
		}
		line = l.Line(c.End - 1)
	}
}

func (l Layout) MarkerLine(line int) int {
	line = l.statementStart(line)
	start, _ := l.LineStart(line)
	offset := start + len(leadingSpace(l.source[start:], " \t"))
	marker := offset
	blanks := 0
	for {
		prevEnd := len(strings.TrimRight(l.source[:offset], " \t\r\n"))
		begin, ok := l.skipTo[prevEnd]
		if !ok {
			break
		}
		blanks += blankLines(l.source[prevEnd:offset])
		if blanks > maxBlankLinesInReach {
			break
		}
		offset = begin
		if l.startsLine(begin) {
			marker = begin
		}
	}
	return l.Line(marker)
}

func (l Layout) TrailingCommentAt(line int) (offset int, style CommentStyle, ok bool) {
	start, ok := l.LineStart(line)
	if !ok {
		return 0, 0, false
	}
	rest := l.source[start:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	end := start + len(strings.TrimRight(rest, " \t\r"))
	if s, inPHP := l.StyleAt(end); !inPHP || s != PHPComment {
		return 0, 0, false
	}
	offset, style = end, PHPComment
	if c, found := spanBefore(l.comments, end); found && c.End == end && c.Start >= start && isLineComment(l.source[c.Start:c.End]) {
		offset, style = c.Start, PHPBlockComment
	}
	if !l.codeBefore(offset) {
		return 0, 0, false
	}
	return offset, style, true
}

func (l Layout) codeBefore(offset int) bool {
	lineStart := l.lineStarts[l.Line(offset)]
	for {
		trimmed := lineStart + len(strings.TrimRight(l.source[lineStart:offset], " \t"))
		if trimmed == lineStart {
			return false
		}
		c, ok := spanBefore(l.comments, trimmed)
		if !ok || c.End != trimmed {
			return true
		}
		if c.Start < lineStart {
			return false
		}
		offset = c.Start
	}
}

func (l Layout) nextNonComment(offset int) int {
	for {
		offset += len(leadingSpace(l.source[offset:], " \t\r\n"))
		i := sort.Search(len(l.comments), func(i int) bool { return l.comments[i].Start >= offset })
		if i == len(l.comments) || l.comments[i].Start != offset {
			return offset
		}
		offset = l.comments[i].End
	}
}

func (l Layout) Character(offset int) int {
	n := 0
	for _, r := range l.source[l.lineStarts[l.Line(offset)]:offset] {
		n += utf16.RuneLen(r)
	}
	return n
}

func leadingSpace(s, cutset string) string {
	return s[:len(s)-len(strings.TrimLeft(s, cutset))]
}

func blankLines(gap string) int {
	return max(strings.Count(gap, "\n")-1, 0)
}

func isLineComment(text string) bool {
	return strings.HasPrefix(text, "//") || strings.HasPrefix(text, "#")
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
	return leadingSpace(l.source[start:], " \t")
}
