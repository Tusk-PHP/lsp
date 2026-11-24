package analyzer

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

type ignoreComment struct {
	title, text string
}

func IgnoreDiagnosticActions(uri, source string, params protocol.CodeActionParams) []protocol.CodeAction {
	actions := make([]protocol.CodeAction, 0)
	if len(params.Context.Diagnostics) == 0 || !protocol.CodeActionQuickFix.In(params.Context.Only) {
		return actions
	}
	layout := checks.PHPLayout
	if workspace.IsBlade(uri) {
		layout = checks.BladeLayout
	}
	l := layout(source)
	type lineTitle struct {
		line  int
		title string
	}
	seen := map[lineTitle]bool{}
	for _, d := range params.Context.Diagnostics {
		if _, ok := l.LineStart(d.Range.Start.Line); !ok || d.Code == "" {
			continue
		}
		line := l.MarkerLine(d.Range.Start.Line)
		start, _ := l.LineStart(line)
		style, ok := l.StyleAt(start)
		if !ok {
			continue
		}
		for _, c := range ignoreComments(d, style) {
			key := lineTitle{line, c.title}
			if seen[key] {
				continue
			}
			seen[key] = true
			actions = append(actions, protocol.CodeAction{
				Title:       c.title,
				Kind:        protocol.CodeActionQuickFix,
				Diagnostics: []protocol.Diagnostic{d},
				Edit: &protocol.WorkspaceEdit{Changes: map[string][]protocol.TextEdit{uri: {{
					Range:   protocol.Range{Start: protocol.Position{Line: line}, End: protocol.Position{Line: line}},
					NewText: l.Indent(line) + c.text + l.LineEnding(),
				}}}},
			})
		}
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].Title < actions[j].Title })
	return actions
}

func ignoreComments(d protocol.Diagnostic, style checks.CommentStyle) []ignoreComment {
	targets, err := rule.TargetsOf(d.Source, d.Code)
	if err != nil {
		return nil
	}
	if d.Source == string(rule.SourcePHPStan) && style == checks.PHPComment {
		return []ignoreComment{{"Ignore " + d.Code + " with @phpstan-ignore", style.Wrap(checks.PHPStanSuppressionComment(d.Code))}}
	}
	comments := make([]ignoreComment, len(targets))
	for i, target := range targets {
		comments[i] = ignoreComment{"Ignore " + target.String() + " on this line", style.Wrap(checks.SuppressionComment(target))}
	}
	return comments
}
