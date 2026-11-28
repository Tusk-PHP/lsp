package completion

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/php/ast"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Builder methods whose first string argument is a column name.
// These suggest all model properties (declared, virtual, migration-derived, etc.)
// except relations and accessors.
var columnMethods = map[string]bool{
	"where": true, "whereIn": true, "whereNotIn": true,
	"whereNull": true, "whereNotNull": true,
	"whereBetween": true, "whereNotBetween": true,
	"whereDate": true, "whereMonth": true, "whereDay": true,
	"whereYear": true, "whereTime": true, "whereColumn": true,
	"orderBy": true, "orderByDesc": true,
	"latest": true, "oldest": true,
	"groupBy": true, "having": true,
	"select": true, "addSelect": true,
	"pluck": true, "value": true,
	"increment": true, "decrement": true,
}

// Builder methods that only accept actual database column names (from
// migrations, database introspection, IDE helper @property, or Doctrine).
// These are stricter than columnMethods — they exclude declared PHP properties
// and accessors since those aren't real DB columns.
var dbColumnMethods = map[string]bool{
	"get": true,
}

// Builder methods whose first string argument is a relation name.
var relationMethods = map[string]bool{
	"with": true, "without": true,
	"has": true, "doesntHave": true,
	"whereHas": true, "whereDoesntHave": true,
	"withCount": true, "withSum": true, "withAvg": true,
	"withMin": true, "withMax": true, "withExists": true,
	"load": true, "loadMissing": true, "loadCount": true,
}

// Methods that accept arrays of column/relation names as elements.
// Methods NOT in this set (e.g. where) use associative array syntax
// and should not trigger completion inside array arguments.
var arrayArgMethods = map[string]bool{
	// Column arrays
	"select": true, "addSelect": true, "get": true,
	"groupBy": true, "whereNull": true, "whereNotNull": true,
	// Relation arrays
	"with": true, "without": true,
	"withCount": true, "withSum": true, "withAvg": true,
	"withMin": true, "withMax": true, "withExists": true,
	"load": true, "loadMissing": true, "loadCount": true,
}

// Relation return type FQN fragments used to identify relation methods on models.
var relationReturnTypes = map[string]bool{
	"HasOne": true, "HasMany": true,
	"BelongsTo": true, "BelongsToMany": true,
	"MorphOne": true, "MorphMany": true,
	"MorphTo": true, "MorphToMany": true, "MorphedByMany": true,
	"HasOneThrough": true, "HasManyThrough": true,
}

func isBuilderArg(arg ast.CallArg) bool {
	m := arg.Method
	if m == "" || arg.Index != 0 || arg.Value == ast.ArgName {
		return false
	}
	if !columnMethods[m] && !relationMethods[m] && !dbColumnMethods[m] {
		return false
	}
	return !arg.InArray || arrayArgMethods[m]
}

func (p *Provider) completeBuilderArg(r *resolve.Resolver, arg ast.CallArg, file *parser.FileNode) []protocol.CompletionItem {
	modelFQN := frameworklaravel.BuilderModel(r, p.index, file, arg.Receiver).FQN
	if modelFQN == "" {
		return nil
	}
	if dbColumnMethods[arg.Method] {
		return p.completeBuilderDBColumns(modelFQN, arg.Partial, arg.Quote)
	}
	if columnMethods[arg.Method] {
		return p.completeBuilderColumns(modelFQN, arg.Partial, arg.Quote)
	}
	return p.completeBuilderRelations(modelFQN, arg.Partial, arg.Quote)
}

// isDBColumn returns true if the symbol represents an actual database column
// (from IDE helper @property, migrations, database introspection, or Doctrine).
func isDBColumn(m *symbols.Symbol) bool {
	if m.Kind != symbols.KindProperty {
		return false
	}
	if !m.IsVirtual {
		return false
	}
	doc := m.DocComment
	// Migration-derived
	if strings.HasPrefix(doc, "From migration") {
		return true
	}
	// Database introspection
	if strings.HasPrefix(doc, "(database column)") {
		return true
	}
	// Doctrine ORM
	if doc == "Doctrine column" || doc == "Doctrine ID column" {
		return true
	}
	// Skip relations and accessors — everything else from @property is a column
	if strings.HasSuffix(doc, " relation") {
		return false
	}
	if strings.HasPrefix(doc, "Accessor from ") {
		return false
	}
	// IDE helper @property and other docblock-derived properties
	// These are virtual properties from class docblocks, typically DB columns
	return true
}

// completeBuilderDBColumns returns completion items for strict database column
// names only (IDE helper, migrations, database introspection, Doctrine).
func (p *Provider) completeBuilderDBColumns(modelFQN, partial, quote string) []protocol.CompletionItem {
	members := p.index.GetClassMembers(modelFQN)
	lpartial := strings.ToLower(partial)

	q := "'"
	if quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	for _, m := range members {
		if !isDBColumn(m) {
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

// completeBuilderColumns returns completion items for column names on the given model.
func (p *Provider) completeBuilderColumns(modelFQN, partial, quote string) []protocol.CompletionItem {
	members := p.index.GetClassMembers(modelFQN)
	lpartial := strings.ToLower(partial)

	q := "'"
	if quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	for _, m := range members {
		if m.Kind != symbols.KindProperty {
			continue
		}
		// Skip relation-derived virtual properties (they appear as columns otherwise)
		if m.IsVirtual && strings.HasSuffix(m.DocComment, " relation") {
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
		if !relationReturnTypes[shortRet] {
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
