package parser

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type FoldingRangeKind string

const (
	FoldingRangeKindComment FoldingRangeKind = "comment"
	FoldingRangeKindRegion  FoldingRangeKind = "region"
)

type FoldingRange struct {
	StartLine int
	EndLine   int
	Kind      FoldingRangeKind
}

type foldEntry struct {
	opener    token.Kind
	startLine int
	kind      FoldingRangeKind
}

type pendingFold struct {
	startLine int
	kind      FoldingRangeKind
}

type folder struct {
	src        string
	lines      []string
	offsets    ast.Lines
	at         int
	toks       []lexer.Token
	ranges     []FoldingRange
	namespaces []int
	stack      []foldEntry
	pending    *pendingFold
}

// ExtractFoldingRanges returns foldable regions derived from declarations,
// balanced delimiters, and multi-line comments.
func ExtractFoldingRanges(source string) []FoldingRange {
	f := &folder{src: source, lines: strings.Split(source, "\n"), offsets: ast.NewLines(source), toks: significantTokens(lexer.Lex(source, token.Latest))}
	for i := 0; i < len(f.toks); i++ {
		i = f.token(i)
	}
	appendNamespaceFolds(&f.ranges, f.lines, f.namespaces)
	slices.SortFunc(f.ranges, func(a, b FoldingRange) int {
		if a.StartLine != b.StartLine {
			return a.StartLine - b.StartLine
		}
		if a.EndLine != b.EndLine {
			return a.EndLine - b.EndLine
		}
		return strings.Compare(string(a.Kind), string(b.Kind))
	})
	return compactFoldingRanges(f.ranges)
}

func significantTokens(all []lexer.Token) []lexer.Token {
	toks := all[:0]
	for _, tok := range all {
		switch tok.Kind {
		case token.Whitespace, token.OpenTag, token.OpenTagWithEcho, token.CloseTag, token.InlineHTML:
		default:
			toks = append(toks, tok)
		}
	}
	return toks
}

func (f *folder) line(offset uint32) int {
	if offset < f.offsets[f.at] {
		f.at = 0
	}
	for f.at+1 < len(f.offsets) && f.offsets[f.at+1] <= offset {
		f.at++
	}
	return f.at
}

func (f *folder) token(i int) int {
	tok := f.toks[i]
	line := f.line(tok.Start)
	switch tok.Kind {
	case token.DocComment:
		f.fold(line, f.line(tok.End-1), FoldingRangeKindComment)
	case token.Comment:
		if f.isBlockComment(tok) {
			f.fold(line, f.line(tok.End-1), FoldingRangeKindComment)
			return i
		}
		end := line
		for i+1 < len(f.toks) && f.toks[i+1].Kind == token.Comment && !f.isBlockComment(f.toks[i+1]) && f.line(f.toks[i+1].Start) == end+1 {
			end++
			i++
		}
		f.fold(line, end, FoldingRangeKindComment)
	case token.Namespace:
		if f.namespaceIsBraced(i) {
			f.pending = &pendingFold{startLine: line}
		} else {
			f.namespaces = append(f.namespaces, line)
		}
	case token.Class:
		if f.previousKind(i) != token.New {
			f.pending = &pendingFold{startLine: line}
		}
	case token.Interface, token.Trait, token.Enum:
		f.pending = &pendingFold{startLine: line}
	case token.Function:
		if f.functionIsNamed(i) {
			f.pending = &pendingFold{startLine: line}
		}
	case token.LeftBrace, token.CurlyOpen, token.DollarOpenCurlyBraces:
		entry := foldEntry{opener: token.LeftBrace, startLine: line, kind: FoldingRangeKindRegion}
		if f.pending != nil {
			entry.startLine, entry.kind = f.pending.startLine, f.pending.kind
			f.pending = nil
		}
		f.stack = append(f.stack, entry)
	case token.RightBrace:
		entry, ok := f.pop(token.LeftBrace)
		f.pending = nil
		if ok {
			f.fold(entry.startLine, line, entry.kind)
		}
	case token.Attribute:
		f.stack = append(f.stack, foldEntry{opener: token.LeftBracket, startLine: line})
	case token.LeftBracket:
		f.stack = append(f.stack, foldEntry{opener: token.LeftBracket, startLine: line, kind: FoldingRangeKindRegion})
	case token.RightBracket:
		if entry, ok := f.pop(token.LeftBracket); ok && entry.kind != "" {
			f.fold(entry.startLine, line, entry.kind)
		}
	}
	return i
}

