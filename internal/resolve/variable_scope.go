package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

// variableScope says where a variable lives, so that type lookups don't pick up
// a same-named variable or parameter from somewhere it can't be seen.
type variableScope struct {
	firstLine  int  // assignments to it can't be above this line
	inClosure  bool // the enclosing method's parameters aren't visible
	isLocal    bool // it's bound in the enclosing function, method or closure
	param      *scope.Binding
	capturedAt *protocol.Position // resolve it here instead: it comes from the parent scope
	// reassigned: a parameter given a new value that holds here, so its type
	// comes from that value, not its declaration.
	reassigned bool
	// unknownValue: a parameter given a new value that may or may not hold
	// here, as after `if ($c) { $x = new Other; }`. Its type isn't known.
	unknownValue bool
}

func (r *Resolver) scopeDocument(lines []string) *scope.Document {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.cachedLines == nil || !sameLines(r.cachedLines, lines) {
		r.setSourceLocked(strings.Join(lines, "\n"), lines)
	}
	if r.scopeDoc == nil {
		r.scopeDoc = scope.Collect(r.scopeSource)
	}
	return r.scopeDoc
}

// ScopeDocument returns the scopes and variable bindings of the source, cached
// for as long as the source doesn't change. Treat it as read-only.
func (r *Resolver) ScopeDocument(lines []string) *scope.Document {
	return r.scopeDocument(lines)
}

func (r *Resolver) variableScopeAt(varName string, lines []string, pos protocol.Position) variableScope {
	s := r.scopeDocument(lines).ScopeAt(pos)
	if s == nil || s.Kind == scope.ScopeFile {
		return variableScope{}
	}

	vs := variableScope{firstLine: s.Range.Start.Line, inClosure: s.Kind == scope.ScopeClosure}
	var binding *scope.Binding
	for _, b := range s.Bindings {
		if b.Name == varName {
			binding = b
			break
		}
	}
	if binding != nil {
		vs.isLocal = true
		if binding.Kind == scope.BindingParameter {
			if r := binding.LatestRebindBefore(pos); r != nil {
				vs.reassigned = r.HoldsAt(pos)
				vs.unknownValue = !vs.reassigned
			}
		}
	}
	if !vs.inClosure {
		return vs
	}

	switch {
	case binding != nil && binding.Kind == scope.BindingParameter:
		if !vs.reassigned {
			vs.param = binding
		}
	case binding != nil && binding.Kind == scope.BindingClosureUse, binding == nil && s.ExprBody:
		// `use ($x)`, or an arrow function reading its parent's variable.
		parent := protocol.Position{Line: s.Range.Start.Line, Character: s.Range.Start.Character - 1}
		if parent.Character < 0 {
			parent = protocol.Position{Line: s.Range.Start.Line - 1, Character: 1 << 30}
		}
		if parent.Line >= 0 {
			vs.capturedAt = &parent
		}
	}
	return vs
}

// closureThisAt returns what $this is at pos when pos is inside a closure
// passed for a parameter tagged @param-closure-this.
func (r *Resolver) closureThisAt(lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	s := r.scopeDocument(lines).ScopeAt(pos)
	for ; s != nil; s = s.Parent {
		if s.Kind != scope.ScopeClosure {
			return ResolvedType{}
		}
		if _, this, ok := r.closureExpectations(s, lines, file); ok && !this.IsEmpty() {
			return this
		}
	}
	return ResolvedType{}
}

// IsLocalVariable reports whether the variable is bound in the function, method
// or closure at pos, whether or not its type can be worked out.
func (r *Resolver) IsLocalVariable(varName string, lines []string, pos protocol.Position) bool {
	vs := r.variableScopeAt(varName, lines, pos)
	if vs.capturedAt != nil {
		return r.IsLocalVariable(varName, lines, *vs.capturedAt)
	}
	return vs.isLocal
}

