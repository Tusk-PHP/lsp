package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type syntaxTree struct {
	src     string
	grammar token.Version
	lines   ast.Lines
	assigns map[uint32]assignment
}

type assignment struct {
	name      string
	value     ast.Expr
	statement bool
}

func newSyntaxTree(src string, tree *ast.File) *syntaxTree {
	t := &syntaxTree{src: src, grammar: tree.Version, lines: ast.NewLines(src), assigns: make(map[uint32]assignment)}
	ast.Walk(tree, t.collect)
	return t
}

func (r *Resolver) syntaxTree(lines []string, file *parser.FileNode) *syntaxTree {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.cachedLines == nil || !sameLines(r.cachedLines, lines) {
		r.setSourceLocked(strings.Join(lines, "\n"), lines)
	}
	if r.scopeTree != nil && r.scopeTree.grammar == file.Grammar {
		return r.scopeTree
	}
	tree := file.Tree
	if tree == nil || file.Source != r.scopeSource {
		tree = parser.ParseTree(r.scopeSource, file.Grammar)
	}
	r.scopeTree = newSyntaxTree(r.scopeSource, tree)
	return r.scopeTree
}

func (t *syntaxTree) collect(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.ExprStmt:
		for e := n.Expr; ; {
			a, ok := e.(*ast.Assign)
			if !ok || !t.add(a, true) {
				break
			}
			e = a.Value
		}
	case *ast.Assign:
		if _, seen := t.assigns[n.Target.Bounds().Start]; !seen {
			t.add(n, false)
		}
	case *ast.Static:
		if len(n.Vars) > 0 && n.Vars[0].Default != nil {
			t.assigns[n.Start] = assignment{name: t.text(n.Vars[0].Var), value: n.Vars[0].Default, statement: true}
		}
		for _, v := range n.Vars {
			if v.Default != nil {
				t.assigns[v.Var.Start] = assignment{name: t.text(v.Var), value: v.Default, statement: true}
			}
		}
	}
	return true
}

func (t *syntaxTree) add(a *ast.Assign, statement bool) bool {
	v, ok := a.Target.(*ast.Variable)
	if !ok || a.Op != token.Equals && a.Op != token.CoalesceEqual {
		return false
	}
	t.assigns[v.Start] = assignment{name: t.text(v), value: a.Value, statement: statement}
	return true
}

func (t *syntaxTree) text(n ast.Node) string {
	return n.Bounds().Text(t.src)
}

func (t *syntaxTree) offset(p textpos.Position) (uint32, bool) {
	if p.Line < 0 || p.Line >= len(t.lines) || p.Character < 0 {
		return 0, false
	}
	off := t.lines[p.Line] + uint32(p.Character)
	if int(off) > len(t.src) {
		return 0, false
	}
	return off, true
}

func (t *syntaxTree) assignmentAt(off uint32, name string) (assignment, bool) {
	a, ok := t.assigns[off]
	return a, ok && a.name == name
}

func (t *syntaxTree) assignmentStartingLine(line int, name string) (ast.Expr, bool) {
	off, ok := t.offset(textpos.Position{Line: line})
	if !ok {
		return nil, false
	}
	a, ok := t.assignmentAt(t.indented(off), name)
	return a.value, ok
}

func (t *syntaxTree) startsLine(off uint32) bool {
	line, _ := t.lines.Position(off)
	return t.indented(t.lines[line]) == off
}

func (t *syntaxTree) indented(lineStart uint32) uint32 {
	off := lineStart
	for int(off) < len(t.src) && (t.src[off] == ' ' || t.src[off] == '\t') {
		off++
	}
	return off
}

type assignedValue struct {
	expr     ast.Expr
	called   bool
	declared ast.Type
}

func (t *syntaxTree) reduce(e ast.Expr) assignedValue {
	switch n := e.(type) {
	case *ast.Binary:
		switch {
		case n.Op == token.Pipe:
			return t.pipeStage(n.Right)
		case n.Op == token.Coalesce && isThrow(n.Right):
			return t.reduce(n.Left)
		}
	case *ast.Ternary:
		if n.Then == nil && isThrow(n.Else) {
			return t.reduce(n.Cond)
		}
	case *ast.Clone:
		if n.Operand != nil {
			return t.reduce(n.Operand)
		}
		if len(n.Args.Args) > 0 && n.Args.Args[0].Value != nil {
			return t.reduce(n.Args.Args[0].Value)
		}
	}
	return assignedValue{expr: e}
}

func (t *syntaxTree) pipeStage(stage ast.Expr) assignedValue {
	switch n := stage.(type) {
	case *ast.ArrowFunction:
		if n.ReturnType != nil {
			return assignedValue{declared: n.ReturnType}
		}
		return t.reduce(n.Body)
	case *ast.Closure:
		if n.ReturnType != nil {
			return assignedValue{declared: n.ReturnType}
		}
	}
	if args := callArgs(stage); args != nil && isFirstClassCallable(args) {
		return assignedValue{expr: stage, called: true}
	}
	return assignedValue{expr: stage}
}

func isThrow(e ast.Expr) bool {
	_, ok := e.(*ast.Throw)
	return ok
}

