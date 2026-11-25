package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ChainType gives the type of an expression such as "Category::query()->first()"
// or "$this->repo->find($id)": the class and type arguments it ends with, with
// call arguments, constructors, closures and @mixin forwarding taken into
// account. Every feature asks it, so hover, completion, go-to-definition,
// inlay hints and diagnostics agree on a chain's type. A resolver wired to
// another one through TypedChainResolver asks that one.
func (r *Resolver) ChainType(expr, source string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	if r.TypedChainResolver != nil {
		return r.TypedChainResolver(expr, source, pos, file)
	}
	return r.chainType(expr, source, pos, file)
}

// chainType is ChainType with this resolver's own walker.
func (r *Resolver) chainType(expr, source string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ResolvedType{}
	}
	// A dummy member after the expression: the owner of that member is the
	// expression's type.
	line := expr + "->__dummy__"
	return r.accessOwnerType(line, len(expr)+2, source, pos, file)
}

// AccessOwnerType gives the type of the receiver of the member that starts at
// line[wordStart], after its -> or ::, as ChainType types it: for
// "$a->b()->c" with wordStart at c, the type $a->b() gives.
func (r *Resolver) AccessOwnerType(line string, wordStart int, source string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	if r.TypedChainResolver == nil {
		return r.accessOwnerType(line, wordStart, source, pos, file)
	}
	i := wordStart
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	switch {
	case i >= 3 && line[i-3:i] == "?->":
		i -= 3
	case i >= 2 && (line[i-2:i] == "->" || line[i-2:i] == "::"):
		i -= 2
	default:
		return ResolvedType{}
	}
	return r.TypedChainResolver(line[:i], source, pos, file)
}