// closureParamType returns a closure parameter's type: from a docblock above the
// statement it's in, as `@param` tags aren't attached to closures by the parser,
// or else from its declaration.
func (r *Resolver) closureParamType(param *scope.Binding, lines []string, file *parser.FileNode) ResolvedType {
	doc := parser.ParseDocBlock(docBlockAbove(lines, param.Scope.Range.Start.Line))
	if docType := resolveDocParam(doc, param.Name); docType != "" {
		if rt := r.resolveDocType(docType, file); !rt.IsEmpty() {
			return rt
		}
	}
	line := param.Decl.Start.Line
	if line < 0 || line >= len(lines) || param.Decl.Start.Character > len(lines[line]) {
		return ResolvedType{}
	}
	declared := declaredParamType(lines[line][:param.Decl.Start.Character])
	if declared == "" {
		// Untyped: what the callable(…) it's passed for says.
		if expected, _, ok := r.closureExpectations(param.Scope, lines, file); ok {
			if sig := callableSignature(expected); sig != nil {
				if i := closureParamIndex(param); i >= 0 && i < len(sig.Params) {
					return sig.Params[i]
				}
			}
		}
		return ResolvedType{}
	}
	// A union is shown as written. Resolving it would pick one of its members.
	if strings.ContainsAny(declared, "|&") {
		return ResolvedType{FQN: declared}
	}
	return r.resolveDocType(declared, file)
}

// varDocAbove returns the type a docblock right above line i gives varName
// with `@var Type $name`, or with a bare `@var Type`.
func varDocAbove(lines []string, i int, varName string) string {
	raw := docBlockAbove(lines, i)
	if raw == "" || !strings.Contains(raw, "var") {
		return ""
	}
	doc := parser.ParseDocBlock(raw)
	if doc == nil {
		return ""
	}
	for _, v := range doc.Vars {
		if v.Name == varName || v.Name == "" {
			return v.Type
		}
	}
	return ""
}

// docBlockAbove returns the docblock ending on the line above, if there is one.
func docBlockAbove(lines []string, line int) string {
	end := line - 1
	if end < 0 || end >= len(lines) || !strings.HasSuffix(strings.TrimSpace(lines[end]), "*/") {
		return ""
	}
	for i := end; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "/**") {
			return strings.Join(lines[i:end+1], "\n")
		}
	}
	return ""
}

// declaredParamType extracts the type from the text before a parameter's name,
// such as `function (int $a, null|string ` or `    ?Foo &`.
func declaredParamType(before string) string {
	if i := strings.LastIndexAny(before, "(,"); i >= 0 {
		before = before[i+1:]
	}
	fields := strings.Fields(strings.TrimRight(strings.TrimSpace(before), "&."))
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimRight(fields[len(fields)-1], "&.")
}

// assignmentValue returns the expression assigned to the variable on this line,
// which is the text after its `=` or `??=`, both with any lines it continues on
// and without them. It rejects `$a == 1` and `$a => 1`, which also start with the
// variable and `=`.
func assignmentValue(lines []string, i int, varPrefix string) (full, firstLine string, ok bool) {
	trimmed := strings.TrimSpace(lines[i])
	// `static $x = new Foo();` assigns the same way.
	if rest, ok := cutKeyword(trimmed, "static"); ok && strings.HasPrefix(rest, "$") {
		trimmed = rest
	}
	if !strings.HasPrefix(trimmed, varPrefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(trimmed[len(varPrefix):])
	rest = strings.TrimPrefix(rest, "??")
	if !strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "==") || strings.HasPrefix(rest, "=>") {
		return "", "", false
	}
	firstLine = strings.TrimSpace(rest[1:])
	return continuedExpression(lines, i, firstLine), firstLine, true
}

// continuedExpression adds the lines an expression carries on to, when it's still
// inside a call's parentheses or the next line continues a method chain. A
// closure passed as an argument is read through, so the chain after it counts.
func continuedExpression(lines []string, i int, expr string) string {
	const maxLines = 30
	for j := i + 1; j < len(lines) && j <= i+maxLines; j++ {
		open := strings.Count(expr, "(") - strings.Count(expr, ")")
		trimmed := strings.TrimSpace(expr)
		// A complete statement, or a block that isn't a call's argument (such
		// as a closure assigned to the variable), ends it. A closure passed to
		// a call is followed to its end, since the chain may go on after it.
		if open <= 0 && (strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "{")) {
			break
		}
		next := strings.TrimSpace(lines[j])
		if open <= 0 && !strings.HasPrefix(next, "->") && !strings.HasPrefix(next, "?->") {
			break
		}
		expr += next
	}
	return strings.TrimSpace(expr)
}
