package resolve

import (
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// assignedExpression reduces the right side of an assignment to the expression
// that decides its type, or to the type itself when that's written out, as on a
// closure's return. It sees through `clone`, `|>` and `?? throw`.
func assignedExpression(rhs string) (expr, declaredType string) {
	expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rhs), ";"))

	if stages := splitTopLevel(expr, "|>"); len(stages) > 1 {
		return pipeStage(stages[len(stages)-1])
	}
	for _, op := range []string{"??", "?:"} {
		if parts := splitTopLevel(expr, op); len(parts) == 2 && strings.HasPrefix(strings.TrimSpace(parts[1]), "throw ") {
			return assignedExpression(parts[0])
		}
	}
	if rest, ok := cutKeyword(expr, "clone"); ok {
		if strings.HasPrefix(rest, "(") && matchingClose(rest, 0) == len(rest)-1 {
			// PHP 8.5 `clone($a, [...])`: the result is a copy of the first argument.
			args := splitTopLevel(rest[1:len(rest)-1], ",")
			rest = strings.TrimSpace(args[0])
		}
		return assignedExpression(rest)
	}
	return expr, ""
}

// pipeStage returns what the stage of a pipe gives back: `f(...)` and `$a->f(...)`
// return what `f` does, and closures what they're declared to or what they evaluate to.
func pipeStage(stage string) (expr, declaredType string) {
	stage = strings.TrimSpace(stage)
	for strings.HasPrefix(stage, "(") && matchingClose(stage, 0) == len(stage)-1 {
		stage = strings.TrimSpace(stage[1 : len(stage)-1])
	}
	if callable, ok := strings.CutSuffix(stage, "(...)"); ok {
		return callable + "()", ""
	}
	return closureResult(stage)
}

var closureStartRe = regexp.MustCompile(`^(?:static\s+)?(fn|function)\s*&?\s*\(`)

func closureResult(expr string) (string, string) {
	head := closureStartRe.FindStringSubmatch(expr)
	if head == nil {
		return expr, ""
	}
	open := len(head[0]) - 1
	end := matchingClose(expr, open)
	if end < 0 {
		return expr, ""
	}
	rest := strings.TrimSpace(expr[end+1:])
	if head[1] == "fn" {
		arrow := strings.Index(rest, "=>")
		if arrow < 0 {
			return expr, ""
		}
		declared := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest[:arrow]), ":"))
		if declared != "" {
			return "", declared
		}
		return assignedExpression(rest[arrow+2:])
	}
	if brace := strings.IndexByte(rest, '{'); brace >= 0 {
		rest = rest[:brace]
	}
	if declared := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), ":")); declared != "" {
		return "", declared
	}
	return expr, ""
}

// cutKeyword removes a leading keyword that's followed by something other than
// more of the same word.
func cutKeyword(expr, keyword string) (string, bool) {
	if len(expr) <= len(keyword) || !strings.EqualFold(expr[:len(keyword)], keyword) {
		return "", false
	}
	if next := expr[len(keyword)]; next != ' ' && next != '\t' && next != '(' {
		return "", false
	}
	return strings.TrimSpace(expr[len(keyword):]), true
}

// splitTopLevel splits on a separator outside brackets and strings.
func splitTopLevel(expr, sep string) []string {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
		if depth == 0 && strings.HasPrefix(expr[i:], sep) {
			parts = append(parts, expr[start:i])
			i += len(sep) - 1
			start = i + 1
		}
	}
	return append(parts, expr[start:])
}

// matchingClose returns the index of what closes the bracket at open, or -1.
func matchingClose(expr string, open int) int {
	depth := 0
	var quote byte
	for i := open; i < len(expr); i++ {
		c := expr[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// newHasChain reports whether a `new` expression carries on into a member access,
// as PHP 8.4 allows without wrapping it in parentheses: `new Foo()->bar()`.
func newHasChain(expr string) bool {
	open := strings.IndexByte(expr, '(')
	if open < 0 {
		return false
	}
	end := matchingClose(expr, open)
	if end < 0 {
		return false
	}
	rest := strings.TrimSpace(expr[end+1:])
	return strings.HasPrefix(rest, "->") || strings.HasPrefix(rest, "?->") || strings.HasPrefix(rest, "::")
}

var spreadOnlyRe = regexp.MustCompile(`^\[\s*\.\.\.(\$[A-Za-z_][A-Za-z0-9_]*)\s*(?:,\s*\.\.\.\$[A-Za-z_][A-Za-z0-9_]*\s*)*,?\s*\]$`)

// spreadOnlyVariable returns the first variable of an array made only of spreads,
// such as `[...$a, ...$b]`, whose elements are those variables' elements.
func spreadOnlyVariable(expr string) string {
	if m := spreadOnlyRe.FindStringSubmatch(expr); m != nil {
		return m[1]
	}
	return ""
}

// paramTypeName returns a parameter's declared type, or else the class its
// PHP 8.1 `= new Foo()` default creates.
func paramTypeName(param parser.ParamNode) string {
	if param.Type.Name != "" {
		return param.Type.Name
	}
	rest, ok := cutKeyword(param.DefaultValue, "new")
	if !ok {
		return ""
	}
	if open := strings.IndexByte(rest, '('); open >= 0 {
		rest = rest[:open]
	}
	return strings.TrimSpace(rest)
}
