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
	sig := &CallableType{Params: r.callableParams(member, env)}
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
	sig := &CallableType{Params: r.callableParams(fn, typeEnv{subst: subst, owner: fn.FQN, file: file})}
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
		if m := r.FindAccessedMember(rt.FQN, "__invoke", true); m != nil {
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
