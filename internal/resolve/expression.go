package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// ExprContext is where an expression is read: the file's lines, the position
// it's read at, and the types of variables that aren't in the source as
// written, such as the parameters of a closure being typed.
type ExprContext struct {
	Lines  []string
	Pos    textpos.Position
	File   *parser.FileNode
	Locals map[string]ResolvedType
	depth  int
}

// maxExprDepth bounds how deeply nested expressions are followed.
const maxExprDepth = 12

func (ctx ExprContext) grammar() token.Version {
	if ctx.File == nil {
		return token.Latest
	}
	return ctx.File.Grammar
}

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

// CallSiteFor returns the call site for the text between a call's
// parentheses, typing each argument where it's read.
func (r *Resolver) CallSiteFor(argStr string, ctx ExprContext) *CallSite {
	t, e := expressionTree("f("+argStr+")", ctx.grammar())
	call, ok := e.(*ast.Call)
	if !ok {
		return &CallSite{}
	}
	return r.callSite(t.callArgList(call.Args), ctx)
}

func (r *Resolver) callSite(args []CallArg, ctx ExprContext) *CallSite {
	if len(args) == 0 {
		return &CallSite{}
	}
	return &CallSite{Args: args, ArgType: func(arg CallArg, expected *ResolvedType) ResolvedType {
		return r.typeOf(arg.tree, arg.node, expected, ctx)
	}}
}

// ExpressionType returns the type of a PHP expression: a variable, a literal,
// `new Foo(…)`, `Foo::class`, an array, a closure, a function call or a
// chain of member accesses. It's empty when the type can't be worked out.
func (r *Resolver) ExpressionType(expr string, ctx ExprContext) ResolvedType {
	return r.ExpressionTypeExpecting(expr, nil, ctx)
}

// ExpressionTypeExpecting is ExpressionType for an expression passed where
// expected is declared. A closure takes its untyped parameters' types from an
// expected callable(…) signature.
func (r *Resolver) ExpressionTypeExpecting(expr string, expected *ResolvedType, ctx ExprContext) ResolvedType {
	t, e := expressionTree(expr, ctx.grammar())
	return r.typeOf(t, e, expected, ctx)
}

func (r *Resolver) typeOf(t *syntaxTree, e ast.Expr, expected *ResolvedType, ctx ExprContext) ResolvedType {
	if e == nil || ctx.depth > maxExprDepth {
		return ResolvedType{}
	}
	ctx.depth++
	switch n := e.(type) {
	case *ast.Variable:
		return r.variableType(t.text(n), ctx)
	case *ast.Literal:
		return literalType(t, n)
	case *ast.InterpolatedString:
		return ResolvedType{FQN: "string"}
	case *ast.MagicConst:
		if n.Kind == token.LineConst {
			return ResolvedType{FQN: "int"}
		}
		return ResolvedType{FQN: "string"}
	case *ast.Unary:
		if lit, ok := n.Operand.(*ast.Literal); ok && (n.Op == token.Minus || n.Op == token.Plus) && (lit.Kind == token.IntegerLiteral || lit.Kind == token.FloatLiteral) {
			rt := literalType(t, lit)
			rt.Literal = t.src[n.Start:lit.End]
			return rt
		}
		if n.Op == token.At {
			return r.typeOf(t, n.Operand, expected, ctx)
		}
		return r.operatorType(t, n, ctx)
	case *ast.InstanceOf:
		return r.operatorType(t, n, ctx)
	case *ast.Cast:
		return ResolvedType{FQN: parser.CastType(n.Kind)}
	case *ast.ConstFetch:
		if lit := constantLiteral(t.text(n)); !lit.IsEmpty() {
			return lit
		}
		return r.classReference(t, n, ctx)
	case *ast.Array:
		return r.arrayType(t, n, ctx)
	case *ast.New:
		return r.newType(t, n, ctx)
	case *ast.Clone:
		if n.Operand != nil {
			return r.typeOf(t, n.Operand, nil, ctx)
		}
		if len(n.Args.Args) > 0 {
			return r.typeOf(t, n.Args.Args[0].Value, nil, ctx)
		}
	case *ast.Closure:
		return r.closureType(t, closureOf(n), expected, ctx)
	case *ast.ArrowFunction:
		return r.closureType(t, arrowOf(n), expected, ctx)
	case *ast.Binary:
		switch n.Op {
		case token.Coalesce:
			return r.eitherType(t, n.Left, n.Right, ctx)
		case token.Pipe:
			return r.InvokeType(r.typeOf(t, n.Right, nil, ctx))
		}
		return r.operatorType(t, n, ctx)
	case *ast.Ternary:
		return r.ternaryType(t, n, ctx)
	case *ast.Call:
		return r.callType(t, n, ctx)
	case *ast.MethodCall, *ast.PropertyFetch, *ast.StaticCall, *ast.StaticPropertyFetch:
		return r.memberChainType(t, e, ctx)
	case *ast.ClassConstFetch:
		if strings.EqualFold(t.text(n.Name), "class") {
			return r.classStringOf(t, n.Class, ctx)
		}
		return r.memberChainType(t, e, ctx)
	case *ast.ArrayDimFetch:
		if n.Dim == nil {
			return ResolvedType{}
		}
		base := r.typeOf(t, n.Array, nil, ctx)
		if base.IsEmpty() {
			return ResolvedType{}
		}
		return r.OffsetType(base, t.text(n.Dim))
	}
	return ResolvedType{}
}

