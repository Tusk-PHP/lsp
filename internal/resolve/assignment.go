package resolve

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (vs variableScope) assignedValue(t *syntaxTree, line int, varName string) (ast.Expr, bool) {
	w := vs.write
	if w == nil || line != w.At.Line {
		return t.assignmentStartingLine(line, varName)
	}
	off, ok := t.offset(w.At)
	if !ok {
		return nil, false
	}
	a, ok := t.assignmentAt(off, varName)
	if !ok || !a.statement && !t.startsLine(off) {
		return nil, false
	}
	return a.value, true
}

func (r *Resolver) assignedType(varName string, t *syntaxTree, v assignedValue, lines []string, pos protocol.Position, file *parser.FileNode, line int) (ResolvedType, bool) {
	if v.declared != nil {
		return r.resolveDocType(t.text(v.declared), file), true
	}
	if declared := varDocAbove(lines, line, varName); declared != "" {
		owner := FindEnclosingClass(file, pos)
		if rt := r.resolveDocTypeIn(r.expandTypeAliases(declared, owner, file), owner, file); !rt.IsEmpty() {
			return rt, true
		}
	}
	ctx := ExprContext{Lines: lines, Pos: pos, File: file}
	text := t.text(v.expr)
	if v.called {
		rt := r.InvokeType(r.ExpressionType(text, ctx))
		return rt, !rt.IsEmpty()
	}
	switch n := v.expr.(type) {
	case *ast.Yield:
		if gen := r.enclosingReturnType(lines, pos, file); gen.FQN == "Generator" && len(gen.Params) >= 3 {
			return gen.Params[2], true
		}
	case *ast.Binary:
		if n.Op == token.Dot {
			return ResolvedType{FQN: "string"}, true
		}
	case *ast.ClassConstFetch:
		if strings.EqualFold(t.text(n.Name), "class") {
			if rt := r.ExpressionType(text, ctx); !rt.IsEmpty() {
				return rt, true
			}
		}
	case *ast.Array:
		if spread := spreadOnlyVariable(t, n); spread != "" && spread != varName {
			if rt := r.ResolveVariableTypeTyped(spread, lines, pos, file); !rt.IsEmpty() {
				return rt, true
			}
		}
		if n.Short {
			if rt := InferArrayLiteralType(text); !rt.IsEmpty() {
				return rt, true
			}
		}
	case *ast.New:
		return r.assignedNewType(t, n, ctx)
	case *ast.ArrayDimFetch:
		if origin, dims, ok := variableOffsets(t, n); ok {
			cur := r.ResolveVariableTypeTyped(origin, lines, pos, file)
			for _, d := range dims {
				cur = r.OffsetType(cur, d)
			}
			if !cur.IsEmpty() {
				return cur, true
			}
		}
	case *ast.Variable:
		if text != varName {
			if rt := r.ResolveVariableTypeTyped(text, lines, pos, file); !rt.IsEmpty() {
				return rt, true
			}
		}
	case *ast.Closure, *ast.ArrowFunction:
		return r.ExpressionType(text, ctx), true
	case *ast.Call:
		if name, ok := n.Callee.(*ast.ConstFetch); ok && r.Index != nil {
			if rt := r.functionCallType(t.text(name), t.argsText(n.Args), isCallableArgList(n.Args), t.callArgList(n.Args), ctx); !rt.IsEmpty() {
				return rt, true
			}
		}
	}
	if callee, ok := invokedVariable(t, v.expr); ok && callee != varName {
		if rt := r.ExpressionType(text, ctx); !rt.IsEmpty() {
			return rt, true
		}
	}
	if !isMemberChain(v.expr) {
		return ResolvedType{}, false
	}
	if rt := r.ChainType(text, r.JoinLines(lines), pos, file); !rt.IsEmpty() {
		return rt, true
	}
	if rt := r.syntaxType(t, v.expr, ctx); !rt.IsEmpty() {
		return rt, true
	}
	if r.ChainResolver != nil {
		if fqn := r.ChainResolver(r, text, r.JoinLines(lines), pos, file); fqn != "" {
			return ResolvedType{FQN: fqn}, true
		}
	}
	return ResolvedType{}, false
}

