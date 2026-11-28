package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// PHP 8.1's first-class callable syntax, f(...), $obj->m(...) and
// Foo::m(...), makes a Closure of the callee instead of calling it. Its type
// is the callee's signature as a Closure(…): T, so calling the closure or
// passing it to a callable(…) parameter reads like a written closure.

// IsFirstClassCallableArgs reports whether args, the text between a call's
// parentheses, is the `...` of first-class callable syntax. A spread,
// f(...$args), is an ordinary call.
func IsFirstClassCallableArgs(args string) bool {
	return strings.TrimSpace(args) == "..."
}

// IsFirstClassCallable reports whether expr ends in first-class callable
// syntax: `strlen(...)`, `$this->format(...)`.
func IsFirstClassCallable(expr string) bool {
	s := strings.TrimRight(strings.TrimSpace(expr), "; \t")
	if !strings.HasSuffix(s, ")") {
		return false
	}
	s = strings.TrimRight(s[:len(s)-1], " \t\r\n")
	if !strings.HasSuffix(s, "...") {
		return false
	}
	s = strings.TrimRight(s[:len(s)-3], " \t\r\n")
	return strings.HasSuffix(s, "(")
}

// plainClosure is a Closure whose signature isn't known.
func plainClosure() ResolvedType {
	return ResolvedType{FQN: "Closure"}
}

// MemberCallableType is the Closure that `$obj->m(...)` or `Foo::m(...)`
// makes of a method: its parameters and return type with the receiver's
// templates (callerFQN with typeContext as its type arguments) applied. The
// method's own templates can't be bound without a call, so they fall back to
// their default, bound or mixed.
func (r *Resolver) MemberCallableType(member *symbols.Symbol, file *parser.FileNode, callerFQN string, typeContext []ResolvedType) ResolvedType {
	if member == nil || member.Kind != symbols.KindMethod || r.Index == nil {
		return plainClosure()
	}
	staticFQN := callerFQN
	if staticFQN == "" {
		staticFQN = member.ParentFQN
	}
	subst := r.classSubst(member.ParentFQN, staticFQN, typeContext)
	subst = r.fillUnbound(subst, member.Templates, member.ParentFQN, file)
	subst = r.fillUnbound(subst, r.templatesOf(member.ParentFQN), member.ParentFQN, file)
	env := typeEnv{subst: subst, static: ResolvedType{FQN: staticFQN, Params: typeContext}, owner: member.ParentFQN, file: file}
	sig := &CallableType{Params: r.callableParams(member, env), Names: paramNames(member)}
	if ret := r.MemberTypeForCall(member, file, callerFQN, typeContext, nil); !ret.IsEmpty() || ret.Nullable {
		sig.Return = &ret
	}
	return ResolvedType{FQN: "Closure", Callable: sig}
}

// FunctionCallableType is the Closure that `f(...)` makes of a function.
func (r *Resolver) FunctionCallableType(fn *symbols.Symbol, file *parser.FileNode) ResolvedType {
	if fn == nil || r.Index == nil {
		return plainClosure()
	}
	subst := r.fillUnbound(nil, fn.Templates, fn.FQN, file)
	sig := &CallableType{Params: r.callableParams(fn, typeEnv{subst: subst, owner: fn.FQN, file: file}), Names: paramNames(fn)}
	if ret := r.FunctionTypeForCall(fn, file, nil); !ret.IsEmpty() || ret.Nullable {
		sig.Return = &ret
	}
	return ResolvedType{FQN: "Closure", Callable: sig}
}

// CallableOf is the Closure that `$fn(...)` makes of a value of type rt: the
// same signature for a closure or callable, __invoke's for an invokable
// object.
func (r *Resolver) CallableOf(rt ResolvedType) ResolvedType {
	if sig := callableSignature(rt); sig != nil {
		return ResolvedType{FQN: "Closure", Callable: sig}
	}
	// A Closure without a signature stays one: its stubbed __invoke(mixed
	// ...$args) says nothing more.
	if rt.FQN != "" && rt.FQN != "Closure" && !IsBuiltinTypeName(rt.FQN) && r.Index != nil {
		if m := r.FindAccessedMember(rt.FQN, "__invoke", CallAccess); m != nil {
			return r.MemberCallableType(m, nil, rt.FQN, rt.Params)
		}
	}
	return plainClosure()
}

// ClosureCallType is what `$closure->call($newThis, …)` gives on a value of
// type rt with a known signature: what the closure returns. ok is false when
// name isn't call or rt's signature doesn't say.
func ClosureCallType(rt ResolvedType, name string) (ResolvedType, bool) {
	if rt.Callable == nil || rt.Callable.Return == nil || name != "call" {
		return ResolvedType{}, false
	}
	return *rt.Callable.Return, true
}

