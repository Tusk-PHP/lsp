package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/types"
)

func (p *Provider) completeArrayKeys(r *resolve.Resolver, file *parser.FileNode, dim sourcectx.StringDim) []protocol.CompletionItem {
	keys := types.FieldsAt(p.arrayResolver.Keys(r, file, dim.Base), dim.Keys)
	if len(keys) == 0 {
		return nil
	}

	q := "'"
	if dim.Quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(dim.Partial)
	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}
		detail := k.Type
		if k.Optional {
			detail += " (optional)"
		}

		insertText := k.Key
		if dim.Quote == "" {
			insertText = q + k.Key + q
		}

		sortText := "0" + k.Key
		if k.Optional {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}
