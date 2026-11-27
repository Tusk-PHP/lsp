package symbols

import "github.com/Tusk-PHP/lsp/internal/parser"

// indexConstants handles both file constants and constants owned by a class-like
// symbol. Global constants need the same source/profile handling as functions.
func (idx *Index) indexConstants(uri string, parent *Symbol, consts []parser.ConstantNode, uses []parser.UseNode, src SymbolSource, resolve func(string) string) {
	for _, co := range consts {
		fqn, owner := co.FullName, ""
		if parent != nil {
			fqn, owner = parent.FQN+"::"+co.Name, parent.FQN
		}
		cs := &Symbol{Name: co.Name, FQN: fqn, Kind: KindConstant, URI: uri, ParentFQN: owner,
			Value: co.Value, Type: resolve(co.Type.Name), Visibility: co.Visibility,
			Attributes: indexAttributes(co.Attributes, resolve), DocComment: qualifyDocTypes(co.DocComment, uses),
			Range: symRange(co.StartLine, co.StartCol, len(co.Name))}
		if parent != nil {
			parent.Children = append(parent.Children, cs)
		}
		idx.addSymbolWithSource(uri, cs, src)
	}
}