func (r *Resolver) operatorType(t *syntaxTree, e ast.Expr, ctx ExprContext) ResolvedType {
	return scalarsType(parser.OperatorType(e, t.src, func(operand ast.Expr) parser.Scalars {
		return scalarsOf(r.typeOf(t, operand, nil, ctx))
	}))
}

func scalarsOf(rt ResolvedType) parser.Scalars {
	if rt.IsEmpty() || len(rt.Intersection) > 0 || rt.Conditional != nil {
		return 0
	}
	var s parser.Scalars
	if rt.Nullable {
		s = parser.ScalarNull
	}
	for _, part := range rt.Parts() {
		ps := parser.ScalarOf(part.FQN)
		if ps == 0 {
			return 0
		}
		s |= ps
	}
	return s
}

func scalarsType(s parser.Scalars) ResolvedType {
	if s == 0 {
		return ResolvedType{}
	}
	names := s.Names()
	if len(names) == 1 {
		return ResolvedType{FQN: names[0]}
	}
	parts := make([]ResolvedType, len(names))
	for i, name := range names {
		parts[i] = ResolvedType{FQN: name}
	}
	return MakeUnion(parts, false)
}

func (r *Resolver) variableType(name string, ctx ExprContext) ResolvedType {
	if rt, ok := ctx.Locals[name]; ok {
		return rt
	}
	if ctx.File == nil {
		return ResolvedType{}
	}
	return r.ResolveVariableTypeTyped(name, ctx.Lines, ctx.Pos, ctx.File)
}

func literalType(t *syntaxTree, n *ast.Literal) ResolvedType {
	text := t.text(n)
	switch n.Kind {
	case token.ConstantString:
		return ResolvedType{FQN: "string", Literal: text}
	case token.IntegerLiteral:
		return ResolvedType{FQN: "int", Literal: text}
	case token.FloatLiteral:
		return ResolvedType{FQN: "float", Literal: text}
	case token.StartHeredoc:
		return ResolvedType{FQN: "string"}
	}
	return ResolvedType{}
}

func constantLiteral(name string) ResolvedType {
	switch lower := strings.ToLower(strings.TrimPrefix(name, "\\")); lower {
	case "true", "false":
		return ResolvedType{FQN: "bool", Literal: lower}
	case "null":
		return ResolvedType{FQN: "null"}
	}
	return ResolvedType{}
}

func (r *Resolver) classReference(t *syntaxTree, n *ast.ConstFetch, ctx ExprContext) ResolvedType {
	name := t.text(n)
	if fqn, isKeyword := r.ClassKeywordFQN(name, ctx.Pos, ctx.File); isKeyword {
		if fqn == "" {
			return ResolvedType{}
		}
		return ResolvedType{FQN: fqn}
	}
	if r.Index == nil {
		return ResolvedType{}
	}
	fqn := r.ResolveClassNameLoose(name, ctx.File)
	if fqn == "" {
		return ResolvedType{}
	}
	if sym := r.Index.Lookup(fqn); sym != nil {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
			return ResolvedType{FQN: fqn}
		}
		return ResolvedType{}
	}
	return ResolvedType{FQN: fqn}
}

func (r *Resolver) classStringOf(t *syntaxTree, class ast.Expr, ctx ExprContext) ResolvedType {
	name, ok := class.(*ast.ConstFetch)
	if !ok {
		return ResolvedType{FQN: "class-string"}
	}
	fqn, isKeyword := r.ClassKeywordFQN(t.text(name), ctx.Pos, ctx.File)
	if !isKeyword {
		fqn = r.ResolveClassName(t.text(name), ctx.File)
	}
	if fqn == "" {
		return ResolvedType{FQN: "class-string"}
	}
	return ResolvedType{FQN: "class-string", Params: []ResolvedType{{FQN: fqn}}}
}

