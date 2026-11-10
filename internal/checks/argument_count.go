package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ArgumentCountRule flags function and method calls where the argument count
// is clearly wrong: fewer arguments than the required parameters, or more than
// all of them when none is variadic. Calls through `->` are checked when the
// receiver's type is one known class.
//
// Conservative by design: it skips calls that contain named arguments, spread
// expressions, variadic signatures, magic __call/__callStatic, methods declared
// only by @method, and any builtin symbols whose signature may be incomplete.
type ArgumentCountRule struct {
	// Shared, when set, shares the file's parse with the other rules.
	Shared *Shared
	// TypeResolver types the receiver of a `->` call. Without it, those calls
	// aren't checked.
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string
}

func (r *ArgumentCountRule) Code() string { return "argument-count-mismatch" }

func (r *ArgumentCountRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || index == nil {
		return nil
	}
	return checkArgumentCounts(file, source, index, r.Shared, r.TypeResolver)
}

func checkArgumentCounts(file *parser.FileNode, source string, index *symbols.Index, shared *Shared, typeResolver func(string, string, int, int, *parser.FileNode) string) []Finding {
	result := shared.parse(source)
	if result == nil {
		return nil
	}

	var findings []Finding
	var lines []string
	var scopes *scope.Document
	tokens := result.Tokens
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		// We're looking for plain function call: IDENTIFIER (  or  IDENTIFIER::IDENTIFIER (
		// and method call via -> is too risky without type resolution.
		if tok.Kind != parser.TokenIdentifier {
			continue
		}
		name := tok.Value
		if isSkippedCallKeyword(name) {
			continue
		}

		next := nextSignificantTokenIdx(tokens, i+1)
		if next < 0 {
			continue
		}

		// Static call: Foo::bar(...)
		if tokens[next].Kind == parser.TokenDoubleColon {
			methodIdx := nextSignificantTokenIdx(tokens, next+1)
			if methodIdx < 0 {
				continue
			}
			methodTok := tokens[methodIdx]
			if methodTok.Kind != parser.TokenIdentifier {
				continue
			}
			openIdx := nextSignificantTokenIdx(tokens, methodIdx+1)
			if openIdx < 0 || tokens[openIdx].Kind != parser.TokenOpenParen {
				continue
			}
			// Resolve class FQN from left side
			classFQN, ok := resolveClassLikeName(name, tok.Line, file)
			if !ok || classFQN == "" {
				continue
			}
			// Look up the method
			sym := findProjectMethod(classFQN, methodTok.Value, index)
			if sym == nil {
				continue
			}
			if !shouldCheckArgCount(sym, classFQN, index) {
				continue
			}
			closeIdx := findMatchingTokenIdx(tokens, openIdx, parser.TokenOpenParen, parser.TokenCloseParen)
			if closeIdx < 0 {
				continue
			}
			argCount, safe := countArguments(tokens, openIdx, closeIdx)
			if !safe {
				continue
			}
			// As for -> calls, too many arguments to a method is left alone.
			if f := buildArgCountFinding(sym, argCount, tok.Line, tok.Column, methodTok.Value); f != nil && argCount < len(sym.Params) {
				findings = append(findings, *f)
			}
			i = closeIdx
			continue
		}

		// Plain function call: name(...)
		if tokens[next].Kind != parser.TokenOpenParen {
			continue
		}
		// Skip names that aren't calls to a global function:
		//   $obj->name( / Foo::name(   method calls we can't resolve without types
		//   function name( / function &name(   declarations, including methods
		//   new name(   a class, not a function
		prev := prevSignificantTokenIdx(tokens, i-1)
		if prev >= 0 && tokens[prev].Kind == parser.TokenArrow {
			if typeResolver == nil {
				continue
			}
			if lines == nil {
				lines = strings.Split(source, "\n")
			}
			sym, owner, receiver := calledMethod(tokens, prev, tok, lines, source, file, index, typeResolver)
			if sym == nil || !shouldCheckArgCount(sym, owner, index) {
				continue
			}
			// $this in a closure may be bound to another object, as
			// Closure::bind() does.
			if receiver == "$this" || strings.HasPrefix(receiver, "$this->") || strings.HasPrefix(receiver, "$this?->") {
				if scopes == nil {
					scopes = scope.Collect(source)
				}
				if inClosure(scopes, tok) {
					continue
				}
			}
			closeIdx := findMatchingTokenIdx(tokens, next, parser.TokenOpenParen, parser.TokenCloseParen)
			if closeIdx < 0 {
				continue
			}
			argCount, safe := countArguments(tokens, next, closeIdx)
			if !safe {
				continue
			}
			// Too many arguments to a method is left alone: PHP allows it, and
			// libraries read extra ones with func_get_args() to stay compatible.
			if f := buildArgCountFinding(sym, argCount, tok.Line, tok.Column, name); f != nil && argCount < len(sym.Params) {
				findings = append(findings, *f)
			}
			continue
		}
		if prev >= 0 {
			switch tokens[prev].Kind {
			case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenFunction, parser.TokenNew:
				continue
			case parser.TokenAmpersand:
				if pp := prevSignificantTokenIdx(tokens, prev-1); pp >= 0 && tokens[pp].Kind == parser.TokenFunction {
					continue
				}
			}
		}
		// Resolve function FQN
		fqns, ok := resolveFunctionNames(name, file)
		if !ok || len(fqns) == 0 {
			continue
		}
		sym := findProjectFunction(fqns, index)
		if sym == nil {
			continue
		}
		if sym.Source == symbols.SourceBuiltin {
			continue
		}
		if hasVariadicParam(sym.Params) {
			continue
		}
		closeIdx := findMatchingTokenIdx(tokens, next, parser.TokenOpenParen, parser.TokenCloseParen)
		if closeIdx < 0 {
			continue
		}
		argCount, safe := countArguments(tokens, next, closeIdx)
		if !safe {
			continue
		}
		if f := buildArgCountFinding(sym, argCount, tok.Line, tok.Column, name); f != nil {
			findings = append(findings, *f)
		}
		i = closeIdx
	}
	return findings
}

