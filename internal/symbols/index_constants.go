package symbols

import "github.com/Tusk-PHP/lsp/internal/parser"

// indexConstants handles both file constants and constants owned by a class-like
// symbol. Global constants need the same source/profile handling as functions.
func (idx *Index) indexConstants(uri string, parent *Symbol, parentFQN string, consts []parser.ConstantNode, src SymbolSource, resolve func(string) string) {
	for _, co := range consts {
		// A global constant's FQN is namespace\NAME and it has no owner; only a
		// class constant has a ParentFQN (hover shows it as Owner::NAME).
		fqn, owner := buildFQN(parentFQN, co.Name), ""
		if parent != nil {
			fqn, owner = parentFQN+"::"+co.Name, parentFQN
		}
		cs := &Symbol{Name: co.Name, FQN: fqn, Kind: KindConstant, URI: uri, ParentFQN: owner,
			Value: co.Value, Type: resolve(co.Type.Name), Visibility: co.Visibility,
			Attributes: indexAttributes(co.Attributes, resolve),
			Range:      symRange(co.StartLine, co.StartCol, len(co.Name))}
		if parent != nil {
			parent.Children = append(parent.Children, cs)
		}
		idx.addSymbolWithSource(uri, cs, src)
	}
}
