package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// CallArg is one argument of a call as written: `$user`, or `name: $user`
// for a named argument.
type CallArg struct {
	Name   string // the parameter a named argument is for, without $
	Expr   string
	Spread bool // ...$args
}

// ParseCallArgs splits the text between a call's parentheses into its
// arguments.
func ParseCallArgs(argStr string) []CallArg {
	parts := splitArgList(argStr)
	args := make([]CallArg, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		arg := CallArg{Expr: p}
		if strings.HasPrefix(p, "...") {
			arg.Spread = true
			arg.Expr = strings.TrimSpace(p[3:])
		} else if name, rest, ok := namedArg(p); ok {
			arg.Name, arg.Expr = name, rest
		}
		args = append(args, arg)
	}
	return args
}

// namedArg splits `name: expr`, leaving `Foo::bar` and ternaries alone.
func namedArg(p string) (name, rest string, ok bool) {
	i := 0
	for i < len(p) && (isIdentByte(p[i]) || (i > 0 && p[i] >= '0' && p[i] <= '9')) {
		i++
	}
	if i == 0 {
		return "", "", false
	}
	j := i
	for j < len(p) && (p[j] == ' ' || p[j] == '\t') {
		j++
	}
	if j >= len(p) || p[j] != ':' || (j+1 < len(p) && p[j+1] == ':') {
		return "", "", false
	}
	return p[:i], strings.TrimSpace(p[j+1:]), true
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// CallSite is a call's arguments, with a way to work out each one's type.
// expected, when known, is the type the parameter declares; a closure passed
// to a callable(User): T parameter uses it to type its own parameters.
type CallSite struct {
	Args    []CallArg
	ArgType func(expr string, expected *ResolvedType) ResolvedType
}

// paramPattern is a parameter's declared type, as written in its PHPDoc or
// signature, before templates are substituted.
type paramPattern struct {
	name     string // with $
	pattern  ResolvedType
	variadic bool
	def      string
}

// paramPatterns returns the declared types of a method's or function's
// parameters: @param where it's given, else the native type.
func (r *Resolver) paramPatterns(sym *symbols.Symbol, doc *parser.DocBlock) []paramPattern {
	out := make([]paramPattern, 0, len(sym.Params))
	for _, p := range sym.Params {
		raw := ""
		if doc != nil {
			raw = resolveDocParam(doc, p.Name)
			if raw == "" {
				raw = resolveDocParam(doc, "..."+p.Name)
			}
		}
		if raw == "" {
			raw = p.Type
		}
		pp := paramPattern{name: p.Name, variadic: p.IsVariadic, def: p.DefaultValue}
		if raw != "" {
			pp.pattern = r.parseType(r.expandTypeAliases(raw, ownerScopeFQN(sym, positionZero, nil), nil))
		}
		out = append(out, pp)
	}
	return out
}

// argumentsByParam pairs each parameter with the arguments passed for it:
// positional ones in order, named ones by name, and the rest to a variadic.
func argumentsByParam(params []paramPattern, args []CallArg) [][]CallArg {
	out := make([][]CallArg, len(params))
	pos := 0
	for _, a := range args {
		if a.Name != "" {
			for i, p := range params {
				if strings.TrimPrefix(p.name, "$") == a.Name {
					out[i] = append(out[i], a)
					break
				}
			}
			continue
		}
		if pos >= len(params) {
			if n := len(params); n > 0 && params[n-1].variadic {
				out[n-1] = append(out[n-1], a)
			}
			continue
		}
		out[pos] = append(out[pos], a)
		if !params[pos].variadic {
			pos++
		}
	}
	return out
}

// templateNames is the set of names in tmpls.
func templateNames(tmpls ...[]symbols.TemplateParam) map[string]bool {
	n := 0
	for _, t := range tmpls {
		n += len(t)
	}
	names := make(map[string]bool, n)
	for _, t := range tmpls {
		for _, p := range t {
			names[p.Name] = true
		}
	}
	return names
}

// bindCallTemplates binds the templates in names from a call's arguments,
// matching each argument's type against its parameter's declared type. Class
// templates already known from the receiver (in fixed) are substituted into
// the parameter types first, so a closure passed to map(callable(TValue))
// sees the collection's value type.
func (r *Resolver) bindCallTemplates(params []paramPattern, call *CallSite, names map[string]bool, fixed map[string]ResolvedType, owner string, file *parser.FileNode) map[string]ResolvedType {
	bound := make(map[string]ResolvedType, len(names))
	if len(names) == 0 {
		return bound
	}
	if call == nil {
		call = &CallSite{}
	}
	byParam := argumentsByParam(params, call.Args)
	// Patterns are matched with their class names resolved and the receiver's
	// templates substituted; the call's own templates stay as names.
	keep := make(map[string]ResolvedType, len(names)+len(fixed))
	for k, v := range fixed {
		keep[k] = v
	}
	for name := range names {
		keep[name] = ResolvedType{FQN: name}
	}
	for i := range params {
		if !params[i].pattern.IsEmpty() {
			params[i].pattern = r.applyEnv(params[i].pattern, typeEnv{subst: keep, owner: owner, file: file})
		}
	}
	for i, p := range params {
		if p.pattern.IsEmpty() {
			continue
		}
		if len(byParam[i]) == 0 {
			// Not passed: a template only this parameter names takes the type
			// of its default value, as `$default = null` makes it null.
			if p.def != "" && !p.variadic {
				if dt := literalExprType(p.def); !dt.IsEmpty() {
					r.bindTemplates(p.pattern, dt, names, bound, 0)
				}
			}
			continue
		}
		if call.ArgType == nil {
			continue
		}
		expected := r.applyEnv(p.pattern, typeEnv{subst: fixed, owner: owner, file: file})
		for _, a := range byParam[i] {
			actual := call.ArgType(a.Expr, &expected)
			if actual.IsEmpty() && !actual.Nullable {
				continue
			}
			if a.Spread {
				if _, v, ok := r.IterableTypes(actual); ok {
					actual = v
				}
			}
			r.bindTemplates(p.pattern, actual, names, bound, 0)
		}
	}
	return bound
}

// dropOutOfBound forgets bindings that can't be right because they break
// their template's bound, as a string given for `T of object`. The call is
// wrong there, and PHPStan reports it; the template is left unbound.
func (r *Resolver) dropOutOfBound(bound map[string]ResolvedType, tmpls []symbols.TemplateParam, owner string, file *parser.FileNode) {
	for _, t := range tmpls {
		v, ok := bound[t.Name]
		if !ok || t.Bound == "" {
			continue
		}
		b := r.applyEnv(r.parseType(t.Bound), typeEnv{owner: owner, file: file})
		if r.typeMatches(v, b) == matchNo {
			delete(bound, t.Name)
		}
	}
}

// MemberTypeForCall returns what a member gives where it's used: a
// property's type, or the type a method call returns. Templates are bound
// from the receiver (callerFQN with typeContext as its type arguments, passed
// up through @extends and friends to the declaring class) and, for a call,
// from its arguments; conditional return types are decided by the arguments;
// and what nothing binds falls back to the template's default or bound.
func (r *Resolver) MemberTypeForCall(member *symbols.Symbol, file *parser.FileNode, callerFQN string, typeContext []ResolvedType, call *CallSite) ResolvedType {
	if member == nil {
		return ResolvedType{}
	}
	// Framework classes whose PHPDoc has no generics use the hard-coded
	// registry; ones that declare templates are read from their PHPDoc.
	if len(typeContext) > 0 && member.Kind == symbols.KindMethod && HasTemplateMapping(member.ParentFQN) {
		if sym := r.Index.Lookup(member.ParentFQN); sym == nil || len(sym.Templates) == 0 {
			if rt := r.memberTypeFallback(member, file, callerFQN, typeContext); !rt.IsEmpty() {
				return rt
			}
		}
	}
	// A method without a PHPDoc return type inherits the one its parent,
	// interface or trait documents, read with the arguments passed up to it.
	if member.Kind == symbols.KindMethod && !strings.Contains(member.DocComment, "return") {
		if inh := r.inheritedDocMethod(member); inh != nil {
			if callerFQN == "" {
				callerFQN = member.ParentFQN
			}
			member = inh
		}
	}
	raw := r.rawMemberReturnType(member)
	staticFQN := callerFQN
	if staticFQN == "" {
		staticFQN = member.ParentFQN
	}
	if raw == "" || raw == "void" {
		return r.memberTypeFallback(member, file, callerFQN, typeContext)
	}
	pattern := r.parseType(raw)
	if pattern.IsEmpty() {
		return r.memberTypeFallback(member, file, callerFQN, typeContext)
	}

	classTmpls := r.templatesOf(member.ParentFQN)
	subst := r.classSubst(member.ParentFQN, staticFQN, typeContext)
	var staticTmpls []symbols.TemplateParam
	if !strings.EqualFold(staticFQN, member.ParentFQN) {
		staticTmpls = r.templatesOf(staticFQN)
	}

	var params []paramPattern
	if member.Kind == symbols.KindMethod {
		var doc *parser.DocBlock
		if member.DocComment != "" && (len(member.Templates) > 0 || pattern.hasConditional()) {
			doc = r.parseDoc(member.DocComment)
		}
		if len(member.Templates) > 0 || pattern.hasConditional() {
			params = r.paramPatterns(member, doc)
		}
		if len(member.Templates) > 0 {
			bound := r.bindCallTemplates(params, call, templateNames(member.Templates), subst, member.ParentFQN, file)
			r.dropOutOfBound(bound, member.Templates, member.ParentFQN, file)
			if len(bound) > 0 && subst == nil {
				subst = make(map[string]ResolvedType, len(bound))
			}
			for k, v := range bound {
				subst[k] = v
			}
		}
	}
	subst = r.fillUnbound(subst, member.Templates, member.ParentFQN, file)
	subst = r.fillUnbound(subst, classTmpls, member.ParentFQN, file)
	subst = r.fillUnbound(subst, staticTmpls, staticFQN, file)

	if pattern.hasConditional() {
		pattern = r.decideConditionals(pattern, params, call, subst, member.ParentFQN, file)
	}

	static := ResolvedType{FQN: staticFQN, Params: typeContext}
	rt := r.applyEnv(pattern, typeEnv{subst: subst, static: static, owner: member.ParentFQN, file: file})
	if rt.IsEmpty() && !rt.Nullable {
		if fb := r.memberTypeFallback(member, file, callerFQN, typeContext); !fb.IsEmpty() {
			return fb
		}
	}
	return rt
}

// memberTypeFallback is the type of a member whose PHPDoc and signature say
// nothing useful: the hard-coded generic registry for the framework classes it
// covers, else the plain member type.
func (r *Resolver) memberTypeFallback(member *symbols.Symbol, file *parser.FileNode, callerFQN string, typeContext []ResolvedType) ResolvedType {
	if len(typeContext) > 0 && member.Kind == symbols.KindMethod {
		if rt := ResolveTemplateReturn(member.ParentFQN, member.Name, typeContext); !rt.IsEmpty() {
			if rt.FQN == member.ParentFQN && callerFQN != "" {
				rt.FQN = callerFQN
			}
			return r.applyEnv(rt, typeEnv{owner: member.ParentFQN, file: file})
		}
	}
	fqn := r.MemberType(member, file)
	if fqn == "" {
		return ResolvedType{}
	}
	return ResolvedType{FQN: fqn}
}

// FunctionTypeForCall returns what a call to a function gives, binding the
// function's templates from the call's arguments and deciding its
// conditional return type.
func (r *Resolver) FunctionTypeForCall(fn *symbols.Symbol, file *parser.FileNode, call *CallSite) ResolvedType {
	if fn == nil {
		return ResolvedType{}
	}
	raw := r.functionReturnType(fn)
	if raw == "" || raw == "void" {
		return ResolvedType{}
	}
	pattern := r.parseType(raw)
	var params []paramPattern
	var subst map[string]ResolvedType
	if len(fn.Templates) > 0 || pattern.hasConditional() {
		params = r.paramPatterns(fn, r.parseDoc(fn.DocComment))
		if len(fn.Templates) > 0 {
			subst = r.bindCallTemplates(params, call, templateNames(fn.Templates), nil, fn.FQN, file)
			r.dropOutOfBound(subst, fn.Templates, fn.FQN, file)
		}
	}
	subst = r.fillUnbound(subst, fn.Templates, fn.FQN, file)
	if pattern.hasConditional() {
		pattern = r.decideConditionals(pattern, params, call, subst, fn.FQN, file)
	}
	return r.applyEnv(pattern, typeEnv{subst: subst, owner: fn.FQN, file: file})
}

// functionReturnType is a function's return type, preferring PHPDoc where it
// says more than the signature.
func (r *Resolver) functionReturnType(fn *symbols.Symbol) string {
	var docType string
	if fn.DocComment != "" {
		if doc := r.parseDoc(fn.DocComment); doc != nil {
			docType = doc.Return.Type
		}
	}
	return r.preferDocType(fn.ReturnType, docType, fn.Templates, "")
}

// hasConditional reports whether a conditional type appears anywhere in rt.
func (rt ResolvedType) hasConditional() bool {
	if rt.Conditional != nil {
		return true
	}
	for _, p := range rt.Union {
		if p.hasConditional() {
			return true
		}
	}
	for _, p := range rt.Params {
		if p.hasConditional() {
			return true
		}
	}
	return false
}

// decideConditionals replaces each conditional type in rt with the branch the
// call picks: `($as_float is true ? float : string)` is float when true is
// passed, string when false is, and either when it can't be told.
func (r *Resolver) decideConditionals(rt ResolvedType, params []paramPattern, call *CallSite, subst map[string]ResolvedType, owner string, file *parser.FileNode) ResolvedType {
	var byParam [][]CallArg
	if call != nil {
		byParam = argumentsByParam(params, call.Args)
	}
	subject := func(name string) (ResolvedType, bool) {
		if !strings.HasPrefix(name, "$") {
			t, ok := subst[name]
			return t, ok
		}
		for i, p := range params {
			if p.name != name {
				continue
			}
			if i < len(byParam) && len(byParam[i]) > 0 {
				if call.ArgType == nil {
					return ResolvedType{}, false
				}
				t := call.ArgType(byParam[i][0].Expr, nil)
				return t, !t.IsEmpty() || t.Nullable
			}
			if p.def != "" {
				return literalExprType(p.def), true
			}
			return ResolvedType{}, false
		}
		return ResolvedType{}, false
	}
	var decide func(t ResolvedType) ResolvedType
	decide = func(t ResolvedType) ResolvedType {
		return t.transformConditionals(func(c *ConditionalType, nullable bool) ResolvedType {
			s, ok := subject(c.Subject)
			target := r.applyEnv(c.Target, typeEnv{subst: subst, owner: owner, file: file})
			verdict := matchUnknown
			if ok {
				verdict = r.typeMatches(s, target)
			}
			if c.Negated && verdict != matchUnknown {
				if verdict == matchYes {
					verdict = matchNo
				} else {
					verdict = matchYes
				}
			}
			var out ResolvedType
			switch verdict {
			case matchYes:
				out = decide(c.IfTrue)
			case matchNo:
				out = decide(c.IfFalse)
			default:
				out = MakeUnion([]ResolvedType{decide(c.IfTrue), decide(c.IfFalse)}, false)
			}
			if nullable {
				out = MakeUnion([]ResolvedType{out}, true)
			}
			return out
		})
	}
	return decide(rt)
}

// transformConditionals replaces every conditional type in rt with what fn
// makes of it.
func (rt ResolvedType) transformConditionals(fn func(c *ConditionalType, nullable bool) ResolvedType) ResolvedType {
	if rt.Conditional != nil {
		return fn(rt.Conditional, rt.Nullable)
	}
	if len(rt.Union) > 0 {
		parts := make([]ResolvedType, len(rt.Union))
		for i, p := range rt.Union {
			parts[i] = p.transformConditionals(fn)
		}
		return MakeUnion(parts, rt.Nullable)
	}
	if len(rt.Params) > 0 {
		out := rt
		out.Params = make([]ResolvedType, len(rt.Params))
		for i, p := range rt.Params {
			out.Params[i] = p.transformConditionals(fn)
		}
		return out
	}
	return rt
}

type matchVerdict int

const (
	matchUnknown matchVerdict = iota
	matchYes
	matchNo
)

// typeMatches decides `subject is target` for a conditional type, when it can.
func (r *Resolver) typeMatches(subject, target ResolvedType) matchVerdict {
	if len(target.Union) > 0 {
		any := false
		unknown := false
		for _, t := range target.Union {
			switch r.typeMatches(subject, t) {
			case matchYes:
				any = true
			case matchUnknown:
				unknown = true
			}
		}
		if target.Nullable && subject.FQN == "" && subject.Nullable {
			any = true
		}
		switch {
		case any:
			return matchYes
		case unknown:
			return matchUnknown
		}
		return matchNo
	}
	// null
	if target.FQN == "" && target.Nullable || target.FQN == "null" {
		switch {
		case subject.FQN == "" && subject.Nullable, subject.FQN == "null":
			return matchYes
		case subject.Nullable, subject.FQN == "mixed":
			return matchUnknown
		}
		return matchNo
	}
	if len(subject.Union) > 0 || subject.Nullable && subject.FQN != "" {
		yes, no := 0, 0
		parts := subject.Parts()
		if subject.Nullable {
			parts = append(parts, ResolvedType{FQN: "null"})
		}
		for _, p := range parts {
			switch r.typeMatches(p, target) {
			case matchYes:
				yes++
			case matchNo:
				no++
			}
		}
		switch {
		case yes == len(parts):
			return matchYes
		case no == len(parts):
			return matchNo
		}
		return matchUnknown
	}
	if subject.FQN == "" || subject.FQN == "mixed" {
		return matchUnknown
	}
	// Literal targets: true, false, 'x', 42.
	if target.Literal != "" {
		if subject.Literal != "" {
			if subject.Literal == target.Literal {
				return matchYes
			}
			return matchNo
		}
		if subject.FQN == target.FQN || subject.FQN == "bool" && target.FQN == "bool" {
			return matchUnknown
		}
		return matchNo
	}
	sf, tf := strings.ToLower(subject.FQN), strings.ToLower(target.FQN)
	if sf == tf {
		return matchYes
	}
	sb, tb := IsBuiltinTypeName(subject.FQN), IsBuiltinTypeName(target.FQN)
	switch {
	case !sb && !tb:
		if r.Index != nil && r.Index.IsSubtypeOf(subject.FQN, target.FQN) {
			return matchYes
		}
		return matchNo
	case tf == "object":
		if !sb {
			return matchYes
		}
		return matchNo
	case tf == "scalar":
		switch sf {
		case "int", "float", "string", "bool":
			return matchYes
		}
		return matchNo
	case tf == "array" || tf == "iterable":
		if sf == "array" || sf == "list" || strings.HasSuffix(sf, "-array") || strings.HasSuffix(sf, "-list") {
			return matchYes
		}
		if tf == "iterable" && !sb {
			return matchUnknown
		}
		return matchNo
	case tb && sb:
		// int against positive-int and the like can't be told apart here.
		if strings.HasSuffix(tf, "-"+sf) || strings.HasSuffix(sf, "-"+tf) || strings.Contains(tf, "-") || strings.Contains(sf, "-") {
			return matchUnknown
		}
		return matchNo
	}
	return matchNo
}

// literalExprType is the type of a literal written in PHP code, keeping the
// value of true, false, numbers and strings so conditional types can test it.
func literalExprType(expr string) ResolvedType {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ResolvedType{}
	}
	lower := strings.ToLower(expr)
	switch lower {
	case "true", "false":
		return ResolvedType{FQN: "bool", Literal: lower}
	case "null":
		return ResolvedType{FQN: "null"}
	}
	switch t := InferLiteralType(expr); t {
	case "string":
		if expr[0] == '\'' || expr[0] == '"' {
			return ResolvedType{FQN: "string", Literal: expr}
		}
		return ResolvedType{FQN: "string"}
	case "int", "float":
		if expr[0] >= '0' && expr[0] <= '9' || expr[0] == '-' {
			return ResolvedType{FQN: t, Literal: expr}
		}
		return ResolvedType{FQN: t}
	case "":
		return ResolvedType{}
	default:
		return ResolvedType{FQN: t}
	}
}