// resolveChainWithContext is the generic-aware chain resolver. It walks backward
// through the chain and tracks generic type parameters at each step.
func (r *Resolver) accessOwnerType(line string, wordStart int, source string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	i := wordStart

	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i < 2 {
		return ResolvedType{}
	}

	op := ""
	if line[i-2] == '-' && line[i-1] == '>' {
		op = "->"
		i -= 2
		if i > 0 && line[i-1] == '?' {
			i-- // ?->
		}
	} else if line[i-2] == ':' && line[i-1] == ':' {
		op = "::"
		i -= 2
	} else {
		return ResolvedType{}
	}

	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}

	parenEnd := 0
	callArgStr := "" // captures the text inside (...) for method-level template inference
	if i > 0 && line[i-1] == ')' {
		parenEnd = i
		depth := 1
		i--
		for i > 0 && depth > 0 {
			i--
			if line[i] == ')' {
				depth++
			} else if line[i] == '(' {
				depth--
			}
		}
		// Capture argument content: text between ( and )
		if i >= 0 && i+1 < parenEnd-1 {
			callArgStr = strings.TrimSpace(line[i+1 : parenEnd-1])
		}

		if op == "->" && r.ContainerCall != nil {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if concrete := r.ContainerCall(callExpr, source, file); concrete != "" {
				if concrete == "-" {
					return ResolvedType{}
				}
				return ResolvedType{FQN: concrete}
			}
		}

		if op == "->" {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if newClass := ExtractNewClass(callExpr); newClass != "" {
				fqn := r.ResolveClassNameLoose(newClass, file)
				if args := extractNewArgs(callExpr); args != "" {
					ctx := ExprContext{Lines: r.Lines(source), Pos: pos, File: file}
					if rt := r.NewExpressionType(newClass, args, ctx); rt.FQN == fqn {
						return rt
					}
				}
				return ResolvedType{FQN: fqn}
			}
		}

		for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
			i--
		}
	}

	// An offset: $data['user']->, $rows[0]->
	if parenEnd == 0 && i > 0 && line[i-1] == ']' {
		closeAt := i - 1
		depth := 0
		j := closeAt
		for ; j >= 0; j-- {
			if line[j] == ']' {
				depth++
			} else if line[j] == '[' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if j <= 0 {
			return ResolvedType{}
		}
		base := r.chainType(strings.TrimSpace(line[:j]), source, pos, file)
		if base.IsEmpty() {
			return ResolvedType{}
		}
		return r.OffsetType(base, line[j+1:closeAt])
	}

	end := i
	for i > 0 && IsWordChar(line[i-1]) {
		i--
	}
	if i > 0 && line[i-1] == '$' {
		i--
	}
	if i >= end {
		return ResolvedType{}
	}
	target := line[i:end]

	// Resolve variables and keywords using standard resolution. self, static
	// and parent are keywords, which PHP matches regardless of case.
	switch ClassKeyword(target) {
	case "$this":
		if file != nil {
			return r.ResolveVariableTypeTyped("$this", r.Lines(source), pos, file)
		}
		return ResolvedType{}
	case "self", "static":
		if file != nil {
			return ResolvedType{FQN: FindEnclosingClass(file, pos)}
		}
		return ResolvedType{}
	case "parent":
		if file != nil {
			classFQN := FindEnclosingClass(file, pos)
			if classFQN != "" {
				chain := r.Index.GetInheritanceChain(classFQN)
				if len(chain) > 0 {
					return ResolvedType{FQN: chain[0]}
				}
			}
		}
		return ResolvedType{}
	}

	if strings.HasPrefix(target, "$") {
		rt := r.ResolveVariableTypeTyped(target, r.Lines(source), pos, file)
		if parenEnd > 0 && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "->") && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::") {
			// $fn(…): what calling it gives; $fn(...) is a Closure of it.
			if IsCallableArgs(callArgStr) {
				return r.CallableOf(rt)
			}
			return r.InvokeType(rt)
		}
		return rt
	}

	// Try as class name — this is the chain origin (e.g., Category::). A word
	// after -> or :: is a member (UserRole::Client, $x->Client), never a class.
	if parenEnd == 0 {
		if fqn := r.ChainOriginClassName(line, i, target, file); fqn != "" {
			if sym := r.Index.Lookup(fqn); sym != nil {
				if sym.IsClassLike() {
					return ResolvedType{FQN: fqn}
				}
			} else if fqn != target && !strings.Contains(fqn, "::") {
				return ResolvedType{FQN: fqn}
			}
		}
	}

	// Recursive chain resolution with context propagation
	if file == nil {
		return ResolvedType{}
	}
	// A function call starts the chain: identity($x)->…
	if parenEnd > 0 && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "->") && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::") {
		ctx := ExprContext{Lines: r.Lines(source), Pos: pos, File: file}
		if rt := r.FunctionCallType(target, callArgStr, ctx); !rt.IsEmpty() {
			return rt
		}
	}
	ownerResolved := r.accessOwnerType(line, i, source, pos, file)
	if ownerResolved.IsEmpty() {
		return ResolvedType{}
	}

	if parenEnd > 0 && strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "->") {
		// $closure->call($newThis): what the closure returns.
		if rt, ok := ClosureCallType(ownerResolved, target); ok {
			return rt
		}
	}
	ownerFQN := ownerResolved.BaseFQN()
	if strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::") {
		ownerResolved = ClassStringTarget(ownerResolved) // $class::create()
	}
	if fwd, ok := r.ForwardedReceiver(ownerResolved, target, parenEnd > 0); ok {
		ownerResolved = fwd // Category::where() runs on Builder<Category>
	}
	member, foundOn := r.FindMemberOn(ownerResolved, target, parenEnd > 0)
	firstClass := parenEnd > 0 && IsCallableArgs(callArgStr)
	if member != nil && foundOn.FQN != ownerResolved.FQN {
		// A member a @mixin brings in. A fluent one gives back the object it
		// was called on, as forwarding through __call does.
		if !firstClass && r.ReturnsCallingClass(member) {
			return ownerResolved
		}
		ownerResolved = foundOn
	}
	ownerFQN = ownerResolved.BaseFQN()
	if member == nil && parenEnd > 0 && r.ScopeMethod(ownerResolved, target) != nil {
		return ownerResolved // a local scope gives back the builder
	}
	if member == nil {
		// A property of an object shape: object{user: User}.
		if ownerResolved.FQN == "object" && parenEnd == 0 {
			for _, f := range ownerResolved.Fields {
				if f.Key == target {
					return f.Type
				}
			}
		}
		return ResolvedType{}
	}
	ownerFQN = r.ReceiverFor(ownerResolved, member)

	// Pass the owner's generic params as context for template resolution.
	// E.g., Builder<Category> passes [Category] so Builder::get() → Collection<int, Category>.
	// callerFQN is the owner class for late static binding (Category::query() → static=Category).
	ownerContext := ownerResolved.Params

	// self::, parent:: and static:: pass the calling class along, so a method
	// returning static gives the class being edited: parent::create() in Child
	// is a Child.
	callerFQN := ownerFQN
	if ForwardsCallingClass(line[:i]) {
		if enclosing := FindEnclosingClass(file, pos); enclosing != "" {
			callerFQN = enclosing
		}
	}
	if firstClass {
		// $obj->m(...) and $obj->m(?, 1) make a Closure of m instead of calling it.
		return PartialClosure(r.MemberCallableType(member, file, callerFQN, ownerContext), member, callArgStr)
	}
	var call *CallSite
	if parenEnd > 0 {
		call = r.CallSiteFor(callArgStr, ExprContext{Lines: r.Lines(source), Pos: pos, File: file})
	}
	return r.MemberTypeForCall(member, file, callerFQN, ownerContext, call)
}

