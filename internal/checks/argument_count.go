package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ArgumentCountRule flags function/method calls where the argument count is
// clearly wrong — fewer arguments than required params, or more arguments
// than the total params when no variadic param exists.
//
// Conservative by design: it skips calls that contain named arguments, spread
// expressions, variadic signatures, magic __call/__callStatic, and any builtin
// symbols whose signature may be incomplete.
type ArgumentCountRule struct{}

func (r *ArgumentCountRule) Code() string { return "argument-count-mismatch" }

func (r *ArgumentCountRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || index == nil {
		return nil
	}
	return checkArgumentCounts(file, source, index)
}

func checkArgumentCounts(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	result := parser.New().Parse(source)
	if result == nil {
		return nil
	}

	var findings []Finding
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
			if f := buildArgCountFinding(sym, argCount, tok.Line, tok.Column, methodTok.Value); f != nil {
				findings = append(findings, *f)
			}
			i = closeIdx
			continue
		}

		// Plain function call: name(...)
		if tokens[next].Kind != parser.TokenOpenParen {
			continue
		}
		// Skip: preceded by -> or :: — it's a method call we can't resolve without types
		prev := prevSignificantTokenIdx(tokens, i-1)
		if prev >= 0 {
			switch tokens[prev].Kind {
			case parser.TokenArrow, parser.TokenDoubleColon:
				continue
			}
			// ?-> nullsafe arrow: TokenQuestion + TokenArrow preceding the identifier
			if tokens[prev].Kind == parser.TokenArrow {
				continue
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
			if depth == 0 {
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

// buildArgCountFinding creates a Finding if the argument count is clearly wrong.
//
// Note: "too few arguments" is only flagged when we can reliably determine the
// required parameter count. Because the symbol index's ParamInfo.DefaultValue
// is not populated for project functions (the parser does not currently
// propagate HasDefault through the ParamNode→ParamInfo path), we conservatively
// skip the "too few" check. Only "too many" arguments are flagged.
func buildArgCountFinding(sym *symbols.Symbol, argCount, line, col int, callName string) *Finding {
	total := len(sym.Params)

	// Only flag too-many-arguments (the too-few path is disabled because we
	// cannot reliably distinguish required vs optional params — see comment above).
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
