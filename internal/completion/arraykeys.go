package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/types"
)

type arrayKeyContext struct {
	Var        *ast.Variable
	AccessKeys []string
	Partial    string
	Quote      string
}

func arrayKeyContextOf(dim ast.StringDim) (*arrayKeyContext, bool) {
	v, ok := dim.Base.(*ast.Variable)
	if !ok {
		return nil, false
	}
	return &arrayKeyContext{Var: v, AccessKeys: dim.Keys, Partial: dim.Partial, Quote: dim.Quote}, true
}

func (p *Provider) completeArrayKeys(r *resolve.Resolver, file *parser.FileNode, ctx *arrayKeyContext) []protocol.CompletionItem {
	keys := types.FieldsAt(p.arrayResolver.VariableKeys(r, file, ctx.Var), ctx.AccessKeys)
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
		detail := k.Type
		if k.Optional {
			detail += " (optional)"
		}

		insertText := k.Key
		if ctx.Quote == "" {
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
