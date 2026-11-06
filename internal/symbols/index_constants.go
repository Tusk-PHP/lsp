package symbols

import "github.com/Tusk-PHP/lsp/internal/parser"

// indexConstants handles both file constants and constants owned by a class-like
// symbol. Global constants need the same source/profile handling as functions.
func (idx *Index) indexConstants(uri string, parent *Symbol, parentFQN string, consts []parser.ConstantNode, src SymbolSource, resolve func(string) string) {
	for _, co := range consts {
		fqn := buildFQN(parentFQN, co.Name)
		if parent != nil {
			fqn = parentFQN + "::" + co.Name
		}
		cs := &Symbol{Name: co.Name, FQN: fqn, Kind: KindConstant, URI: uri, ParentFQN: parentFQN,
			Value: co.Value, Type: resolve(co.Type.Name), Visibility: co.Visibility,
			Attributes: indexAttributes(co.Attributes, resolve),
			Range:      symRange(co.StartLine, co.StartCol, len(co.Name))}
		if parent != nil {
			parent.Children = append(parent.Children, cs)
		}
		idx.addSymbolWithSource(uri, cs, src)
	}
}
