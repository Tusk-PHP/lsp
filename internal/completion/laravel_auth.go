package completion

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// AuthName reads the Gate ability or middleware name arg holds, typing its
// receiver with the provider's resolver.
func (p *Provider) AuthName(file *parser.FileNode, arg ast.CallArg) (frameworklaravel.AuthName, bool) {
	return frameworklaravel.AuthNameArg(p.engine.Resolver(), p.index, file, arg)
}

// ModelClass types the model an ability is checked against.
func (p *Provider) ModelClass(file *parser.FileNode, e ast.Expr) string {
	return frameworklaravel.ModelClass(p.engine.Resolver(), file, e)
}

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
