package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// inferClassStringBinding attempts to bind @template parameters for a generic
// constructor call when a constructor parameter is typed class-string<T>.
//
// It handles three argument forms:
//   - string literal:  new Box('App\\MyClass')
//   - ::class constant: new Box(MyClass::class)
//   - variable with a single, unambiguous ::class assignment: new Box($cls)
//
// Variables from parameters or with multiple assignments are not bound —
// kept restrictive to avoid false positives.
func (r *Resolver) inferClassStringBinding(classFQN, argStr string, lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	sym := r.Index.Lookup(classFQN)
	if sym == nil || len(sym.Templates) == 0 {
		return ResolvedType{}
	}

	paramIdx, templateName := classStringConstructorParam(r, sym, classFQN)
	if paramIdx < 0 || templateName == "" {
		return ResolvedType{}
	}

	arg := nthConstructorArg(argStr, paramIdx)
	if arg == "" {
		return ResolvedType{}
	}

	boundFQN := r.resolveClassStringArg(arg, lines, pos, file)
	if boundFQN == "" {
		return ResolvedType{}
	}

	// Build the Params slice, placing the bound type at the template's position.
	var params []ResolvedType
	for i, tmpl := range sym.Templates {
		if tmpl.Name == templateName {
			for len(params) < i {
				params = append(params, ResolvedType{})
			}
			params = append(params, ResolvedType{FQN: boundFQN})
			break
		}
	}
	if len(params) == 0 {
		return ResolvedType{}
	}
	return ResolvedType{FQN: classFQN, Params: params}
}

// classStringConstructorParam finds the first __construct parameter whose
// @param type references class-string<T> or the bare template name T, where T
// is one of the class's @template declarations. Returns (paramIndex, templateName).
func classStringConstructorParam(r *Resolver, sym *symbols.Symbol, classFQN string) (int, string) {
	if len(sym.Templates) == 0 {
		return -1, ""
	}

	templateNames := make(map[string]bool, len(sym.Templates))
	for _, t := range sym.Templates {
		templateNames[t.Name] = true
	}

	ctor := r.FindMember(classFQN, "__construct")
	if ctor == nil {
		return -1, ""
	}

	doc := parser.ParseDocBlock(ctor.DocComment)
	if doc == nil {
		// Fall back to plain param types
		for i, p := range ctor.Params {
			if t := extractClassStringTemplate(p.Type, templateNames); t != "" {
				return i, t
			}
		}
		return -1, ""
	}

	// Match @param annotations by position against ctor.Params
	for i, p := range ctor.Params {
		docType := resolveDocParam(doc, p.Name)
		if docType == "" {
			docType = p.Type
		}
		if t := extractClassStringTemplate(docType, templateNames); t != "" {
			return i, t
		}
	}
	return -1, ""
}

// extractClassStringTemplate checks whether rawType contains class-string<T>
// or is a union that includes class-string<T>, where T is in templateNames.
// Returns the template name T if found, otherwise "".
func extractClassStringTemplate(rawType string, templateNames map[string]bool) string {
	// Split union parts and check each one
	parts := splitUnionParts(rawType)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// class-string<T>
		if strings.HasPrefix(part, "class-string<") && strings.HasSuffix(part, ">") {
			inner := part[len("class-string<") : len(part)-1]
			inner = strings.TrimSpace(inner)
			if templateNames[inner] {
				return inner
			}
		}
		// bare template name (e.g., @param T $cls)
		if templateNames[part] {
			return part
		}
	}
	return ""
}

