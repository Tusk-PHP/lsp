package symbols

import "github.com/Tusk-PHP/lsp/internal/parser"

// indexConstants handles both file constants and constants owned by a class-like
// symbol. Global constants need the same source/profile handling as functions.
func (idx *Index) indexConstants(uri string, parent *Symbol, parentFQN string, consts []parser.ConstantNode, uses []parser.UseNode, src SymbolSource, resolve func(string) string) {
	for _, co := range consts {
		// A global constant's FQN is namespace\NAME and it has no owner; only a
		// class constant has a ParentFQN (hover shows it as Owner::NAME).
		fqn, owner := buildFQN(parentFQN, co.Name), ""
		if parent != nil {
			fqn, owner = parentFQN+"::"+co.Name, parentFQN
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

func (idx *Index) indexGlobalConstants(uri string, consts []parser.ConstantNode, src SymbolSource) {
	for start := 0; start < len(consts); {
		ns := consts[start].Namespace
		end := start + 1
		for end < len(consts) && consts[end].Namespace == ns {
			end++
		}
		idx.indexConstants(uri, nil, ns.Name, consts[start:end], ns.Uses, src, namespaceResolver(ns))
		start = end
	}
}

func namespaceResolver(ns *parser.NamespaceNode) func(string) string {
	return func(name string) string {
		return resolveTypeName(name, ns.Name, ns.Uses)
	}
}
