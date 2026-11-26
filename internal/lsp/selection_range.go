package lsp

import (
	"cmp"
	"slices"
	"sort"
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

type selectionCursors struct {
	offsets []uint32
	spans   [][]ast.Span
}

func (c selectionCursors) attach(span ast.Span) {
	k, _ := slices.BinarySearch(c.offsets, span.Start)
	for ; k < len(c.offsets) && c.offsets[k] <= span.End; k++ {
		c.spans[k] = append(c.spans[k], span)
	}
}

func buildSelectionChains(source string, positions []protocol.Position) []protocol.SelectionRange {
	lines := ast.NewLines(source)
	offsets := make([]uint32, len(positions))
	order := make([]int, 0, len(positions))
	for i, pos := range positions {
		if pos.Line < len(lines) {
			offsets[i] = lineOffset(source, lines, pos)
			order = append(order, i)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(offsets[a], offsets[b]) })
	cursors := selectionCursors{offsets: make([]uint32, len(order)), spans: make([][]ast.Span, len(order))}
	for k, i := range order {
		cursors.offsets[k] = offsets[i]
	}
	file := parser.ParseSeeingTokens(source, token.Latest, func(toks []lexer.Token) {
		for k, off := range cursors.offsets {
			cursors.spans[k] = cursorSpans(source, toks, off)
		}
		attachLexicalSpans(source, toks, cursors)
	})

	whole := protocol.SelectionRange{Range: spanRange(lines, file.Span)}
	chains := make([]protocol.SelectionRange, len(positions))
	for i := range chains {
		chains[i] = whole
	}
	for k, i := range order {
		spans := appendSyntaxSpans(cursors.spans[k], source, lines, file)
		chains[i] = nestedChain(lines, spans)
	}
	return chains
}

func cursorSpans(source string, toks []lexer.Token, off uint32) []ast.Span {
	at := sort.Search(len(toks), func(i int) bool { return toks[i].End > off })
	for _, i := range [2]int{at, at - 1} {
		if i < 0 || i >= len(toks) || off > toks[i].End {
			continue
		}
		if word := wordAround(source, toks[i], off); word.Start < word.End {
			return append(make([]ast.Span, 0, 16), word, tokenSpan(toks[i]))
		}
	}
	spans := append(make([]ast.Span, 0, 16), ast.Span{Start: off, End: off})
	if at < len(toks) && toks[at].Kind != token.Whitespace {
		spans = append(spans, tokenSpan(toks[at]))
	}
	return spans
}

func wordAround(source string, tok lexer.Token, off uint32) ast.Span {
	isWordChar := func(r rune) bool {
		return r == '$' || oldparser.IsIdentByte(string(r)[0])
	}
	start := off
	for start > tok.Start {
		r, size := utf8.DecodeLastRuneInString(source[tok.Start:start])
		if !isWordChar(r) {
			break
		}
		start -= uint32(size)
	}
	end := off
	for end < tok.End {
		r, size := utf8.DecodeRuneInString(source[end:tok.End])
		if !isWordChar(r) {
			break
		}
		end += uint32(size)
	}
	return ast.Span{Start: start, End: end}
}

func attachLexicalSpans(source string, toks []lexer.Token, cursors selectionCursors) {
	opens := make([]lexer.Token, 0, 16)
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		if tok.Kind == token.ConstantString {
			cursors.attach(quotedContents(source, tok))
			continue
		}
		if lexer.OpensLiteral(tok.Kind) {
			end := lexer.LiteralEnd(toks, i)
			if closer := toks[end]; end > i && (closer.Kind == tok.Kind || closer.Kind == token.EndHeredoc) {
				cursors.attach(ast.Span{Start: tok.End, End: closer.Start})
			}
			i = end
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
		cursors.attach(ast.Span{Start: opens[at].Start, End: tok.End})
		cursors.attach(ast.Span{Start: opens[at].End, End: tok.Start})
		opens = slices.Delete(opens, at, at+1)
	}
}

func quotedContents(source string, tok lexer.Token) ast.Span {
	start := tok.Start + 1
	if source[tok.Start] == 'b' || source[tok.Start] == 'B' {
		start++
	}
	return ast.Span{Start: start, End: tok.End - 1}
}

func appendSyntaxSpans(spans []ast.Span, source string, lines ast.Lines, file *ast.File) []ast.Span {
	inner := spans[0]
	ast.Walk(file, func(n ast.Node) bool {
		span := n.Bounds()
		if !spanContains(span, inner) {
			return false
		}
		spans = append(spans, span)
		switch n.(type) {
		case *ast.ClassDecl, *ast.InterfaceDecl, *ast.TraitDecl, *ast.EnumDecl, *ast.Method, *ast.FunctionDecl:
			spans = append(spans, wholeLines(source, lines, span))
		}
		return true
	})
	return spans
}

func nestedChain(lines ast.Lines, spans []ast.Span) protocol.SelectionRange {
	outer := spans[1:]
	slices.SortFunc(outer, func(a, b ast.Span) int {
		return cmp.Or(cmp.Compare(a.End-a.Start, b.End-b.Start), cmp.Compare(a.Start, b.Start))
	})
	chain := spans[:1]
	for _, s := range outer {
		if last := chain[len(chain)-1]; s != last && spanContains(s, last) {
			chain = append(chain, s)
		}
	}
	nodes := make([]protocol.SelectionRange, len(chain))
	for i := range nodes {
		nodes[i].Range = spanRange(lines, chain[len(chain)-1-i])
		if i > 0 {
			nodes[i].Parent = &nodes[i-1]
		}
	}
	return nodes[len(nodes)-1]
}

func spanContains(outer, inner ast.Span) bool {
	return outer.Start <= inner.Start && inner.End <= outer.End
}

func tokenSpan(tok lexer.Token) ast.Span {
	return ast.Span{Start: tok.Start, End: tok.End}
}

func wholeLines(source string, lines ast.Lines, span ast.Span) ast.Span {
	first, _ := lines.Position(span.Start)
	last, _ := lines.Position(max(span.End, span.Start+1) - 1)
	return ast.Span{Start: lines[first], End: lineEnd(source, lines, last)}
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

func spanRange(lines ast.Lines, span ast.Span) protocol.Range {
	return protocol.Range{Start: spanPosition(lines, span.Start), End: spanPosition(lines, span.End)}
}

func spanPosition(lines ast.Lines, offset uint32) protocol.Position {
	line, col := lines.Position(offset)
	return protocol.Position{Line: line, Character: col}
}