func (r *Resolver) newType(t *syntaxTree, n *ast.New, ctx ExprContext) ResolvedType {
	if n.Anonymous != nil {
		return ResolvedType{}
	}
	args := t.callArgList(n.Args)
	if v, ok := n.Class.(*ast.Variable); ok {
		target := ClassStringTarget(r.variableType(t.text(v), ctx))
		if target.FQN == "class-string" || target.FQN == "string" {
			return ResolvedType{}
		}
		return target
	}
	name, ok := n.Class.(*ast.ConstFetch)
	if !ok {
		return ResolvedType{}
	}
	return r.newExpressionType(t.text(name), args, ctx)
}

func (r *Resolver) callType(t *syntaxTree, n *ast.Call, ctx ExprContext) ResolvedType {
	if rt, ok := r.containerCallType(t, n, ctx); ok {
		return rt
	}
	args := t.callArgList(n.Args)
	if name, ok := n.Callee.(*ast.ConstFetch); ok {
		return r.functionCallType(t.text(name), args, ctx)
	}
	callee := r.typeOf(t, n.Callee, nil, ctx)
	if isCallableArgs(args) {
		return r.CallableOf(callee)
	}
	return r.InvokeType(callee)
}

func (r *Resolver) containerCallType(t *syntaxTree, call ast.Expr, ctx ExprContext) (ResolvedType, bool) {
	if r.ContainerCall == nil || len(ctx.Lines) == 0 {
		return ResolvedType{}, false
	}
	switch concrete := r.ContainerCall(r, t.text(call), r.JoinLines(ctx.Lines), ctx.File); concrete {
	case "":
		return ResolvedType{}, false
	case "-":
		return ResolvedType{}, true
	default:
		return ResolvedType{FQN: concrete}, true
	}
}

type memberLink struct {
	node     ast.Expr
	receiver ast.Expr
	op       string
	name     ast.Expr
	args     *ast.ArgList
}

func memberLinkOf(t *syntaxTree, e ast.Expr) (memberLink, bool) {
	switch n := e.(type) {
	case *ast.MethodCall:
		return memberLink{node: n, receiver: n.Object, op: "->", name: n.Name, args: n.Args}, true
	case *ast.PropertyFetch:
		return memberLink{node: n, receiver: n.Object, op: "->", name: n.Name}, true
	case *ast.StaticCall:
		return memberLink{node: n, receiver: n.Class, op: "::", name: n.Name, args: n.Args}, true
	case *ast.StaticPropertyFetch:
		return memberLink{node: n, receiver: n.Class, op: "::", name: n.Name}, true
	case *ast.ClassConstFetch:
		if strings.EqualFold(t.text(n.Name), "class") {
			return memberLink{}, false
		}
		return memberLink{node: n, receiver: n.Class, op: "::", name: n.Name}, true
	}
	return memberLink{}, false
}

func (r *Resolver) memberChainType(t *syntaxTree, e ast.Expr, ctx ExprContext) ResolvedType {
	var buf [8]memberLink
	links := buf[:0]
	var cur ResolvedType
	for {
		link, ok := memberLinkOf(t, e)
		if !ok {
			cur = r.typeOf(t, e, nil, ctx)
			break
		}
		if link.args != nil {
			if rt, ok := r.containerCallType(t, link.node, ctx); ok {
				cur = rt
				break
			}
		}
		links = append(links, link)
		e = link.receiver
	}
	for i := len(links) - 1; i >= 0; i-- {
		cur = r.linkType(t, links[i], cur, ctx)
	}
	return cur
}