// extractNewArgs returns the constructor arguments of the `new Foo(…)`
// expression that s ends with, as ExtractNewClass reads it.
func extractNewArgs(s string) string {
	t := strings.TrimSpace(s)
	for strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") && parenBalanced(t[1:len(t)-1]) {
		t = strings.TrimSpace(t[1 : len(t)-1])
	}
	if !strings.HasSuffix(t, ")") {
		return ""
	}
	depth := 0
	for i := len(t) - 1; i >= 0; i-- {
		switch t[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return t[i+1 : len(t)-1]
			}
		}
	}
	return ""
}

// ForwardsCallingClass reports whether the code before a member name ends in
// `self::`, `parent::` or `static::`, which keep the calling class for static.
func ForwardsCallingClass(before string) bool {
	before = strings.TrimRight(before, " \t")
	if !strings.HasSuffix(before, "::") {
		return false
	}
	before = strings.TrimRight(before[:len(before)-2], " \t")
	start := len(before)
	for start > 0 && IsWordChar(before[start-1]) {
		start--
	}
	if start > 0 && before[start-1] == '$' {
		return false // $parent:: is a variable
	}
	switch strings.ToLower(before[start:]) {
	case "self", "parent", "static":
		return true
	}
	return false
}

// ClassKeyword lower-cases target when it's one of the class keywords
// self, static or parent, so SELF:: and Parent:: resolve like their lower-case
// forms; any other name is returned as it is.
func ClassKeyword(target string) string {
	switch lower := strings.ToLower(target); lower {
	case "self", "static", "parent":
		return lower
	}
	return target
}

// ChainOriginClassName resolves target, which starts at line[start], as a
// class name, unless it follows a member operator (-> or ::) and so names a
// member such as an enum case, constant or property.
func (r *Resolver) ChainOriginClassName(line string, start int, target string, file *parser.FileNode) string {
	before := strings.TrimRight(line[:start], " \t")
	if strings.HasSuffix(before, "->") || strings.HasSuffix(before, "::") {
		return ""
	}
	return r.ResolveClassNameLoose(target, file)
}

// resolveClassNameFromSource resolves a short or FQN class name using
// the source file's use statements and namespace.
func (r *Resolver) ResolveClassNameLoose(name string, file *parser.FileNode) string {
	if name == "" {
		return ""
	}
	// Already fully qualified
	if strings.HasPrefix(name, "\\") {
		fqn := strings.TrimPrefix(name, "\\")
		if r.Index.Lookup(fqn) != nil {
			return fqn
		}
		return fqn
	}

	if file == nil {
		if cls := r.classNamed(name); cls != nil {
			return cls.FQN
		}
		return name
	}

	// Check use statements
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range file.Uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	// Try in current namespace
	if file.Namespace != "" {
		fqn := file.Namespace + "\\" + name
		if r.Index.Lookup(fqn) != nil {
			return fqn
		}
	}
	// Try as global
	if r.Index.Lookup(name) != nil {
		return name
	}
	if cls := r.classNamed(name); cls != nil {
		return cls.FQN
	}
	return name
}

func (r *Resolver) classNamed(name string) *symbols.Symbol {
	var classes []*symbols.Symbol
	for _, sym := range r.Index.LookupByName(name) {
		if sym.IsClassLike() {
			classes = append(classes, sym)
		}
	}
	return symbols.PickBestStandalone(classes, name)
}

// ExtractNewClass extracts the class name from patterns like:
// "new ClassName()", "(new ClassName())", "new ClassName", "(new ClassName)"
func ExtractNewClass(s string) string {
	t := strings.TrimSpace(s)

	// Strip wrapping parens: "(new ClassName())" → "new ClassName()"
	for strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		inner := t[1 : len(t)-1]
		// Only strip if the parens are balanced wrapping parens (not constructor args)
		if parenBalanced(inner) {
			t = strings.TrimSpace(inner)
		} else {
			break
		}
	}

	// Strip constructor args: "new ClassName(...)" → "new ClassName"
	if strings.HasSuffix(t, ")") {
		depth := 1
		i := len(t) - 2
		for i >= 0 && depth > 0 {
			if t[i] == ')' {
				depth++
			} else if t[i] == '(' {
				depth--
			}
			i--
		}
		t = strings.TrimSpace(t[:i+1])
	}

	// Extract class name after "new"
	if idx := strings.LastIndex(t, "new "); idx >= 0 {
		className := strings.TrimSpace(t[idx+4:])
		if className != "" && !strings.ContainsAny(className, " \t(") {
			return className
		}
	}
	return ""
}

func parenBalanced(s string) bool {
	depth := 0
	for _, ch := range s {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}
