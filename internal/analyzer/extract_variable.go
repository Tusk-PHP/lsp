package analyzer

import (
	"fmt"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/scope"
)

func (a *Analyzer) extractVariableCodeAction(uri, source string, params protocol.CodeActionParams) *protocol.CodeAction {
	if !importCodeActionKindAllowed(params.Context.Only, "refactor.extract") {
		return nil
	}
	if params.Range.Start.Line != params.Range.End.Line {
		return nil
	}
	if comparePosition(params.Range.Start, params.Range.End) >= 0 {
		return nil
	}

	doc := scope.Collect(source)
	targetScope := doc.ScopeAt(params.Range.Start)
	if targetScope == nil || targetScope.Kind == scope.ScopeFile || targetScope.ExprBody {
		return nil
	}
	if endScope := doc.ScopeAt(params.Range.End); endScope != targetScope {
		return nil
	}

	result := parser.New().Parse(source)
	if result == nil {
		return nil
	}

	selection, ok := newExtractSelection(source, result, params.Range)
	if !ok {
		return nil
	}

	name := nextExtractVariableName(targetScope)
	statementLine := selection.statementStart.Line
	indent := leadingIndent(result.Lines[statementLine])
	insert := fmt.Sprintf("%s%s = %s;\n", indent, name, selection.text)

	action := &protocol.CodeAction{
		Title: "Extract variable",
		Kind:  "refactor.extract",
		Edit: &protocol.WorkspaceEdit{
			Changes: map[string][]protocol.TextEdit{
				uri: {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: statementLine, Character: 0},
							End:   protocol.Position{Line: statementLine, Character: 0},
						},
						NewText: insert,
					},
					{
						Range:   selection.rng,
						NewText: name,
					},
				},
			},
		},
	}

	return action
}

type extractSelection struct {
	rng            protocol.Range
	text           string
	statementStart protocol.Position
}

func newExtractSelection(source string, result *parser.ParseResult, rng protocol.Range) (extractSelection, bool) {
	startOffset, ok := positionToOffset(source, rng.Start)
	if !ok {
		return extractSelection{}, false
	}
	endOffset, ok := positionToOffset(source, rng.End)
	if !ok || endOffset <= startOffset {
		return extractSelection{}, false
	}

	selected := source[startOffset:endOffset]
	if strings.TrimSpace(selected) == "" {
		return extractSelection{}, false
	}

	first, last := selectionTokenBounds(result.Tokens, rng)
	if first < 0 || last < first {
		return extractSelection{}, false
	}

	firstRange := tokenRange(result.Tokens[first])
	lastRange := tokenRange(result.Tokens[last])
	if trimStart := strings.IndexFunc(selected, func(r rune) bool { return r != ' ' && r != '\t' }); trimStart >= 0 {
		if startOffset+trimStart != offsetAtPosition(source, firstRange.Start) {
			return extractSelection{}, false
		}
	}
	trimmedEndOffset := startOffset + len(strings.TrimRight(selected, " \t"))
	if trimmedEndOffset != offsetAtPosition(source, lastRange.End) {
		return extractSelection{}, false
	}

	if !extractSelectionIsSafe(result.Tokens, first, last) {
		return extractSelection{}, false
	}

	statementStart := findStatementStart(result.Tokens, first)
	if statementStart < 0 {
		return extractSelection{}, false
	}

	return extractSelection{
		rng:            rng,
		text:           selected,
		statementStart: tokenRange(result.Tokens[statementStart]).Start,
	}, true
}

func selectionTokenBounds(tokens []parser.Token, rng protocol.Range) (int, int) {
	first := -1
	last := -1
	for i, tok := range tokens {
		if isTriviaToken(tok.Kind) {
			continue
		}
		tokRange := tokenRange(tok)
		if comparePosition(tokRange.End, rng.Start) <= 0 {
			continue
		}
		if comparePosition(tokRange.Start, rng.End) >= 0 {
			break
		}
		if first == -1 {
			first = i
		}
		last = i
	}
	return first, last
}

