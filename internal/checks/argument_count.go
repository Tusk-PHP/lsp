package checks

import (
	"fmt"
	"math"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
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

func (r *ArgumentCountRule) Codes() []rule.Code { return []rule.Code{rule.ArgumentCountMismatch} }

func (r *ArgumentCountRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || index == nil {
		return nil
	}
	return checkArgumentCounts(file, source, index, r.Shared, r.TypeResolver)
}

func checkArgumentCounts(file *parser.FileNode, source string, index *symbols.Index, shared *Shared, typeResolver func(string, string, int, int, *parser.FileNode) string) []Finding {
	var findings []Finding
	var gatedSpans []tokenSpan
	gatedDone := false
	skipUntil := -1
	eachResolvedCall(file, source, index, shared, typeResolver, nil, func(call resolvedCall) {
		if call.kind == callConstructor || call.at <= skipUntil {
			return
		}
		versionGated := func() bool {
			if !gatedDone {
				gatedSpans, gatedDone = versionGatedSpans(call.tokens), true
			}
			return inTokenSpans(gatedSpans, call.at)
		}
		if !allCheckable(call.callees) {
			return
		}
		argCount, safe := countArguments(call.tokens, call.open, call.close)
		if !safe {
			return
		}
		if call.kind != callMethod {
			skipUntil = call.close
		}
		f := buildArgCountFinding(call.callees, argCount, call.kind == callFunction, call.site.Line, call.site.Column, call.name)
		if f == nil || versionGated() {
			return
		}
		findings = append(findings, *f)
	})
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

func calledMethod(tokens []parser.Token, arrow int, name parser.Token, lines []string, source string, file *parser.FileNode, index *symbols.Index, typeResolver func(string, string, int, int, *parser.FileNode) string) ([]callee, string) {
	opTok := tokens[arrow]
	if opTok.Line != name.Line || opTok.Line >= len(lines) {
		return nil, ""
	}
	var mask maskState
	line := maskPHPLine(lines[opTok.Line], &mask)
	opStart := opTok.Column
	if q := parser.PrevSignificant(tokens, arrow-1); q >= 0 && tokens[q].Kind == parser.TokenQuestion && tokens[q].Line == opTok.Line {
		opStart = tokens[q].Column
	}
	if opStart > len(line) {
		return nil, ""
	}
	start := backwardExprStart(line, opStart)
	ownerExpr := strings.TrimSpace(line[start:opStart])
	// A line like `})->name()` continues a chain begun on an earlier line.
	if ownerExpr == "" || strings.Count(ownerExpr, ")") > strings.Count(ownerExpr, "(") {
		return nil, ""
	}
	owners := resolvedClassLikes(typeResolver(ownerExpr, source, opTok.Line, start, file), file, index)
	if len(owners) != 1 {
		return nil, ""
	}
	callees := methodCallees(owners[0], name.Value, index)
	for _, c := range callees {
		if c.sym.IsVirtual {
			return nil, ""
		}
	}
	return callees, ownerExpr
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
			if depth == 0 && parser.NextSignificant(tokens, i+1) != closeIdx {
				count++
			}
		case parser.TokenIdentifier:
			// Named argument: "name:" pattern — identifier followed by ':'
			// We check if the next significant token is a colon.
			if depth == 0 {
				nextIdx := parser.NextSignificant(tokens, i+1)
				if nextIdx >= 0 && nextIdx < closeIdx && tokens[nextIdx].Kind == parser.TokenColon {
					return 0, false
				}
			}
		}
	}
	if parser.NextSignificant(tokens, openIdx+1) == closeIdx {
		return 0, true
	}
	return count, true
}

func buildArgCountFinding(callees []callee, argCount int, tooManyCounts bool, line, col int, callName string) *Finding {
	minRequired, maxTotal := math.MaxInt, 0
	for _, c := range callees {
		required, total := countRequired(c.sym.Params), len(c.sym.Params)
		if argCount >= required && (argCount <= total || !tooManyCounts) {
			return nil
		}
		minRequired, maxTotal = min(minRequired, required), max(maxTotal, total)
	}
	var message string
	switch {
	case argCount < minRequired:
		message = fmt.Sprintf("Too few arguments to %s(): %d passed, at least %d required", callName, argCount, minRequired)
	case argCount > maxTotal:
		message = fmt.Sprintf("Too many arguments to %s(): %d passed, at most %d expected", callName, argCount, maxTotal)
	default:
		message = fmt.Sprintf("Wrong number of arguments to %s(): %d passed, which none of its %d declarations accepts", callName, argCount, len(callees))
	}
	return &Finding{
		StartLine: line,
		StartCol:  col,
		EndLine:   line,
		EndCol:    col + len(callName),
		Severity:  rule.SeverityWarning,
		Code:      rule.ArgumentCountMismatch,
		Message:   message,
	}
}