// splitUnionParts splits a type string by | at depth 0.
func splitUnionParts(raw string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '<', '(', '[':
			depth++
		case '>', ')', ']':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == 0 {
				parts = append(parts, raw[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, raw[start:])
	return parts
}

// nthConstructorArg splits a constructor argument string by top-level commas
// and returns the argument at 0-based index n.
func nthConstructorArg(argStr string, n int) string {
	args := splitArgList(argStr)
	if n >= len(args) {
		return ""
	}
	return strings.TrimSpace(args[n])
}

// splitArgList splits a function argument list by top-level commas.
func splitArgList(argStr string) []string {
	var args []string
	depth := 0
	inStr := byte(0)
	start := 0
	for i := 0; i < len(argStr); i++ {
		ch := argStr[i]
		if inStr != 0 {
			if ch == inStr && (i == 0 || argStr[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inStr = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(argStr[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(argStr[start:]); tail != "" {
		args = append(args, tail)
	}
	return args
}

// resolveClassStringArg extracts a class FQN from a constructor argument:
//   - string literal `'App\\MyModel'` → strips leading backslash
//   - `MyClass::class` constant → resolved via use statements
//   - `$var` → traces single unambiguous ::class assignment in enclosing scope
//
// Returns "" if the argument cannot be resolved to a class name.
func (r *Resolver) resolveClassStringArg(arg string, lines []string, pos protocol.Position, file *parser.FileNode) string {
	arg = strings.TrimSpace(arg)

	// String literal: 'App\\MyModel' or "App\\MyModel"
	if len(arg) >= 2 && (arg[0] == '\'' || arg[0] == '"') {
		inner := arg[1 : len(arg)-1]
		// Unescape PHP \\ → \ and \' → ' in single-quoted strings
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		inner = strings.TrimPrefix(inner, "\\")
		if isValidPHPClassName(inner) {
			return inner
		}
		return ""
	}

	// ClassName::class constant
	if strings.HasSuffix(arg, "::class") {
		className := strings.TrimSuffix(arg, "::class")
		className = strings.TrimSpace(className)
		if className == "" {
			return ""
		}
		return r.ResolveClassName(className, file)
	}

	// Variable reference: trace to a single ::class assignment
	if strings.HasPrefix(arg, "$") {
		return r.traceVariableToClassString(arg, lines, pos, file)
	}

	return ""
}

// traceVariableToClassString walks the scope document to find if varName has
// exactly one assignment from a ClassName::class expression in the enclosing
// function. Returns "" if the variable is a parameter, reassigned, or the
// assignment RHS is not a ::class form.
func (r *Resolver) traceVariableToClassString(varName string, lines []string, pos protocol.Position, file *parser.FileNode) string {
	source := strings.Join(lines, "\n")
	doc := scope.Collect(source)

	var binding *scope.Binding
	for _, b := range doc.Bindings {
		if b.Name != varName {
			continue
		}
		// Pick the binding whose scope contains pos or starts before pos.
		if b.Scope != nil && scopeCoversOrPrecedesPos(b.Scope.Range, pos) {
			binding = b
		}
	}
	if binding == nil {
		return ""
	}

	if binding.Kind == scope.BindingParameter {
		return ""
	}

	if len(binding.Assignments) != 1 {
		return ""
	}

	assign := binding.Assignments[0]
	exprText := extractRangeText(source, assign.Expression)
	exprText = strings.TrimSpace(exprText)

	if !strings.HasSuffix(exprText, "::class") {
		return ""
	}

	className := strings.TrimSuffix(exprText, "::class")
	className = strings.TrimSpace(className)
	if className == "" {
		return ""
	}
	return r.ResolveClassName(className, file)
}

// extractRangeText extracts the substring from source covered by rng.
func extractRangeText(source string, rng protocol.Range) string {
	lines := strings.Split(source, "\n")
	if rng.Start.Line >= len(lines) {
		return ""
	}
	if rng.Start.Line == rng.End.Line {
		line := lines[rng.Start.Line]
		end := rng.End.Character
		if end > len(line) {
			end = len(line)
		}
		start := rng.Start.Character
		if start > len(line) || start > end {
			return ""
		}
		return line[start:end]
	}
	var sb strings.Builder
	for l := rng.Start.Line; l <= rng.End.Line && l < len(lines); l++ {
		line := lines[l]
		if l == rng.Start.Line {
			if rng.Start.Character < len(line) {
				sb.WriteString(line[rng.Start.Character:])
			}
			sb.WriteByte('\n')
		} else if l == rng.End.Line {
			end := rng.End.Character
			if end > len(line) {
				end = len(line)
			}
			sb.WriteString(line[:end])
		} else {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// scopeCoversOrPrecedesPos returns true if pos is within or before the end of rng.
func scopeCoversOrPrecedesPos(rng protocol.Range, pos protocol.Position) bool {
	if pos.Line > rng.End.Line {
		return false
	}
	if pos.Line == rng.End.Line && pos.Character > rng.End.Character {
		return false
	}
	return true
}

// isValidPHPClassName returns true if s looks like a PHP class name.
func isValidPHPClassName(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '\\') {
			return false
		}
	}
	return true
}
