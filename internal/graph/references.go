package graph

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// builtinTypeDenylist is the set of PHP built-in/pseudo-type names that must
// never appear as reference edge targets. Matched case-insensitively.
var builtinTypeDenylist = map[string]bool{
	"int": true, "float": true, "bool": true, "string": true, "array": true,
	"object": true, "callable": true, "iterable": true, "void": true, "mixed": true,
	"never": true, "null": true, "false": true, "true": true, "self": true,
	"static": true, "parent": true, "$this": true, "resource": true,
}

// isBuiltinType reports whether name is in the denylist (case-insensitive).
func isBuiltinType(name string) bool {
	return builtinTypeDenylist[strings.ToLower(name)]
}

// normalizeTypeTargets decomposes a PHP type string (which may be a union,
// intersection, generic, nullable, or array type) into a flat list of FQN
// targets suitable for reference edges. The input is assumed to be an already-
// resolved FQN (as stored in the symbol index).
//
// Rules:
//   - Strip a leading '?'.
//   - Strip trailing '[]' (repeatedly).
//   - Split on top-level '|' and '&'.
//   - For generics like Foo<Bar,Baz>, yield 'Foo' plus the inner types (recursed).
//   - Drop empty strings and case-insensitive denylist entries.
func normalizeTypeTargets(raw string) []string {
	var out []string
	collectTypeTargets(raw, &out)
	return out
}

func collectTypeTargets(raw string, out *[]string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	// Strip leading nullable prefix.
	if strings.HasPrefix(raw, "?") {
		raw = raw[1:]
	}

	// Strip trailing [] repeatedly (handles T[], T[][], etc.)
	for strings.HasSuffix(raw, "[]") {
		raw = raw[:len(raw)-2]
		raw = strings.TrimSpace(raw)
	}

	if raw == "" {
		return
	}

	// Split on top-level '|'.
	if parts := types.ParseTypeExpr(raw); parts.Kind == types.TypeKindUnion {
		for _, t := range parts.Types {
			collectTypeTargets(t.String(), out)
		}
		return
	}

	// Split on top-level '&'.
	if parts := types.ParseTypeExpr(raw); parts.Kind == types.TypeKindIntersection {
		for _, t := range parts.Types {
			collectTypeTargets(t.String(), out)
		}
		return
	}

	// Handle generic: Foo<Bar,Baz> => yield Foo, recurse Bar, Baz.
	expr := types.ParseTypeExpr(raw)
	switch expr.Kind {
	case types.TypeKindGeneric:
		name := strings.TrimPrefix(strings.TrimSpace(expr.Name), "\\")
		if name != "" && !isBuiltinType(name) {
			*out = append(*out, name)
		}
		for _, p := range expr.Params {
			collectTypeTargets(p.String(), out)
		}
		return
	case types.TypeKindArray:
		if expr.Element != nil {
			collectTypeTargets(expr.Element.String(), out)
		}
		return
	case types.TypeKindCallable:
		// callable / Closure — skip (not a class reference).
		return
	case types.TypeKindShape:
		// array shapes — skip outer name ("array","list","object") which are builtins.
		for _, f := range expr.ShapeFields {
			collectTypeTargets(f.Type.String(), out)
		}
		return
	}

	// Named (or unrecognised) — use the raw string as-is (it's already an FQN).
	name := strings.TrimPrefix(strings.TrimSpace(raw), "\\")
	if name == "" || isBuiltinType(name) {
		return
	}
	*out = append(*out, name)
}