func (r *Resolver) assignedNewType(t *syntaxTree, n *ast.New, ctx ExprContext) (ResolvedType, bool) {
	if n.Anonymous != nil {
		return ResolvedType{}, false
	}
	className := t.text(n.Class)
	args := t.callArgList(n.Args)
	if _, named := n.Class.(*ast.ConstFetch); !named {
		rt := r.newExpressionType(className, args, ctx)
		return rt, !rt.IsEmpty()
	}
	classFQN, isKeyword := r.ClassKeywordFQN(className, ctx.Pos, ctx.File)
	if !isKeyword {
		classFQN = r.ResolveClassName(className, ctx.File)
	}
	if len(args) == 0 {
		return ResolvedType{FQN: classFQN}, true
	}
	if rt := r.newExpressionType(className, args, ctx); rt.FQN == classFQN && len(rt.Params) > 0 {
		return rt, true
	}
	if argRT := r.firstArgType(t, n.Args.Args[0].Value, ctx); !argRT.IsEmpty() && (argRT.IsGeneric() || argRT.Shape != "") {
		if rt := r.inferConstructorGenerics(classFQN, argRT); !rt.IsEmpty() {
			return rt, true
		}
	}
	if rt := r.inferClassStringBinding(classFQN, args, ctx.Lines, ctx.Pos, ctx.File); !rt.IsEmpty() {
		return rt, true
	}
	return ResolvedType{FQN: classFQN}, true
}

func (r *Resolver) firstArgType(t *syntaxTree, arg ast.Expr, ctx ExprContext) ResolvedType {
	switch n := arg.(type) {
	case *ast.Array:
		if n.Short {
			return InferArrayLiteralType(t.text(n))
		}
	case *ast.Variable:
		return r.ResolveVariableTypeTyped(t.text(n), ctx.Lines, ctx.Pos, ctx.File)
	}
	return ResolvedType{}
}

func (r *Resolver) assignedPlainType(varName string, t *syntaxTree, v assignedValue, lines []string, pos protocol.Position, file *parser.FileNode) (string, bool) {
	if v.declared != nil {
		return r.resolveDocType(t.text(v.declared), file).String(), true
	}
	text := t.text(v.expr)
	if v.called {
		return PlainTypeName(r.InvokeType(r.ExpressionType(text, ExprContext{Lines: lines, Pos: pos, File: file}))), true
	}
	switch n := v.expr.(type) {
	case *ast.New:
		if name, ok := n.Class.(*ast.ConstFetch); ok && n.Anonymous == nil {
			className := t.text(name)
			if fqn, isKeyword := r.ClassKeywordFQN(className, pos, file); isKeyword {
				return fqn, true
			}
			return r.ResolveClassName(className, file), true
		}
	case *ast.Variable:
		if text != varName {
			if typ := r.ResolveVariableType(text, lines, pos, file); typ != "" {
				return typ, true
			}
		}
	case *ast.Call:
		if name, ok := n.Callee.(*ast.ConstFetch); ok && r.Index != nil {
			if isCallableArgList(n.Args) {
				return "Closure", true
			}
			if fn := r.CalledFunction(t.text(name), file); fn != nil && fn.ReturnType != "" {
				return fn.ReturnType, true
			}
		}
	}
	if typ := InferLiteralType(text); typ != "" {
		return typ, true
	}
	if r.ChainResolver == nil || !isMemberChain(v.expr) {
		return "", false
	}
	return r.ChainResolver(r, text, r.JoinLines(lines), pos, file), true
}

func variableOffsets(t *syntaxTree, e *ast.ArrayDimFetch) (origin string, dims []string, ok bool) {
	var cur ast.Expr = e
	for {
		switch n := cur.(type) {
		case *ast.ArrayDimFetch:
			if n.Dim == nil {
				return "", nil, false
			}
			dims = append(dims, t.text(n.Dim))
			cur = n.Array
		case *ast.Variable:
			slices.Reverse(dims)
			return t.text(n), dims, true
		default:
			return "", nil, false
		}
	}
}

func invokedVariable(t *syntaxTree, e ast.Expr) (string, bool) {
	for {
		switch n := e.(type) {
		case *ast.Call:
			if v, ok := n.Callee.(*ast.Variable); ok {
				return t.text(v), true
			}
			e = n.Callee
		case *ast.MethodCall:
			e = n.Object
		case *ast.PropertyFetch:
			e = n.Object
		case *ast.ArrayDimFetch:
			e = n.Array
		default:
			return "", false
		}
	}
}
