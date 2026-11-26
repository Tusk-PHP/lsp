package resolve

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func (vs variableScope) assignedValue(t *syntaxTree, line int, varName string) (ast.Expr, bool) {
	w := vs.write
	if w == nil || line != w.At.Line {
		return t.assignmentStartingLine(line, varName)
	}
	off := t.offset(w.At)
	a, ok := t.assignmentAt(off, varName)
	if !ok {
		return nil, false
	}
	return a.value, true
}

func (r *Resolver) AssignedValue(file *parser.FileNode, v *ast.Variable) (ast.Expr, bool) {
	lines := r.Lines(file.Source)
	t := r.syntaxTree(lines, file)
	name := t.text(v)
	line, col := t.lines.Position(v.Start)
	vs := r.variableScopeAt(name, lines, textpos.Position{Line: line, Character: col})
	b, reach := vs.binding, vs.reach
	if b == nil || reach.OnWrite || !reach.Single() || reach.Latest == scope.NoWrite {
		return nil, false
	}
	site := b.Site(reach.Latest)
	return writeScope(b, reach.Latest, &site).assignedValue(t, site.At.Line, name)
}

func (r *Resolver) assignedType(varName string, t *syntaxTree, v assignedValue, lines []string, pos textpos.Position, file *parser.FileNode) (ResolvedType, bool) {
	if rt, ok := r.documentedAssignment(varName, t, v, pos, file); ok {
		return rt, true
	}
	ctx := ExprContext{Lines: lines, Pos: pos, File: file}
	text := t.text(v.expr)
	if v.called {
		rt := r.InvokeType(r.typeOf(t, v.expr, nil, ctx))
		return rt, !rt.IsEmpty()
	}
	switch n := v.expr.(type) {
	case *ast.Yield:
		if gen := r.enclosingReturnType(lines, pos, file); gen.FQN == "Generator" && len(gen.Params) >= 3 {
			return gen.Params[2], true
		}
	case *ast.Binary:
		switch n.Op {
		case token.Dot:
			return ResolvedType{FQN: "string"}, true
		case token.Coalesce:
			rt := r.eitherType(t, n.Left, n.Right, ctx)
			return rt, !rt.IsEmpty()
		}
	case *ast.Ternary:
		rt := r.ternaryType(t, n, ctx)
		return rt, !rt.IsEmpty()
	case *ast.ClassConstFetch:
		if strings.EqualFold(t.text(n.Name), "class") {
			if rt := r.typeOf(t, n, nil, ctx); !rt.IsEmpty() {
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
			if rt := literalArrayType(t, n); !rt.IsEmpty() {
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
		return r.typeOf(t, n, nil, ctx), true
	case *ast.Call:
		if rt, ok := r.containerCallType(t, n, ctx); ok {
			return rt, !rt.IsEmpty()
		}
		if name, ok := n.Callee.(*ast.ConstFetch); ok && r.Index != nil {
			if rt := r.functionCallType(t.text(name), t.callArgList(n.Args), ctx); !rt.IsEmpty() {
				return rt, true
			}
		}
	}
	if callee, ok := invokedVariable(t, v.expr); ok && callee != varName {
		if rt := r.typeOf(t, v.expr, nil, ctx); !rt.IsEmpty() {
			return rt, true
		}
	}
	if !isMemberChain(v.expr) {
		return ResolvedType{}, false
	}
	rt := r.typeOf(t, v.expr, nil, ctx)
	return rt, !rt.IsEmpty()
}

func (r *Resolver) assignedNewType(t *syntaxTree, n *ast.New, ctx ExprContext) (ResolvedType, bool) {
	if n.Anonymous != nil {
		return ResolvedType{}, false
	}
	className := t.text(n.Class)
	args := t.callArgList(n.Args)
	if _, named := n.Class.(*ast.ConstFetch); !named {
		rt := r.newType(t, n, ctx)
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
	if argRT := r.typeOf(t, n.Args.Args[0].Value, nil, ctx); !argRT.IsEmpty() && (argRT.IsGeneric() || argRT.Shape != "") {
		if rt := r.inferConstructorGenerics(classFQN, argRT); !rt.IsEmpty() {
			return rt, true
		}
	}
	return ResolvedType{FQN: classFQN}, true
}

// documentedAssignment returns the type an assignment declares: the closure
// return type it was reduced from, or the @var of its statement's doc comment.
func (r *Resolver) documentedAssignment(varName string, t *syntaxTree, v assignedValue, pos textpos.Position, file *parser.FileNode) (ResolvedType, bool) {
	if v.declared != nil {
		return r.resolveDocType(t.text(v.declared), file), true
	}
	if declared := t.statementVarDoc(v.expr.Bounds().Start, varName); declared != "" {
		if rt := r.declaredVarType(declared, pos, file); !rt.IsEmpty() {
			return rt, true
		}
	}
	return ResolvedType{}, false
}

func (r *Resolver) assignedPlainType(t *syntaxTree, v assignedValue) (string, bool) {
	if typ := literalNodeType(t, v.expr); typ != "" {
		return typ, true
	}
	_, ternary := v.expr.(*ast.Ternary)
	coalesce, _ := v.expr.(*ast.Binary)
	return "", v.called || ternary || coalesce != nil && coalesce.Op == token.Coalesce || isMemberChain(v.expr)
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

func (r *Resolver) ternaryType(t *syntaxTree, n *ast.Ternary, ctx ExprContext) ResolvedType {
	if n.Then == nil {
		return r.eitherType(t, n.Cond, n.Else, ctx)
	}
	then, els := r.typeOf(t, n.Then, nil, ctx), r.typeOf(t, n.Else, nil, ctx)
	if then.IsEmpty() && !then.Nullable || els.IsEmpty() && !els.Nullable {
		return ResolvedType{}
	}
	return MakeUnion([]ResolvedType{then, els}, false)
}

func (r *Resolver) eitherType(t *syntaxTree, left, right ast.Expr, ctx ExprContext) ResolvedType {
	l, rt := r.typeOf(t, left, nil, ctx), r.typeOf(t, right, nil, ctx)
	if l.IsEmpty() && !l.Nullable || rt.IsEmpty() && !rt.Nullable {
		return ResolvedType{}
	}
	return MakeUnion([]ResolvedType{withoutNull(l), rt}, false)
}

func withoutNull(rt ResolvedType) ResolvedType {
	out := MakeUnion([]ResolvedType{rt}, false)
	out.Nullable = false
	return out
}