func extractSelectionIsSafe(tokens []parser.Token, first, last int) bool {
	depthParen := 0
	depthBracket := 0
	for i := first; i <= last; i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen:
			depthParen++
		case parser.TokenCloseParen:
			depthParen--
		case parser.TokenOpenBracket:
			depthBracket++
		case parser.TokenCloseBracket:
			depthBracket--
		case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
			return false
		case parser.TokenEquals:
			if depthParen == 0 && depthBracket == 0 {
				return false
			}
		case parser.TokenComma:
			if depthParen == 0 && depthBracket == 0 {
				return false
			}
		}
		if depthParen < 0 || depthBracket < 0 {
			return false
		}
	}
	return depthParen == 0 && depthBracket == 0
}

func findStatementStart(tokens []parser.Token, first int) int {
	depthParen := 0
	depthBracket := 0
	for i := first - 1; i >= 0; i-- {
		tok := tokens[i]
		if isTriviaToken(tok.Kind) {
			continue
		}
		switch tok.Kind {
		case parser.TokenCloseParen:
			depthParen++
		case parser.TokenOpenParen:
			if depthParen > 0 {
				depthParen--
				continue
			}
		case parser.TokenCloseBracket:
			depthBracket++
		case parser.TokenOpenBracket:
			if depthBracket > 0 {
				depthBracket--
				continue
			}
		}
		if depthParen == 0 && depthBracket == 0 {
			switch tok.Kind {
			case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
				return nextSignificantToken(tokens, i+1)
			}
		}
	}
	return nextSignificantToken(tokens, 0)
}

func nextExtractVariableName(targetScope *scope.Scope) string {
	base := "$extracted"
	for idx := 0; idx < 1000; idx++ {
		name := base
		if idx > 0 {
			name = fmt.Sprintf("%s%d", base, idx)
		}
		if !scopeHasVisibleBinding(targetScope, name) {
			return name
		}
	}
	return "$extractedValue"
}

func scopeHasVisibleBinding(targetScope *scope.Scope, name string) bool {
	for current := targetScope; current != nil; current = current.Parent {
		for _, binding := range current.Bindings {
			if binding.Name == name {
				return true
			}
		}
	}
	return false
}

func nextSignificantToken(tokens []parser.Token, start int) int {
	for i := start; i < len(tokens); i++ {
		if !isTriviaToken(tokens[i].Kind) {
			return i
		}
	}
	return -1
}

func isTriviaToken(kind parser.TokenKind) bool {
	return kind == parser.TokenWhitespace || kind == parser.TokenComment || kind == parser.TokenDocComment
}

func positionToOffset(source string, pos protocol.Position) (int, bool) {
	if pos.Line < 0 || pos.Character < 0 {
		return 0, false
	}
	line := 0
	col := 0
	for idx, r := range source {
		if line == pos.Line && col == pos.Character {
			return idx, true
		}
		if r == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	if line == pos.Line && col == pos.Character {
		return len(source), true
	}
	return 0, false
}

func offsetAtPosition(source string, pos protocol.Position) int {
	offset, _ := positionToOffset(source, pos)
	return offset
}

func tokenRange(tok parser.Token) protocol.Range {
	end := tok.Column + len(tok.Value)
	return protocol.Range{
		Start: protocol.Position{Line: tok.Line, Character: tok.Column},
		End:   protocol.Position{Line: tok.Line, Character: end},
	}
}

func comparePosition(a, b protocol.Position) int {
	if a.Line != b.Line {
		if a.Line < b.Line {
			return -1
		}
		return 1
	}
	if a.Character < b.Character {
		return -1
	}
	if a.Character > b.Character {
		return 1
	}
	return 0
}

func leadingIndent(line string) string {
	end := 0
	for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
		end++
	}
	return line[:end]
}
