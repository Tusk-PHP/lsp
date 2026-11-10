package checks

import (
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// memberGuardCallRe finds the calls code makes to ask whether an object has a
// member before using it. It runs on masked lines, so calls in comments and
// strings are already blanked.
var memberGuardCallRe = regexp.MustCompile(`(?i)\b(method_exists|property_exists|is_callable)\s*\(`)

// memberGuard is one `method_exists($x, 'name')`, `property_exists($x, 'name')`
// or `is_callable([$x, 'name'])` check.
type memberGuard struct {
	receiver string // normalized receiver expression, see normalizeGuardReceiver
	name     string // lowercased for methods, as written for properties
	method   bool   // method_exists() or is_callable(), otherwise property_exists()
	line     int
	col      int
	scope    funcScope
}

// funcScope is the function or method body a line is in, by its declaration's
// lines; the zero-width {-1, -1} is code outside any function.
type funcScope struct{ start, end int }

// memberGuards collects the guards in a file as UnknownMemberRule reads it
// line by line, so a guard only answers for accesses after it.
//
// A guard counts for accesses of the same member on the same receiver
// expression, later in the same function or method body. That covers
// `if (method_exists($x, 'm')) { $x->m(); }`, the early return
// `if (! method_exists($x, 'm')) { return; } $x->m();`, ternaries and `&&`
// chains without following control flow. Code that checks for a member and
// then calls it anyway when it's missing is rare enough not to warn about.
type memberGuards struct {
	file   *parser.FileNode
	scopes []funcScope // built on the first guard found
	byName map[string][]memberGuard
}

// add records the guards on one line. masked is the line with comments and
// strings blanked, raw the same line as written; they have the same length.
func (g *memberGuards) add(masked, raw string, lineNum int) {
	if !strings.Contains(masked, "xists") && !strings.Contains(masked, "allable") {
		return
	}
	for _, m := range memberGuardCallRe.FindAllStringSubmatchIndex(masked, -1) {
		if m[0] > 0 {
			// A method or a variable of that name isn't the builtin function.
			switch masked[m[0]-1] {
			case '>', ':', '$':
				continue
			}
		}
		fn := strings.ToLower(masked[m[2]:m[3]])
		pos := m[1]
		closer := byte(')')
		if fn == "is_callable" {
			pos = skipBlanks(masked, pos)
			if pos >= len(masked) || masked[pos] != '[' {
				continue
			}
			pos++
			closer = ']'
		}
		receiver, name, ok := guardArguments(masked, raw, pos, closer)
		if !ok {
			continue
		}
		guard := memberGuard{receiver: receiver, name: name, method: fn != "property_exists", line: lineNum, col: m[0]}
		if guard.method {
			guard.name = strings.ToLower(name)
		}
		guard.scope = g.scopeAt(lineNum)
		if g.byName == nil {
			g.byName = map[string][]memberGuard{}
		}
		g.byName[guard.name] = append(g.byName[guard.name], guard)
	}
}

// covers reports whether a guard before the access at lineNum:col checks for
// member on ownerExpr. Calls are covered by method_exists() and is_callable(),
// instance property reads by property_exists(); class constants by neither.
func (g *memberGuards) covers(ownerExpr, member string, isCall, isStatic bool, lineNum, col int) bool {
	if len(g.byName) == 0 || (!isCall && isStatic) {
		return false
	}
	key := member
	if isCall {
		key = strings.ToLower(member)
	}
	candidates := g.byName[key]
	if len(candidates) == 0 {
		return false
	}
	receiver := normalizeGuardReceiver(ownerExpr)
	var scope funcScope
	scoped := false
	for _, guard := range candidates {
		if guard.method != isCall || guard.receiver != receiver {
			continue
		}
		if guard.line > lineNum || (guard.line == lineNum && guard.col > col) {
			continue
		}
		if !scoped {
			scope, scoped = g.scopeAt(lineNum), true
		}
		if guard.scope == scope {
			return true
		}
	}
	return false
}

// guardArguments reads `receiver, 'name'` followed by closer from pos. The
// receiver must be a variable, a property chain or a class reference
// (`$x`, `$this->items`, `static::class`, `$x::class`, `Foo::class`); the name
// must be a plain string literal naming a member.
func guardArguments(masked, raw string, pos int, closer byte) (receiver, name string, ok bool) {
	depth := 0
	comma := -1
	for i := pos; i < len(masked) && comma < 0; i++ {
		switch masked[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth == 0 {
				return "", "", false
			}
			depth--
		case ',':
			if depth == 0 {
				comma = i
			}
		}
	}
	if comma < 0 {
		return "", "", false
	}
	receiver = normalizeGuardReceiver(masked[pos:comma])
	if !isGuardReceiver(receiver) {
		return "", "", false
	}

	i := skipBlanks(raw, comma+1)
	if i >= len(raw) || (raw[i] != '\'' && raw[i] != '"') {
		return "", "", false
	}
	quote := raw[i]
	start := i + 1
	end := start
	for end < len(raw) && isIdentChar(raw[end]) {
		end++
	}
	if end == start || end >= len(raw) || raw[end] != quote || raw[start] >= '0' && raw[start] <= '9' {
		return "", "", false
	}
	i = skipBlanks(masked, end+1)
	if i >= len(masked) || masked[i] != closer {
		return "", "", false
	}
	return receiver, raw[start:end], true
}

// normalizeGuardReceiver puts a guard's receiver and an access's owner in the
// same form: no blanks, `?->` as `->`, `X::class` as `X`, and `static` (the
// class of `$this`) as `$this`.
func normalizeGuardReceiver(expr string) string {
	if strings.ContainsAny(expr, " \t") {
		expr = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, expr)
	}
	if strings.Contains(expr, "?->") {
		expr = strings.ReplaceAll(expr, "?->", "->")
	}
	if n := len(expr) - len("::class"); n > 0 && strings.EqualFold(expr[n:], "::class") {
		expr = expr[:n]
	}
	switch strings.ToLower(expr) {
	case "static":
		return "$this"
	case "self", "parent":
		return strings.ToLower(expr)
	}
	return expr
}

