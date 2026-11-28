package analyzer

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

func (a *Analyzer) inlineVariableCodeAction(uri string, toks codeTokens, params protocol.CodeActionParams) *protocol.CodeAction {
	if !protocol.CodeActionRefactorInline.In(params.Context.Only) {
		return nil
	}

	doc := scope.Collect(toks.src)
	binding := doc.BindingAt(params.Range.Start)
	plan, ok := newInlineVariablePlan(toks, doc, binding)
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

func newInlineVariablePlan(toks codeTokens, doc *scope.Document, binding *scope.Binding) (inlineVariablePlan, bool) {
	if binding == nil || binding.Kind != scope.BindingVariable || binding.Scope == nil {
		return inlineVariablePlan{}, false
	}
	if binding.Scope.Kind == scope.ScopeFile {
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
		if inlineReferenceIsMutation(toks, ref) {
			return inlineVariablePlan{}, false
		}
	}

	exprStart, _ := toks.offset(assignment.Expression.Start)
	exprEnd, _ := toks.offset(assignment.Expression.End)
	first, last := selectionTokenBounds(toks, exprStart, exprEnd)
	if first < 0 || !inlineExpressionIsSafe(toks, first, last) {
		return inlineVariablePlan{}, false
	}

	exprText := toks.src[exprStart:exprEnd]
	replacement := exprText
	if !inlineExpressionIsAtomic(toks, first, last) {
		replacement = "(" + exprText + ")"
	}

	return inlineVariablePlan{
		replacement: replacement,
		deleteRange: expandInlineDeleteRange(toks, assignment.Statement),
		useRanges:   append([]protocol.Range(nil), binding.References...),
	}, true
}

var inlineUnsafeKinds = map[token.Kind]bool{
	token.Semicolon: true, token.CloseTag: true, token.LeftBrace: true, token.RightBrace: true,
	token.ObjectOperator: true, token.NullsafeObjectOperator: true, token.DoubleColon: true,
	token.New: true, token.At: true, token.Increment: true, token.Decrement: true,
	token.Backtick: true, token.Dollar: true, token.Clone: true, token.Echo: true, token.Eval: true, token.Exit: true,
	token.Include: true, token.IncludeOnce: true, token.Require: true, token.RequireOnce: true,
	token.Print: true, token.Throw: true, token.Yield: true, token.YieldFrom: true,
}

func inlineExpressionIsSafe(toks codeTokens, first, last int) bool {
	for i := first; i <= last; i++ {
		kind := toks.all[i].Kind
		if inlineUnsafeKinds[kind] || kind.IsAssignment() || isCallee(kind) && toks.kind(i+1) == token.LeftParen {
			return false
		}
	}
	return true
}

func isCallee(kind token.Kind) bool {
	switch kind {
	case token.Function, token.Fn:
		return false
	case token.Variable, token.NameQualified, token.NameFullyQualified, token.NameRelative:
		return true
	}
	return kind.IsSemiReserved()
}

func inlineReferenceIsMutation(toks codeTokens, ref protocol.Range) bool {
	start, _ := toks.offset(ref.Start)
	end, _ := toks.offset(ref.End)
	i := sort.Search(len(toks.all), func(i int) bool { return int(toks.all[i].Start) >= start })
	if i == len(toks.all) || toks.all[i].Kind != token.Variable || int(toks.all[i].Start) != start || int(toks.all[i].End) != end {
		return true
	}
	if prev := toks.kind(i - 1); prev == token.Increment || prev == token.Decrement {
		return true
	}
	next := toks.kind(i + 1)
	return next.IsAssignment() || next == token.Increment || next == token.Decrement
}

func inlineExpressionIsAtomic(toks codeTokens, first, last int) bool {
	if toks.all[first].Kind == token.LeftParen && lexer.MatchingClose(toks.all, first) == last {
		return true
	}
	if first != last {
		return false
	}
	switch toks.all[first].Kind {
	case token.Variable, token.ConstantString, token.DoubleQuote, token.StartHeredoc, token.IntegerLiteral, token.FloatLiteral:
		return true
	}
	return false
}

func expandInlineDeleteRange(toks codeTokens, stmt protocol.Range) protocol.Range {
	start := protocol.Position{Line: stmt.Start.Line, Character: 0}
	if stmt.Start.Line != stmt.End.Line {
		return protocol.Range{Start: start, End: stmt.End}
	}
	line := toks.line(stmt.Start.Line)
	if strings.TrimSpace(line[:stmt.Start.Character]) != "" || strings.TrimSpace(line[stmt.End.Character:]) != "" {
		return stmt
	}
	end := stmt.End
	if stmt.Start.Line+1 < len(toks.lines) {
		end = protocol.Position{Line: stmt.Start.Line + 1, Character: 0}
	}
	return protocol.Range{Start: start, End: end}
}
