package lsp

import (
	"encoding/json"
	"unicode"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (s *Server) handleSelectionRange(msg *jsonRPCMessage) {
	var params protocol.SelectionRangeParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}

	source := s.getDocument(params.TextDocument.URI)
	pr := parser.New().Parse(source)

	result := make([]protocol.SelectionRange, 0, len(params.Positions))
	for _, pos := range params.Positions {
		result = append(result, buildSelectionChain(pr, pos, source))
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
	if br := findBracketRange(pr, pos); br != nil {
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
	runes := []rune(line)
	ch := pos.Character
	if ch > len(runes) {
		ch = len(runes)
	}

	isWordChar := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
	}

	// Find start of word
	start := ch
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}
	// Find end of word
	end := ch
	for end < len(runes) && isWordChar(runes[end]) {
		end++
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

// findBracketRange finds the innermost balanced bracket pair ({}, (), []) that
// contains the given position.
func findBracketRange(pr *parser.ParseResult, pos protocol.Position) *protocol.Range {
	// Walk tokens to find bracket pairs that contain the position
	type bracket struct {
		open  parser.Token
		close parser.Token
	}

	// We need to track open brackets before position and find their closes after.
	type openBracket struct {
		tok   parser.Token
		depth int
	}

	posLine := pos.Line
	posChar := pos.Character

	// Find the innermost open bracket before or at position
	// and its matching close bracket after position.
	type candidatePair struct {
		startLine, startChar int
		endLine, endChar     int
		size                 int // (endLine-startLine)*big + endChar - startChar
	}

	var best *candidatePair

	// Simple bracket matching: find all matching pairs and pick the smallest
	// that contains the position.
	openStack := make([]openBracket, 0, 16)
	depth := 0

	for _, tok := range pr.Tokens {
		switch tok.Kind {
		case parser.TokenOpenBrace, parser.TokenOpenParen, parser.TokenOpenBracket:
			depth++
			openStack = append(openStack, openBracket{tok: tok, depth: depth})
		case parser.TokenCloseBrace, parser.TokenCloseParen, parser.TokenCloseBracket:
			if len(openStack) == 0 {
				continue
			}
			// Find matching open bracket
			matchIdx := -1
			for i := len(openStack) - 1; i >= 0; i-- {
				open := openStack[i]
				if (tok.Kind == parser.TokenCloseBrace && open.tok.Kind == parser.TokenOpenBrace) ||
					(tok.Kind == parser.TokenCloseParen && open.tok.Kind == parser.TokenOpenParen) ||
					(tok.Kind == parser.TokenCloseBracket && open.tok.Kind == parser.TokenOpenBracket) {
					matchIdx = i
					break
				}
			}
			if matchIdx < 0 {
				depth--
				continue
			}
			open := openStack[matchIdx]
			openStack = append(openStack[:matchIdx], openStack[matchIdx+1:]...)
			depth--

			// Check if this bracket pair contains the position
			startLine := open.tok.Line
			startChar := open.tok.Column
			endLine := tok.Line
			endChar := tok.Column + len(tok.Value)

			// Position must be strictly inside
			startBefore := startLine < posLine || (startLine == posLine && startChar <= posChar)
			endAfter := endLine > posLine || (endLine == posLine && endChar >= posChar)

			if !startBefore || !endAfter {
				continue
			}

			size := (endLine-startLine)*10000000 + (endChar - startChar)
			if best == nil || size < best.size {
				best = &candidatePair{
					startLine: startLine,
					startChar: startChar,
					endLine:   endLine,
					endChar:   endChar,
					size:      size,
				}
			}
		}
	}

	if best == nil {
		return nil
	}

	return &protocol.Range{
		Start: protocol.Position{Line: best.startLine, Character: best.startChar},
		End:   protocol.Position{Line: best.endLine, Character: best.endChar},
	}
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
