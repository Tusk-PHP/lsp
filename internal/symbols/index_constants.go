package symbols

import "github.com/Tusk-PHP/lsp/internal/parser"

func constantSymbol(uri, fqn, owner string, co *parser.ConstantNode, resolve func(string) string, uses []parser.UseNode) *Symbol {
	return &Symbol{Name: co.Name, FQN: fqn, Kind: KindConstant, URI: uri, ParentFQN: owner,
		Value: co.Value, Type: resolve(co.Type.Name), Visibility: co.Visibility,
		Attributes: indexAttributes(co.Attributes, resolve), DocComment: qualifyDocTypes(co.DocComment, uses),
		Range: symRange(co.StartLine, co.StartCol, len(co.Name))}
}

func addClassConstants(uri string, parent *Symbol, consts []parser.ConstantNode, resolve func(string) string, uses []parser.UseNode) {
	for i := range consts {
		parent.Children = append(parent.Children, constantSymbol(uri, parent.FQN+"::"+consts[i].Name, parent.FQN, &consts[i], resolve, uses))
	}
}

func appendGlobalConstants(decls []*Symbol, uri string, consts []parser.ConstantNode) []*Symbol {
	for start := 0; start < len(consts); {
		ns := consts[start].Namespace
		resolve := namespaceResolver(ns)
		end := start
		for ; end < len(consts) && consts[end].Namespace == ns; end++ {
			decls = append(decls, constantSymbol(uri, buildFQN(ns.Name, consts[end].Name), "", &consts[end], resolve, ns.Uses))
		}
		start = end
	}
	return decls
}

func namespaceResolver(ns *parser.NamespaceNode) func(string) string {
	return func(name string) string {
		return resolveTypeName(name, ns.Name, ns.Uses)
	}
}
