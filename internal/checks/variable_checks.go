package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// superglobals contains variables that are always available in any scope.
var superglobals = map[string]bool{
	"$_GET":     true,
	"$_POST":    true,
	"$_SERVER":  true,
	"$_SESSION": true,
	"$_COOKIE":  true,
	"$_FILES":   true,
	"$_ENV":     true,
	"$_REQUEST": true,
	"$GLOBALS":  true,
	"$this":     true,
	"$argv":     true,
	"$argc":     true,
}

// skipScopeFunctions are function names whose presence in a scope body
// means we cannot reliably analyze variables in that scope.
// These must be called with parentheses.
var skipScopeFunctions = map[string]bool{
	"extract": true,
	"compact": true,
	"eval":    true,
}

// skipScopeKeywords are PHP language constructs (no parentheses required) that,
// when used in a scope, make variable analysis unreliable.
var skipScopeKeywords = map[string]bool{
	"include":      true,
	"include_once": true,
	"require":      true,
	"require_once": true,
}

// UndefinedVariableRule flags reads of variables that have no binding in scope.
type UndefinedVariableRule struct{}

func (r *UndefinedVariableRule) Code() string { return "undefined-variable" }

func (r *UndefinedVariableRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	return checkUndefinedVariables(source)
}

// UnusedVariableRule flags local variable assignments with no subsequent read.
type UnusedVariableRule struct{}

func (r *UnusedVariableRule) Code() string { return "unused-variable" }

func (r *UnusedVariableRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	return checkUnusedVariables(source)
}

// checkUndefinedVariables detects reads of undefined variables.
func checkUndefinedVariables(source string) []Finding {
	doc := scope.Collect(source)
	if doc == nil {
		return nil
	}

	// Build a set of all declared/resolved positions from the bindings.
	// A position is "known" if it appears as a Decl or Reference of any binding.
	type posKey struct{ line, col int }
	knownPositions := make(map[posKey]bool)
	for _, b := range doc.Bindings {
		knownPositions[posKey{b.Decl.Start.Line, b.Decl.Start.Character}] = true
		for _, ref := range b.References {
			knownPositions[posKey{ref.Start.Line, ref.Start.Character}] = true
		}
	}

	// Parse tokens to find all variable occurrences not tracked by the scope collector.
	result := parser.New().Parse(source)
	if result == nil {
		return nil
	}

	// Determine which scopes are "tainted" (contain skip functions).
	taintedScopes := findTaintedScopes(doc, result.Tokens)

	var findings []Finding
	for _, tok := range result.Tokens {
		if tok.Kind != parser.TokenVariable {
			continue
		}
		name := tok.Value
		if superglobals[name] {
			continue
		}
		// Skip variables inside tainted scopes.
		pos := protocol.Position{Line: tok.Line, Character: tok.Column}
		if isScopesTainted(doc, pos, taintedScopes) {
			continue
		}
		k := posKey{tok.Line, tok.Column}
		if knownPositions[k] {
			// Known position — this is either a declaration or a resolved reference.
			continue
		}
		// This variable occurrence was not tracked by the scope collector,
		// which means resolveVariable returned nil for it (undefined read).
		// But we must be conservative: skip if the token could be a by-reference
		// assignment target (&$x), dynamic variable ($$x), or inside a string.
		if isReferenceAssignmentTarget(result.Tokens, tok) {
			continue
		}
		if isDynamicVariableTarget(result.Tokens, tok) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: tok.Line,
			StartCol:  tok.Column,
			EndLine:   tok.Line,
			EndCol:    tok.Column + len(name),
			Severity:  SeverityWarning,
			Code:      "undefined-variable",
			Message:   "Undefined variable " + name,
		})
	}
	return findings
}

