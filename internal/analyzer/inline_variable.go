package analyzer

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

func (a *Analyzer) inlineVariableCodeAction(uri, source string, params protocol.CodeActionParams) *protocol.CodeAction {
	if !protocol.CodeActionRefactorInline.In(params.Context.Only) {
		return nil
	}

	result := parser.New().Parse(source)
	if result == nil {
		return nil
	}

	doc := scope.Collect(source)
	binding := doc.BindingAt(params.Range.Start)
	plan, ok := newInlineVariablePlan(source, result, doc, binding)
	if !ok {
		return nil
	}

	edits := make([]protocol.TextEdit, 0, len(plan.useRanges)+1)
	for _, rng := range plan.useRanges {
		edits = append(edits, protocol.TextEdit{Range: rng, NewText: plan.replacement})
	}
	edits = append(edits, protocol.TextEdit{Range: plan.deleteRange, NewText: ""})
	sort.SliceStable(edits, func(i, j int) bool {
		return comparePosition(edits[i].Range.Start, edits[j].Range.Start) > 0
	})

	return &protocol.CodeAction{
		Title: "Inline variable",
		Kind:  protocol.CodeActionRefactorInline,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[string][]protocol.TextEdit{
				uri: edits,
			},
		},
	}
}

type inlineVariablePlan struct {
	replacement string
	deleteRange protocol.Range
	useRanges   []protocol.Range
}

func newInlineVariablePlan(source string, result *parser.ParseResult, doc *scope.Document, binding *scope.Binding) (inlineVariablePlan, bool) {
	if binding == nil || binding.Kind != scope.BindingVariable || binding.Scope == nil {
		return inlineVariablePlan{}, false
	}
	if binding.Scope.Kind == scope.ScopeFile || scopeAccessesVariablesByName(result.Tokens, doc, binding.Scope) {
		return inlineVariablePlan{}, false
	}
	if len(binding.Assignments) != 1 {
		return inlineVariablePlan{}, false
	}

	assignment := binding.Assignments[0]
	if !assignment.DirectStatement || assignment.RelativeBraceDepth != 0 {
		return inlineVariablePlan{}, false
	}
	if comparePosition(binding.Decl.Start, assignment.Variable.Start) != 0 {
		return inlineVariablePlan{}, false
	}

	for _, candidate := range doc.Bindings {
		if candidate == binding || candidate.Origin != binding {
			continue
		}
		return inlineVariablePlan{}, false
	}

	if len(binding.References) == 0 {
		return inlineVariablePlan{}, false
	}
	for _, ref := range binding.References {
		if comparePosition(ref.Start, assignment.Statement.End) <= 0 {
			return inlineVariablePlan{}, false
		}
		if inlineReferenceIsMutation(result.Tokens, ref) {
			return inlineVariablePlan{}, false
		}
	}

	first, last := selectionTokenBounds(result.Tokens, assignment.Expression)
	if first < 0 || last < first {
		return inlineVariablePlan{}, false
	}
	if !inlineExpressionIsSafe(result.Tokens, first, last) {
		return inlineVariablePlan{}, false
	}

	exprStart, ok := positionToOffset(source, assignment.Expression.Start)
	if !ok {
		return inlineVariablePlan{}, false
	}
	exprEnd, ok := positionToOffset(source, assignment.Expression.End)
	if !ok || exprEnd <= exprStart {
		return inlineVariablePlan{}, false
	}

	exprText := source[exprStart:exprEnd]
	replacement := exprText
	if !inlineExpressionIsAtomic(result.Tokens, first, last) {
		replacement = "(" + exprText + ")"
	}

	return inlineVariablePlan{
		replacement: replacement,
		deleteRange: expandInlineDeleteRange(source, assignment.Statement),
		useRanges:   append([]protocol.Range(nil), binding.References...),
	}, true
}

var nameScopeFunctions = map[string]bool{"compact": true, "extract": true, "get_defined_vars": true, "eval": true}

var includeConstructs = map[string]bool{"include": true, "include_once": true, "require": true, "require_once": true}

func scopeAccessesVariablesByName(tokens []parser.Token, doc *scope.Document, owner *scope.Scope) bool {
	for i, tok := range tokens {
		if accessesVariablesByName(tokens, i) && variableScopeAt(doc, tokenRange(tok).Start) == owner {
			return true
		}
	}
	return false
}

func variableScopeAt(doc *scope.Document, pos protocol.Position) *scope.Scope {
	s := doc.ScopeAt(pos)
	for s != nil && s.ExprBody {
		s = s.Parent
	}
	return s
}

func accessesVariablesByName(tokens []parser.Token, i int) bool {
	tok := tokens[i]
	prev := parser.PrevSignificant(tokens, i-1)
	switch tok.Kind {
	case parser.TokenUnknown:
		return tok.Value == "$" && (prev < 0 || tokens[prev].Kind != parser.TokenDoubleColon)
	case parser.TokenIdentifier:
		name := strings.ToLower(tok.Value)
		if includeConstructs[name] {
			return isIncludeConstruct(tokens, prev, i)
		}
		if !nameScopeFunctions[name] && name != "parse_str" {
			return false
		}
		open := parser.NextSignificant(tokens, i+1)
		if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
			return false
		}
		if prev >= 0 {
			switch tokens[prev].Kind {
			case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenFunction, parser.TokenNew:
				return false
			}
		}
		return name != "parse_str" || callArgumentCount(tokens, open) < 2
	}
	return false
}

