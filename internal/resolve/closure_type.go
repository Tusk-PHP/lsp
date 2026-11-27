package resolve

import "github.com/Tusk-PHP/lsp/internal/php/ast"

type closureNode struct {
	params []*ast.Param
	ret    ast.Type
	arrow  ast.Expr
	body   *ast.Block
}

func closureOf(n *ast.Closure) closureNode {
	return closureNode{params: n.Params, ret: n.ReturnType, body: n.Body}
}

func arrowOf(n *ast.ArrowFunction) closureNode {
	return closureNode{params: n.Params, ret: n.ReturnType, arrow: n.Body}
}

// closureType types a closure or arrow function as Closure(params): return.
// Untyped parameters take their types from expected, the callable type the
// closure is passed for; the return type is the declared one or, without
// one, what the arrow function's body or the first return statement gives.
func (r *Resolver) closureType(t *syntaxTree, c closureNode, expected *ResolvedType, ctx ExprContext) ResolvedType {
	var expParams []ResolvedType
	if expected != nil {
		if sig := callableSignature(*expected); sig != nil {
			expParams = sig.Params
		}
	}
	sig := &CallableType{}
	locals := make(map[string]ResolvedType, len(ctx.Locals)+len(c.params))
	for k, v := range ctx.Locals {
		locals[k] = v
	}
	for i, p := range c.params {
		var pt ResolvedType
		switch {
		case p.Type != nil:
			pt = r.resolveDocType(t.text(p.Type), ctx.File)
		case i < len(expParams):
			pt = expParams[i]
		}
		sig.Params = append(sig.Params, pt)
		if p.Var != nil && !pt.IsEmpty() {
			locals[t.text(p.Var)] = pt
		}
	}
	inner := ctx
	inner.Locals = locals
	var ret ResolvedType
	switch {
	case c.ret != nil:
		ret = r.resolveDocType(t.text(c.ret), ctx.File)
	case c.arrow != nil:
		ret = r.typeOf(t, c.arrow, nil, inner)
	case c.body != nil:
		ret = r.typeOf(t, firstReturn(c.body), nil, inner)
	}
	if !ret.IsEmpty() || ret.Nullable {
		sig.Return = &ret
	}
	return ResolvedType{FQN: "Closure", Callable: sig}
}

func firstReturn(body *ast.Block) ast.Expr {
	for _, stmt := range body.Stmts {
		if ret, ok := stmt.(*ast.Return); ok && ret.Value != nil {
			return ret.Value
		}
	}
	return nil
}
