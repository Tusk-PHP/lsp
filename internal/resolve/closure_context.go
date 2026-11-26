package resolve

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A closure passed to a call takes what the parameter it's passed for says
// about it: `map(callable(TValue): TMapValue $callback)` types the closure's
// first parameter as the collection's value type, and
// `@param-closure-this Builder $callback` makes $this a Builder inside it.
// This file finds that call and reads the parameter.

// closureCallSite is the call a closure is an argument of.
type closureCallSite struct {
	tree       *syntaxTree
	receiver   ast.Expr // what's before -> or ::
	op         string   // "->", "::", "new" or "" for a function call
	name       string   // the method, function or class called
	argIndex   int      // which positional argument the closure is
	argName    string   // the parameter a named argument is for
	argsBefore []CallArg
	at         protocol.Position
}

func (r *Resolver) closureCall(lines []string, file *parser.FileNode, start protocol.Position) (closureCallSite, bool) {
	if file == nil {
		return closureCallSite{}, false
	}
	t := r.syntaxTree(lines, file)
	off, ok := t.offset(start)
	if !ok {
		return closureCallSite{}, false
	}
	path := t.path(off)
	i := len(path) - 1
	for i >= 0 {
		if _, ok := path[i].(*ast.Closure); ok {
			break
		}
		if _, ok := path[i].(*ast.ArrowFunction); ok {
			break
		}
		i--
	}
	if i < 3 {
		return closureCallSite{}, false
	}
	arg, isArg := path[i-1].(*ast.Arg)
	list, isList := path[i-2].(*ast.ArgList)
	if !isArg || !isList {
		return closureCallSite{}, false
	}
	k := slices.Index(list.Args, arg)
	line, col := t.lines.Position(list.Start)
	site := closureCallSite{tree: t, argIndex: k, argsBefore: t.callArgList(&ast.ArgList{Args: list.Args[:k]}), at: protocol.Position{Line: line, Character: col}}
	if arg.Name != nil {
		site.argName = t.text(arg.Name)
	}
	switch call := path[i-3].(type) {
	case *ast.MethodCall:
		site.receiver, site.op, site.name = call.Object, "->", t.text(call.Name)
	case *ast.StaticCall:
		site.receiver, site.op, site.name = call.Class, "::", t.text(call.Name)
	case *ast.New:
		name, named := call.Class.(*ast.ConstFetch)
		if !named {
			return closureCallSite{}, false
		}
		site.op, site.name = "new", t.text(name)
	case *ast.Call:
		name, named := call.Callee.(*ast.ConstFetch)
		if !named {
			return closureCallSite{}, false
		}
		site.name = t.text(name)
	default:
		return closureCallSite{}, false
	}
	return site, true
}

// calleeForClosure works out the method, function or constructor a closure
// is passed to, with the receiver type the call is made on.
func (r *Resolver) calleeForClosure(site closureCallSite, lines []string, file *parser.FileNode) (member *symbols.Symbol, receiver ResolvedType, ok bool) {
	if r.Index == nil {
		return nil, ResolvedType{}, false
	}
	switch site.op {
	case "->", "::":
		ctx := ExprContext{Lines: lines, Pos: site.at, File: file}
		rt := r.delegatedType(site.tree, site.receiver, ctx)
		if rt.IsEmpty() {
			rt = r.typeOf(site.tree, site.receiver, nil, ctx)
		}
		if rt.IsEmpty() {
			return nil, ResolvedType{}, false
		}
		m := r.FindAccessedMember(rt.MemberOwner(), site.name, CallAccess)
		if m == nil {
			return nil, ResolvedType{}, false
		}
		return m, rt, true
	case "new":
		fqn, isKeyword := r.ClassKeywordFQN(site.name, site.at, file)
		if !isKeyword {
			fqn = r.ResolveClassName(site.name, file)
		}
		m := r.FindMember(fqn, "__construct")
		if m == nil {
			return nil, ResolvedType{}, false
		}
		return m, ResolvedType{FQN: fqn}, true
	default:
		fn := r.CalledFunction(site.name, file)
		if fn == nil {
			return nil, ResolvedType{}, false
		}
		return fn, ResolvedType{}, true
	}
}

// closureParamFromCall returns the expected type of a closure's parameter,
// and what $this is inside the closure, from the call it's passed to.
func (r *Resolver) closureExpectations(closure *scope.Scope, lines []string, file *parser.FileNode) (expected ResolvedType, closureThis ResolvedType, ok bool) {
	if closure == nil || closure.Kind != scope.ScopeClosure {
		return ResolvedType{}, ResolvedType{}, false
	}
	// Every $this and parameter in a closure asks the same question: answer
	// it once per closure while the file doesn't change.
	if m, found := r.closureMemoGet(closure); found {
		return m.expected, m.this, m.ok
	}
	expected, closureThis, ok = r.computeClosureExpectations(closure, lines, file)
	r.closureMemoPut(closure, closureMemo{expected: expected, this: closureThis, ok: ok})
	return expected, closureThis, ok
}