// checkUnusedVariables detects local variable assignments with no reads.
func checkUnusedVariables(source string) []Finding {
	doc := scope.Collect(source)
	if doc == nil {
		return nil
	}

	result := parser.New().Parse(source)
	if result == nil {
		return nil
	}

	// Determine which scopes are "tainted".
	taintedScopes := findTaintedScopes(doc, result.Tokens)

	// Build a set of bindings that are captured as closure-use origins.
	// A BindingVariable that is someone's Origin should not be flagged as unused.
	closureOrigins := make(map[*scope.Binding]bool)
	for _, b := range doc.Bindings {
		if b.Kind == scope.BindingClosureUse && b.Origin != nil {
			closureOrigins[b.Origin] = true
		}
	}

	var findings []Finding
	for _, b := range doc.Bindings {
		// Only flag plain variable assignments (not parameters, foreach, catch, closure-use, destructure, import).
		if b.Kind != scope.BindingVariable {
			continue
		}
		// Skip underscore-prefixed names ($_ or $_foo).
		name := strings.TrimPrefix(b.Name, "$")
		if name == "_" || strings.HasPrefix(name, "_") {
			continue
		}
		// Skip variables in tainted scopes.
		if isScopesTainted(doc, b.Decl.Start, taintedScopes) {
			continue
		}
		// Skip if there are any references (reads) after the declaration.
		if len(b.References) > 0 {
			continue
		}
		// Skip if this binding is captured as a closure-use origin.
		if closureOrigins[b] {
			continue
		}
		// Skip by-reference bindings (=$x where left is &$var, or global $x, static $x)
		if isReferenceOrGlobalBinding(source, b) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: b.Decl.Start.Line,
			StartCol:  b.Decl.Start.Character,
			EndLine:   b.Decl.End.Line,
			EndCol:    b.Decl.End.Character,
			Severity:  SeverityHint,
			Code:      "unused-variable",
			Message:   "Variable $" + name + " is assigned but never read",
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

// findTaintedScopes returns a set of scope pointers whose body contains calls
// to functions or use of language constructs that make variable analysis unreliable.
func findTaintedScopes(doc *scope.Document, tokens []parser.Token) map[*scope.Scope]bool {
	tainted := make(map[*scope.Scope]bool)
	for i, tok := range tokens {
		if tok.Kind != parser.TokenIdentifier {
			continue
		}
		lower := strings.ToLower(tok.Value)
		if skipScopeFunctions[lower] {
			// These must be followed by '(' to be a real call.
			next := nextSignificantTokenIdx(tokens, i+1)
			if next < 0 || tokens[next].Kind != parser.TokenOpenParen {
				continue
			}
		} else if skipScopeKeywords[lower] {
			// Language constructs don't need parentheses.
		} else {
			continue
		}
		pos := protocol.Position{Line: tok.Line, Character: tok.Column}
		s := doc.ScopeAt(pos)
		if s != nil {
			tainted[s] = true
		}
	}
	return tainted
}

// isScopesTainted checks if the scope containing pos is in the tainted set.
func isScopesTainted(doc *scope.Document, pos protocol.Position, tainted map[*scope.Scope]bool) bool {
	s := doc.ScopeAt(pos)
	return s != nil && tainted[s]
}

// isReferenceAssignmentTarget returns true if the variable token is preceded by '&'
// indicating a by-reference assignment (&$var = ...).
func isReferenceAssignmentTarget(tokens []parser.Token, tok parser.Token) bool {
	for i, t := range tokens {
		if t.Kind == tok.Kind && t.Line == tok.Line && t.Column == tok.Column {
			// Look backward for '&'
			prev := prevSignificantTokenIdx(tokens, i-1)
			if prev >= 0 && tokens[prev].Kind == parser.TokenAmpersand {
				return true
			}
			return false
		}
	}
	return false
}

// isDynamicVariableTarget returns true if the variable token is preceded by '$'
// making it a dynamic variable access ($$foo).
func isDynamicVariableTarget(tokens []parser.Token, tok parser.Token) bool {
	// If the previous token in the raw stream (not just significant) is '$'
	// then this is a variable variable.
	for i, t := range tokens {
		if t.Kind == tok.Kind && t.Line == tok.Line && t.Column == tok.Column {
			if i > 0 {
				prev := tokens[i-1]
				// Variable variables: prev is "$" as part of "$$name"
				// In the tokenizer, "$$name" may tokenize as Variable("$name") with a preceding Variable("$")
				// or as a single variable — check if directly preceded by '$'
				if prev.Kind == parser.TokenVariable && strings.HasSuffix(prev.Value, "$") {
					return true
				}
			}
			return false
		}
	}
	return false
}

// isReferenceOrGlobalBinding checks whether a binding was created by a
// by-reference assignment (`$a = &$b`), a `global $x`, or `static $x` declaration.
// These bindings should not be flagged as "unused" even if they have no reads.
func isReferenceOrGlobalBinding(source string, b *scope.Binding) bool {
	lines := strings.Split(source, "\n")
	line := b.Decl.Start.Line
	if line < 0 || line >= len(lines) {
		return false
	}
	lineText := lines[line]
	// Check for global/static keyword on the declaration line.
	trimmed := strings.TrimSpace(lineText)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "global ") || strings.HasPrefix(lower, "static ") {
		return true
	}
	// Check for by-reference assignment: "= &$var" or "&$var ="
	col := b.Decl.Start.Character
	prefix := lineText[:col]
	trimmedPrefix := strings.TrimRight(prefix, " \t")
	if strings.HasSuffix(trimmedPrefix, "&") {
		return true
	}
	// Check for "= &" pattern after the variable
	suffix := ""
	if col+len(b.Name) < len(lineText) {
		suffix = lineText[col+len(b.Name):]
	}
	trimmedSuffix := strings.TrimLeft(suffix, " \t")
	if strings.HasPrefix(trimmedSuffix, "= &") || strings.HasPrefix(trimmedSuffix, "=&") {
		return true
	}
	return false
}

// nextSignificantTokenIdx returns the index of the next non-trivia token.
func nextSignificantTokenIdx(tokens []parser.Token, i int) int {
	for i < len(tokens) {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i++
		default:
			return i
		}
	}
	return -1
}

// prevSignificantTokenIdx returns the index of the previous non-trivia token.
func prevSignificantTokenIdx(tokens []parser.Token, i int) int {
	for i >= 0 {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i--
		default:
			return i
		}
	}
	return -1
}