var positionZero = protocol.Position{}

// inheritedDocMethod finds the nearest method of the same name above
// member's class (parent classes, interfaces, traits) whose PHPDoc gives a
// return type, as PHPStan inherits PHPDoc from what a method overrides.
func (r *Resolver) inheritedDocMethod(member *symbols.Symbol) *symbols.Symbol {
	if r.Index == nil || member.ParentFQN == "" {
		return nil
	}
	visited := map[string]bool{member.ParentFQN: true}
	queue := []string{member.ParentFQN}
	for steps := 0; len(queue) > 0 && steps < 64; steps++ {
		cur := queue[0]
		queue = queue[1:]
		sym := r.Index.Lookup(cur)
		if sym == nil {
			continue
		}
		next := make([]string, 0, 2+len(sym.Implements))
		if sym.Extends != "" {
			next = append(next, sym.Extends)
		}
		next = append(next, sym.Implements...)
		next = append(next, r.Index.TraitsOf(cur)...)
		for _, anc := range next {
			if visited[anc] {
				continue
			}
			visited[anc] = true
			if m := r.Index.Lookup(anc + "::" + member.Name); m != nil && m.Kind == symbols.KindMethod && strings.Contains(m.DocComment, "return") {
				if doc := r.parseDoc(m.DocComment); doc != nil && doc.Return.Type != "" {
					return m
				}
			}
			queue = append(queue, anc)
		}
	}
	return nil
}