// isSkippedCallKeyword returns true for PHP language constructs that look like
// function calls but are not real calls.
var skippedCallKeywordMap = map[string]bool{
	"if": true, "elseif": true, "for": true, "foreach": true, "while": true,
	"switch": true, "catch": true, "echo": true, "empty": true, "isset": true,
	"unset": true, "include": true, "include_once": true, "require": true,
	"require_once": true, "array": true, "match": true, "clone": true,
	"eval": true, "print": true, "die": true, "exit": true, "list": true,
}

func isSkippedCallKeyword(name string) bool {
	return skippedCallKeywordMap[strings.ToLower(name)]
}

// findProjectMethod looks up a method on a class in the index, returning
// only project-source symbols (not builtins).
func findProjectMethod(classFQN, methodName string, index *symbols.Index) *symbols.Symbol {
	for _, member := range index.GetClassMembers(classFQN) {
		if member.Kind != symbols.KindMethod {
			continue
		}
		if member.Name != methodName {
			continue
		}
		return member
	}
	return nil
}

// findProjectFunction looks up a plain function by its candidate FQNs.
func findProjectFunction(fqns []string, index *symbols.Index) *symbols.Symbol {
	for _, fqn := range fqns {
		sym := index.Lookup(fqn)
		if sym != nil && sym.Kind == symbols.KindFunction {
			return sym
		}
	}
	return nil
}

// calledMethod returns the method a `->name(` call calls and the class it's
// found on, when the receiver's type is exactly one known class-like. arrow
// is the index of the `->` token.
func calledMethod(tokens []parser.Token, arrow int, name parser.Token, lines []string, source string, file *parser.FileNode, index *symbols.Index, typeResolver func(string, string, int, int, *parser.FileNode) string) (sym *symbols.Symbol, owner, receiver string) {
	opTok := tokens[arrow]
	if opTok.Line != name.Line || opTok.Line >= len(lines) {
		return nil, "", ""
	}
	var mask maskState
	line := maskPHPLine(lines[opTok.Line], &mask)
	opStart := opTok.Column
	if q := prevSignificantTokenIdx(tokens, arrow-1); q >= 0 && tokens[q].Kind == parser.TokenQuestion && tokens[q].Line == opTok.Line {
		opStart = tokens[q].Column
	}
	if opStart > len(line) {
		return nil, "", ""
	}
	start := backwardExprStart(line, opStart)
	ownerExpr := strings.TrimSpace(line[start:opStart])
	// A line like `})->name()` continues a chain begun on an earlier line.
	if ownerExpr == "" || strings.Count(ownerExpr, ")") > strings.Count(ownerExpr, "(") {
		return nil, "", ""
	}
	owners := resolvedClassLikes(typeResolver(ownerExpr, source, opTok.Line, start, file), file, index)
	if len(owners) != 1 {
		return nil, "", ""
	}
	sym = findProjectMethod(owners[0], name.Value, index)
	if sym == nil || sym.IsVirtual {
		return nil, "", ""
	}
	return sym, owners[0], ownerExpr
}

// inClosure reports whether a token is inside a closure or arrow function.
func inClosure(doc *scope.Document, tok parser.Token) bool {
	for s := doc.ScopeAt(protocol.Position{Line: tok.Line, Character: tok.Column}); s != nil; s = s.Parent {
		if s.Kind == scope.ScopeClosure {
			return true
		}
	}
	return false
}