func callArgs(e ast.Expr) *ast.ArgList {
	switch n := e.(type) {
	case *ast.Call:
		return n.Args
	case *ast.MethodCall:
		return n.Args
	case *ast.StaticCall:
		return n.Args
	case *ast.New:
		return n.Args
	}
	return nil
}

func isFirstClassCallable(args *ast.ArgList) bool {
	return args.RestPlaceholder && len(args.Args) == 0
}

func isCallableArgList(args *ast.ArgList) bool {
	if args.RestPlaceholder {
		return true
	}
	for _, a := range args.Args {
		if a.Placeholder {
			return true
		}
	}
	return false
}

func (t *syntaxTree) argsText(args *ast.ArgList) string {
	if args == nil {
		return ""
	}
	var text string
	if n := len(args.Args); n > 0 {
		text = t.src[args.Args[0].Start:args.Args[n-1].End]
	}
	if !args.RestPlaceholder {
		return text
	}
	if text == "" {
		return "..."
	}
	return text + ", ..."
}

func (t *syntaxTree) callArgList(args *ast.ArgList) []CallArg {
	if args == nil || len(args.Args) == 0 {
		return nil
	}
	out := make([]CallArg, 0, len(args.Args))
	for _, a := range args.Args {
		if a.Value == nil {
			continue
		}
		arg := CallArg{Expr: t.text(a.Value), Spread: a.Unpack}
		if a.Name != nil {
			arg.Name = t.text(a.Name)
		}
		out = append(out, arg)
	}
	return out
}

func spreadOnlyVariable(t *syntaxTree, e ast.Expr) string {
	arr, ok := e.(*ast.Array)
	if !ok || len(arr.Items) == 0 {
		return ""
	}
	for _, item := range arr.Items {
		if item == nil || !item.Unpack || item.Key != nil {
			return ""
		}
		if _, ok := item.Value.(*ast.Variable); !ok {
			return ""
		}
	}
	return t.text(arr.Items[0].Value)
}

func isMemberChain(e ast.Expr) bool {
	for {
		switch n := e.(type) {
		case *ast.MethodCall, *ast.PropertyFetch, *ast.StaticCall, *ast.StaticPropertyFetch, *ast.ClassConstFetch:
			return true
		case *ast.ArrayDimFetch:
			e = n.Array
		case *ast.Call:
			e = n.Callee
		default:
			return false
		}
	}
}

func (r *Resolver) syntaxType(t *syntaxTree, e ast.Expr, ctx ExprContext) ResolvedType {
	if ctx.depth > maxExprDepth {
		return ResolvedType{}
	}
	ctx.depth++
	switch n := e.(type) {
	case *ast.Variable:
		return r.chainOriginType(chainOrigin{kind: '$', text: t.text(n)}, ctx)
	case *ast.New:
		if n.Anonymous != nil {
			return ResolvedType{}
		}
		return r.newExpressionType(t.text(n.Class), t.callArgList(n.Args), ctx)
	case *ast.Call:
		if name, ok := n.Callee.(*ast.ConstFetch); ok {
			return r.functionCallType(t.text(name), t.argsText(n.Args), isCallableArgList(n.Args), t.callArgList(n.Args), ctx)
		}
		return r.chainStepType(r.syntaxType(t, n.Callee, ctx), t.callStep("()", "", n.Args), ctx)
	case *ast.MethodCall:
		return r.memberStep(t, n.Object, "->", n.Name, n.Args, ctx)
	case *ast.PropertyFetch:
		return r.memberStep(t, n.Object, "->", n.Name, nil, ctx)
	case *ast.StaticCall:
		return r.memberStep(t, n.Class, "::", n.Name, n.Args, ctx)
	case *ast.StaticPropertyFetch:
		return r.memberStep(t, n.Class, "::", n.Name, nil, ctx)
	case *ast.ClassConstFetch:
		return r.memberStep(t, n.Class, "::", n.Name, nil, ctx)
	case *ast.ArrayDimFetch:
		if n.Dim == nil {
			return ResolvedType{}
		}
		cur := r.syntaxType(t, n.Array, ctx)
		if cur.IsEmpty() {
			return ResolvedType{}
		}
		return r.OffsetType(cur, t.text(n.Dim))
	case *ast.ConstFetch:
		return r.chainOriginType(chainOrigin{kind: 'c', text: t.text(n)}, ctx)
	}
	return r.ExpressionType(t.text(e), ctx)
}

func (r *Resolver) memberStep(t *syntaxTree, receiver ast.Expr, op string, name ast.Expr, args *ast.ArgList, ctx ExprContext) ResolvedType {
	if _, ok := name.(*ast.IdentifierExpr); !ok && op == "->" {
		return ResolvedType{}
	}
	cur := r.syntaxType(t, receiver, ctx)
	if cur.IsEmpty() {
		return ResolvedType{}
	}
	if args == nil {
		return r.chainStepType(cur, chainStep{op: op, name: t.text(name)}, ctx)
	}
	return r.chainStepType(cur, t.callStep(op, t.text(name), args), ctx)
}

func (t *syntaxTree) callStep(op, name string, args *ast.ArgList) chainStep {
	return chainStep{op: op, name: name, call: true, args: t.argsText(args), callArgs: t.callArgList(args), parsedArgs: true}
}
