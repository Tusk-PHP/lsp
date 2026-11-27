package checks

import (
	"fmt"
	"math"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/rule"
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
	TypeResolver TypeResolver
}

func (r *ArgumentCountRule) Codes() []rule.Code { return []rule.Code{rule.ArgumentCountMismatch} }

func (r *ArgumentCountRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || index == nil {
		return nil
	}
	return checkArgumentCounts(file, source, index, r.Shared, r.TypeResolver)
}

func checkArgumentCounts(file *parser.FileNode, source string, index *symbols.Index, shared *Shared, typeResolver TypeResolver) []Finding {
	var findings []Finding
	var gatedSpans []ast.Span
	gatedDone := false
	syntax := shared.tree(source, file)
	eachResolvedCall(syntax, file, source, index, typeResolver, nil, func(call resolvedCall) {
		at := call.name.Bounds().Start
		if call.kind == callConstructor || !allCheckable(call.callees) {
			return
		}
		argCount, safe := countArguments(call.args)
		if !safe {
			return
		}
		line, col := syntax.lines.Position(at)
		f := buildArgCountFinding(call.callees, argCount, call.kind == callFunction, line, col, syntax.text(call.name))
		if f == nil {
			return
		}
		if !gatedDone {
			gatedSpans, gatedDone = versionGatedSpans(shared.parse(source).Tokens), true
		}
		if inSpans(gatedSpans, at) {
			return
		}
		findings = append(findings, *f)
	})
	return findings
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

func countArguments(args *ast.ArgList) (int, bool) {
	if args.RestPlaceholder {
		return 0, false
	}
	for _, a := range args.Args {
		if a.Unpack || a.Placeholder || a.Name != nil {
			return 0, false
		}
	}
	return len(args.Args), true
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
