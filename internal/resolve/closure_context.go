package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
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
	receiver   string // the expression before -> or ::, or the class after new
	op         string // "->", "::", "new" or "" for a function call
	name       string // the method or function called
	argIndex   int    // which positional argument the closure is
	argName    string // the parameter a named argument is for
	argsBefore string // the arguments written before the closure
	at         protocol.Position
}

// maxCallLookback bounds how many lines above a closure its call is looked for.
const maxCallLookback = 40

// findClosureCall finds the call whose argument list the closure starting
// at start is in. ok is false when the closure isn't a call argument.
func findClosureCall(lines []string, start protocol.Position) (closureCallSite, bool) {
	if start.Line < 0 || start.Line >= len(lines) {
		return closureCallSite{}, false
	}
	first := start.Line - maxCallLookback
	if first < 0 {
		first = 0
	}
	var sb strings.Builder
	lineStarts := make([]int, 0, start.Line-first+1)
	for i := first; i < start.Line; i++ {
		lineStarts = append(lineStarts, sb.Len())
		sb.WriteString(lines[i])
		sb.WriteByte(' ')
	}
	lineStarts = append(lineStarts, sb.Len())
	cur := lines[start.Line]
	if start.Character > len(cur) {
		return closureCallSite{}, false
	}
	sb.WriteString(cur[:start.Character])
	text := sb.String()

	// Walk back to the unmatched ( that opens the argument list.
	depth := 0
	open := -1
	for i := len(text) - 1; i >= 0 && open < 0; i-- {
		switch text[i] {
		case ')', ']', '}':
			depth++
		case '(':
			if depth == 0 {
				open = i
			} else {
				depth--
			}
		case '[', '{':
			if depth == 0 {
				return closureCallSite{}, false
			}
			depth--
		case ';':
			if depth == 0 {
				return closureCallSite{}, false
			}
		}
	}
	if open <= 0 {
		return closureCallSite{}, false
	}
	site := closureCallSite{}
	before := strings.TrimSpace(text[open+1:])
	// A named argument: callback: function (…)
	if strings.HasSuffix(before, ":") && !strings.HasSuffix(before, "::") {
		k := len(before) - 1
		j := k
		for j > 0 && isIdentByte(before[j-1]) || j > 0 && before[j-1] >= '0' && before[j-1] <= '9' {
			j--
		}
		site.argName = before[j:k]
		before = strings.TrimSpace(before[:j])
	}
	before = strings.TrimSpace(strings.TrimSuffix(before, ","))
	site.argsBefore = before
	if before != "" {
		site.argIndex = len(splitArgList(before))
	}

	// The callee before the (.
	callee := strings.TrimRight(text[:open], " \t")
	end := len(callee)
	k := end
	for k > 0 && (isIdentByte(callee[k-1]) || callee[k-1] >= '0' && callee[k-1] <= '9' || callee[k-1] == '\\') {
		k--
	}
	if k == end {
		return closureCallSite{}, false
	}
	site.name = callee[k:end]
	rest := strings.TrimRight(callee[:k], " \t")
	switch {
	case strings.HasSuffix(rest, "->"):
		site.op = "->"
		site.receiver = strings.TrimSuffix(strings.TrimSuffix(rest, "->"), "?")
	case strings.HasSuffix(rest, "::"):
		site.op = "::"
		site.receiver = strings.TrimSuffix(rest, "::")
	case strings.HasSuffix(rest, "new"):
		site.op = "new"
		site.receiver = site.name
	default:
		if strings.HasSuffix(rest, "$") {
			return closureCallSite{}, false // $fn(function () {…})
		}
		site.op = ""
	}
	// Where the receiver is read: just before the call, in the enclosing scope.
	at := start
	for li := len(lineStarts) - 1; li >= 0; li-- {
		if open >= lineStarts[li] {
			at = protocol.Position{Line: first + li, Character: open - lineStarts[li]}
			break
		}
	}
	site.at = at
	return site, true
}

// calleeForClosure works out the method, function or constructor a closure
// is passed to, with the receiver type the call is made on.
func (r *Resolver) calleeForClosure(site closureCallSite, lines []string, file *parser.FileNode) (member *symbols.Symbol, receiver ResolvedType, ok bool) {
	if r.Index == nil {
		return nil, ResolvedType{}, false
	}
	ctx := ExprContext{Lines: lines, Pos: site.at, File: file}
	switch site.op {
	case "->", "::":
		recv := strings.TrimSpace(site.receiver)
		var rt ResolvedType
		if r.TypedChainResolver != nil && file != nil {
			rt = r.TypedChainResolver(recv, r.JoinLines(lines), site.at, file)
		}
		if rt.IsEmpty() {
			rt = r.ExpressionType(lastExpression(recv), ctx)
		}
		if rt.IsEmpty() {
			return nil, ResolvedType{}, false
		}
		m := r.FindAccessedMember(rt.MemberOwner(), site.name, true)
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
		fn := r.lookupFunction(site.name, file)
		if fn == nil {
			return nil, ResolvedType{}, false
		}
		return fn, ResolvedType{}, true
	}
}

// lastExpression returns the trailing expression of text, as far back as a
// chain of variables, names, calls and accesses goes.
func lastExpression(text string) string {
	text = strings.TrimSpace(text)
	i := len(text)
	depth := 0
	for i > 0 {
		c := text[i-1]
		switch {
		case c == ')' || c == ']':
			depth++
		case c == '(' || c == '[':
			if depth == 0 {
				return strings.TrimSpace(text[i:])
			}
			depth--
		case depth > 0:
		case isIdentByte(c) || c >= '0' && c <= '9' || c == '$' || c == '\\' || c == '>' || c == ':' || c == '?':
			if c == '>' && (i < 2 || text[i-2] != '-') {
				return strings.TrimSpace(text[i:])
			}
		case c == '-':
			if i >= len(text) || text[i] != '>' {
				return strings.TrimSpace(text[i:])
			}
		default:
			return strings.TrimSpace(text[i:])
		}
		i--
	}
	return text
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

	site, found := findClosureCall(lines, closure.Range.Start)
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
	if len(member.Templates) > 0 && site.argsBefore != "" {
		ctx := ExprContext{Lines: lines, Pos: site.at, File: file}
		bound := r.bindCallTemplates(append([]paramPattern(nil), params...), r.CallSiteFor(site.argsBefore, ctx), templateNames(member.Templates), subst, owner, file)
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
	if rt.FQN != "" && !IsBuiltinTypeName(rt.FQN) && r.Index != nil {
		if m := r.FindAccessedMember(rt.FQN, "__invoke", true); m != nil {
			return r.MemberTypeForCall(m, nil, rt.FQN, rt.Params, nil)
		}
	}
	return ResolvedType{}
}
