package analyzer

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func IgnoreDiagnosticActions(uri, source string, params protocol.CodeActionParams) []protocol.CodeAction {
	actions := make([]protocol.CodeAction, 0)
	if !protocol.CodeActionQuickFix.In(params.Context.Only) {
		return actions
	}
	blade := workspace.IsBlade(uri)
	var tokens []parser.Token
	if !blade {
		tokens = parser.Tokenize(source)
	}
	type lineTitle struct {
		line  int
		title string
	}
	seen := map[lineTitle]bool{}
	for _, d := range params.Context.Diagnostics {
		if d.Code == "" {
			continue
		}
		line := d.Range.Start.Line
		lineStart, ok := positionToOffset(source, protocol.Position{Line: line})
		if !ok || (!blade && !phpCodeAt(tokens, lineStart)) {
			continue
		}
		title, comment, ok := ignoreComment(d, blade)
		if !ok {
			continue
		}
		key := lineTitle{line, title}
		if seen[key] {
			continue
		}
		seen[key] = true
		indent := leadingWhitespace(source[lineStart:])
		actions = append(actions, protocol.CodeAction{
			Title:       title,
			Kind:        protocol.CodeActionQuickFix,
			Diagnostics: []protocol.Diagnostic{d},
			Edit: &protocol.WorkspaceEdit{Changes: map[string][]protocol.TextEdit{uri: {{
				Range:   protocol.Range{Start: protocol.Position{Line: line}, End: protocol.Position{Line: line}},
				NewText: indent + comment + "\n",
			}}}},
		})
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].Title < actions[j].Title })
	return actions
}

func ignoreComment(d protocol.Diagnostic, blade bool) (title, comment string, ok bool) {
	target, err := rule.TargetOf(d.Source, d.Code)
	if err != nil {
		return "", "", false
	}
	if d.Source == string(rule.SourcePHPStan) && !blade {
		return "Ignore " + d.Code + " with @phpstan-ignore", "// " + checks.PHPStanSuppressionComment(d.Code), true
	}
	title = "Ignore " + target.String() + " on this line"
	if blade {
		return title, "{{-- " + checks.SuppressionComment(target) + " --}}", true
	}
	return title, "// " + checks.SuppressionComment(target), true
}

func phpCodeAt(tokens []parser.Token, offset int) bool {
	i := sort.Search(len(tokens), func(i int) bool { return tokens[i].Offset >= offset }) - 1
	if i < 0 {
		return false
	}
	prev := tokens[i]
	if prev.Offset+len(prev.Value) > offset {
		return prev.Kind == parser.TokenWhitespace
	}
	return prev.Value != "?>"
}
