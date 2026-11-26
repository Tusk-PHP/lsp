package ast

func IsMissing(e Expr) bool {
	b, ok := e.(*BadExpr)
	return ok && b.Start == b.End
}
