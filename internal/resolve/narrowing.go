package resolve

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Narrowing: what calls between a variable's last assignment and the cursor
// say about it, following PHPStan (https://phpstan.org/writing-php-code/narrowing-types):
//
//   - @phpstan-assert Type $param after a call that passes the variable:
//     PHPUnit's assertInstanceOf() and webmozart/assert work this way;
//   - @phpstan-assert-if-true and -if-false inside an if on the call, and
//     after an early return guarded by it;
//   - @param-out Type $param after a call that passes the variable by
//     reference;
//   - @phpstan-self-out and @phpstan-this-out on the object a method is
//     called on.
//
// `!Type` removes a type, `=Type` sets it exactly.

// maxNarrowingStatements bounds how many calls before the cursor are read
// for one variable.
const maxNarrowingStatements = 24

// narrowingFact is what one call says about the variable.
type narrowingFact struct {
	typ      ResolvedType
	negated  bool
	equality bool
}

// narrowVariable applies to t what the code between the variable's last
// assignment and pos says about varName: calls that assert it, early-leaving
// guards, the if, elseif, else and while blocks pos is in, a `switch (true)`
// case, and tests before `&&`, `||`, `?` and a `match (true)` arm in the
// read's own statement. A test on an assignment, `($r = f()) instanceof X`,
// is a test on the variable assigned.
func (r *Resolver) narrowVariable(varName string, t ResolvedType, lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	if file == nil || r.Index == nil || pos.Line >= len(lines) {
		return t
	}
	n, path, ok := r.narrowingAt(varName, lines, pos, file)
	if !ok {
		return t
	}
	// Narrowing doesn't nest: the calls it reads type their receivers and
	// arguments without narrowing them in turn, which keeps it linear.
	exit, entered := r.enterResolution("narrowing")
	if !entered {
		return t
	}
	defer exit()

	for _, stmt := range n.statementsBefore(path) {
		t = n.applyStatement(t, stmt)
	}
	t = n.enclosingConditions(t, path)
	t = n.switchCase(t, path)
	return n.statementTests(t, path)
}

func (r *Resolver) mayBeNarrowed(varName string, lines []string, pos protocol.Position, file *parser.FileNode) bool {
	if file == nil || r.Index == nil || pos.Line >= len(lines) {
		return false
	}
	_, _, ok := r.narrowingAt(varName, lines, pos, file)
	return ok
}

func (r *Resolver) narrowingAt(varName string, lines []string, pos protocol.Position, file *parser.FileNode) (narrowing, []ast.Node, bool) {
	tree := r.syntaxTree(lines, file)
	off, ok := tree.offset(pos)
	if !ok {
		return narrowing{}, nil, false
	}
	after, ok := r.lastWrite(varName, lines, tree, pos, off)
	if !ok {
		return narrowing{}, nil, false
	}
	path := tree.path(off)
	n := narrowing{r: r, t: tree, varName: varName, file: file, lines: lines, after: after, off: off}
	return n, path, n.mayNarrow(path)
}

type narrowing struct {
	r       *Resolver
	t       *syntaxTree
	varName string
	file    *parser.FileNode
	lines   []string
	after   uint32
	off     uint32
}

func (n narrowing) ctxAt(e ast.Node, known map[string]ResolvedType) ExprContext {
	line, col := n.t.lines.Position(e.Bounds().Start)
	return ExprContext{Lines: n.lines, Pos: protocol.Position{Line: line, Character: col}, File: n.file, Locals: known}
}