func isIncludeConstruct(tokens []parser.Token, prev, i int) bool {
	if prev >= 0 {
		switch tokens[prev].Kind {
		case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenConst:
			return false
		}
	}
	next := parser.NextSignificant(tokens, i+1)
	return next < 0 || tokens[next].Kind != parser.TokenColon
}

func callArgumentCount(tokens []parser.Token, open int) int {
	end := parser.MatchingClose(tokens, open)
	if end < 0 {
		return 0
	}
	count, depth, argumentStart := 0, 0, true
	for i := open + 1; i < end; i++ {
		tok := tokens[i]
		if tok.Kind.IsComment() {
			continue
		}
		switch tok.Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
		case parser.TokenComma:
			if depth == 0 {
				argumentStart = true
				continue
			}
		}
		if !argumentStart {
			continue
		}
		if tok.Kind == parser.TokenUnknown && tok.Value == "." {
			return 0
		}
		argumentStart = false
		count++
	}
	return count
}

func inlineExpressionIsSafe(tokens []parser.Token, first, last int) bool {
	for i := first; i <= last; i++ {
		tok := tokens[i]
		if tok.Kind.IsComment() {
			continue
		}
		switch tok.Kind {
		case parser.TokenEquals, parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace,
			parser.TokenArrow, parser.TokenDoubleColon, parser.TokenNew, parser.TokenAt:
			return false
		case parser.TokenUnknown:
			if !inlineUnknownTokenAllowed(tok.Value) {
				return false
			}
		case parser.TokenIdentifier:
			if inlineIdentifierHasSideEffects(tok.Value) {
				return false
			}
			if next := parser.NextSignificant(tokens, i+1); next >= 0 && tokens[next].Kind == parser.TokenOpenParen {
				return false
			}
		case parser.TokenVariable:
			if next := parser.NextSignificant(tokens, i+1); next >= 0 && tokens[next].Kind == parser.TokenOpenParen {
				return false
			}
		}
	}
	return true
}

func inlineReferenceIsMutation(tokens []parser.Token, ref protocol.Range) bool {
	for i, tok := range tokens {
		if tok.Kind != parser.TokenVariable || tokenRange(tok) != ref {
			continue
		}
		if prev := parser.PrevSignificant(tokens, i-1); prev >= 0 &&
			tokens[prev].Kind == parser.TokenUnknown &&
			(strings.Contains(tokens[prev].Value, "++") || strings.Contains(tokens[prev].Value, "--")) {
			return true
		}
		next := parser.NextSignificant(tokens, i+1)
		if next < 0 {
			return false
		}
		switch tokens[next].Kind {
		case parser.TokenEquals:
			return true
		case parser.TokenUnknown:
			return strings.Contains(tokens[next].Value, "++") ||
				strings.Contains(tokens[next].Value, "--") ||
				(parser.NextSignificant(tokens, next+1) >= 0 &&
					tokens[parser.NextSignificant(tokens, next+1)].Kind == parser.TokenEquals)
		}
		return false
	}
	return true
}

func inlineExpressionIsAtomic(tokens []parser.Token, first, last int) bool {
	first = parser.NextSignificant(tokens, first)
	last = parser.PrevSignificant(tokens, last)
	if first < 0 || last < first {
		return false
	}
	if tokens[first].Kind == parser.TokenOpenParen && tokens[last].Kind == parser.TokenCloseParen && parser.MatchingClose(tokens, first) == last {
		return true
	}
	return first == last && (tokens[first].Kind == parser.TokenVariable || tokens[first].Kind == parser.TokenStringLiteral || tokens[first].Kind == parser.TokenNumber)
}

func inlineUnknownTokenAllowed(value string) bool {
	return strings.ContainsRune("+-*/.%!<>~^", rune(value[0]))
}

func inlineIdentifierHasSideEffects(value string) bool {
	switch strings.ToLower(value) {
	case "clone", "echo", "eval", "exit", "include", "include_once", "print", "require", "require_once", "throw", "yield", "yieldfrom":
		return true
	default:
		return false
	}
}

func expandInlineDeleteRange(source string, stmt protocol.Range) protocol.Range {
	start := protocol.Position{Line: stmt.Start.Line, Character: 0}
	end := stmt.End

	if stmt.Start.Line != stmt.End.Line {
		return protocol.Range{Start: start, End: end}
	}

	lines := strings.Split(source, "\n")
	if stmt.Start.Line < 0 || stmt.Start.Line >= len(lines) {
		return stmt
	}
	line := lines[stmt.Start.Line]
	if strings.TrimSpace(line[:stmt.Start.Character]) != "" {
		return stmt
	}
	if strings.TrimSpace(line[stmt.End.Character:]) != "" {
		return stmt
	}
	if stmt.Start.Line+1 < len(lines) {
		end = protocol.Position{Line: stmt.Start.Line + 1, Character: 0}
	}
	return protocol.Range{Start: start, End: end}
}