func (r *Resolver) linkType(t *syntaxTree, link memberLink, cur ResolvedType, ctx ExprContext) ResolvedType {
	receiver, op, nameNode, argList := link.receiver, link.op, link.name, link.args
	call := argList != nil
	name := t.text(nameNode)
	access := MemberAccessOf(call, op == "::", name)
	switch nameNode.(type) {
	case *ast.IdentifierExpr:
	case *ast.Variable:
		if op == "->" || call {
			return ResolvedType{}
		}
		name = name[1:]
	default:
		return ResolvedType{}
	}
	if cur.IsEmpty() || r.Index == nil {
		return ResolvedType{}
	}
	args := t.callArgList(argList)
	if call && op == "->" {
		if rt, ok := ClosureCallType(cur, name); ok {
			return rt
		}
	}
	if op == "::" {
		cur = ClassStringTarget(cur)
	}
	if fwd, ok := r.ForwardedReceiver(cur, name, access); ok {
		cur = fwd
	}
	callable := call && isCallableArgs(args)
	member, owner := r.FindMemberOn(cur, name, access)
	if member != nil && owner.FQN != cur.FQN {
		if !callable && r.ReturnsCallingClass(member) {
			return cur
		}
		cur = owner
	}
	if member == nil {
		if call && r.ScopeMethod(cur, name) != nil {
			return cur
		}
		if !call && cur.FQN == "object" {
			for _, f := range cur.Fields {
				if f.Key == name {
					return f.Type
				}
			}
		}
		return ResolvedType{}
	}
	callerFQN := r.ReceiverFor(cur, member)
	if op == "::" && forwardsCallingClass(t, receiver) {
		if enclosing := FindEnclosingClass(ctx.File, ctx.Pos); enclosing != "" {
			callerFQN = enclosing
		}
	}
	if callable {
		return PartialClosure(r.MemberCallableType(member, ctx.File, callerFQN, cur.Params), member, args)
	}
	var site *CallSite
	if call {
		site = r.callSite(args, ctx)
	}
	return r.MemberTypeForCall(member, ctx.File, callerFQN, cur.Params, site)
}

func forwardsCallingClass(t *syntaxTree, class ast.Expr) bool {
	name, ok := class.(*ast.ConstFetch)
	if !ok {
		return false
	}
	switch ClassKeyword(t.text(name)) {
	case "self", "parent", "static":
		return true
	}
	return false
}

// arrayType types an array literal. A list of values that aren't plain
// literals, such as [$user, new User], gets the union of their types.
func (r *Resolver) arrayType(t *syntaxTree, n *ast.Array, ctx ExprContext) ResolvedType {
	rt := r.literalArrayType(t, n)
	if len(n.Items) == 0 {
		return rt
	}
	if n.Items[0] != nil && n.Items[0].Key != nil {
		if rt.Shape == "" {
			return rt
		}
		fields := make([]ShapeEntry, 0, len(n.Items))
		for _, item := range n.Items {
			if item == nil || item.Key == nil {
				return rt
			}
			key := t.text(item.Key)
			if lit, ok := item.Key.(*ast.Literal); ok && lit.Kind == token.ConstantString {
				key = key[1 : len(key)-1]
			}
			vt := r.typeOf(t, item.Value, nil, ctx)
			if vt.IsEmpty() {
				vt = ResolvedType{FQN: "mixed"}
			}
			fields = append(fields, ShapeEntry{Key: key, Type: vt})
		}
		out := ResolvedType{FQN: "array", Fields: fields}
		out.Shape = shapeString("array", "array{", fields)
		return out
	}
	if rt.FQN == "array" && len(rt.Params) == 2 && rt.Params[1].FQN != "mixed" && rt.Params[1].FQN != "" {
		return rt
	}
	parts := make([]ResolvedType, 0, len(n.Items))
	for _, item := range n.Items {
		if item == nil {
			continue
		}
		vt := r.typeOf(t, item.Value, nil, ctx)
		if item.Unpack {
			_, ev, ok := r.IterableTypes(vt)
			if !ok {
				return rt
			}
			vt = ev
		}
		if vt.IsEmpty() {
			return rt
		}
		parts = append(parts, vt)
	}
	if len(parts) == 0 {
		return rt
	}
	return ResolvedType{FQN: "array", Params: []ResolvedType{{FQN: "int"}, MakeUnion(parts, false)}}
}

func (r *Resolver) literalArrayType(t *syntaxTree, n *ast.Array) ResolvedType {
	return r.parseType(parser.LiteralExprType(n, t.src))
}

// ReceiverFor returns which class of a union or intersection receiver a
// member was found on, for late static binding.
func (r *Resolver) ReceiverFor(rt ResolvedType, member *symbols.Symbol) string {
	base := rt.BaseFQN()
	if len(rt.Union)+len(rt.Intersection) == 0 || member == nil || r.Index == nil {
		return base
	}
	for _, name := range rt.ClassNames() {
		if r.Index.IsSubtypeOf(name, member.ParentFQN) {
			return name
		}
	}
	return base
}

// OffsetType is the type of $value[key] for a value of type rt: the field of
// an array shape, the value type of an array, or what offsetGet gives.
func (r *Resolver) OffsetType(rt ResolvedType, key string) ResolvedType {
	key = strings.TrimSpace(key)
	if len(rt.Fields) > 0 {
		k := strings.Trim(key, `'"`)
		for i, f := range rt.Fields {
			if f.Key == k || f.Key == "" && k == itoa(i) {
				return f.Type
			}
		}
	}
	if _, v, ok := r.IterableTypes(rt); ok && (rt.FQN == "array" || IsBuiltinTypeName(rt.FQN)) {
		return v
	}
	if rt.FQN != "" && !IsBuiltinTypeName(rt.FQN) && r.Index != nil {
		if args, ok := r.AncestorTypeArgs(rt.FQN, rt.Params, "ArrayAccess"); ok && len(args) == 2 {
			return args[1]
		}
		if m := r.FindAccessedMember(rt.FQN, "offsetGet", CallAccess); m != nil {
			return r.MemberTypeForCall(m, nil, rt.FQN, rt.Params, nil)
		}
	}
	return ResolvedType{}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}

