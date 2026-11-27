package completion

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (p *Provider) completeAuthNames(name frameworklaravel.AuthName, pos protocol.Position) []protocol.CompletionItem {
	names := p.authorization.MiddlewareNames()
	if name.Kind == frameworklaravel.AbilityName {
		names = p.authorization.AbilityNames()
	}
	replace := protocol.Range{Start: protocol.Position{Line: pos.Line, Character: pos.Character - len(name.Partial)}, End: pos}
	lowerPartial := strings.ToLower(name.Partial)
	items := make([]protocol.CompletionItem, 0, len(names))
	for _, n := range names {
		if !strings.HasPrefix(strings.ToLower(n), lowerPartial) {
			continue
		}
		text := n
		if name.Quote == "" {
			text = "'" + n + "'"
		}
		items = append(items, protocol.CompletionItem{
			Label:      n,
			Kind:       protocol.CompletionItemKindValue,
			Detail:     p.authNameDetail(name.Kind, n),
			FilterText: n,
			TextEdit:   &protocol.TextEdit{Range: replace, NewText: text},
			SortText:   "0" + n,
		})
	}
	return items
}

func (p *Provider) authNameDetail(kind frameworklaravel.AuthNameKind, name string) string {
	if kind == frameworklaravel.AbilityName {
		ability := p.authorization.Abilities(name)[0]
		if ability.Policy != "" {
			return ability.Policy + "::" + ability.Method
		}
		return "Gate ability"
	}
	m := p.authorization.Middleware(name)[0]
	if m.Kind == frameworklaravel.MiddlewareGroup {
		return "Middleware group"
	}
	if m.Class != "" {
		return m.Class
	}
	return "Middleware alias"
}
