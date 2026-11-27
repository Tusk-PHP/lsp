package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/types"
)

func configArgContext(arg sourcectx.CallArg) (configPath, partial, quote string, ok bool) {
	if !laravel.IsConfigKeyArg(arg) {
		return "", "", "", false
	}
	lastDot := strings.LastIndex(arg.Partial, ".")
	if lastDot < 0 {
		return "", arg.Partial, arg.Quote, true
	}
	return arg.Partial[:lastDot], arg.Partial[lastDot+1:], arg.Quote, true
}

func (p *Provider) completeConfigKeys(configPath, partial, quote string) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	keys := p.arrayResolver.ConfigKeys(configPath)
	if len(keys) == 0 {
		return nil
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(partial)

	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}

		isNested := strings.HasPrefix(k.Type, "array{") || k.Type == "array"
		detail := k.Type
		if isNested && strings.HasPrefix(k.Type, "array{") {
			inner := types.ParseArrayShape(k.Type)
			if len(inner) > 0 {
				var names []string
				for _, f := range inner {
					if f.Key != "" {
						names = append(names, f.Key)
					}
					if len(names) >= 4 {
						names = append(names, "...")
						break
					}
				}
				detail = "{" + strings.Join(names, ", ") + "}"
			}
		}

		insertText := k.Key
		if isNested {
			insertText = k.Key + "."
		}
		if quote == "" {
			insertText = "'" + insertText + "'"
		}

		kind := protocol.CompletionItemKindProperty
		if isNested {
			kind = protocol.CompletionItemKindModule
		}

		sortText := "0" + k.Key
		if isNested {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       kind,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}
