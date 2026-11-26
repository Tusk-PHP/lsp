package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/types"
)

type configResultArrayContext struct {
	ConfigArg  string
	AccessKeys []string
	Partial    string
	Quote      string
}

func configResultContext(src string, dim sourcectx.StringDim) *configResultArrayContext {
	call, ok := dim.Base.(*ast.Call)
	if !ok || sourcectx.FunctionName(src, call) != models.ConfigHelper || len(call.Args.Args) == 0 {
		return nil
	}
	configArg, ok := sourcectx.StringValue(src, call.Args.Args[0].Value)
	if !ok || configArg == "" {
		return nil
	}
	return &configResultArrayContext{ConfigArg: configArg, AccessKeys: dim.Keys, Partial: dim.Partial, Quote: dim.Quote}
}

func (p *Provider) completeConfigResultKeys(ctx *configResultArrayContext) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	keys := types.FieldsAt(p.arrayResolver.ConfigKeys(ctx.ConfigArg), ctx.AccessKeys)
	if len(keys) == 0 {
		return nil
	}

	q := "'"
	if ctx.Quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(ctx.Partial)
	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}
		insertText := k.Key
		if ctx.Quote == "" {
			insertText = q + k.Key + q
		}
		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     k.Type,
			InsertText: insertText,
			SortText:   "0" + k.Key,
		})
	}
	return items
}

func configArgContext(arg sourcectx.CallArg) (configPath, partial, quote string, ok bool) {
	if arg.Function != models.ConfigHelper || arg.Index != 0 || arg.InArray {
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
