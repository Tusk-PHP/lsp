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
	at          protocol.Position
}

func IgnoreDiagnosticActions(uri, source string, params protocol.CodeActionParams) []protocol.CodeAction {
	actions := make([]protocol.CodeAction, 0)
	if len(params.Context.Diagnostics) == 0 || !protocol.CodeActionQuickFix.In(params.Context.Only) {
		return actions
	}
	blade := workspace.IsBlade(uri)
	l := checks.PHPLayout
	if blade {
		l = checks.BladeLayout
	}
	layout := l(source)
	type placedTitle struct {
		at    protocol.Position
		title string
	}
	seen := map[placedTitle]bool{}
	for _, d := range params.Context.Diagnostics {
		if _, ok := layout.LineStart(d.Range.Start.Line); !ok || d.Code == "" {
			continue
		}
		for _, c := range ignoreComments(layout, d, blade) {
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

func ignoreComments(layout checks.Layout, d protocol.Diagnostic, blade bool) []ignoreComment {
	targets, err := rule.TargetsOf(d.Source, d.Code)
	if err != nil {
		return nil
	}
	if d.Source == string(rule.SourcePHPStan) && !blade {
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
