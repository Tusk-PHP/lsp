package analyzer

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

type ignoreComment struct {
	title, text string
	at          protocol.Position
}

func IgnoreDiagnosticActions(uri, source string, params protocol.CodeActionParams) []protocol.CodeAction {
	return ignoreDiagnosticActions(uri, params, false, func() checks.Layout { return checks.PHPLayout(source) })
}

func BladeIgnoreDiagnosticActions(uri, source string, params protocol.CodeActionParams, custom *blade.Custom) []protocol.CodeAction {
	return ignoreDiagnosticActions(uri, params, true, func() checks.Layout { return checks.BladeLayout(source, custom) })
}

func ignoreDiagnosticActions(uri string, params protocol.CodeActionParams, inBlade bool, layoutOf func() checks.Layout) []protocol.CodeAction {
	actions := make([]protocol.CodeAction, 0)
	if len(params.Context.Diagnostics) == 0 || !protocol.CodeActionQuickFix.In(params.Context.Only) {
		return actions
	}
	layout := layoutOf()
	suppressed := layout.Suppressions()
	type placedTitle struct {
		at    protocol.Position
		title string
	}
	seen := map[placedTitle]bool{}
	for _, d := range params.Context.Diagnostics {
		if _, ok := layout.LineStart(d.Range.Start.Line); !ok || d.Code == "" || suppressed.Suppresses(d.Range.Start.Line, d.Source, d.Code) {
			continue
		}
		for _, c := range ignoreComments(layout, d, inBlade) {
			key := placedTitle{c.at, c.title}
			if seen[key] {
				continue
			}
			seen[key] = true
			actions = append(actions, protocol.CodeAction{
				Title:       c.title,
				Kind:        protocol.CodeActionQuickFix,
				Diagnostics: []protocol.Diagnostic{d},
				Edit: &protocol.WorkspaceEdit{Changes: map[string][]protocol.TextEdit{uri: {{
					Range:   protocol.Range{Start: c.at, End: c.at},
					NewText: c.text,
				}}}},
			})
		}
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].Title < actions[j].Title })
	return actions
}

func ignoreComments(layout checks.Layout, d protocol.Diagnostic, inBlade bool) []ignoreComment {
	targets, err := rule.TargetsOf(d.Source, d.Code)
	if err != nil {
		return nil
	}
	if d.Source == string(rule.SourcePHPStan) && !inBlade {
		return phpstanIgnoreComment(layout, d)
	}
	line := layout.MarkerLine(d.Range.Start.Line)
	start, _ := layout.LineStart(line)
	style, ok := layout.StyleAt(start)
	if !ok {
		return nil
	}
	at := protocol.Position{Line: line}
	comments := make([]ignoreComment, len(targets))
	for i, target := range targets {
		text := layout.Indent(line) + style.Wrap(checks.SuppressionComment(target)) + layout.LineEnding()
		comments[i] = ignoreComment{"Ignore " + target.String() + " on this line", text, at}
	}
	return comments
}

func phpstanIgnoreComment(layout checks.Layout, d protocol.Diagnostic) []ignoreComment {
	offset, style, ok := layout.TrailingCommentAt(d.Range.Start.Line)
	if !ok {
		return nil
	}
	text := " " + style.Wrap(checks.PHPStanSuppressionComment(d.Code))
	if style == checks.PHPBlockComment {
		text = style.Wrap(checks.PHPStanSuppressionComment(d.Code)) + " "
	}
	at := protocol.Position{Line: d.Range.Start.Line, Character: layout.Character(offset)}
	return []ignoreComment{{"Ignore " + d.Code + " with @phpstan-ignore", text, at}}
}