func (r *Resolver) lastWrite(varName string, lines []string, t *syntaxTree, pos protocol.Position, off uint32) (uint32, bool) {
	s := r.scopeDocument(lines).ScopeAt(pos)
	for ; s != nil; s = s.Parent {
		captured := false
		for _, b := range s.Bindings {
			if b.Name != varName {
				continue
			}
			if b.Kind == scope.BindingClosureUse && rangeHolds(b.Decl, pos) {
				captured = true
				break
			}
			last, ok := t.offset(b.Decl.Start)
			if b.Kind == scope.BindingParameter {
				last, ok = t.offset(s.Range.Start)
			}
			if !ok {
				return 0, false
			}
			for _, w := range b.Writes {
				at, found := t.offset(w.Start)
				if !found || at >= off || at <= last || t.writeHolds(at, off) {
					continue
				}
				last = at
			}
			if last > off {
				return 0, false
			}
			return last, true
		}
		if captured {
			continue
		}
		if s.Kind != scope.ScopeClosure || !s.ExprBody {
			start, ok := t.offset(s.Range.Start)
			return start, ok
		}
	}
	return 0, false
}

func rangeHolds(r protocol.Range, pos protocol.Position) bool {
	return !positionBefore(pos, r.Start) && !positionBefore(r.End, pos)
}

func (t *syntaxTree) writeHolds(at, off uint32) bool {
	a, ok := t.assigns[at]
	if !ok {
		return false
	}
	span := a.value.Bounds()
	return span.Start <= off && off <= span.End
}

func (t *syntaxTree) path(off uint32) []ast.Node {
	path := []ast.Node{t.root}
	for {
		var next ast.Node
		ast.Children(path[len(path)-1], func(c ast.Node) {
			if next != nil {
				return
			}
			if span := c.Bounds(); span.Start <= off && off <= span.End && span.End > span.Start {
				next = c
			}
		})
		if next == nil {
			return path
		}
		path = append(path, next)
	}
}

func isFunctionLike(n ast.Node) bool {
	switch n.(type) {
	case *ast.Closure, *ast.ArrowFunction, *ast.FunctionDecl, *ast.Method, *ast.ClassDecl, *ast.InterfaceDecl, *ast.TraitDecl, *ast.EnumDecl:
		return true
	}
	return false
}

func innerPath(path []ast.Node) []ast.Node {
	for i := len(path) - 1; i >= 0; i-- {
		if !isFunctionLike(path[i]) {
			continue
		}
		if i+1 == len(path) {
			continue
		}
		if _, capture := path[i+1].(*ast.ClosureUse); capture {
			continue
		}
		return path[i+1:]
	}
	return path
}

func (n narrowing) mayNarrow(path []ast.Node) bool {
	inner := innerPath(path)
	if len(inner) == 0 {
		return false
	}
	found := false
	ast.Walk(inner[0], func(node ast.Node) bool {
		if found {
			return false
		}
		span := node.Bounds()
		if span.End <= n.after || span.Start > n.off {
			return false
		}
		switch node := node.(type) {
		case *ast.InstanceOf:
			found = true
		case *ast.Call:
			if name, ok := node.Callee.(*ast.ConstFetch); ok && n.narrowsByName(n.t.text(name)) {
				found = true
			}
		case *ast.MethodCall:
			found = n.narrowsByName(n.t.text(node.Name))
		case *ast.StaticCall:
			found = n.narrowsByName(n.t.text(node.Name))
		}
		return true
	})
	return found
}

func (n narrowing) narrowsByName(name string) bool {
	if i := strings.LastIndexByte(name, '\\'); i >= 0 {
		name = name[i+1:]
	}
	return strings.EqualFold(name, "assertInstanceOf") || strings.EqualFold(name, "assert") || n.r.Index.MayNarrow(name)
}