// closureMemo is what closureExpectations found for one closure.
type closureMemo struct {
	expected, this ResolvedType
	ok             bool
}

func (r *Resolver) closureMemoGet(closure *scope.Scope) (closureMemo, bool) {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	m, ok := r.closureMemos[closure]
	return m, ok
}

// closureMemoPut keeps a result for as long as the scopes it was found in
// are current; switching files drops them all.
func (r *Resolver) closureMemoPut(closure *scope.Scope, m closureMemo) {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.scopeDoc == nil || r.closureMemoDoc != r.scopeDoc {
		r.closureMemos = make(map[*scope.Scope]closureMemo)
		r.closureMemoDoc = r.scopeDoc
	}
	if r.closureMemoDoc != nil && len(r.closureMemos) < 4096 {
		r.closureMemos[closure] = m
	}
}

func (r *Resolver) computeClosureExpectations(closure *scope.Scope, lines []string, file *parser.FileNode) (expected ResolvedType, closureThis ResolvedType, ok bool) {
	key := "closure|" + itoa(closure.Range.Start.Line) + ":" + itoa(closure.Range.Start.Character)
	exit, entered := r.enterResolution(key)
	if !entered {
		return ResolvedType{}, ResolvedType{}, false
	}
	defer exit()

	site, found := r.closureCall(lines, file, closure.Range.Start)
	if !found {
		return ResolvedType{}, ResolvedType{}, false
	}
	member, receiver, found := r.calleeForClosure(site, lines, file)
	if !found {
		return ResolvedType{}, ResolvedType{}, false
	}
	var doc *parser.DocBlock
	if member.DocComment != "" {
		doc = r.parseDoc(member.DocComment)
	}
	params := r.paramPatterns(member, doc)
	idx := -1
	for i, p := range params {
		if site.argName != "" && strings.TrimPrefix(p.name, "$") == site.argName {
			idx = i
			break
		}
	}
	if site.argName == "" {
		idx = site.argIndex
		if idx >= len(params) && len(params) > 0 && params[len(params)-1].variadic {
			idx = len(params) - 1
		}
	}
	if idx < 0 || idx >= len(params) {
		return ResolvedType{}, ResolvedType{}, false
	}

	owner := member.ParentFQN
	if member.Kind == symbols.KindFunction {
		owner = member.FQN
	}
	var subst map[string]ResolvedType
	if member.Kind == symbols.KindMethod {
		staticFQN := r.ReceiverFor(receiver, member)
		subst = r.classSubst(member.ParentFQN, staticFQN, receiver.Params)
	}
	if len(member.Templates) > 0 && len(site.argsBefore) > 0 {
		ctx := ExprContext{Lines: lines, Pos: site.at, File: file}
		bound := r.bindCallTemplates(append([]paramPattern(nil), params...), r.callSite(site.argsBefore, ctx), templateNames(member.Templates), subst, owner, file)
		if subst == nil {
			subst = make(map[string]ResolvedType, len(bound))
		}
		for k, v := range bound {
			subst[k] = v
		}
	}
	subst = r.fillUnbound(subst, member.Templates, owner, file)
	subst = r.fillUnbound(subst, r.templatesOf(member.ParentFQN), owner, file)
	env := typeEnv{subst: subst, static: receiver, owner: owner, file: file}
	expected = r.applyEnv(params[idx].pattern, env)
	if doc != nil {
		for _, ct := range doc.ClosureThis {
			if ct.Name == params[idx].name {
				closureThis = r.applyEnv(r.parseType(ct.Type), env)
			}
		}
	}
	return expected, closureThis, true
}

// closureParamIndex returns which parameter of its closure a binding is.
func closureParamIndex(param *scope.Binding) int {
	n := 0
	for _, b := range param.Scope.Bindings {
		if b.Kind != scope.BindingParameter {
			continue
		}
		if b == param {
			return n
		}
		n++
	}
	return -1
}

// callableSignature returns the callable(…) or Closure(…) signature in rt.
func callableSignature(rt ResolvedType) *CallableType {
	for _, p := range rt.Parts() {
		if p.Callable != nil {
			return p.Callable
		}
	}
	return nil
}

// InvokeType is what calling a value of type rt gives: a Closure(…): T's T,
// or what an object's __invoke() returns.
func (r *Resolver) InvokeType(rt ResolvedType) ResolvedType {
	if sig := callableSignature(rt); sig != nil {
		if sig.Return != nil {
			return *sig.Return
		}
		return ResolvedType{}
	}
	// A Closure without a signature: its stubbed __invoke says only mixed.
	if rt.FQN != "" && rt.FQN != "Closure" && !IsBuiltinTypeName(rt.FQN) && r.Index != nil {
		if m := r.FindAccessedMember(rt.FQN, "__invoke", CallAccess); m != nil {
			return r.MemberTypeForCall(m, nil, rt.FQN, rt.Params, nil)
		}
	}
	return ResolvedType{}
}
