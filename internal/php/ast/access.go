package ast

func MemberAccess(n Node) (receiver, name Expr, static bool) {
	switch n := n.(type) {
	case *MethodCall:
		return n.Object, n.Name, false
	case *PropertyFetch:
		return n.Object, n.Name, false
	case *StaticCall:
		return n.Class, n.Name, true
	case *StaticPropertyFetch:
		return n.Class, n.Name, true
	case *ClassConstFetch:
		return n.Class, n.Name, true
	}
	return nil, nil, false
}

func IsMissing(e Expr) bool {
	b, ok := e.(*BadExpr)
	return ok && b.Start == b.End
}

func MemberAccessAt(root Node, off uint32) Expr {
	var best Expr
	var visit func(n Node)
	visit = func(n Node) {
		span := n.Bounds()
		if off < span.Start || off > span.End {
			return
		}
		if recv, name, _ := MemberAccess(n); recv != nil && recv.Bounds().End < off && (off <= name.Bounds().End || IsMissing(name)) {
			if best == nil || span.End-span.Start <= best.Bounds().End-best.Bounds().Start {
				best = n.(Expr)
			}
		}
		Children(n, visit)
	}
	visit(root)
	return best
}
