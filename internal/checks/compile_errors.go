package checks

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type CompileErrorRule struct {
	Shared *Shared
}

func (r *CompileErrorRule) Codes() []rule.Code { return []rule.Code{rule.CompileError} }

func (r *CompileErrorRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	syntax := r.Shared.tree(source, file.Grammar)
	if len(syntax.tree.Errors) > 0 {
		return nil
	}
	c := &compileChecker{syntax: syntax, version: syntax.tree.Version}
	c.visitChild = c.visit
	c.file(syntax.tree)
	return c.findings
}

type compileChecker struct {
	syntax     *fileSyntax
	version    token.Version
	namespace  string
	leading    []*ast.Declare
	class      *classContext
	fn         *functionScope
	visitChild func(ast.Node)
	findings   []Finding
}

type classKind uint8

const (
	kindClass classKind = iota
	kindInterface
	kindTrait
	kindEnum
)

type classContext struct {
	name     string
	kind     classKind
	abstract bool
	backed   bool
}

func (c *classContext) named() bool {
	return c != nil && c.name != ""
}

type functionScope struct {
	inClass    bool
	returnType ast.Type
	loops      int64
	generator  bool
	returns    []*ast.Return
	labels     map[string]bool
	gotos      []*ast.Goto
}

func (c *compileChecker) report(n ast.Node, message string) {
	c.findings = append(c.findings, c.syntax.at(n, Finding{Severity: rule.SeverityError, Code: rule.CompileError, Message: message}))
}

func (c *compileChecker) text(n ast.Node) string {
	return c.syntax.text(n)
}

func (c *compileChecker) since(v token.Version) bool {
	return c.version >= v
}

func (c *compileChecker) file(tree *ast.File) {
	c.leading = leadingDeclares(tree.Stmts, c.since(token.PHP85))
	c.fn = &functionScope{}
	for _, stmt := range tree.Stmts {
		if ns, ok := stmt.(*ast.Namespace); ok && !ns.Braced {
			c.namespace = c.namespaceName(ns)
			continue
		}
		c.visit(stmt)
	}
	c.closeFunction(c.fn)
}

func leadingDeclares(stmts []ast.Stmt, nopAllowed bool) []*ast.Declare {
	var leading []*ast.Declare
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.Declare:
			leading = append(leading, s)
		case *ast.InlineHTML:
		case *ast.Nop:
			if !nopAllowed {
				return leading
			}
		default:
			return leading
		}
	}
	return leading
}

func (c *compileChecker) namespaceName(ns *ast.Namespace) string {
	if ns.Name == nil {
		return ""
	}
	return c.text(ns.Name)
}

func (c *compileChecker) visit(n ast.Node) {
	switch n := n.(type) {
	case *ast.Namespace:
		outer := c.namespace
		c.namespace = c.namespaceName(n)
		ast.Children(n, c.visitChild)
		c.namespace = outer
		return
	case *ast.ClassDecl:
		c.classDecl(n)
		return
	case *ast.InterfaceDecl:
		c.classLike(n, n.Name, &classContext{kind: kindInterface}, n.Members)
		return
	case *ast.TraitDecl:
		c.classLike(n, n.Name, &classContext{kind: kindTrait}, n.Members)
		return
	case *ast.EnumDecl:
		c.classLike(n, n.Name, &classContext{kind: kindEnum, backed: n.BackingType != nil}, n.Members)
		return
	case *ast.FunctionDecl:
		c.params(n.Params)
		c.inFunction(n, &functionScope{returnType: n.ReturnType})
		return
	case *ast.Closure:
		c.params(n.Params)
		c.closureUses(n)
		c.inFunction(n, &functionScope{inClass: c.fn.inClass, returnType: n.ReturnType})
		return
	case *ast.ArrowFunction:
		c.params(n.Params)
		c.inFunction(n, &functionScope{inClass: c.fn.inClass})
		return
	case *ast.Method:
		c.method(n)
		return
	case *ast.Property:
		c.property(n)
	case *ast.EnumCase:
		c.enumCase(n)
	case *ast.While, *ast.DoWhile, *ast.For, *ast.Switch:
		c.inLoop(n)
		return
	case *ast.Foreach:
		c.reassigned(n.Key)
		c.reassigned(n.Value)
		c.inLoop(n)
		return
	case *ast.Break:
		c.jump(n, n.Levels, "break")
	case *ast.Continue:
		c.jump(n, n.Levels, "continue")
	case *ast.Label:
		c.label(n)
	case *ast.Goto:
		c.fn.gotos = append(c.fn.gotos, n)
	case *ast.Return:
		c.fn.returns = append(c.fn.returns, n)
	case *ast.Yield, *ast.YieldFrom:
		c.fn.generator = true
	case *ast.Declare:
		c.declare(n)
	case *ast.Assign:
		if n.Op == token.Equals || n.Op == token.CoalesceEqual {
			c.reassigned(n.Target)
		}
	case *ast.Catch:
		if n.Var != nil {
			c.reassigned(n.Var)
		}
	case *ast.Unset:
		c.thisIn(n.Vars, "Cannot unset $this")
	case *ast.Global:
		c.thisIn(n.Vars, "Cannot use $this as global variable")
	case *ast.Static:
		for _, v := range n.Vars {
			if c.isThis(v.Var) {
				c.report(v.Var, "Cannot use $this as static variable")
			}
		}
	case *ast.Ternary:
		if c.ternary(n) {
			ast.Children(n.Cond, c.visitChild)
			c.visitAll(n.Then, n.Else)
			return
		}
	case *ast.ArrayDimFetch:
		if n.Braced && c.since(token.PHP80) {
			c.report(n, "Array and string offset access syntax with curly braces is no longer supported")
		}
	case *ast.Cast:
		if n.Kind == token.UnsetCast && c.since(token.PHP80) {
			c.report(n, "The (unset) cast is no longer supported")
		}
	case *ast.Call:
		c.args(n.Args)
	case *ast.MethodCall:
		c.args(n.Args)
	case *ast.StaticCall:
		c.args(n.Args)
	case *ast.New:
		c.args(n.Args)
	case *ast.Attribute:
		c.args(n.Args)
	}
	ast.Children(n, c.visitChild)
}