func (n narrowing) statementsBefore(path []ast.Node) []ast.Stmt {
	inner := innerPath(path)
	var out []ast.Stmt
	for i, node := range inner {
		var stmts []ast.Stmt
		switch node := node.(type) {
		case *ast.Block:
			stmts = node.Stmts
		case *ast.Case:
			stmts = node.Stmts
		case *ast.Namespace:
			stmts = node.Stmts
		case *ast.File:
			stmts = node.Stmts
		default:
			continue
		}
		var child ast.Node
		if i+1 < len(inner) {
			child = inner[i+1]
		}
		for _, stmt := range stmts {
			if ast.Node(stmt) == child || stmt.Bounds().Start >= n.off {
				break
			}
			if stmt.Bounds().End > n.after {
				out = append(out, stmt)
			}
		}
	}
	slices.SortFunc(out, func(a, b ast.Stmt) int { return int(a.Bounds().Start) - int(b.Bounds().Start) })
	if len(out) > maxNarrowingStatements {
		out = out[len(out)-maxNarrowingStatements:]
	}
	return out
}

func (n narrowing) applyStatement(t ResolvedType, stmt ast.Stmt) ResolvedType {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		if call, ok := s.Expr.(*ast.Call); ok && isAssertCall(n.t, call) {
			return n.applyCondition(t, call.Args.Args[0].Value, true, false)
		}
		switch s.Expr.(type) {
		case *ast.Call, *ast.MethodCall, *ast.StaticCall:
			return n.applyCall(t, s.Expr)
		}
	case *ast.If:
		if len(s.ElseIfs) == 0 && s.Else == nil && alwaysLeaves(s.Then) {
			return n.applyCondition(t, s.Cond, false, false)
		}
	}
	return t
}

func isAssertCall(t *syntaxTree, call *ast.Call) bool {
	name, ok := call.Callee.(*ast.ConstFetch)
	return ok && strings.EqualFold(strings.TrimPrefix(t.text(name), "\\"), "assert") && len(call.Args.Args) > 0 && call.Args.Args[0].Value != nil
}

func alwaysLeaves(body ast.Stmt) bool {
	last := body
	if b, ok := body.(*ast.Block); ok {
		if len(b.Stmts) == 0 {
			return false
		}
		last = b.Stmts[len(b.Stmts)-1]
	}
	switch s := last.(type) {
	case *ast.Return, *ast.Continue, *ast.Break:
		return true
	case *ast.ExprStmt:
		switch s.Expr.(type) {
		case *ast.Throw, *ast.Exit:
			return true
		}
	}
	return false
}

func (n narrowing) isVar(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Variable:
		return n.t.text(e) == n.varName
	case *ast.Assign:
		v, ok := e.Target.(*ast.Variable)
		return ok && e.Op == token.Equals && n.t.text(v) == n.varName
	}
	return false
}

func (n narrowing) applyCall(t ResolvedType, call ast.Expr) ResolvedType {
	callee, receiver, args, ok := n.callee(call, map[string]ResolvedType{n.varName: t})
	if !ok {
		return n.applyUnresolvedAssertion(t, call)
	}
	onVar := receiver != nil && n.isVar(receiver)
	if !onVar && !slices.ContainsFunc(args, func(a CallArg) bool { return a.node != nil && n.isVar(a.node) }) {
		return t
	}
	if callee.member.DocComment == "" {
		return t
	}
	doc := n.r.parseDoc(callee.member.DocComment)
	if doc == nil || len(doc.Asserts)+len(doc.ParamOuts) == 0 && doc.SelfOut == "" {
		return t
	}
	if onVar {
		callee.receiver = t
	}
	env := n.r.calleeEnv(callee, args, n.ctxAt(call, nil))
	argIs := n.argIs(callee, args)
	for _, a := range doc.Asserts {
		if a.Kind != parser.AssertAlways {
			continue
		}
		if a.Target == "$this" && onVar || argIs(a.Target) {
			t = n.r.applyFact(t, narrowingFact{typ: n.r.applyEnv(n.r.parseType(a.Type), env), negated: a.Negated, equality: a.Equality})
		}
	}
	for _, out := range doc.ParamOuts {
		if argIs(out.Name) {
			t = n.r.applyEnv(n.r.parseType(out.Type), env)
		}
	}
	if doc.SelfOut != "" && onVar {
		t = n.r.applyEnv(n.r.parseType(doc.SelfOut), env)
	}
	return t
}