// FunctionCallType types a call to the function name with the arguments
// args (the text between its parentheses), binding its templates.
func (r *Resolver) FunctionCallType(name, args string, ctx ExprContext) ResolvedType {
	t, e := expressionTree("f("+args+")", ctx.grammar())
	call, ok := e.(*ast.Call)
	if !ok {
		return ResolvedType{}
	}
	return r.functionCallType(name, t.callArgList(call.Args), ctx)
}

func (r *Resolver) functionCallType(name string, args []CallArg, ctx ExprContext) ResolvedType {
	if r.Index == nil {
		return ResolvedType{}
	}
	fn := r.typedFunction(name, ctx.File)
	if isCallableArgs(args) {
		return PartialClosure(r.FunctionCallableType(fn, ctx.File), fn, args)
	}
	if fn == nil {
		return ResolvedType{}
	}
	return r.FunctionTypeForCall(fn, ctx.File, r.callSite(args, ctx))
}

// NewExpressionType types `new Foo(args)`. The class's templates are bound
// from the constructor's arguments: `new Collection([$user])` is a
// Collection<int, User>.
func (r *Resolver) NewExpressionType(className, args string, ctx ExprContext) ResolvedType {
	t, e := expressionTree("new "+className+"("+args+")", ctx.grammar())
	n, ok := e.(*ast.New)
	if !ok {
		return ResolvedType{}
	}
	return r.newType(t, n, ctx)
}

func (r *Resolver) newExpressionType(className string, args []CallArg, ctx ExprContext) ResolvedType {
	fqn, isKeyword := r.ClassKeywordFQN(className, ctx.Pos, ctx.File)
	if !isKeyword {
		fqn = r.ResolveClassName(className, ctx.File)
	}
	if fqn == "" {
		return ResolvedType{}
	}
	tmpls := r.templatesOf(fqn)
	if len(tmpls) == 0 || len(args) == 0 || r.Index == nil {
		return ResolvedType{FQN: fqn}
	}
	ctor := r.FindMember(fqn, "__construct")
	if ctor == nil {
		return ResolvedType{FQN: fqn}
	}
	params := r.paramPatterns(ctor, parser.ParseDocBlock(ctor.DocComment))
	names := templateNames(tmpls, ctor.Templates)
	site := r.callSite(args, ctx)
	bound := r.bindCallTemplates(params, site, names, nil, ctor.ParentFQN, ctx.File)
	r.dropOutOfBound(bound, tmpls, fqn, ctx.File)
	r.dropOutOfBound(bound, ctor.Templates, ctor.ParentFQN, ctx.File)
	if len(bound) == 0 {
		// A constructor without types for its templates, given an array: its
		// keys and values are the class's arguments, in order.
		if len(params) > 0 {
			if at := site.ArgType(args[0], nil); at.FQN == "array" {
				if rt := r.inferConstructorGenerics(fqn, at); !rt.IsEmpty() {
					return rt
				}
			}
		}
		return ResolvedType{FQN: fqn}
	}
	// The constructor may be declared on a parent with its own templates.
	if !strings.EqualFold(ctor.ParentFQN, fqn) {
		bound = r.fillUnbound(bound, r.templatesOf(ctor.ParentFQN), ctor.ParentFQN, ctx.File)
	}
	bound = r.fillUnbound(bound, tmpls, fqn, ctx.File)
	out := ResolvedType{FQN: fqn, Params: make([]ResolvedType, len(tmpls))}
	for i, t := range tmpls {
		out.Params[i] = r.applyEnv(bound[t.Name], typeEnv{owner: fqn, file: ctx.File})
	}
	return out
}

// cutKeyword removes a leading keyword that's followed by something other than
// more of the same word.
func cutKeyword(expr, keyword string) (string, bool) {
	if len(expr) <= len(keyword) || !strings.EqualFold(expr[:len(keyword)], keyword) {
		return "", false
	}
	if next := expr[len(keyword)]; next != ' ' && next != '\t' && next != '(' {
		return "", false
	}
	return strings.TrimSpace(expr[len(keyword):]), true
}
