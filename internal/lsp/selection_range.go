package lsp

import (
	"slices"
	"unicode/utf8"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
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
	pr := parser.New().Parse(source)
	p := s.positionsOf(uri, source)

	result := make([]protocol.SelectionRange, 0, len(params.Positions))
	for _, pos := range params.Positions {
		result = append(result, p.selectionRangeOut(uri, buildSelectionChain(pr, p.posIn(uri, pos), source)))
	}
	s.sendResponse(msg.ID, result)
}

// buildSelectionChain builds a nested SelectionRange chain for a given position.
// The returned SelectionRange is the innermost (tightest) range; each Parent
// field points to the next-larger enclosing range, up to the whole document.
func buildSelectionChain(pr *parser.ParseResult, pos protocol.Position, source string) protocol.SelectionRange {
	lastLine := 0
	lastChar := 0
	if len(pr.Lines) > 0 {
		lastLine = len(pr.Lines) - 1
		lastChar = len(pr.Lines[lastLine])
	}

	// Collect candidate ranges from outermost to innermost.
	// We'll build the parent chain by linking them in order.
	type candidate struct {
		r protocol.Range
	}

	var candidates []candidate

	// Outermost: whole document
	candidates = append(candidates, candidate{r: protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: lastLine, Character: lastChar},
	}})

	// Class/interface/trait/enum range
	if cr := findContainingClassRange(pr, pos); cr != nil {
		candidates = append(candidates, candidate{r: *cr})
	}

	// Method/function range
	if mr := findContainingMethodRange(pr, pos); mr != nil {
		candidates = append(candidates, candidate{r: *mr})
	}

	// Innermost balanced bracket region
	if br := findBracketRange(source, ast.NewLines(source), pos); br != nil {
		candidates = append(candidates, candidate{r: *br})
	}

	// Word range
	if wr := findWordRange(pr, pos); wr != nil {
		candidates = append(candidates, candidate{r: *wr})
	}

	// Deduplicate consecutive identical ranges (avoid cycles or no-ops)
	deduped := make([]candidate, 0, len(candidates))
	for i, c := range candidates {
		if i == 0 || !rangeEquals(c.r, deduped[len(deduped)-1].r) {
			deduped = append(deduped, c)
		}
	}

	if len(deduped) == 0 {
		// Fallback: return the whole-document range with no parent
		return protocol.SelectionRange{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: lastLine, Character: lastChar},
			},
		}
	}

	// Build chain: deduped[0] is outermost (no parent), deduped[n-1] is innermost.
	// Each node's Parent points to the next-outer node.
	// We allocate all nodes upfront to avoid pointer aliasing issues with the loop.
	nodes := make([]protocol.SelectionRange, len(deduped))
	for i, c := range deduped {
		nodes[i] = protocol.SelectionRange{Range: c.r}
	}
	// Link: nodes[i].Parent = &nodes[i-1] (outermost = index 0, no parent)
	for i := 1; i < len(nodes); i++ {
		nodes[i].Parent = &nodes[i-1]
	}

	return nodes[len(nodes)-1]
}

// rangeEquals returns true if two ranges are equal.
func rangeEquals(a, b protocol.Range) bool {
	return a.Start.Line == b.Start.Line && a.Start.Character == b.Start.Character &&
		a.End.Line == b.End.Line && a.End.Character == b.End.Character
}

// findWordRange returns the range of the word/identifier at the given position.
func findWordRange(pr *parser.ParseResult, pos protocol.Position) *protocol.Range {
	if pos.Line >= len(pr.Lines) {
		return nil
	}
	line := pr.Lines[pos.Line]
	ch := min(pos.Character, len(line))

	isWordChar := func(r rune) bool {
		return r == '$' || parser.IsIdentByte(string(r)[0])
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
		// No word at position, return a zero-length range
		return &protocol.Range{
			Start: protocol.Position{Line: pos.Line, Character: pos.Character},
			End:   protocol.Position{Line: pos.Line, Character: pos.Character},
		}
	}

	return &protocol.Range{
		Start: protocol.Position{Line: pos.Line, Character: start},
		End:   protocol.Position{Line: pos.Line, Character: end},
	}
}

