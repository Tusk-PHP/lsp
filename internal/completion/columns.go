package completion

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func isBuilderArg(arg ast.CallArg) bool {
	if arg.Method == "" || arg.Value == ast.ArgName {
		return false
	}
	method, ok := frameworklaravel.BuilderMethodNamed(arg.Method)
	if !ok {
		return false
	}
	switch {
	case arg.Index == 1:
		return method.RelatedModelColumn && !arg.InArray
	case arg.Index != 0:
		return false
	}
	return !arg.InArray || method.NamesValues()
}

func (p *Provider) completeBuilderArg(r *resolve.Resolver, arg ast.CallArg, file *parser.FileNode) []protocol.CompletionItem {
	modelFQN := frameworklaravel.BuilderModel(r, p.index, file, arg.Receiver).FQN
	if modelFQN == "" {
		return nil
	}
	method, _ := frameworklaravel.BuilderMethodNamed(arg.Method)
	if arg.Index == 1 {
		relation, ok := arg.StringArg(0)
		if !ok {
			return nil
		}
		related := frameworklaravel.RelatedModel(p.index, modelFQN, frameworklaravel.WithoutAlias(relation))
		if related == "" {
			return nil
		}
		return p.completeBuilderProperties(related, arg.Partial, arg.Quote, frameworklaravel.IsColumnProperty)
	}
	switch method.Names {
	case frameworklaravel.BuilderDBColumns:
		return p.completeBuilderProperties(modelFQN, arg.Partial, arg.Quote, frameworklaravel.IsDBColumnProperty)
	case frameworklaravel.BuilderColumns:
		return p.completeBuilderProperties(modelFQN, arg.Partial, arg.Quote, frameworklaravel.IsColumnProperty)
	}
	return p.completeBuilderRelations(modelFQN, arg.Partial, arg.Quote)
}

func (p *Provider) completeBuilderProperties(modelFQN, partial, quote string, is func(*symbols.Symbol) bool) []protocol.CompletionItem {
	lpartial := strings.ToLower(partial)
	q := "'"
	if quote == "\"" {
		q = "\""
	}
	var items []protocol.CompletionItem
	for _, m := range p.index.GetClassMembers(modelFQN) {
		if !is(m) {
			continue
		}
		colName := strings.TrimPrefix(m.Name, "$")
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(colName), lpartial) {
			continue
		}
		insertText := colName
		if quote == "" {
			insertText = q + colName + q
		}
		items = append(items, protocol.CompletionItem{
			Label:      colName,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     m.Type,
			InsertText: insertText,
			SortText:   "0" + colName,
		})
	}
	return items
}

// completeBuilderRelations returns completion items for relation names on the given model.
func (p *Provider) completeBuilderRelations(modelFQN, partial, quote string) []protocol.CompletionItem {
	members := p.index.GetClassMembers(modelFQN)
	lpartial := strings.ToLower(partial)

	q := "'"
	if quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	for _, m := range members {
		if m.Kind != symbols.KindMethod {
			continue
		}

		// Check if the return type is a relation class
		retType := m.ReturnType
		if retType == "" {
			continue
		}
		shortRet := shortClassName(retType)
		if !frameworklaravel.IsRelationType(shortRet) {
			continue
		}

		relName := m.Name
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(relName), lpartial) {
			continue
		}

		insertText := relName
		if quote == "" {
			insertText = q + relName + q
		}

		items = append(items, protocol.CompletionItem{
			Label:      relName,
			Kind:       protocol.CompletionItemKindMethod,
			Detail:     shortRet + " relation",
			InsertText: insertText,
			SortText:   "0" + relName,
		})
	}
	return items
}

// shortClassName extracts the short class name from a potentially qualified name.
func shortClassName(name string) string {
	if i := strings.LastIndex(name, "\\"); i >= 0 {
		return name[i+1:]
	}
	return name
}
