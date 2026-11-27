package completion

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func isBuilderArg(arg sourcectx.CallArg) bool {
	if arg.Method == "" || arg.Value == sourcectx.ArgName {
		return false
	}
	method, ok := frameworklaravel.BuilderMethodNamed(arg.Method)
	if !ok {
		return false
	}
	if arg.Index == 0 {
		return !arg.InArray || method.NamesValues()
	}
	second, ok := method.SecondNameAt(arg.ArgCount())
	return ok && arg.Index == second && !arg.InArray
}

func (p *Provider) completeBuilderArg(r *resolve.Resolver, arg sourcectx.CallArg, file *parser.FileNode) []protocol.CompletionItem {
	modelFQN := p.builderModel(r, arg.Receiver, file).FQN
	if modelFQN == "" {
		return nil
	}
	method, _ := frameworklaravel.BuilderMethodNamed(arg.Method)
	if arg.Index > 0 && method.Second == frameworklaravel.RelatedModelColumn {
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

// ResolveBuilderModel returns the Eloquent model a builder call with the
// receiver node of file is made on, for the unknown-column and
// unknown-relation diagnostics. Its FQN is "" when the model can't be worked
// out.
func (p *Provider) ResolveBuilderModel(file *parser.FileNode, receiver ast.Expr) frameworklaravel.QueriedModel {
	return p.builderModel(p.engine.Resolver(), receiver, file)
}

func (p *Provider) builderModel(r *resolve.Resolver, receiver ast.Expr, file *parser.FileNode) frameworklaravel.QueriedModel {
	if class, ok := receiver.(*ast.ConstFetch); ok {
		return p.modelClass(r, class, file)
	}
	return frameworklaravel.ModelQueriedBy(p.index, r.NodeType(file, receiver))
}

func (p *Provider) modelClass(r *resolve.Resolver, e ast.Expr, file *parser.FileNode) frameworklaravel.QueriedModel {
	name, ok := e.(*ast.ConstFetch)
	if !ok {
		return frameworklaravel.QueriedModel{}
	}
	if fqn := r.ResolveClassNameLoose(name.Name.Bounds().Text(file.Source), file); frameworklaravel.IsModel(p.index, fqn) {
		return frameworklaravel.QueriedModel{FQN: fqn}
	}
	return frameworklaravel.QueriedModel{}
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
