package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

func (a *Analyzer) extractVariableCodeAction(uri string, toks codeTokens, params protocol.CodeActionParams) *protocol.CodeAction {
	if !protocol.CodeActionRefactorExtract.In(params.Context.Only) {
		return nil
	}
	if params.Range.Start.Line != params.Range.End.Line {
		return nil
	}
	if comparePosition(params.Range.Start, params.Range.End) >= 0 {
		return nil
	}

	doc := scope.Collect(toks.src)
	targetScope := doc.ScopeAt(params.Range.Start)
	if targetScope == nil || targetScope.Kind == scope.ScopeFile || targetScope.ExprBody {
		return nil
	}
	if endScope := doc.ScopeAt(params.Range.End); endScope != targetScope {
		return nil
	}

	selection, ok := newExtractSelection(toks, params.Range)
	if !ok {
		return nil
	}

	name := nextExtractVariableName(targetScope)
	statementLine := selection.statementStart.Line
	indent := leadingIndent(toks.line(statementLine))
	insert := fmt.Sprintf("%s%s = %s;\n", indent, name, selection.text)

	action := &protocol.CodeAction{
		Title: "Extract variable",
		Kind:  protocol.CodeActionRefactorExtract,
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

func newExtractSelection(toks codeTokens, rng protocol.Range) (extractSelection, bool) {
	startOffset, ok := toks.offset(rng.Start)
	if !ok {
		return extractSelection{}, false
	}
	endOffset, ok := toks.offset(rng.End)
	if !ok || endOffset <= startOffset {
		return extractSelection{}, false
	}

	selected := toks.src[startOffset:endOffset]
	if strings.TrimSpace(selected) == "" {
		return extractSelection{}, false
	}

	first, last := selectionTokenBounds(toks, startOffset, endOffset)
	if first < 0 || last < first {
		return extractSelection{}, false
	}

	if trimStart := strings.IndexFunc(selected, func(r rune) bool { return r != ' ' && r != '\t' }); trimStart >= 0 {
		if startOffset+trimStart != int(toks.all[first].Start) {
			return extractSelection{}, false
		}
	}
	if startOffset+len(strings.TrimRight(selected, " \t")) != int(toks.all[last].End) {
		return extractSelection{}, false
	}

	if !extractSelectionIsSafe(toks, first, last) {
		return extractSelection{}, false
	}

	return extractSelection{
		rng:            rng,
		text:           selected,
		statementStart: toks.position(toks.all[lexer.StatementStart(toks.all, first)].Start),
	}, true
}

func selectionTokenBounds(toks codeTokens, start, end int) (int, int) {
	first := sort.Search(len(toks.all), func(i int) bool { return int(toks.all[i].End) > start })
	last := first - 1
	for last+1 < len(toks.all) && int(toks.all[last+1].Start) < end {
		last++
	}
	if last < first {
		return -1, -1
	}
	return first, last
}

func extractSelectionIsSafe(toks codeTokens, first, last int) bool {
	depthParen := 0
	depthBracket := 0
	for i := first; i <= last; i++ {
		switch kind := toks.all[i].Kind; {
		case kind == token.LeftParen:
			depthParen++
		case kind == token.RightParen:
			depthParen--
		case kind == token.LeftBracket || kind == token.Attribute:
			depthBracket++
		case kind == token.RightBracket:
			depthBracket--
		case endsStatement(kind):
			return false
		case kind.IsAssignment() || kind == token.Comma:
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

func endsStatement(kind token.Kind) bool {
	return kind == token.Semicolon || kind == token.CloseTag || kind == token.LeftBrace || kind == token.RightBrace
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
