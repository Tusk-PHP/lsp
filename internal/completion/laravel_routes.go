package completion

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func (p *Provider) completeLaravelRouteNames(partial, quote string) []protocol.CompletionItem {
	if p.framework != "laravel" || p.routes == nil {
		return nil
	}

	q := "'"
	if quote == `"` {
		q = `"`
	}

	lpartial := strings.ToLower(partial)
	var items []protocol.CompletionItem
	for _, name := range p.routes.Names() {
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(name), lpartial) {
			continue
		}

		insertText := name
		if quote == "" {
			insertText = q + name + q
		}

		items = append(items, protocol.CompletionItem{
			Label:      name,
			Kind:       protocol.CompletionItemKindValue,
			Detail:     "Laravel route name",
			InsertText: insertText,
			SortText:   "0" + name,
		})
	}

	return items
}
