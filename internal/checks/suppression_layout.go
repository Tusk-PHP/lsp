package checks

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type Span struct{ Start, End int }

type CommentStyle int

const (
	PHPComment CommentStyle = iota
	PHPBlockComment
	// TemplateComment is the comment syntax of the template language a
	// TemplateLayout was built for.
	TemplateComment
)

const maxBlankLinesInReach = 1

var phpCommentDelimiters = map[CommentStyle][2]string{
	PHPComment:      {"// ", ""},
	PHPBlockComment: {"/* ", " */"},
}

// Wrap writes text as a comment in style: a PHP comment, or one in the
// template's own syntax.
func (l Layout) Wrap(style CommentStyle, text string) string {
	d := phpCommentDelimiters[style]
	if style == TemplateComment {
		d = [2]string{l.template.CommentOpen + " ", " " + l.template.CommentClose}
	}
	return d[0] + text + d[1]
}

// commentOpeners are what a comment in this file can start with.
func (l Layout) commentOpeners() []string {
	if l.template == nil {
		return phpCommentOpeners
	}
	return append([]string{l.template.CommentOpen}, phpCommentOpeners...)
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
	// template is set for a TemplateLayout.
	template *Template
}

// Template is what another package finds in a file written in a template
// language around PHP (Blade): the language's comments, the PHP code it
// embeds, the constructs that span lines, and its comment delimiters. The
// generic checks know PHP; the template's own syntax stays with its package.
type Template struct {
	Comments   []Span
	PHP        []Span
	Constructs []Span
	// CommentOpen and CommentClose delimit a comment, like "{{--" and "--}}".
	CommentOpen, CommentClose string
}

func PHPLayout(source string) Layout {
	return phpLayout(source, lexer.Lex(source, token.Latest))
}

func phpLayout(source string, tokens []lexer.Token) Layout {
	l := newLayout(source)
	regionStart := -1
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		span := Span{int(tok.Start), int(tok.End)}
		switch tok.Kind {
		case token.OpenTag, token.OpenTagWithEcho:
			regionStart = span.Start + len(strings.TrimRight(tok.Text(source), " \t\r\n"))
			continue
		case token.CloseTag:
			if regionStart >= 0 {
				l.php = append(l.php, Span{regionStart, span.Start})
				regionStart = -1
			}
			continue
		case token.Comment, token.DocComment:
			l.addComment(span)
		case token.Attribute:
			l.addAttribute(Span{span.Start, int(tokens[attributeEnd(tokens, i)].End)})
			continue
		case token.DoubleQuote, token.StartHeredoc, token.Backtick:
			i = lexer.LiteralEnd(tokens, i)
			span.End = int(tokens[i].End)
		case token.ConstantString:
		default:
			continue
		}
		if strings.Contains(source[span.Start:span.End], "\n") {
			l.opaque = append(l.opaque, span)
		}
	}
	if regionStart >= 0 {
		l.php = append(l.php, Span{regionStart, len(source)})
	}
	return l
}

func attributeEnd(tokens []lexer.Token, open int) int {
	if end := lexer.MatchingClose(tokens, open); end >= 0 {
		return end
	}
	return len(tokens) - 1
}

// TemplateLayout lays out a template file from what its language's package
// found in it, reading the embedded PHP itself.
func TemplateLayout(source string, t Template) Layout {
	l := newLayout(source)
	l.template = &t
	for _, c := range t.Comments {
		l.addComment(c)
	}
	l.constructs = append(l.constructs, t.Constructs...)
	for _, region := range t.PHP {
		l.php = append(l.php, region)
		body := PHPLayout("<?php " + source[region.Start:region.End])
		shift := region.Start - len("<?php ")
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

func newLayout(source string) Layout {
	starts := []int{0}
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return Layout{source: source, lineStarts: starts, skipFrom: map[int]int{}, skipTo: map[int]int{}}
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
	return offset - l.lineStarts[l.Line(offset)]
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
	return TemplateComment, l.template != nil
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