// invokedVariableType types an assigned `$fn(…)`, what calling the variable
// gives, and `$fn(...)`, a Closure of it. ok is false for anything else, and
// for a variable assigned from calling itself.
func (r *Resolver) invokedVariableType(rhs, varName string, ctx ExprContext) (ResolvedType, bool) {
	if rhs == "" || rhs[0] != '$' {
		return ResolvedType{}, false
	}
	steps, origin, ok := parseChain(rhs)
	if !ok || len(steps) == 0 || steps[0].op != "()" || origin.text == varName {
		return ResolvedType{}, false
	}
	rt := r.ExpressionType(rhs, ctx)
	return rt, !rt.IsEmpty()
}

// paramNames returns the names of sym's parameters, or nil when it has none.
func paramNames(sym *symbols.Symbol) []string {
	if len(sym.Params) == 0 {
		return nil
	}
	names := make([]string, len(sym.Params))
	for i, p := range sym.Params {
		names[i] = p.Name
	}
	return names
}

// callableParams is the parameter types of a method or function as a
// callable signature lists them, read in env; an untyped one is mixed.
func (r *Resolver) callableParams(sym *symbols.Symbol, env typeEnv) []ResolvedType {
	if len(sym.Params) == 0 {
		return nil
	}
	params := r.paramPatterns(sym, r.parseDoc(sym.DocComment))
	out := make([]ResolvedType, len(params))
	for i, p := range params {
		if p.pattern.IsEmpty() {
			out[i] = ResolvedType{FQN: "mixed"}
			continue
		}
		out[i] = r.applyEnv(p.pattern, env)
	}
	return out
}

// PHP 8.6's partial function application, f(?, 'x'), f('x', ...) and
// f(?, name: ?), also makes a Closure instead of calling: of the parameters
// the `?` placeholders leave open, plus every parameter not given when a
// `...` placeholder ends the list. f(...) is the case where nothing is given.

// IsPartialApplicationArgs reports whether args, the text between a call's
// parentheses, has a `?` or bare `...` placeholder, so the call makes a
// Closure. f(...) on its own is first-class callable syntax and counts too.
func IsPartialApplicationArgs(args string) bool {
	if !strings.Contains(args, "?") && !strings.Contains(args, "...") {
		return false
	}
	for _, a := range ParseCallArgs(args) {
		if isPlaceholderArg(a) {
			return true
		}
	}
	return false
}

// IsCallableArgs reports whether a call with these arguments makes a Closure
// rather than calling: first-class callable syntax or partial application.
func IsCallableArgs(args string) bool {
	return IsFirstClassCallableArgs(args) || IsPartialApplicationArgs(args)
}

// isPartialApplicationCall reports whether expr is a single call whose
// arguments are a partial application: `greet('Hi', ?)`.
func isPartialApplicationCall(expr string) bool {
	s := strings.TrimRight(strings.TrimSpace(expr), "; \t")
	open := strings.IndexByte(s, '(')
	if open < 0 || !strings.Contains(s, "?") && !strings.Contains(s, "...") {
		return false
	}
	end := matchingParen(s, open)
	return end == len(s)-1 && IsPartialApplicationArgs(s[open+1:end])
}

func isPlaceholderArg(a CallArg) bool {
	return (!a.Spread && a.Expr == "?") || (a.Spread && a.Expr == "")
}

// PartialClosure narrows full, the Closure of the whole callee sym, to the
// parameters a partial application's args leave open. A first-class
// callable, or a callee or signature that isn't known, keeps full.
func PartialClosure(full ResolvedType, sym *symbols.Symbol, args string) ResolvedType {
	if IsFirstClassCallableArgs(args) || sym == nil || full.Callable == nil || len(full.Callable.Params) != len(sym.Params) {
		return full
	}
	open := make([]bool, len(sym.Params))
	given := make([]bool, len(sym.Params))
	rest := false
	pos := 0
	for _, a := range ParseCallArgs(args) {
		if a.Spread && a.Expr == "" {
			rest = true
			continue
		}
		i := -1
		if a.Name != "" {
			for j, p := range sym.Params {
				if strings.TrimPrefix(p.Name, "$") == a.Name {
					i = j
					break
				}
			}
		} else if pos < len(sym.Params) {
			i = pos
			pos++
		}
		if i < 0 {
			continue
		}
		if a.Expr == "?" {
			open[i] = true
		} else {
			given[i] = true
		}
	}
	sig := &CallableType{Return: full.Callable.Return}
	for i, p := range full.Callable.Params {
		if open[i] || (rest && !given[i]) {
			sig.Params = append(sig.Params, p)
			sig.Names = append(sig.Names, sym.Params[i].Name)
		}
	}
	return ResolvedType{FQN: "Closure", Callable: sig}
}
