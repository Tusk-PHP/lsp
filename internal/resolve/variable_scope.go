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
}

func (r *Resolver) scopeDocument(lines []string) *scope.Document {
	source := strings.Join(lines, "\n")
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.scopeDoc == nil || r.scopeSource != source {
		r.scopeSource, r.scopeDoc = source, scope.Collect(source)
	}
	return r.scopeDoc
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
	}
	if !vs.inClosure {
		return vs
	}

	switch {
	case binding != nil && binding.Kind == scope.BindingParameter:
		vs.param = binding
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
		return ResolvedType{}
	}
	// A union is shown as written. Resolving it would pick one of its members.
	if strings.ContainsAny(declared, "|&") {
		return ResolvedType{FQN: declared}
	}
	return r.resolveDocType(declared, file)
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
// inside a call's parentheses or the next line continues a method chain. Bodies
// of closures aren't followed, since nothing in them changes the expression's type.
func continuedExpression(lines []string, i int, expr string) string {
	const maxLines = 30
	for j := i + 1; j < len(lines) && j <= i+maxLines; j++ {
		if strings.Contains(expr, "{") || strings.HasSuffix(strings.TrimSpace(expr), ";") {
			break
		}
		next := strings.TrimSpace(lines[j])
		if strings.Count(expr, "(") <= strings.Count(expr, ")") && !strings.HasPrefix(next, "->") && !strings.HasPrefix(next, "?->") {
			break
		}
		expr += next
	}
	return strings.TrimSpace(expr)
}
