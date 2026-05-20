package parser

import (
	"slices"
	"strings"
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
	openKind  TokenKind
	startLine int
	kind      FoldingRangeKind
}

type pendingFold struct {
	startLine int
	kind      FoldingRangeKind
}

// ExtractFoldingRanges returns foldable regions derived from declarations,
// balanced delimiters, and multi-line comments in the current lightweight parser.
func ExtractFoldingRanges(source string) []FoldingRange {
	result := New().Parse(source)
	if result == nil {
		return nil
	}

	var ranges []FoldingRange
	var namespaceStarts []int
	var stack []foldEntry
	var pending *pendingFold

	for i := 0; i < len(result.Tokens); i++ {
		tok := result.Tokens[i]
		switch tok.Kind {
		case TokenDocComment:
			appendFoldRange(&ranges, result.Lines, tok.Line, tok.Line+strings.Count(tok.Value, "\n"), FoldingRangeKindComment)
		case TokenComment:
			if strings.HasPrefix(tok.Value, "/*") {
				appendFoldRange(&ranges, result.Lines, tok.Line, tok.Line+strings.Count(tok.Value, "\n"), FoldingRangeKindComment)
				continue
			}
			if !isLineCommentToken(tok.Value) {
				continue
			}
			startLine, endLine := tok.Line, tok.Line
			for i+1 < len(result.Tokens) {
				next := result.Tokens[i+1]
				if next.Kind != TokenComment || !isLineCommentToken(next.Value) || next.Line != endLine+1 {
					break
				}
				endLine = next.Line
				i++
			}
			appendFoldRange(&ranges, result.Lines, startLine, endLine, FoldingRangeKindComment)
		case TokenNamespace:
			if tokenSequenceContains(result.Tokens, i+1, TokenOpenBrace, TokenSemicolon) == TokenOpenBrace {
				pending = &pendingFold{startLine: tok.Line}
			} else {
				namespaceStarts = append(namespaceStarts, tok.Line)
			}
		case TokenClass:
			if prev := previousSignificantToken(result.Tokens, i); prev != nil && prev.Kind == TokenNew {
				continue
			}
			pending = &pendingFold{startLine: tok.Line}
		case TokenInterface, TokenTrait, TokenEnum:
			pending = &pendingFold{startLine: tok.Line}
		case TokenFunction:
			if !isNamedFunctionToken(result.Tokens, i) {
				continue
			}
			pending = &pendingFold{startLine: tok.Line}
		case TokenOpenBrace:
			entry := foldEntry{openKind: TokenOpenBrace, startLine: tok.Line, kind: FoldingRangeKindRegion}
			if pending != nil {
				entry.startLine = pending.startLine
				entry.kind = pending.kind
				pending = nil
			}
			stack = append(stack, entry)
		case TokenCloseBrace:
			entry, ok := popFoldEntry(&stack, TokenOpenBrace)
			if !ok {
				pending = nil
				continue
			}
			appendFoldRange(&ranges, result.Lines, entry.startLine, tok.Line, entry.kind)
			pending = nil
		case TokenOpenBracket:
			if prev := previousSignificantToken(result.Tokens, i); prev != nil && prev.Kind == TokenHash {
				stack = append(stack, foldEntry{openKind: TokenOpenBracket, startLine: tok.Line, kind: ""})
				continue
			}
			stack = append(stack, foldEntry{openKind: TokenOpenBracket, startLine: tok.Line, kind: FoldingRangeKindRegion})
		case TokenCloseBracket:
			entry, ok := popFoldEntry(&stack, TokenOpenBracket)
			if !ok {
				continue
			}
			if entry.kind == "" {
				continue
			}
			appendFoldRange(&ranges, result.Lines, entry.startLine, tok.Line, entry.kind)
		}
	}

	appendNamespaceFolds(&ranges, result.Lines, namespaceStarts)

	slices.SortFunc(ranges, func(a, b FoldingRange) int {
		if a.StartLine != b.StartLine {
			return a.StartLine - b.StartLine
		}
		if a.EndLine != b.EndLine {
			return a.EndLine - b.EndLine
		}
		return strings.Compare(string(a.Kind), string(b.Kind))
	})

	return compactFoldingRanges(ranges)
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

func popFoldEntry(stack *[]foldEntry, kind TokenKind) (foldEntry, bool) {
	for i := len(*stack) - 1; i >= 0; i-- {
		if (*stack)[i].openKind != kind {
			continue
		}
		entry := (*stack)[i]
		*stack = append((*stack)[:i], (*stack)[i+1:]...)
		return entry, true
	}
	return foldEntry{}, false
}

func previousSignificantToken(tokens []Token, index int) *Token {
	for i := index - 1; i >= 0; i-- {
		switch tokens[i].Kind {
		case TokenComment, TokenDocComment:
			continue
		default:
			return &tokens[i]
		}
	}
	return nil
}

func tokenSequenceContains(tokens []Token, start int, stopKinds ...TokenKind) TokenKind {
	for i := start; i < len(tokens); i++ {
		tok := tokens[i]
		for _, stop := range stopKinds {
			if tok.Kind == stop {
				return tok.Kind
			}
		}
	}
	return TokenUnknown
}

func isNamedFunctionToken(tokens []Token, index int) bool {
	for i := index + 1; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case TokenComment, TokenDocComment, TokenAmpersand:
			continue
		case TokenIdentifier:
			return true
		default:
			return isKeywordToken(tokens[i].Kind)
		}
	}
	return false
}

func isLineCommentToken(value string) bool {
	return strings.HasPrefix(value, "//") || (strings.HasPrefix(value, "#") && !strings.HasPrefix(value, "#["))
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
