package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

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

// collect records the guards in the file: `method_exists($x, 'name')`,
// `property_exists($x, 'name')` and `is_callable([$x, 'name'])`, in any case
// and over any number of lines.
func (g *memberGuards) collect(syntax *fileSyntax) {
	for _, call := range syntax.references().calls {
		fn := strings.ToLower(strings.TrimPrefix(syntax.text(call.name), "\\"))
		if !memberGuardFuncs[fn] {
			continue
		}
		receiver, name, ok := guardArguments(syntax, fn, call.args.Args)
		if !ok {
			continue
		}
		line, col := syntax.lines.Position(call.name.Start)
		guard := memberGuard{receiver: receiver, name: name, method: fn != "property_exists", line: line, col: col}
		if guard.method {
			guard.name = strings.ToLower(name)
		}
		guard.scope = g.scopeAt(line)
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

// guardArguments reads the receiver and member name of a guard call. The
// receiver must be a variable, a property chain or a class reference (`$x`,
// `$this->items`, `static::class`, `$x::class`, `Foo::class`); the name must
// be a plain string literal naming a member.
func guardArguments(syntax *fileSyntax, fn string, args []*ast.Arg) (receiver, name string, ok bool) {
	var parts []ast.Expr
	switch {
	case fn != "is_callable" && len(args) == 2:
		parts = []ast.Expr{args[0].Value, args[1].Value}
	case fn == "is_callable" && len(args) == 1:
		array, isArray := args[0].Value.(*ast.Array)
		if !isArray || len(array.Items) != 2 || array.Items[0].Key != nil || array.Items[1].Key != nil {
			return "", "", false
		}
		parts = []ast.Expr{array.Items[0].Value, array.Items[1].Value}
	default:
		return "", "", false
	}
	if parts[0] == nil || parts[1] == nil {
		return "", "", false
	}
	receiver = normalizeGuardReceiver(syntax.text(parts[0]))
	if !isGuardReceiver(receiver) {
		return "", "", false
	}
	name, ok = plainMemberName(syntax.text(parts[1]))
	return receiver, name, ok
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
		if !parser.IsIdentByte(name[i]) {
			return "", false
		}
	}
	return name, true
}

// normalizeGuardReceiver puts a guard's receiver and an access's owner in the
// same form: no blanks, `?->` as `->`, `X::class` as `X`, and `static` (the
// class of `$this`) as `$this`.
func normalizeGuardReceiver(expr string) string {
	if strings.ContainsAny(expr, " \t\r\n") {
		expr = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
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
			if !parser.IsIdentByte(part[i]) && part[i] != '\\' {
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