// isGuardReceiver accepts a variable or property chain (`$x`, `$this->a->b`)
// or a class name, and nothing that is computed, like a call or an index.
func isGuardReceiver(expr string) bool {
	if expr == "" {
		return false
	}
	for _, part := range strings.Split(expr, "->") {
		part = strings.TrimPrefix(part, "$")
		part = strings.TrimPrefix(part, "\\")
		if part == "" || part[0] >= '0' && part[0] <= '9' {
			return false
		}
		for i := 0; i < len(part); i++ {
			if !isIdentChar(part[i]) && part[i] != '\\' {
				return false
			}
		}
	}
	return true
}

// scopeAt returns the innermost function or method declared around line.
// Closures aren't in the AST, so they share the scope of what encloses them.
func (g *memberGuards) scopeAt(line int) funcScope {
	if g.scopes == nil {
		g.scopes = functionScopes(g.file)
	}
	best := funcScope{-1, -1}
	for _, s := range g.scopes {
		if s.start <= line && line <= s.end && s.start >= best.start {
			best = s
		}
	}
	return best
}

func functionScopes(file *parser.FileNode) []funcScope {
	scopes := []funcScope{}
	if file == nil {
		return scopes
	}
	addMethods := func(methods []parser.MethodNode) {
		for _, m := range methods {
			scopes = append(scopes, funcScope{m.StartLine, m.EndLine})
		}
	}
	for i := range file.Classes {
		addMethods(file.Classes[i].Methods)
	}
	for i := range file.AnonymousClasses {
		addMethods(file.AnonymousClasses[i].Methods)
	}
	for i := range file.Traits {
		addMethods(file.Traits[i].Methods)
	}
	for i := range file.Enums {
		addMethods(file.Enums[i].Methods)
	}
	for _, fn := range file.Functions {
		scopes = append(scopes, funcScope{fn.StartLine, fn.EndLine})
	}
	return scopes
}

func skipBlanks(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}