func (c *compileChecker) visitAll(nodes ...ast.Expr) {
	for _, n := range nodes {
		if n != nil {
			c.visit(n)
		}
	}
}

func (c *compileChecker) inLoop(n ast.Node) {
	c.fn.loops++
	ast.Children(n, c.visitChild)
	c.fn.loops--
}

func (c *compileChecker) inFunction(n ast.Node, fn *functionScope) {
	outer := c.fn
	c.fn = fn
	ast.Children(n, c.visitChild)
	c.closeFunction(fn)
	c.fn = outer
}

func (c *compileChecker) closeFunction(fn *functionScope) {
	for _, g := range fn.gotos {
		if name := c.text(g.Label); !fn.labels[name] {
			c.report(g, "'goto' to undefined label '"+name+"'")
		}
	}
	c.returnsOf(fn)
}

func (c *compileChecker) label(n *ast.Label) {
	name := c.text(n.Name)
	if c.fn.labels[name] {
		c.report(n, "Label '"+name+"' already defined")
		return
	}
	if c.fn.labels == nil {
		c.fn.labels = map[string]bool{}
	}
	c.fn.labels[name] = true
}

func (c *compileChecker) jump(n ast.Stmt, levels ast.Expr, keyword string) {
	depth := int64(1)
	switch lit := levels.(type) {
	case nil:
	case *ast.Literal:
		v, ok := c.integer(lit)
		if !ok || v < 1 {
			c.report(n, "'"+keyword+"' operator accepts only positive integers")
			return
		}
		depth = v
	case *ast.InterpolatedString:
		return
	default:
		c.report(n, "'"+keyword+"' operator with non-integer operand is no longer supported")
		return
	}
	switch {
	case c.fn.loops == 0:
		c.report(n, "'"+keyword+"' not in the 'loop' or 'switch' context")
	case depth > c.fn.loops:
		c.report(n, "Cannot '"+keyword+"' "+strconv.FormatInt(depth, 10)+" levels")
	}
}

func (c *compileChecker) integer(lit *ast.Literal) (int64, bool) {
	if lit.Kind != token.IntegerLiteral {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.ReplaceAll(c.text(lit), "_", ""), 0, 64)
	return v, err == nil
}

func (c *compileChecker) declare(d *ast.Declare) {
	for _, dir := range d.Directives {
		name := c.text(dir.Name)
		switch dir.Value.(type) {
		case *ast.Literal:
		case *ast.MagicConst, *ast.InterpolatedString:
			continue
		default:
			c.report(dir.Value, "declare("+name+") value must be a literal")
			continue
		}
		if !strings.EqualFold(name, "strict_types") {
			continue
		}
		switch {
		case !slices.Contains(c.leading, d):
			c.report(dir, "strict_types declaration must be the very first statement in the script")
		case d.Body != nil:
			c.report(dir, "strict_types declaration must not use block mode")
		default:
			if v, ok := c.integer(dir.Value.(*ast.Literal)); !ok || v != 0 && v != 1 {
				c.report(dir.Value, "strict_types declaration must have 0 or 1 as its value")
			}
		}
	}
}

func (c *compileChecker) ternary(t *ast.Ternary) bool {
	inner, ok := t.Cond.(*ast.Ternary)
	if !ok || !c.since(token.PHP80) || inner.Start != t.Start {
		return false
	}
	switch {
	case inner.Then != nil && t.Then != nil:
		c.report(t, "Unparenthesized `a ? b : c ? d : e` is not supported. Use either `(a ? b : c) ? d : e` or `a ? b : (c ? d : e)`")
	case inner.Then != nil:
		c.report(t, "Unparenthesized `a ? b : c ?: d` is not supported. Use either `(a ? b : c) ?: d` or `a ? b : (c ?: d)`")
	case t.Then != nil:
		c.report(t, "Unparenthesized `a ?: b ? c : d` is not supported. Use either `(a ?: b) ? c : d` or `a ?: (b ? c : d)`")
	default:
		return false
	}
	return true
}