func (n narrowing) argIs(callee resolvedCallee, args []CallArg) func(string) bool {
	byParam := argumentsByParam(callee.params, args)
	return func(paramName string) bool {
		for i, p := range callee.params {
			if p.name == paramName && i < len(byParam) && len(byParam[i]) > 0 {
				a := byParam[i][0]
				return a.node != nil && n.isVar(a.node)
			}
		}
		return false
	}
}

func (n narrowing) callee(call ast.Expr, known map[string]ResolvedType) (callee resolvedCallee, receiver ast.Expr, args []CallArg, ok bool) {
	ctx := n.ctxAt(call, known)
	var name string
	var op string
	switch c := call.(type) {
	case *ast.Call:
		fname, isName := c.Callee.(*ast.ConstFetch)
		if !isName {
			return callee, nil, nil, false
		}
		fn := n.r.CalledFunction(n.t.text(fname), n.file)
		if fn == nil {
			return callee, nil, nil, false
		}
		return resolvedCallee{member: fn, params: n.r.paramPatterns(fn, n.r.parseDoc(fn.DocComment))}, nil, n.t.callArgList(c.Args), true
	case *ast.MethodCall:
		receiver, name, op, args = c.Object, n.t.text(c.Name), "->", n.t.callArgList(c.Args)
	case *ast.StaticCall:
		receiver, name, op, args = c.Class, n.t.text(c.Name), "::", n.t.callArgList(c.Args)
	default:
		return callee, nil, nil, false
	}
	recvType := n.r.typeOf(n.t, receiver, nil, ctx)
	if op == "::" {
		recvType = ClassStringTarget(recvType)
	}
	m, owner := n.r.FindMemberOn(recvType, name, CallAccess)
	if m == nil {
		return callee, nil, nil, false
	}
	return resolvedCallee{member: m, receiver: owner, params: n.r.paramPatterns(m, n.r.parseDoc(m.DocComment))}, receiver, args, true
}

func (n narrowing) applyUnresolvedAssertion(t ResolvedType, call ast.Expr) ResolvedType {
	var name string
	var argList *ast.ArgList
	switch c := call.(type) {
	case *ast.Call:
		name, argList = n.t.text(c.Callee), c.Args
	case *ast.MethodCall:
		name, argList = n.t.text(c.Name), c.Args
	case *ast.StaticCall:
		name, argList = n.t.text(c.Name), c.Args
	}
	if !strings.EqualFold(name, "assertInstanceOf") || len(argList.Args) < 2 || argList.Args[1].Value == nil || !n.isVar(argList.Args[1].Value) {
		return t
	}
	cls := ClassStringTarget(n.r.typeOf(n.t, argList.Args[0].Value, nil, n.ctxAt(call, nil)))
	if cls.FQN == "" || cls.FQN == "class-string" || IsBuiltinTypeName(cls.FQN) {
		return t
	}
	return cls
}

func (n narrowing) applyCondition(t ResolvedType, cond ast.Expr, value, instanceofOnly bool) ResolvedType {
	if cond == nil || cond.Bounds().End <= n.after {
		return t
	}
	switch c := cond.(type) {
	case *ast.Unary:
		if c.Op == token.Bang {
			return n.applyCondition(t, c.Operand, !value, instanceofOnly)
		}
	case *ast.Binary:
		switch {
		case (c.Op == token.BooleanAnd || c.Op == token.And) && value, (c.Op == token.BooleanOr || c.Op == token.Or) && !value:
			t = n.applyCondition(t, c.Left, value, instanceofOnly)
			return n.applyCondition(t, c.Right, value, instanceofOnly)
		}
	case *ast.InstanceOf:
		class, ok := c.Class.(*ast.ConstFetch)
		if ok && n.isVar(c.Operand) {
			return n.r.narrowInstanceof(t, n.t.text(class), value, n.file)
		}
	case *ast.Call, *ast.MethodCall, *ast.StaticCall:
		if !instanceofOnly {
			return n.applyAssertingCall(t, c, value)
		}
	}
	return t
}