func findBracketRange(source string, lines ast.Lines, pos protocol.Position) *protocol.Range {
	if pos.Line >= len(lines) {
		return nil
	}
	offset := lineOffset(source, lines, pos)
	toks := lexer.Lex(source, token.Latest)
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
	if !found {
		return nil
	}
	return &protocol.Range{Start: spanPosition(lines, best.Start), End: spanPosition(lines, best.End)}
}

func lineOffset(source string, lines ast.Lines, pos protocol.Position) uint32 {
	end := uint32(len(source))
	if pos.Line+1 < len(lines) {
		end = lines[pos.Line+1] - 1
	}
	return min(lines[pos.Line]+uint32(pos.Character), end)
}

func spanPosition(lines ast.Lines, offset uint32) protocol.Position {
	line, col := lines.Position(offset)
	return protocol.Position{Line: line, Character: col}
}

// findContainingMethodRange returns the range of the method or function body
// that contains the given position.
func findContainingMethodRange(pr *parser.ParseResult, pos protocol.Position) *protocol.Range {
	type candidate struct {
		startLine, endLine int
	}
	var best *candidate

	checkMethod := func(m parser.MethodDef) {
		if m.Line <= pos.Line && m.EndLine >= pos.Line {
			if best == nil || (m.EndLine-m.Line) < (best.endLine-best.startLine) {
				best = &candidate{startLine: m.Line, endLine: m.EndLine}
			}
		}
	}

	checkFunc := func(f parser.FunctionDef) {
		if f.Line <= pos.Line && f.EndLine >= pos.Line {
			if best == nil || (f.EndLine-f.Line) < (best.endLine-best.startLine) {
				best = &candidate{startLine: f.Line, endLine: f.EndLine}
			}
		}
	}

	for _, cls := range pr.Classes {
		for _, m := range cls.Methods {
			checkMethod(m)
		}
	}
	for _, iface := range pr.Interfaces {
		for _, m := range iface.Methods {
			checkMethod(m)
		}
	}
	for _, trait := range pr.Traits {
		for _, m := range trait.Methods {
			checkMethod(m)
		}
	}
	for _, e := range pr.Enums {
		for _, m := range e.Methods {
			checkMethod(m)
		}
	}
	for _, fn := range pr.Functions {
		checkFunc(fn)
	}

	if best == nil {
		return nil
	}

	endChar := 0
	if best.endLine < len(pr.Lines) {
		endChar = len(pr.Lines[best.endLine])
	}

	return &protocol.Range{
		Start: protocol.Position{Line: best.startLine, Character: 0},
		End:   protocol.Position{Line: best.endLine, Character: endChar},
	}
}

// findContainingClassRange returns the range of the class/interface/trait/enum
// that contains the given position.
func findContainingClassRange(pr *parser.ParseResult, pos protocol.Position) *protocol.Range {
	type candidate struct {
		startLine, endLine int
	}
	var best *candidate

	check := func(startLine, endLine int) {
		if startLine <= pos.Line && endLine >= pos.Line {
			if best == nil || (endLine-startLine) < (best.endLine-best.startLine) {
				best = &candidate{startLine: startLine, endLine: endLine}
			}
		}
	}

	for _, cls := range pr.Classes {
		check(cls.Line, cls.EndLine)
	}
	for _, iface := range pr.Interfaces {
		check(iface.Line, iface.EndLine)
	}
	for _, trait := range pr.Traits {
		check(trait.Line, trait.EndLine)
	}
	for _, e := range pr.Enums {
		check(e.Line, e.EndLine)
	}

	if best == nil {
		return nil
	}

	endChar := 0
	if best.endLine < len(pr.Lines) {
		endChar = len(pr.Lines[best.endLine])
	}

	return &protocol.Range{
		Start: protocol.Position{Line: best.startLine, Character: 0},
		End:   protocol.Position{Line: best.endLine, Character: endChar},
	}
}
