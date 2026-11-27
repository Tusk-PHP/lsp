package completion

import (
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
)

// completeEnvKeys offers the env keys starting with what is typed before the
// cursor, each replacing that text, with the files that set it and never its
// value.
func (p *Provider) completeEnvKeys(arg sourcectx.StringArgument) []protocol.CompletionItem {
	matches := p.envKeys.Matching(arg.Partial)
	start := arg.Content.Start
	typed := protocol.Range{Start: start, End: protocol.Position{Line: start.Line, Character: start.Character + len(arg.Partial)}}
	items := make([]protocol.CompletionItem, 0, len(matches))
	for _, key := range matches {
		items = append(items, protocol.CompletionItem{
			Label:    key.Name,
			Kind:     protocol.CompletionItemKindConstant,
			Detail:   key.DefinedIn,
			SortText: "0" + key.Name,
			TextEdit: &protocol.TextEdit{Range: typed, NewText: key.Name},
		})
	}
	return items
}