func (c *compileChecker) args(list *ast.ArgList) {
	if list == nil {
		return
	}
	named, unpacked := false, false
	for _, a := range list.Args {
		switch {
		case a.Name != nil:
			if unpacked && c.version == token.PHP80 {
				c.report(a, "Cannot combine named arguments and argument unpacking")
				return
			}
			named = true
		case a.Unpack:
			switch {
			case named && c.version == token.PHP80:
				c.report(a, "Cannot combine named arguments and argument unpacking")
				return
			case named:
				c.report(a, "Cannot use argument unpacking after named arguments")
				return
			}
			unpacked = true
		case named:
			c.report(a, "Cannot use positional argument after named argument")
			return
		case unpacked && !a.Placeholder:
			c.report(a, "Cannot use positional argument after argument unpacking")
			return
		}
	}
}

func (c *compileChecker) isThis(e ast.Expr) bool {
	v, ok := e.(*ast.Variable)
	return ok && v != nil && c.text(v) == "$this"
}

func (c *compileChecker) thisIn(vars []ast.Expr, message string) {
	for _, v := range vars {
		if c.isThis(v) {
			c.report(v, message)
		}
	}
}

func (c *compileChecker) reassigned(target ast.Expr) {
	switch t := target.(type) {
	case *ast.Variable:
		if c.isThis(t) {
			c.report(t, "Cannot re-assign $this")
		}
	case *ast.List:
		for _, item := range t.Items {
			if item != nil && !item.ByRef {
				c.reassigned(item.Value)
			}
		}
	}
}

func (c *compileChecker) params(params []*ast.Param) {
	for i, p := range params {
		name := c.text(p.Var)
		switch {
		case name == "$this":
			c.report(p.Var, "Cannot use $this as parameter")
		case scope.IsAutoGlobal(name):
			c.report(p.Var, "Cannot re-assign auto-global variable "+name[1:])
		case slices.ContainsFunc(params[:i], func(q *ast.Param) bool { return c.text(q.Var) == name }):
			c.report(p.Var, "Redefinition of parameter "+name)
		}
		if p.Variadic && i != len(params)-1 {
			c.report(p, "Only the last parameter can be variadic")
		}
		if p.Variadic && p.Default != nil {
			c.report(p, "Variadic parameter cannot have a default value")
		}
	}
}

func (c *compileChecker) closureUses(n *ast.Closure) {
	for i, u := range n.Uses {
		name := c.text(u.Var)
		switch {
		case name == "$this":
			c.report(u.Var, "Cannot use $this as lexical variable")
		case scope.IsAutoGlobal(name):
			c.report(u.Var, "Cannot use auto-global as lexical variable")
		case slices.ContainsFunc(n.Uses[:i], func(v *ast.ClosureUse) bool { return c.text(v.Var) == name }):
			c.report(u.Var, "Cannot use variable "+name+" twice")
		case slices.ContainsFunc(n.Params, func(p *ast.Param) bool { return c.text(p.Var) == name }):
			c.report(u.Var, "Cannot use lexical variable "+name+" as a parameter name")
		}
	}
}

const (
	voidReturnsNull   = ` (did you mean "return;" instead of "return null;"?)`
	typedReturnsEmpty = ` (did you mean "return null;" instead of "return;"?)`
)

func (c *compileChecker) returnsOf(fn *functionScope) {
	if fn.returnType == nil || fn.generator || len(fn.returns) == 0 {
		return
	}
	noun := "function"
	if fn.inClass && c.since(token.PHP84) {
		noun = "method"
	}
	switch name := c.namedType(fn.returnType); {
	case name == "void":
		for _, r := range fn.returns {
			switch {
			case r.Value == nil:
			case c.isNull(r.Value):
				c.report(r, "A void "+noun+" must not return a value"+voidReturnsNull)
			default:
				c.report(r, "A void "+noun+" must not return a value")
			}
		}
	case name == "never" && c.since(token.PHP81):
		for _, r := range fn.returns {
			c.report(r, "A never-returning "+noun+" must not return")
		}
	case name == "null", name == "false", name == "true":
	case !fn.inClass && (name == "static" || name == "self" || name == "parent"):
	default:
		suffix := ""
		if c.allowsNull(fn.returnType) {
			suffix = typedReturnsEmpty
		}
		for _, r := range fn.returns {
			if r.Value == nil {
				c.report(r, "A "+noun+" with return type must return a value"+suffix)
			}
		}
	}
}

func (c *compileChecker) namedType(t ast.Type) string {
	named, ok := t.(*ast.NamedType)
	if !ok {
		return ""
	}
	return strings.ToLower(c.text(named.Name))
}

func (c *compileChecker) allowsNull(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.NullableType:
		return true
	case *ast.UnionType:
		return slices.ContainsFunc(t.Types, func(m ast.Type) bool { return c.namedType(m) == "null" })
	}
	return c.namedType(t) == "mixed" && c.since(token.PHP80)
}

func (c *compileChecker) isNull(e ast.Expr) bool {
	fetch, ok := e.(*ast.ConstFetch)
	if !ok {
		return false
	}
	name := strings.ToLower(c.text(fetch.Name))
	return name == "null" || name == `\null`
}
