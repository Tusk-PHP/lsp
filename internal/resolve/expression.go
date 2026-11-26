package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ExprContext is where an expression is read: the file's lines, the position
// it's read at, and the types of variables that aren't in the source as
// written, such as the parameters of a closure being typed.
type ExprContext struct {
	Lines  []string
	Pos    protocol.Position
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

func (r *Resolver) callSite(args []CallArg, ctx ExprContext) *CallSite {
	if len(args) == 0 {
		return &CallSite{}
	}
	return &CallSite{Args: args, ArgType: func(arg CallArg, expected *ResolvedType) ResolvedType {
		return r.typeOf(arg.tree, arg.node, expected, ctx)
	}}
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
		if lit, ok := n.Operand.(*ast.Literal); ok && (n.Op == token.Minus || n.Op == token.Plus) {
			if rt := literalType(t, lit); rt.Literal != "" {
				rt.Literal = t.src[n.Start:lit.End]
				return rt
			}
			return literalType(t, lit)
		}
	case *ast.Cast:
		return castResult(n.Kind)
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
		case token.Dot:
			return ResolvedType{FQN: "string"}
		case token.Coalesce:
			return r.eitherType(t, n.Left, n.Right, ctx)
		case token.Pipe:
			return r.InvokeType(r.typeOf(t, n.Right, nil, ctx))
		}
	case *ast.Ternary:
		return r.ternaryType(t, n, ctx)
	case *ast.Call:
		return r.callType(t, n, ctx)
	case *ast.MethodCall:
		return r.accessType(t, n, n.Object, "->", n.Name, n.Args, ctx)
	case *ast.PropertyFetch:
		return r.accessType(t, n, n.Object, "->", n.Name, nil, ctx)
	case *ast.StaticCall:
		return r.accessType(t, n, n.Class, "::", n.Name, n.Args, ctx)
	case *ast.StaticPropertyFetch:
		return r.accessType(t, n, n.Class, "::", n.Name, nil, ctx)
	case *ast.ClassConstFetch:
		if strings.EqualFold(t.text(n.Name), "class") {
			return r.classStringOf(t, n.Class, ctx)
		}
		return r.accessType(t, n, n.Class, "::", n.Name, nil, ctx)
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

func castResult(kind token.Kind) ResolvedType {
	switch kind {
	case token.IntCast:
		return ResolvedType{FQN: "int"}
	case token.FloatCast:
		return ResolvedType{FQN: "float"}
	case token.StringCast:
		return ResolvedType{FQN: "string"}
	case token.BoolCast:
		return ResolvedType{FQN: "bool"}
	case token.ArrayCast:
		return ResolvedType{FQN: "array"}
	case token.ObjectCast:
		return ResolvedType{FQN: "object"}
	case token.UnsetCast:
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
	if r.ContainerCall == nil {
		return ResolvedType{}, false
	}
	return r.ContainerCall(r, t.src, ctx.File, call)
}

func (r *Resolver) accessType(t *syntaxTree, n, receiver ast.Expr, op string, nameNode ast.Expr, argList *ast.ArgList, ctx ExprContext) ResolvedType {
	call := argList != nil
	if call {
		if rt, ok := r.containerCallType(t, n, ctx); ok {
			return rt
		}
	}
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
	cur := r.typeOf(t, receiver, nil, ctx)
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
	rt := literalArrayType(t, n)
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

func literalArrayType(t *syntaxTree, n *ast.Array) ResolvedType {
	var fields []string
	keyed := false
	for _, item := range n.Items {
		if item == nil || item.Key == nil {
			continue
		}
		keyed = true
		fields = append(fields, arrayKey(t, item.Key)+": "+literalValueType(t, item.Value))
	}
	if keyed {
		return attachShapeFields(ResolvedType{FQN: "array", Shape: "array{" + strings.Join(fields, ", ") + "}"})
	}
	common, sameShape := "", true
	for i, item := range n.Items {
		if item == nil {
			continue
		}
		vt := "mixed"
		if !item.Unpack {
			vt = literalValueType(t, item.Value)
		}
		if _, nested := item.Value.(*ast.Array); !nested || !strings.HasPrefix(vt, "array{") {
			sameShape = false
		}
		if i > 0 && vt != common {
			return ResolvedType{FQN: "array"}
		}
		common = vt
	}
	switch {
	case common == "":
		return ResolvedType{FQN: "array"}
	case sameShape:
		return attachShapeFields(ResolvedType{FQN: "array", Params: []ResolvedType{{FQN: "int"}, {FQN: "array", Shape: common}}})
	case common == "mixed":
		return ResolvedType{FQN: "array"}
	}
	return ResolvedType{FQN: "array", Params: []ResolvedType{{FQN: "int"}, {FQN: common}}}
}

func arrayKey(t *syntaxTree, key ast.Expr) string {
	text := t.text(key)
	if lit, ok := key.(*ast.Literal); ok && lit.Kind == token.ConstantString {
		return text[1 : len(text)-1]
	}
	return text
}

func literalValueType(t *syntaxTree, e ast.Expr) string {
	switch n := e.(type) {
	case *ast.Array:
		rt := literalArrayType(t, n)
		if rt.Shape != "" {
			return rt.Shape
		}
		return "array"
	case *ast.Literal, *ast.InterpolatedString, *ast.ConstFetch, *ast.Unary:
		if rt := literalNodeType(t, e); rt != "" {
			return rt
		}
	}
	return "mixed"
}

func literalNodeType(t *syntaxTree, e ast.Expr) string {
	switch n := e.(type) {
	case *ast.Literal:
		return literalType(t, n).FQN
	case *ast.InterpolatedString:
		return "string"
	case *ast.ConstFetch:
		return constantLiteral(t.text(n)).FQN
	case *ast.Unary:
		if lit, ok := n.Operand.(*ast.Literal); ok && (n.Op == token.Minus || n.Op == token.Plus) {
			return literalType(t, lit).FQN
		}
	case *ast.Array:
		return "array"
	case *ast.Cast:
		return castResult(n.Kind).FQN
	case *ast.ClassConstFetch:
		if _, named := n.Class.(*ast.ConstFetch); named && strings.EqualFold(t.text(n.Name), "class") {
			return "string"
		}
	}
	return ""
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
