package checks

import (
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// memberGuardHintRe is a cheap test for whether a file may call one of the
// guard functions at all, before it's read as tokens. Function names ignore
// case in PHP, so it does too.
var memberGuardHintRe = regexp.MustCompile(`(?i)_exists|is_callable`)

// memberGuardFuncs are the calls code makes to ask whether an object has a
// member before using it, lowercased.
var memberGuardFuncs = map[string]bool{"method_exists": true, "property_exists": true, "is_callable": true}

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

// memberGuards holds the guards in a file, read from its tokens, so a call
// written over several lines counts; a guard only answers for accesses after it.
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

// collect records the guards among tokens: `method_exists($x, 'name')`,
// `property_exists($x, 'name')` and `is_callable([$x, 'name'])`, in any case
// and over any number of lines.
func (g *memberGuards) collect(tokens []parser.Token) {
	for i, tok := range tokens {
		if tok.Kind != parser.TokenIdentifier {
			continue
		}
		fn := strings.ToLower(tok.Value)
		if !memberGuardFuncs[fn] || !isGlobalFunctionCall(tokens, i) {
			continue
		}
		open := parser.NextSignificant(tokens, i+1)
		if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
			continue
		}
		start, closer := open+1, parser.TokenCloseParen
		if fn == "is_callable" {
			bracket := parser.NextSignificant(tokens, start)
			if bracket < 0 || tokens[bracket].Kind != parser.TokenOpenBracket {
				continue
			}
			start, closer = bracket+1, parser.TokenCloseBracket
		}
		receiver, name, ok := guardArguments(tokens, start, closer)
		if !ok {
			continue
		}
		guard := memberGuard{receiver: receiver, name: name, method: fn != "property_exists", line: tok.Line, col: tok.Column}
		if guard.method {
			guard.name = strings.ToLower(name)
		}
		guard.scope = g.scopeAt(tok.Line)
		if g.byName == nil {
			g.byName = map[string][]memberGuard{}
		}
		g.byName[guard.name] = append(g.byName[guard.name], guard)
	}
}

// isGlobalFunctionCall reports whether the name at tokens[i] calls a global
// function: not a method or static method of that name, a declaration, or a
// function in another namespace (`Foo\method_exists()`).
func isGlobalFunctionCall(tokens []parser.Token, i int) bool {
	prev := parser.PrevSignificant(tokens, i-1)
	if prev < 0 {
		return true
	}
	switch tokens[prev].Kind {
	case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenFunction, parser.TokenNew:
		return false
	case parser.TokenBackslash:
		before := parser.PrevSignificant(tokens, prev-1)
		return before < 0 || tokens[before].Kind != parser.TokenIdentifier
	}
	return true
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

// guardArguments reads `receiver, 'name'` followed by closer from tokens[start].
// The receiver must be a variable, a property chain or a class reference
// (`$x`, `$this->items`, `static::class`, `$x::class`, `Foo::class`); the name
// must be a plain string literal naming a member.
func guardArguments(tokens []parser.Token, start int, closer parser.TokenKind) (receiver, name string, ok bool) {
	var b strings.Builder
	depth := 0
	comma := -1
	for i := start; i < len(tokens) && comma < 0; i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket:
			if depth == 0 {
				return "", "", false
			}
			depth--
		case parser.TokenComma:
			if depth == 0 {
				comma = i
				continue
			}
		case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace, parser.TokenEOF:
			return "", "", false
		case parser.TokenComment, parser.TokenDocComment:
			continue
		}
		b.WriteString(tokens[i].Value)
	}
	if comma < 0 {
		return "", "", false
	}
	receiver = normalizeGuardReceiver(b.String())
	if !isGuardReceiver(receiver) {
		return "", "", false
	}
	lit := parser.NextSignificant(tokens, comma+1)
	if lit < 0 || tokens[lit].Kind != parser.TokenStringLiteral {
		return "", "", false
	}
	name, ok = plainMemberName(tokens[lit].Value)
	if !ok {
		return "", "", false
	}
	end := parser.NextSignificant(tokens, lit+1)
	// A trailing comma, as in `[$x, 'name',]` written over several lines.
	if end >= 0 && tokens[end].Kind == parser.TokenComma {
		end = parser.NextSignificant(tokens, end+1)
	}
	if end < 0 || tokens[end].Kind != closer {
		return "", "", false
	}
	return receiver, name, true
}

// plainMemberName returns the member a quoted string literal names, as
// `'save'` or `"save"`, and false for anything else, such as `"get$name"`.
func plainMemberName(lit string) (string, bool) {
	if len(lit) < 3 || (lit[0] != '\'' && lit[0] != '"') || lit[len(lit)-1] != lit[0] {
		return "", false
	}
	name := lit[1 : len(lit)-1]
	if name[0] >= '0' && name[0] <= '9' {
		return "", false
	}
	for i := 0; i < len(name); i++ {
		if !isIdentChar(name[i]) {
			return "", false
		}
	}
	return name, true
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