func (n narrowing) applyAssertingCall(t ResolvedType, call ast.Expr, value bool) ResolvedType {
	callee, receiver, args, ok := n.callee(call, nil)
	if !ok || !strings.Contains(callee.member.DocComment, "assert") {
		return t
	}
	doc := n.r.parseDoc(callee.member.DocComment)
	if doc == nil {
		return t
	}
	env := n.r.calleeEnv(callee, args, n.ctxAt(call, nil))
	argIs := n.argIs(callee, args)
	for _, a := range doc.Asserts {
		holds := a.Kind == parser.AssertIfTrue && value || a.Kind == parser.AssertIfFalse && !value
		opposite := a.Kind == parser.AssertIfTrue && !value || a.Kind == parser.AssertIfFalse && value
		if !holds && !opposite {
			continue
		}
		target := argIs(a.Target)
		if a.Target == "$this" {
			target = receiver != nil && n.isVar(receiver)
		}
		if !target {
			continue
		}
		fact := narrowingFact{typ: n.r.applyEnv(n.r.parseType(a.Type), env), negated: a.Negated, equality: a.Equality}
		if opposite {
			// The other outcome: the type is ruled out. =Type says nothing then.
			if a.Equality {
				continue
			}
			fact.negated = !fact.negated
		}
		t = n.r.applyFact(t, fact)
	}
	return t
}

func (n narrowing) enclosingConditions(t ResolvedType, path []ast.Node) ResolvedType {
	inner := innerPath(path)
	for i := len(inner) - 2; i >= 0; i-- {
		child := inner[i+1]
		switch node := inner[i].(type) {
		case *ast.If:
			switch {
			case ast.Node(node.Then) == child:
				t = n.applyCondition(t, node.Cond, true, false)
			case node.Else != nil && ast.Node(node.Else) == child && len(node.ElseIfs) == 0:
				t = n.applyCondition(t, node.Cond, false, false)
			}
		case *ast.ElseIf:
			if ast.Node(node.Body) == child {
				t = n.applyCondition(t, node.Cond, true, false)
			}
		case *ast.While:
			if ast.Node(node.Body) == child {
				t = n.applyCondition(t, node.Cond, true, false)
			}
		}
	}
	return t
}

func (n narrowing) switchCase(t ResolvedType, path []ast.Node) ResolvedType {
	inner := innerPath(path)
	for i := len(inner) - 2; i >= 0; i-- {
		sw, ok := inner[i].(*ast.Switch)
		if !ok {
			continue
		}
		if lit := constantLiteral(n.t.text(sw.Subject)); lit.Literal != "true" {
			return t
		}
		k := slices.IndexFunc(sw.Cases, func(c *ast.Case) bool { return ast.Node(c) == inner[i+1] })
		if k < 0 {
			return t
		}
		var parts []ResolvedType
		for j := k; j >= 0 && (j == k || len(sw.Cases[j].Stmts) == 0); j-- {
			cond := sw.Cases[j].Cond
			if cond == nil || !n.testsVar(cond) {
				return t
			}
			narrowed := n.applyCondition(t, cond, true, true)
			if narrowed.IsEmpty() {
				return narrowed
			}
			parts = append(parts, narrowed)
		}
		return MakeUnion(parts, false)
	}
	return t
}

func (n narrowing) testsVar(cond ast.Expr) bool {
	switch c := cond.(type) {
	case *ast.InstanceOf:
		_, named := c.Class.(*ast.ConstFetch)
		return named && n.isVar(c.Operand)
	case *ast.Binary:
		return (c.Op == token.BooleanAnd || c.Op == token.And) && (n.testsVar(c.Left) || n.testsVar(c.Right))
	}
	return false
}

