package analyzer

import (
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (a *Analyzer) PrepareTypeHierarchy(uri, source string, pos protocol.Position) []protocol.TypeHierarchyItem {
	sym, _, _ := a.resolveDefinitionSymbol(uri, source, pos)
	if item, ok := typeHierarchyItem(sym); ok {
		return []protocol.TypeHierarchyItem{item}
	}
	return nil
}

func (a *Analyzer) Supertypes(item protocol.TypeHierarchyItem) []protocol.TypeHierarchyItem {
	return a.typeHierarchyItems(item, a.index.DirectSupertypes)
}

func (a *Analyzer) Subtypes(item protocol.TypeHierarchyItem) []protocol.TypeHierarchyItem {
	return a.typeHierarchyItems(item, a.index.DirectSubtypes)
}

func (a *Analyzer) typeHierarchyItems(item protocol.TypeHierarchyItem, related func(string) []string) []protocol.TypeHierarchyItem {
	items := make([]protocol.TypeHierarchyItem, 0)
	for _, fqn := range related(item.Data.FQN) {
		if next, ok := typeHierarchyItem(a.index.Lookup(fqn)); ok {
			items = append(items, next)
		}
	}
	return items
}

func typeHierarchyItem(sym *symbols.Symbol) (protocol.TypeHierarchyItem, bool) {
	loc := symbolLocation(sym)
	if loc == nil || !sym.IsClassLike() {
		return protocol.TypeHierarchyItem{}, false
	}
	item := protocol.TypeHierarchyItem{
		Name:           sym.Name,
		Kind:           protocolSymbolKind(sym.Kind),
		Detail:         symbolContainerName(sym),
		URI:            loc.URI,
		Range:          loc.Range,
		SelectionRange: loc.Range,
		Data:           protocol.TypeHierarchyData{FQN: sym.FQN},
	}
	if sym.IsDeprecated() {
		item.Tags = []protocol.SymbolTag{protocol.SymbolTagDeprecated}
	}
	return item, true
}