// BuildReferences builds a class-level structural reference graph by iterating
// the symbol index. No method-body parsing is performed for most edges; only
// 'new X' instantiation uses a bounded token scan.
//
// Edge kinds emitted:
//   - "extends"    – class/interface inheritance
//   - "implements" – class/enum implementing an interface
//   - "property"   – property type reference
//   - "param"      – method parameter type reference
//   - "returns"    – method return type reference
//   - "uses"       – trait inclusion (via Index.TraitsOf)
//   - "new"        – instantiation via 'new ClassName'
//
// The graph is post-processed by applyDeps(opts) and sorted before return.
func BuildReferences(idx *symbols.Index, opts Options) *Graph {
	g := &Graph{SchemaVersion: SchemaVersion, Kind: "references"}
	if idx == nil {
		return g
	}

	nodeMap := map[string]Node{}
	edgeSet := map[refEdgeKey]struct{}{}

	addNode := func(id string) {
		if _, ok := nodeMap[id]; !ok {
			nodeMap[id] = Node{ID: id, Kind: "class", Label: id}
		}
	}

	addEdge := func(from, to, kind string) {
		if from == "" || to == "" || from == to {
			return
		}
		k := refEdgeKey{from, to, kind}
		if _, ok := edgeSet[k]; !ok {
			edgeSet[k] = struct{}{}
		}
	}

	emitTypeEdges := func(sourceFQN, rawType, kind string) {
		for _, target := range normalizeTypeTargets(rawType) {
			addNode(target)
			addEdge(sourceFQN, target, kind)
		}
	}

	// sourceKinds is the set of symbol kinds that act as reference sources.
	isSourceKind := func(k symbols.SymbolKind) bool {
		return k == symbols.KindClass || k == symbols.KindInterface ||
			k == symbols.KindTrait || k == symbols.KindEnum
	}

	for _, uri := range idx.GetAllFileURIs() {
		for _, sym := range idx.GetFileSymbols(uri) {
			if !isSourceKind(sym.Kind) {
				continue
			}

			addNode(sym.FQN)

			// extends
			if sym.Extends != "" {
				for _, target := range normalizeTypeTargets(sym.Extends) {
					addNode(target)
					addEdge(sym.FQN, target, "extends")
				}
			}

			// implements
			for _, iface := range sym.Implements {
				for _, target := range normalizeTypeTargets(iface) {
					addNode(target)
					addEdge(sym.FQN, target, "implements")
				}
			}

			// trait uses
			for _, traitFQN := range idx.TraitsOf(sym.FQN) {
				if traitFQN != "" && !isBuiltinType(traitFQN) {
					addNode(traitFQN)
					addEdge(sym.FQN, traitFQN, "uses")
				}
			}

			// children: methods and properties
			for _, child := range sym.Children {
				switch child.Kind {
				case symbols.KindMethod:
					for _, p := range child.Params {
						emitTypeEdges(sym.FQN, p.Type, "param")
					}
					emitTypeEdges(sym.FQN, child.ReturnType, "returns")
				case symbols.KindProperty:
					emitTypeEdges(sym.FQN, child.Type, "property")
				}
			}
		}
	}

	// new X token scan — bounded heuristic for instantiation edges.
	scanNewExpressions(idx, nodeMap, edgeSet, addNode)

	// Assemble slices.
	nodes := make([]Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	edges := make([]Edge, 0, len(edgeSet))
	for ek := range edgeSet {
		edges = append(edges, Edge{From: ek.from, To: ek.to, Kind: ek.kind})
	}
	g.Nodes = nodes
	g.Edges = edges

	g = applyDeps(g, idx, opts)
	g.Sort()
	return g
}

// refEdgeKey is the de-dup key for reference graph edges.
type refEdgeKey struct{ from, to, kind string }

// scanNewExpressions walks the syntax tree of each indexed file to find
// 'new ClassName' instantiations and emit "new" edges to the enclosing class.
// This is the only heuristic part of BuildReferences; all other edges are
// derived directly from the symbol index.
//
// Only a new with a written class name produces an edge: variables,
// expressions, anonymous classes and self/static/parent are skipped.
func scanNewExpressions(
	idx *symbols.Index,
	nodeMap map[string]Node,
	edgeSet map[refEdgeKey]struct{},
	addNode func(string),
) {
	r := resolve.NewResolver(idx)
	for _, uri := range idx.GetAllFileURIs() {
		src := idx.GetFileSource(uri)
		if src == "" {
			continue
		}
		tree := parser.ParseTreeIn(src, idx.GrammarOf(uri))
		file := parser.FileNodeOf(src, tree)
		for _, inst := range instantiations(src, tree) {
			resolved := r.ResolveClassName(inst.name, file)
			if resolved == "" || isBuiltinType(resolved) {
				continue
			}
			addNode(resolved)
			edgeSet[refEdgeKey{inst.enclosing, resolved, "new"}] = struct{}{}
		}
	}
}

type instantiation struct {
	enclosing string
	name      string
}

func instantiations(src string, tree *ast.File) []instantiation {
	var found []instantiation
	namespace := ""
	for _, stmt := range tree.Stmts {
		stmts := []ast.Stmt{stmt}
		if ns, ok := stmt.(*ast.Namespace); ok {
			namespace = ""
			if ns.Name != nil {
				namespace = ns.Name.Text(src)
			}
			stmts = ns.Stmts
		}
		for _, s := range stmts {
			found = appendInstantiations(found, src, namespace, "", s)
		}
	}
	return found
}

func appendInstantiations(found []instantiation, src, namespace, enclosing string, n ast.Node) []instantiation {
	switch n := n.(type) {
	case *ast.ClassDecl:
		if n.Name != nil {
			enclosing = qualified(namespace, n.Name.Text(src))
		}
	case *ast.New:
		if class, ok := n.Class.(*ast.ConstFetch); ok && enclosing != "" && n.Anonymous == nil {
			switch text := class.Name.Text(src); strings.ToLower(text) {
			case "self", "static", "parent":
			default:
				found = append(found, instantiation{enclosing: enclosing, name: text})
			}
		}
	}
	ast.Children(n, func(c ast.Node) {
		found = appendInstantiations(found, src, namespace, enclosing, c)
	})
	return found
}

func qualified(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}