func (n narrowing) statementTests(t ResolvedType, path []ast.Node) ResolvedType {
	inner := innerPath(path)
	for i := len(inner) - 2; i >= 0; i-- {
		child := inner[i+1]
		switch node := inner[i].(type) {
		case *ast.Binary:
			if ast.Node(node.Right) != child {
				continue
			}
			switch node.Op {
			case token.BooleanAnd, token.And:
				t = n.applyCondition(t, node.Left, true, true)
			case token.BooleanOr, token.Or:
				t = n.applyCondition(t, node.Left, false, true)
			}
		case *ast.Ternary:
			if node.Then != nil && ast.Node(node.Then) == child {
				t = n.applyCondition(t, node.Cond, true, true)
			}
		case *ast.MatchArm:
			if ast.Node(node.Body) == child && len(node.Conds) == 1 && i > 0 {
				if m, ok := inner[i-1].(*ast.Match); ok && constantLiteral(n.t.text(m.Subject)).Literal == "true" {
					t = n.applyCondition(t, node.Conds[0], true, true)
				}
			}
		}
		if _, ok := inner[i].(ast.Stmt); ok {
			return t
		}
	}
	return t
}

// resolvedCallee is the method or function a call statement calls.
type resolvedCallee struct {
	member   *symbols.Symbol
	receiver ResolvedType
	params   []paramPattern
}

// calleeEnv is what a callee's PHPDoc types mean at a call: its templates
// bound from the receiver and the arguments.
func (r *Resolver) calleeEnv(c resolvedCallee, args []CallArg, ctx ExprContext) typeEnv {
	file := ctx.File
	owner := c.member.ParentFQN
	if c.member.Kind == symbols.KindFunction {
		owner = c.member.FQN
	}
	var subst map[string]ResolvedType
	if c.member.Kind == symbols.KindMethod {
		subst = r.classSubst(c.member.ParentFQN, r.ReceiverFor(c.receiver, c.member), c.receiver.Params)
	}
	if len(c.member.Templates) > 0 {
		bound := r.bindCallTemplates(append([]paramPattern(nil), c.params...), r.callSite(args, ctx), templateNames(c.member.Templates), subst, owner, file)
		if subst == nil {
			subst = make(map[string]ResolvedType, len(bound))
		}
		for k, v := range bound {
			subst[k] = v
		}
	}
	subst = r.fillUnbound(subst, c.member.Templates, owner, file)
	static := c.receiver
	return typeEnv{subst: subst, static: static, owner: owner, file: file}
}

// applyFact narrows t by one assertion.
func (r *Resolver) applyFact(t ResolvedType, f narrowingFact) ResolvedType {
	if f.typ.IsEmpty() && !f.typ.Nullable {
		return t
	}
	if f.equality && !f.negated {
		return f.typ
	}
	if f.negated {
		return r.removeType(t, f.typ)
	}
	// What t already holds of the asserted type, or the asserted type.
	var kept []ResolvedType
	for _, p := range t.Parts() {
		if r.typeMatches(p, f.typ) == matchYes {
			kept = append(kept, p)
		}
	}
	if len(kept) > 0 && !(len(t.Parts()) == 1 && f.typ.FQN != "" && !strings.EqualFold(kept[0].FQN, f.typ.FQN) && !IsBuiltinTypeName(f.typ.FQN)) {
		return MakeUnion(kept, false)
	}
	return f.typ
}

// removeType takes the parts matching remove out of t: !null drops null.
func (r *Resolver) removeType(t, remove ResolvedType) ResolvedType {
	if remove.FQN == "" && remove.Nullable || remove.FQN == "null" {
		out := t
		if len(out.Union) > 0 {
			out = MakeUnion(out.Union, false)
		}
		out.Nullable = false
		return out
	}
	var kept []ResolvedType
	for _, p := range t.Parts() {
		if r.typeMatches(p, remove) != matchYes {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return t
	}
	return MakeUnion(kept, t.Nullable && !remove.Nullable)
}
