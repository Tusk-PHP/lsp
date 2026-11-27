package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func expressionTree(expr string, grammar token.Version) (*syntaxTree, ast.Expr) {
	src := strings.TrimSpace(expr)
	if src == "" {
		return nil, nil
	}
	file := parser.ParseTree(src+";", grammar)
	if len(file.Stmts) == 0 {
		return nil, nil
	}
	stmt, ok := file.Stmts[0].(*ast.ExprStmt)
	if !ok || stmt.Expr == nil {
		return nil, nil
	}
	return &syntaxTree{src: src + ";", grammar: grammar, lines: ast.NewLines(src)}, stmt.Expr
}

func (r *Resolver) exprType(expr string, ctx ExprContext) ResolvedType {
	return r.exprTypeExpecting(expr, nil, ctx)
}

func (r *Resolver) exprTypeExpecting(expr string, expected *ResolvedType, ctx ExprContext) ResolvedType {
	t, e := expressionTree(expr, ctx.grammar())
	return r.typeOf(t, e, expected, ctx)
}

func (r *Resolver) chainTypeOf(expr, source string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	return r.exprType(expr, ExprContext{Lines: r.Lines(source), Pos: pos, File: file})
}

func (r *Resolver) callSiteFor(argStr string, ctx ExprContext) *CallSite {
	t, e := expressionTree("f("+argStr+")", ctx.grammar())
	call, ok := e.(*ast.Call)
	if !ok {
		return &CallSite{}
	}
	return r.callSite(t.callArgList(call.Args), ctx)
}

func (r *Resolver) functionCallTypeOf(name, args string, ctx ExprContext) ResolvedType {
	t, e := expressionTree("f("+args+")", ctx.grammar())
	call, ok := e.(*ast.Call)
	if !ok {
		return ResolvedType{}
	}
	return r.functionCallType(name, t.callArgList(call.Args), ctx)
}

func (r *Resolver) newExpressionTypeOf(className, args string, ctx ExprContext) ResolvedType {
	t, e := expressionTree("new "+className+"("+args+")", ctx.grammar())
	n, ok := e.(*ast.New)
	if !ok {
		return ResolvedType{}
	}
	return r.newType(t, n, ctx)
}
