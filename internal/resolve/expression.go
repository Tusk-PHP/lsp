package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
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

// CallSiteFor returns the call site for the text between a call's
// parentheses, typing each argument where it's read.
func (r *Resolver) CallSiteFor(argStr string, ctx ExprContext) *CallSite {
	args := ParseCallArgs(argStr)
	if len(args) == 0 {
		return &CallSite{}
	}
	return &CallSite{Args: args, ArgType: func(expr string, expected *ResolvedType) ResolvedType {
		return r.ExpressionTypeExpecting(expr, expected, ctx)
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
	if ctx.depth > maxExprDepth {
		return ResolvedType{}
	}
	ctx.depth++
	expr = strings.TrimSuffix(strings.TrimSpace(expr), ";")
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ResolvedType{}
	}
	if inner, ok := unwrapParens(expr); ok {
		return r.ExpressionTypeExpecting(inner, expected, ctx)
	}
	if isClosureExpr(expr) {
		return r.closureType(expr, expected, ctx)
	}
	if rest, ok := cutKeyword(expr, "clone"); ok {
		return r.ExpressionType(rest, ctx)
	}
	lower := strings.ToLower(expr)
	if strings.HasSuffix(lower, "::class") && classConstantRe.MatchString(expr) {
		name := strings.TrimSpace(expr[:len(expr)-len("::class")])
		fqn, isKeyword := r.ClassKeywordFQN(name, ctx.Pos, ctx.File)
		if !isKeyword {
			fqn = r.ResolveClassName(name, ctx.File)
		}
		if fqn == "" {
			return ResolvedType{FQN: "class-string"}
		}
		return ResolvedType{FQN: "class-string", Params: []ResolvedType{{FQN: fqn}}}
	}
	if expr[0] == '[' || strings.HasPrefix(lower, "array(") {
		return r.arrayLiteralType(expr, ctx)
	}
	if lit := literalExprType(expr); !lit.IsEmpty() {
		return lit
	}
	if expr[0] == '$' {
		if name := extractLeadingVariable(expr); name == expr {
			if t, ok := ctx.Locals[name]; ok {
				return t
			}
			if ctx.File == nil {
				return ResolvedType{}
			}
			return r.ResolveVariableTypeTyped(name, ctx.Lines, ctx.Pos, ctx.File)
		}
	}
	if steps, origin, ok := parseChain(expr); ok {
		if len(steps) == 0 {
			return r.chainOriginType(origin, ctx)
		}
		_, local := ctx.Locals[origin.text]
		if !local && r.TypedChainResolver != nil && ctx.File != nil && len(ctx.Lines) > 0 {
			if rt := r.TypedChainResolver(expr, r.JoinLines(ctx.Lines), ctx.Pos, ctx.File); !rt.IsEmpty() {
				return rt
			}
		}
		cur := r.chainOriginType(origin, ctx)
		for _, st := range steps {
			if cur.IsEmpty() {
				return ResolvedType{}
			}
			cur = r.chainStepType(cur, st, ctx)
		}
		return cur
	}
	if r.TypedChainResolver != nil && ctx.File != nil && len(ctx.Lines) > 0 && (strings.Contains(expr, "->") || strings.Contains(expr, "::")) {
		return r.TypedChainResolver(expr, r.JoinLines(ctx.Lines), ctx.Pos, ctx.File)
	}
	return ResolvedType{}
}

// chainOrigin is the first part of a chain: $var, a class name, `new Foo(…)`
// or a function call.
type chainOrigin struct {
	kind byte // '$' variable, 'c' class name, 'n' new, 'f' function call
	text string
	args string
}

// chainStep is one access in a chain: ->name, ?->name, ::name, with call
// arguments when it's a call, or an array offset [key].
type chainStep struct {
	op     string // "->", "?->", "::" or "[" for an offset
	name   string
	call   bool
	args   string
	offset string
}

// parseChain splits `$a->b(1)::c['k']` into its origin and its accesses. ok
// is false when expr isn't such a chain.
func parseChain(expr string) (steps []chainStep, origin chainOrigin, ok bool) {
	i := 0
	n := len(expr)
	readIdent := func() string {
		start := i
		for i < n && (isIdentByte(expr[i]) || expr[i] >= '0' && expr[i] <= '9' || expr[i] == '\\') {
			i++
		}
		return expr[start:i]
	}
	readBalanced := func(open, close byte) (string, bool) {
		if i >= n || expr[i] != open {
			return "", false
		}
		depth := 0
		var quote byte
		start := i + 1
		for ; i < n; i++ {
			c := expr[i]
			if quote != 0 {
				if c == '\\' {
					i++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			switch c {
			case '\'', '"':
				quote = c
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
				if depth == 0 {
					i++
					return expr[start : i-1], c == close
				}
			}
		}
		return "", false
	}
	skipSpace := func() {
		for i < n && (expr[i] == ' ' || expr[i] == '\t' || expr[i] == '\n' || expr[i] == '\r') {
			i++
		}
	}

	switch {
	case expr[0] == '$':
		i = 1
		origin = chainOrigin{kind: '$', text: "$" + readIdent()}
		if origin.text == "$" {
			return nil, origin, false
		}
		skipSpace()
		// $fn(…): calling the variable.
		if i < n && expr[i] == '(' {
			args, closed := readBalanced('(', ')')
			if !closed {
				return nil, origin, false
			}
			steps = append(steps, chainStep{op: "()", call: true, args: args})
		}
	case strings.HasPrefix(expr, "new ") || strings.HasPrefix(expr, "(new "):
		wrapped := expr[0] == '('
		if wrapped {
			inner, closed := readBalanced('(', ')')
			if !closed {
				return nil, origin, false
			}
			name, args := splitNewExpr(strings.TrimSpace(inner[4:]))
			origin = chainOrigin{kind: 'n', text: name, args: args}
		} else {
			// `new Foo(…)->bar()` only chains in PHP 8.4; take the whole as new.
			i = 4
			skipSpace()
			name := readIdent()
			skipSpace()
			args := ""
			if i < n && expr[i] == '(' {
				a, closed := readBalanced('(', ')')
				if !closed {
					return nil, origin, false
				}
				args = a
			}
			origin = chainOrigin{kind: 'n', text: name, args: args}
		}
	case isIdentByte(expr[0]) || expr[0] == '\\':
		name := readIdent()
		skipSpace()
		if i < n && expr[i] == '(' {
			args, closed := readBalanced('(', ')')
			if !closed {
				return nil, origin, false
			}
			origin = chainOrigin{kind: 'f', text: name, args: args}
		} else {
			origin = chainOrigin{kind: 'c', text: name}
		}
	default:
		return nil, origin, false
	}

	for {
		skipSpace()
		if i >= n {
			return steps, origin, true
		}
		var st chainStep
		switch {
		case strings.HasPrefix(expr[i:], "?->"):
			st.op = "->"
			i += 3
		case strings.HasPrefix(expr[i:], "->"):
			st.op = "->"
			i += 2
		case strings.HasPrefix(expr[i:], "::"):
			st.op = "::"
			i += 2
		case expr[i] == '[':
			key, closed := readBalanced('[', ']')
			if !closed {
				return nil, origin, false
			}
			steps = append(steps, chainStep{op: "[", offset: strings.TrimSpace(key)})
			continue
		default:
			return nil, origin, false
		}
		skipSpace()
		if i < n && expr[i] == '$' && st.op == "::" {
			i++
			st.name = "$" + readIdent()
		} else {
			st.name = readIdent()
		}
		if st.name == "" || st.name == "$" {
			return nil, origin, false
		}
		skipSpace()
		if i < n && expr[i] == '(' {
			args, closed := readBalanced('(', ')')
			if !closed {
				return nil, origin, false
			}
			st.call, st.args = true, args
		}
		steps = append(steps, st)
	}
}

// splitNewExpr splits `Foo(1, 2)` into the class name and its arguments.
func splitNewExpr(s string) (name, args string) {
	open := strings.IndexByte(s, '(')
	if open < 0 {
		return strings.TrimSpace(s), ""
	}
	close := strings.LastIndexByte(s, ')')
	if close < open {
		return strings.TrimSpace(s[:open]), ""
	}
	return strings.TrimSpace(s[:open]), s[open+1 : close]
}

// chainOriginType types the start of a chain.
func (r *Resolver) chainOriginType(o chainOrigin, ctx ExprContext) ResolvedType {
	switch o.kind {
	case '$':
		if t, ok := ctx.Locals[o.text]; ok {
			return t
		}
		if ctx.File == nil {
			return ResolvedType{}
		}
		return r.ResolveVariableTypeTyped(o.text, ctx.Lines, ctx.Pos, ctx.File)
	case 'n':
		return r.NewExpressionType(o.text, o.args, ctx)
	case 'c':
		fqn, isKeyword := r.ClassKeywordFQN(o.text, ctx.Pos, ctx.File)
		if !isKeyword {
			fqn = r.ResolveClassName(o.text, ctx.File)
		}
		if fqn == "" {
			return ResolvedType{}
		}
		return ResolvedType{FQN: fqn}
	case 'f':
		return r.FunctionCallType(o.text, o.args, ctx)
	}
	return ResolvedType{}
}

// chainStepType types one access on a value of type cur.
func (r *Resolver) chainStepType(cur ResolvedType, st chainStep, ctx ExprContext) ResolvedType {
	if st.op == "[" {
		return r.OffsetType(cur, st.offset)
	}
	if st.op == "()" {
		return r.InvokeType(cur)
	}
	if r.Index == nil {
		return ResolvedType{}
	}
	if cur.Callable != nil && st.op == "->" && st.name == "call" {
		if cur.Callable.Return != nil {
			return *cur.Callable.Return
		}
	}
	name := st.name
	if st.op == "::" && !st.call && strings.HasPrefix(name, "$") {
		name = name[1:]
	}
	if st.op == "::" {
		cur = ClassStringTarget(cur)
	}
	member, owner := r.FindMemberOn(cur, name, st.call)
	if member != nil && owner.FQN != cur.FQN {
		// Through @mixin: a fluent method of the mixin gives back the object
		// it was called on, as forwarding through __call does.
		if r.ReturnsCallingClass(member) {
			return cur
		}
		cur = owner
	}
	if member == nil {
		// A member of an object shape.
		for _, f := range cur.Fields {
			if cur.FQN == "object" && f.Key == name {
				return f.Type
			}
		}
		return ResolvedType{}
	}
	var call *CallSite
	if st.call {
		call = r.CallSiteFor(st.args, ctx)
	}
	return r.MemberTypeForCall(member, ctx.File, r.ReceiverFor(cur, member), cur.Params, call)
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
		if m := r.FindAccessedMember(rt.FQN, "offsetGet", true); m != nil {
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
	if r.Index == nil {
		return ResolvedType{}
	}
	fn := r.lookupFunction(name, ctx.File)
	if fn == nil {
		return ResolvedType{}
	}
	return r.FunctionTypeForCall(fn, ctx.File, r.CallSiteFor(args, ctx))
}

// lookupFunction finds the function a call names, in the file's namespace
// first and then globally, as PHP does.
func (r *Resolver) lookupFunction(name string, file *parser.FileNode) *symbols.Symbol {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "\\") {
		if sym := r.Index.Lookup(name[1:]); sym != nil && sym.Kind == symbols.KindFunction {
			return sym
		}
		return nil
	}
	if file != nil && file.Namespace != "" {
		if sym := r.Index.Lookup(file.Namespace + "\\" + name); sym != nil && sym.Kind == symbols.KindFunction {
			return sym
		}
	}
	if sym := r.Index.Lookup(name); sym != nil && sym.Kind == symbols.KindFunction {
		return sym
	}
	short := name
	if i := strings.LastIndexByte(short, '\\'); i >= 0 {
		short = short[i+1:]
	}
	for _, sym := range r.Index.LookupByName(short) {
		if sym.Kind == symbols.KindFunction {
			return sym
		}
	}
	return nil
}

// NewExpressionType types `new Foo(args)`. The class's templates are bound
// from the constructor's arguments: `new Collection([$user])` is a
// Collection<int, User>.
func (r *Resolver) NewExpressionType(className, args string, ctx ExprContext) ResolvedType {
	className = strings.TrimSpace(className)
	if strings.HasPrefix(className, "$") {
		// new $class: the class a class-string<T> names.
		if name := extractLeadingVariable(className); name == className {
			var vt ResolvedType
			if t, ok := ctx.Locals[name]; ok {
				vt = t
			} else if ctx.File != nil {
				vt = r.ResolveVariableTypeTyped(name, ctx.Lines, ctx.Pos, ctx.File)
			}
			if t := ClassStringTarget(vt); t.FQN != "class-string" && t.FQN != "string" && !t.IsEmpty() {
				return t
			}
		}
		return ResolvedType{}
	}
	if className == "" || strings.HasPrefix(className, "class") {
		return ResolvedType{}
	}
	fqn, isKeyword := r.ClassKeywordFQN(className, ctx.Pos, ctx.File)
	if !isKeyword {
		fqn = r.ResolveClassName(className, ctx.File)
	}
	if fqn == "" {
		return ResolvedType{}
	}
	tmpls := r.templatesOf(fqn)
	if len(tmpls) == 0 || strings.TrimSpace(args) == "" || r.Index == nil {
		return ResolvedType{FQN: fqn}
	}
	ctor := r.FindMember(fqn, "__construct")
	if ctor == nil {
		return ResolvedType{FQN: fqn}
	}
	params := r.paramPatterns(ctor, parser.ParseDocBlock(ctor.DocComment))
	names := templateNames(tmpls, ctor.Templates)
	bound := r.bindCallTemplates(params, r.CallSiteFor(args, ctx), names, nil, ctor.ParentFQN, ctx.File)
	r.dropOutOfBound(bound, tmpls, fqn, ctx.File)
	r.dropOutOfBound(bound, ctor.Templates, ctor.ParentFQN, ctx.File)
	if len(bound) == 0 {
		// A constructor without types for its templates, given an array: its
		// keys and values are the class's arguments, in order.
		if len(params) > 0 {
			argList := ParseCallArgs(args)
			if len(argList) > 0 {
				if at := r.ExpressionType(argList[0].Expr, ctx); at.FQN == "array" {
					if rt := r.inferConstructorGenerics(fqn, at); !rt.IsEmpty() {
						return rt
					}
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

// arrayLiteralType types an array literal. A list of values that aren't
// plain literals, such as [$user, new User], gets the union of their types.
func (r *Resolver) arrayLiteralType(literal string, ctx ExprContext) ResolvedType {
	rt := InferArrayLiteralType(literal)
	body := literal
	if strings.HasPrefix(strings.ToLower(body), "array(") {
		body = "[" + body[6:]
		if strings.HasSuffix(body, ")") {
			body = body[:len(body)-1] + "]"
		}
	}
	if len(body) < 2 || body[len(body)-1] != ']' {
		return rt
	}
	inner := strings.TrimSpace(body[1 : len(body)-1])
	if inner == "" {
		return rt
	}
	values := splitArrayValues(inner)
	if len(values) == 0 {
		return rt
	}
	// Associative arrays with values the literal inference can't read.
	if IndexOutsideBrackets(values[0], '=') >= 0 && strings.Contains(values[0], "=>") {
		if rt.Shape == "" {
			return rt
		}
		fields := make([]ShapeEntry, 0, len(values))
		for _, v := range values {
			arrow := strings.Index(v, "=>")
			if arrow < 0 {
				return rt
			}
			key := strings.Trim(strings.TrimSpace(v[:arrow]), `'"`)
			vt := r.ExpressionType(strings.TrimSpace(v[arrow+2:]), ctx)
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
	parts := make([]ResolvedType, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "...") {
			if _, ev, ok := r.IterableTypes(r.ExpressionType(v[3:], ctx)); ok {
				parts = append(parts, ev)
				continue
			}
			return rt
		}
		vt := r.ExpressionType(v, ctx)
		if vt.IsEmpty() {
			return rt
		}
		parts = append(parts, vt)
	}
	return ResolvedType{FQN: "array", Params: []ResolvedType{{FQN: "int"}, MakeUnion(parts, false)}}
}

// matchingParen returns the index of the parenthesis closing the one at
// open, skipping quoted strings.
func matchingParen(s string, open int) int {
	if open >= len(s) || s[open] != '(' {
		return -1
	}
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// unwrapParens returns the inside of an expression wrapped in one pair of
// parentheses, as in `(new Foo)`.
func unwrapParens(expr string) (string, bool) {
	if len(expr) < 2 || expr[0] != '(' {
		return "", false
	}
	if c := matchingParen(expr, 0); c == len(expr)-1 {
		inner := strings.TrimSpace(expr[1:c])
		// A cast like (int) isn't a wrapped expression.
		if castType(expr) != "" && !strings.ContainsAny(inner, " $(") {
			return "", false
		}
		return inner, true
	}
	return "", false
}