func (f *folder) isBlockComment(tok lexer.Token) bool {
	return strings.HasPrefix(tok.Text(f.src), "/*")
}

func (f *folder) fold(startLine, endLine int, kind FoldingRangeKind) {
	appendFoldRange(&f.ranges, f.lines, startLine, endLine, kind)
}

func (f *folder) pop(opener token.Kind) (foldEntry, bool) {
	for i := len(f.stack) - 1; i >= 0; i-- {
		if f.stack[i].opener != opener {
			continue
		}
		entry := f.stack[i]
		f.stack = append(f.stack[:i], f.stack[i+1:]...)
		return entry, true
	}
	return foldEntry{}, false
}

func (f *folder) previousKind(i int) token.Kind {
	for i--; i >= 0; i-- {
		if k := f.toks[i].Kind; k != token.Comment && k != token.DocComment {
			return k
		}
	}
	return token.Invalid
}

func (f *folder) functionIsNamed(i int) bool {
	for i++; i < len(f.toks); i++ {
		switch f.toks[i].Kind {
		case token.Comment, token.DocComment, token.Ampersand, token.AmpersandFollowedByVarOrVararg, token.AmpersandNotFollowedByVarOrVararg:
		case token.LeftParen:
			return false
		default:
			return true
		}
	}
	return false
}

func (f *folder) namespaceIsBraced(i int) bool {
	for i++; i < len(f.toks); i++ {
		switch f.toks[i].Kind {
		case token.LeftBrace:
			return true
		case token.Semicolon:
			return false
		}
	}
	return false
}

func appendNamespaceFolds(ranges *[]FoldingRange, lines []string, starts []int) {
	for i, startLine := range starts {
		endLimit := len(lines) - 1
		if i+1 < len(starts) {
			endLimit = starts[i+1] - 1
		}
		endLine := lastNonBlankLine(lines, startLine+1, endLimit)
		if endLine > startLine {
			*ranges = append(*ranges, FoldingRange{StartLine: startLine, EndLine: endLine})
		}
	}
}

func appendFoldRange(ranges *[]FoldingRange, lines []string, startLine, endLine int, kind FoldingRangeKind) {
	if endLine <= startLine {
		return
	}
	endLine = adjustedFoldEndLine(lines, startLine, endLine)
	if endLine <= startLine {
		return
	}
	*ranges = append(*ranges, FoldingRange{StartLine: startLine, EndLine: endLine, Kind: kind})
}

func adjustedFoldEndLine(lines []string, startLine, endLine int) int {
	if endLine <= startLine || endLine >= len(lines) {
		return endLine
	}
	trimmed := strings.TrimSpace(lines[endLine])
	if trimmed == "" {
		return lastNonBlankLine(lines, startLine+1, endLine-1)
	}
	if isClosingOnlyLine(trimmed) {
		return endLine - 1
	}
	return endLine
}

func isClosingOnlyLine(line string) bool {
	switch line {
	case "}", "};", "]", "],", "];", "]);", ")", ");":
		return true
	default:
		return false
	}
}

func lastNonBlankLine(lines []string, start, end int) int {
	if end >= len(lines) {
		end = len(lines) - 1
	}
	for i := end; i >= start && i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return start - 1
}

func compactFoldingRanges(ranges []FoldingRange) []FoldingRange {
	if len(ranges) == 0 {
		return nil
	}
	out := ranges[:1]
	for _, current := range ranges[1:] {
		last := out[len(out)-1]
		if last.StartLine == current.StartLine && last.EndLine == current.EndLine && last.Kind == current.Kind {
			continue
		}
		out = append(out, current)
	}
	return out
}
