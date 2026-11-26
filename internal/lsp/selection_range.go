package lsp

import (
	"cmp"
	"slices"
	"unicode/utf8"

	oldparser "github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (s *Server) handleSelectionRange(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.SelectionRangeParams](msg)
	if !ok {
		return
	}

	uri := params.TextDocument.URI
	source := s.getDocument(uri)
	p := s.positionsOf(uri, source)

	positions := make([]protocol.Position, len(params.Positions))
	for i, pos := range params.Positions {
		positions[i] = p.posIn(uri, pos)
	}
	chains := buildSelectionChains(source, positions)
	for i, chain := range chains {
		chains[i] = p.selectionRangeOut(uri, chain)
	}
	s.sendResponse(msg.ID, chains)
}

type lineSpan struct {
	start, end int
}

func buildSelectionChains(source string, positions []protocol.Position) []protocol.SelectionRange {
	lines := ast.NewLines(source)
	structures := make([][]ast.Span, len(positions))
	file := parser.ParseSeeingTokens(source, token.Latest, func(toks []lexer.Token) {
		for i, pos := range positions {
			structures[i] = append(make([]ast.Span, 0, 4), ast.Span{End: uint32(len(source))})
			if pos.Line >= len(lines) {
				continue
			}
			if bracket, ok := innermostBracket(toks, lineOffset(source, lines, pos)); ok {
				structures[i] = append(structures[i], bracket)
			}
		}
	})
	var classes, callables []lineSpan
	for _, d := range declarationsOf(source, file) {
		switch {
		case d.isClassLike():
			classes = append(classes, declarationLines(lines, d))
		case d.isCallable():
			callables = append(callables, declarationLines(lines, d))
		}
	}
	chains := make([]protocol.SelectionRange, len(positions))
	for i, pos := range positions {
		structure := structures[i]
		for _, spans := range [][]lineSpan{classes, callables} {
			if s, ok := innermostLines(spans, pos.Line); ok {
				structure = append(structure, ast.Span{Start: lines[s.start], End: lineEnd(source, lines, s.end)})
			}
		}
		chains[i] = buildSelectionChain(source, lines, pos, structure)
	}
	return chains
}

func declarationLines(lines ast.Lines, d declaration) lineSpan {
	start, _ := lines.Position(d.name.Start)
	end, _ := lines.Position(max(d.node.End, d.node.Start+1) - 1)
	return lineSpan{start: start, end: end}
}

func buildSelectionChain(source string, lines ast.Lines, pos protocol.Position, structure []ast.Span) protocol.SelectionRange {
	slices.SortStableFunc(structure, func(a, b ast.Span) int {
		return cmp.Or(cmp.Compare(a.Start, b.Start), cmp.Compare(b.End, a.End))
	})
	candidates := make([]protocol.Range, 0, len(structure)+1)
	for _, span := range structure {
		candidates = append(candidates, protocol.Range{Start: spanPosition(lines, span.Start), End: spanPosition(lines, span.End)})
	}
	if pos.Line < len(lines) {
		candidates = append(candidates, findWordRange(source, lines, pos))
	}

	nodes := make([]protocol.SelectionRange, 0, len(candidates))
	for _, r := range candidates {
		if len(nodes) == 0 || r != nodes[len(nodes)-1].Range {
			nodes = append(nodes, protocol.SelectionRange{Range: r})
		}
	}
	for i := 1; i < len(nodes); i++ {
		nodes[i].Parent = &nodes[i-1]
	}
	return nodes[len(nodes)-1]
}

func innermostLines(spans []lineSpan, line int) (lineSpan, bool) {
	var best lineSpan
	found := false
	for _, s := range spans {
		if s.start <= line && line <= s.end && (!found || s.end-s.start < best.end-best.start) {
			best, found = s, true
		}
	}
	return best, found
}

func findWordRange(source string, lines ast.Lines, pos protocol.Position) protocol.Range {
	line := source[lines[pos.Line]:lineEnd(source, lines, pos.Line)]
	ch := min(pos.Character, len(line))

	isWordChar := func(r rune) bool {
		return r == '$' || oldparser.IsIdentByte(string(r)[0])
	}

	start := ch
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(line[:start])
		if !isWordChar(r) {
			break
		}
		start -= size
	}
	end := ch
	for end < len(line) {
		r, size := utf8.DecodeRuneInString(line[end:])
		if !isWordChar(r) {
			break
		}
		end += size
	}

	if start == end {
		return protocol.Range{Start: pos, End: pos}
	}
	return protocol.Range{
		Start: protocol.Position{Line: pos.Line, Character: start},
		End:   protocol.Position{Line: pos.Line, Character: end},
	}
}

func innermostBracket(toks []lexer.Token, offset uint32) (ast.Span, bool) {
	opens := make([]lexer.Token, 0, 16)
	var best ast.Span
	found := false
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		if lexer.OpensLiteral(tok.Kind) {
			i = lexer.LiteralEnd(toks, i)
			continue
		}
		var opener token.Kind
		switch tok.Kind {
		case token.LeftBrace, token.LeftParen, token.LeftBracket:
			opens = append(opens, tok)
			continue
		case token.Attribute:
			tok.Kind, tok.Start = token.LeftBracket, tok.End-1
			opens = append(opens, tok)
			continue
		case token.RightBrace:
			opener = token.LeftBrace
		case token.RightParen:
			opener = token.LeftParen
		case token.RightBracket:
			opener = token.LeftBracket
		default:
			continue
		}
		at := -1
		for j := len(opens) - 1; j >= 0; j-- {
			if opens[j].Kind == opener {
				at = j
				break
			}
		}
		if at < 0 {
			continue
		}
		pair := ast.Span{Start: opens[at].Start, End: tok.End}
		opens = slices.Delete(opens, at, at+1)
		if pair.Start <= offset && offset <= pair.End && (!found || pair.End-pair.Start < best.End-best.Start) {
			best, found = pair, true
		}
	}
	return best, found
}

func lineEnd(source string, lines ast.Lines, line int) uint32 {
	if line+1 < len(lines) {
		return lines[line+1] - 1
	}
	return uint32(len(source))
}

func lineOffset(source string, lines ast.Lines, pos protocol.Position) uint32 {
	return min(lines[pos.Line]+uint32(pos.Character), lineEnd(source, lines, pos.Line))
}

func spanPosition(lines ast.Lines, offset uint32) protocol.Position {
	line, col := lines.Position(offset)
	return protocol.Position{Line: line, Character: col}
}
