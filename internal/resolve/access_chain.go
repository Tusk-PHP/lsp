package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// NewChainEngine returns an Engine over index that resolves method chains in
// variable assignments ($user = User::first()) with its own AccessChainClass.
// Set ContainerCall on it to type container calls too.
func NewChainEngine(index *symbols.Index) *Engine {
	e := NewEngine(index)
	e.ChainResolver = (*Resolver).ExpressionClass
	return e
}

// ExpressionClass gives the class an expression such as "Category::first()"
// or "$foo->bar()->baz()" evaluates to, without type arguments.
func (r *Resolver) ExpressionClass(expr string, source string, pos protocol.Position, file *parser.FileNode) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	// A dummy member after the expression: the class that owns it is the
	// expression's class.
	return r.AccessChainClass(expr+"->__dummy__", len(expr)+2, source, pos, file)
}

// DiagnosticType returns the type diagnostics check an expression against:
// the generic-aware type, written out in full ("?Foo", "Foo|Bar",
// "Collection<int, User>"), the same one hover and completion see. It falls
// back to ExpressionClass when that can't be worked out. Each lookup is a
// request of its own.
func (e *Engine) DiagnosticType(expr string, source string, pos protocol.Position, file *parser.FileNode) string {
	if rt := e.Resolver().ChainType(expr, source, pos, file); !rt.IsEmpty() {
		return rt.FullString()
	}
	return e.Resolver().ExpressionClass(expr, source, pos, file)
}

// AccessChainClass walks left through a chain of -> and :: accesses in line
// and returns the FQN of the class at that point. wordStart is the position
// just after the operator (-> or ::) to  It handles recursive chains
// like Category::query()->with('form')->, container calls through
// ContainerCall, and members forwarded to an Eloquent builder.
func (r *Resolver) AccessChainClass(line string, wordStart int, source string, pos protocol.Position, file *parser.FileNode) string {
	i := wordStart

	// Skip whitespace before the operator
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i < 2 {
		return ""
	}

	// Detect operator type
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
		return ""
	}

	// Skip whitespace before operator
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}

	// Skip past a method call's closing paren: $foo->bar()->baz
	parenEnd := 0
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

		// Check for container call pattern (only for ->)
		if op == "->" && r.ContainerCall != nil {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if concrete := r.ContainerCall(r, callExpr, source, file); concrete != "" {
				if concrete == "-" {
					return ""
				}
				return concrete
			}
		}

		// Check for "new ClassName(...)" pattern
		if op == "->" {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if newClass := ExtractNewClass(callExpr); newClass != "" {
				return r.ResolveClassNameLoose(newClass, file)
			}
		}
		if line[i] == '(' && IsCallableArgs(line[i+1:parenEnd-1]) {
			return "Closure" // f(...), $obj->m(...), f(?, 1): a Closure, not a call
		}

		// Skip whitespace before paren
		for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
			i--
		}
	}

	// Extract the target word
	end := i
	for i > 0 && IsWordChar(line[i-1]) {
		i--
	}
	// Include $ for variables
	if i > 0 && line[i-1] == '$' {
		i--
	}
	if i >= end {
		return ""
	}
	target := line[i:end]

	// Resolve the target to a class FQN
	switch ClassKeyword(target) {
	case "$this", "self", "static":
		if file != nil {
			return FindEnclosingClass(file, pos)
		}
		return ""
	case "parent":
		if file != nil {
			classFQN := FindEnclosingClass(file, pos)
			if classFQN != "" {
				chain := r.Index.GetInheritanceChain(classFQN)
				if len(chain) > 0 {
					return chain[0]
				}
			}
		}
		return ""
	}

	// $variable-> or $variable::
	if strings.HasPrefix(target, "$") {
		if parenEnd > 0 && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "->") && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::") {
			// $fn(…)->: what calling it gives, not the closure itself. A
			// Closure or callable without a signature gives nothing known.
			if file == nil {
				return ""
			}
			rt := r.ResolveVariableTypeTyped(target, r.Lines(source), pos, file)
			return PlainTypeName(r.InvokeType(rt))
		}
		return r.ResolveVariableType(target, r.Lines(source), pos, file)
	}

	// Try as a class name (for static access or direct use). A word after
	// -> or :: is a member (UserRole::Client, $x->Client), never a class.
	if fqn := r.ChainOriginClassName(line, i, target, file); fqn != "" {
		if sym := r.Index.Lookup(fqn); sym != nil {
			switch sym.Kind {
			case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
				return fqn
			}
			// sym exists but is a method/property — don't return a member FQN as a class
		} else if fqn != target && !strings.Contains(fqn, "::") {
			// Resolved to a different name (via use statement/namespace) but not in index yet — trust it
			// Skip if it contains "::" (it's a member FQN, not a class)
			return fqn
		}
	}

	// Not a class — must be a method/property in a chain.
	// Recursively resolve the owner, then look up the target as a member.
	if file == nil {
		return ""
	}
	ownerFQN := r.AccessChainClass(line, i, source, pos, file)
	if ownerFQN == "" {
		return ""
	}
	access := MemberAccessOf(parenEnd > 0, strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::"), target)
	member := r.FindAccessedMember(ownerFQN, target, access)
	if member == nil {
		// Category::where() runs on Builder<Category>, and a fluent builder
		// method gives back the builder.
		fwd, ok := r.ForwardedReceiver(ResolvedType{FQN: ownerFQN}, target, access)
		if !ok {
			return ""
		}
		member, _ = r.FindMemberOn(fwd, target, access)
		if member == nil {
			if parenEnd > 0 && r.ScopeMethod(fwd, target) != nil {
				return fwd.BaseFQN() // Category::active() is the builder
			}
			return ""
		}
		if r.ReturnsCallingClass(member) {
			return fwd.BaseFQN()
		}
		return PlainTypeName(r.MemberTypeResolved(member, file, fwd.BaseFQN(), fwd.Params))
	}
	// A method returning static/$this/self gives back the class it was called
	// on, not the one that declares it: Child::create() is a Child. Calls through
	// self::, parent:: and static:: pass the calling class along, which here is
	// the class being edited.
	if r.ReturnsCallingClass(member) {
		if ForwardsCallingClass(line[:i]) {
			if enclosing := r.AccessChainClass("static->__dummy__", len("static->"), source, pos, file); enclosing != "" {
				return enclosing
			}
		}
		return ownerFQN
	}
	return r.MemberType(member, file)
}