// shouldCheckArgCount returns false if the given method belongs to a class
// that defines __call or __callStatic (magic invocation).
func shouldCheckArgCount(sym *symbols.Symbol, classFQN string, index *symbols.Index) bool {
	if sym.Source == symbols.SourceBuiltin {
		return false
	}
	if hasVariadicParam(sym.Params) {
		return false
	}
	// Check for __call/__callStatic on the class
	for _, member := range index.GetClassMembers(classFQN) {
		if member.Kind != symbols.KindMethod {
			continue
		}
		if member.Name == "__call" || member.Name == "__callStatic" {
			return false
		}
	}
	return true
}

// hasVariadicParam reports whether any param in the list is variadic.
func hasVariadicParam(params []symbols.ParamInfo) bool {
	for _, p := range params {
		if p.IsVariadic {
			return true
		}
	}
	return false
}

// countRequired returns the number of required (non-optional, non-variadic) params.
func countRequired(params []symbols.ParamInfo) int {
	count := 0
	for _, p := range params {
		if !p.IsVariadic && p.DefaultValue == "" {
			count++
		}
	}
	return count
}

// countArguments counts actual arguments in a call's parentheses.
// Returns (count, safe) where safe=false means the call cannot be analyzed
// (contains spread, named args, or parser couldn't find balanced parens).
func countArguments(tokens []parser.Token, openIdx, closeIdx int) (int, bool) {
	if closeIdx <= openIdx+1 {
		// Empty argument list
		return 0, true
	}
	// Scan between openIdx+1 and closeIdx-1
	depth := 0
	count := 1
	for i := openIdx + 1; i < closeIdx; i++ {
		tok := tokens[i]
		switch tok.Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			if depth > 0 {
				depth--
			}
		case parser.TokenUnknown:
			// The tokenizer emits '.' as TokenUnknown. Three consecutive '.' = spread.
			// Also, if value is ".", check for "..." spread.
			if tok.Value == "." && depth == 0 {
				// Two more dots ahead = spread operator
				if i+2 < closeIdx && tokens[i+1].Kind == parser.TokenUnknown && tokens[i+1].Value == "." &&
					i+2 < len(tokens) && tokens[i+2].Kind == parser.TokenUnknown && tokens[i+2].Value == "." {
					return 0, false
				}
			}
		case parser.TokenComma:
			// A trailing comma before `)` (PHP 7.3+) doesn't start another argument.
			if depth == 0 && nextSignificantTokenIdx(tokens, i+1) != closeIdx {
				count++
			}
		case parser.TokenIdentifier:
			// Named argument: "name:" pattern — identifier followed by ':'
			// We check if the next significant token is a colon.
			if depth == 0 {
				nextIdx := nextSignificantTokenIdx(tokens, i+1)
				if nextIdx >= 0 && nextIdx < closeIdx && tokens[nextIdx].Kind == parser.TokenColon {
					return 0, false
				}
			}
		}
	}
	// Check for whitespace-only content (no actual args)
	hasContent := false
	for i := openIdx + 1; i < closeIdx; i++ {
		if tokens[i].Kind != parser.TokenWhitespace && tokens[i].Kind != parser.TokenComment && tokens[i].Kind != parser.TokenDocComment {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return 0, true
	}
	return count, true
}

// findMatchingTokenIdx finds the closing token matching an opening token.
func findMatchingTokenIdx(tokens []parser.Token, start int, openKind, closeKind parser.TokenKind) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case openKind:
			depth++
		case closeKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// buildArgCountFinding creates a Finding if the argument count is clearly
// wrong. Too few arguments is an error PHP throws on; too many is allowed by
// PHP but almost always a mistake.
func buildArgCountFinding(sym *symbols.Symbol, argCount, line, col int, callName string) *Finding {
	total := len(sym.Params)

	if required := countRequired(sym.Params); argCount < required {
		return &Finding{
			StartLine: line,
			StartCol:  col,
			EndLine:   line,
			EndCol:    col + len(callName),
			Severity:  SeverityWarning,
			Code:      "argument-count-mismatch",
			Message: fmt.Sprintf(
				"Too few arguments to %s(): %d passed, at least %d required",
				callName, argCount, required,
			),
		}
	}
	if argCount > total {
		return &Finding{
			StartLine: line,
			StartCol:  col,
			EndLine:   line,
			EndCol:    col + len(callName),
			Severity:  SeverityWarning,
			Code:      "argument-count-mismatch",
			Message: fmt.Sprintf(
				"Too many arguments to %s(): %d passed, at most %d expected",
				callName, argCount, total,
			),
		}
	}
	return nil
}
