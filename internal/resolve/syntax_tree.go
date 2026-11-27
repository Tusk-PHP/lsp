package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type syntaxTree struct {
	root      *ast.File
	src       string
	grammar   token.Version
	lines     ast.Lines
	assigns   map[uint32]assignment
	foreaches []*ast.Foreach
}

type assignment struct {
	name  string
	value ast.Expr
}

func newSyntaxTree(src string, tree *ast.File) *syntaxTree {
	t := &syntaxTree{root: tree, src: src, grammar: tree.Version, lines: ast.NewLines(src), assigns: make(map[uint32]assignment)}
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
	case *ast.Foreach:
		t.foreaches = append(t.foreaches, n)
	case *ast.Assign:
		if v, ok := n.Target.(*ast.Variable); ok && (n.Op == token.Equals || n.Op == token.CoalesceEqual) {
			t.assigns[v.Start] = assignment{name: t.text(v), value: n.Value}
		}
	case *ast.Static:
		if len(n.Vars) > 0 && n.Vars[0].Default != nil {
			t.assigns[n.Start] = assignment{name: t.text(n.Vars[0].Var), value: n.Vars[0].Default}
		}
		for _, v := range n.Vars {
			if v.Default != nil {
				t.assigns[v.Var.Start] = assignment{name: t.text(v.Var), value: v.Default}
			}
		}
	}
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

func (t *syntaxTree) callArgList(args *ast.ArgList) []CallArg {
	if args == nil {
		return nil
	}
	out := make([]CallArg, 0, len(args.Args)+1)
	for _, a := range args.Args {
		arg := CallArg{Spread: a.Unpack, node: a.Value, tree: t}
		switch {
		case a.Placeholder:
			arg.Expr = "?"
		case a.Value != nil:
			arg.Expr = t.text(a.Value)
		default:
			continue
		}
		if a.Name != nil {
			arg.Name = t.text(a.Name)
		}
		out = append(out, arg)
	}
	if args.RestPlaceholder {
		out = append(out, CallArg{Spread: true, tree: t})
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
