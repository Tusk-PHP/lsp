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
	// reassigned: a parameter given a new value before the read, so its type
	// comes from the values that reach it, not only its declaration.
	reassigned bool
	binding    *scope.Binding // the variable's binding here, when it's local
	// reach is which of the binding's writes can give it its value here.
	reach scope.Reach
	// write is set when the type asked for is one write's value, which is
	// read from that write's line alone.
	write *scope.WriteSite
	// readPos is where the binding is read, and inRHS is set when that's in
	// the right-hand side of an assignment to it, before it takes effect.
	readPos protocol.Position
	inRHS   bool
}

// readPosition returns where a read at pos reads the binding. A position in
// the indentation of a line that's inside an assignment's right-hand side, as
// diagnostics ask about a whole line, or as the line after a multi-line
// assignment is asked about, reads after that assignment; anywhere else in a
// right-hand side reads before it.
func readPosition(b *scope.Binding, lines []string, pos protocol.Position) (protocol.Position, bool) {
	site, ok := b.AssignmentAround(pos)
	if !ok {
		return pos, false
	}
	if pos.Line > site.At.Line && pos.Line < len(lines) {
		line := lines[pos.Line]
		if indent := len(line) - len(strings.TrimLeft(line, " \t")); pos.Character <= indent {
			return site.End, false
		}
	}
	return pos, true
}

// severalValues reports whether more than the last write before the read can
// give the variable its value, as after `if ($c) { $x = new Other; }`, so its
// type is the union of theirs.
func (vs variableScope) severalValues() bool {
	return vs.binding != nil && !vs.reach.Single()
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
	doc := r.scopeDocument(lines)
	s := doc.ScopeAt(pos)
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
		vs.binding = binding
		vs.readPos, vs.inRHS = readPosition(binding, lines, pos)
		vs.reach = doc.ReachingWrites(binding, vs.readPos)
		// A parameter's declared type is gone once a write's value is the
		// one it has.
		vs.reassigned = binding.Kind == scope.BindingParameter && vs.reach.Latest >= 0
	}
	vs.closureParts(s, binding)
	return vs
}

// writeScope is the scope for the value one write of b gives: only the
// write's line is read for it, and a parameter has its declared type only at
// its declaration.
func writeScope(b *scope.Binding, write int, site *scope.WriteSite) variableScope {
	s := b.Scope
	vs := variableScope{
		firstLine: site.At.Line,
		inClosure: s.Kind == scope.ScopeClosure,
		isLocal:   true,
		binding:   b,
		reach:     scope.Reach{Writes: []int{write}, OnWrite: true, Latest: write},
		write:     site,
	}
	if write != scope.DeclWrite {
		vs.reassigned = b.Kind == scope.BindingParameter
		return vs
	}
	if b.Kind == scope.BindingParameter {
		vs.firstLine = s.Range.Start.Line
	}
	vs.closureParts(s, b)
	return vs
}

// assignmentValue is assignmentValue for line i, except that one write's
// value is read from the write itself: from its own column to the end of the
// statement when that's on the same line (`if ($c) { $x = new Foo(); }`,
// `case 1: $x = new Foo(); break;`, `$x = foo(); // note`), and from the start
// of the line when it goes on to later lines. When it can't be read that way,
// it isn't read at all.
func (vs variableScope) assignmentValue(lines []string, i int, varPrefix string) (full, firstLine string, ok bool) {
	w := vs.write
	if w == nil || i != w.At.Line || i >= len(lines) {
		return assignmentValue(lines, i, varPrefix)
	}
	line := lines[i]
	col := w.At.Character
	if col > len(line) {
		return "", "", false
	}
	// A statement that ends on its line is read up to its end, so a comment
	// after it isn't taken for part of the value.
	if w.Statement && w.End.Line == i && w.End.Character <= len(line) && w.End.Character >= col {
		return assignmentValue([]string{line[col:w.End.Character] + ";"}, 0, varPrefix)
	}
	// One that goes on to later lines is read as before, from the start of
	// its line.
	if before := strings.TrimSpace(line[:col]); before == "" || strings.EqualFold(before, "static") {
		return assignmentValue(lines, i, varPrefix)
	}
	return "", "", false
}

// closureParts fills in what a closure's variable comes from: its own
// parameter, or the parent scope's variable.
func (vs *variableScope) closureParts(s *scope.Scope, binding *scope.Binding) {
	if !vs.inClosure {
		return
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
