package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const (
	gateFacade = `Illuminate\Support\Facades\Gate`
	gateAlias  = "Gate"
)

type authFile struct {
	abilities  []Ability
	resources  []resourceRegistration
	policies   []policyRegistration
	middleware []Middleware
}

func (f authFile) empty() bool {
	return len(f.abilities)+len(f.resources)+len(f.policies)+len(f.middleware) == 0
}

type resourceAbility struct {
	ability, method string
}

type resourceRegistration struct {
	name, policy string
	uri          string
	rng          textpos.Range
	abilities    []resourceAbility
}

type policyRegistration struct {
	model, policy string
}

type authReader struct {
	uri   string
	src   string
	file  *parser.FileNode
	lines ast.Lines
}

func newAuthReader(uri, src string, grammars phpparser.GrammarRange) (authReader, *ast.File) {
	tree := parser.ParseTreeIn(src, grammars)
	return authReader{uri: uri, src: src, file: parser.FileNodeOf(src, tree), lines: ast.NewLines(src)}, tree
}

var (
	gateRegistrations       = []string{"define", "resource", "policy"}
	middlewareRegistrations = []string{"aliasMiddleware", "middlewareGroup", "pushMiddlewareToGroup", "prependMiddlewareToGroup"}
	instanceRegistrations   = append([]string{"define", "withMiddleware"}, middlewareRegistrations...)
)

func isOneOf(name string, names []string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) })
}

func readAuthFile(uri, src string, grammars phpparser.GrammarRange) authFile {
	r, tree := newAuthReader(uri, src, grammars)
	var out authFile
	ast.Walk(tree, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.StaticCall:
			if r.isGate(n) {
				r.gateCall(n, &out)
			} else {
				r.routerCall(ast.MethodName(r.src, n), n.Args, &out)
			}
		case *ast.MethodCall:
			switch name := ast.MethodName(r.src, n); {
			case strings.EqualFold(name, "withMiddleware"):
				r.middlewareConfiguration(n.Args, &out)
			case strings.EqualFold(name, "define"):
				r.instanceDefine(n.Args, &out)
			default:
				r.routerCall(name, n.Args, &out)
			}
		case *ast.PropertyItem:
			r.property(n, &out)
		}
		return true
	})
	return out
}

func (r authReader) isGate(call *ast.StaticCall) bool {
	class, ok := call.Class.(*ast.ConstFetch)
	if !ok {
		return false
	}
	resolved := r.file.ResolveClassName(r.text(class))
	return strings.EqualFold(resolved, gateFacade) || strings.EqualFold(resolved, gateAlias)
}

func (r authReader) instanceDefine(args *ast.ArgList, out *authFile) {
	if args == nil {
		return
	}
	if name, lit, ok := r.argString(args.Args, 0); ok {
		out.abilities = append(out.abilities, Ability{Name: name, URI: r.uri, Range: r.literalRange(lit)})
	}
}

func (r authReader) routerCall(method string, args *ast.ArgList, out *authFile) {
	if args == nil || !isOneOf(method, middlewareRegistrations) {
		return
	}
	name, lit, ok := r.argString(args.Args, 0)
	if !ok {
		return
	}
	m := Middleware{Name: name, Kind: MiddlewareGroup, URI: r.uri, Range: r.literalRange(lit)}
	if strings.EqualFold(method, "aliasMiddleware") {
		m.Kind, m.Class = MiddlewareAlias, r.argClass(args.Args, 1)
	}
	out.middleware = append(out.middleware, m)
}

func (r authReader) gateCall(call *ast.StaticCall, out *authFile) {
	if call.Args == nil {
		return
	}
	args := call.Args.Args
	switch strings.ToLower(ast.MethodName(r.src, call)) {
	case "define":
		if name, lit, ok := r.argString(args, 0); ok {
			out.abilities = append(out.abilities, Ability{Name: name, URI: r.uri, Range: r.literalRange(lit)})
		}
	case "resource":
		name, lit, ok := r.argString(args, 0)
		policy := r.argClass(args, 1)
		if !ok || policy == "" {
			return
		}
		res := resourceRegistration{name: name, policy: policy, uri: r.uri, rng: r.literalRange(lit)}
		if len(args) > 2 {
			array, isArray := args[2].Value.(*ast.Array)
			if !isArray {
				return
			}
			res.abilities = r.resourceMap(array)
		}
		out.resources = append(out.resources, res)
	case "policy":
		if model, policy := r.argClass(args, 0), r.argClass(args, 1); model != "" && policy != "" {
			out.policies = append(out.policies, policyRegistration{model: model, policy: policy})
		}
	}
}

func (r authReader) middlewareConfiguration(args *ast.ArgList, out *authFile) {
	if args == nil {
		return
	}
	ast.Walk(args, func(n ast.Node) bool {
		call, ok := n.(*ast.MethodCall)
		if !ok || call.Args == nil {
			return true
		}
		switch strings.ToLower(ast.MethodName(r.src, call)) {
		case "alias":
			if len(call.Args.Args) > 0 {
				if array, ok := call.Args.Args[0].Value.(*ast.Array); ok {
					out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareAlias)...)
				}
			}
		case "group", "appendtogroup", "prependtogroup":
			if name, lit, ok := r.argString(call.Args.Args, 0); ok {
				out.middleware = append(out.middleware, Middleware{Name: name, Kind: MiddlewareGroup, URI: r.uri, Range: r.literalRange(lit)})
			}
		}
		return true
	})
}

func (r authReader) property(item *ast.PropertyItem, out *authFile) {
	array, ok := item.Default.(*ast.Array)
	if !ok || item.Var == nil {
		return
	}
	switch r.text(item.Var) {
	case "$middlewareAliases", "$routeMiddleware":
		out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareAlias)...)
	case "$middlewareGroups":
		out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareGroup)...)
	case "$policies":
		for _, entry := range array.Items {
			if entry == nil {
				continue
			}
			if model, policy := r.class(entry.Key), r.class(entry.Value); model != "" && policy != "" {
				out.policies = append(out.policies, policyRegistration{model: model, policy: policy})
			}
		}
	}
}

func (r authReader) middlewareArray(array *ast.Array, kind MiddlewareKind) []Middleware {
	out := make([]Middleware, 0, len(array.Items))
	for _, entry := range array.Items {
		if entry == nil {
			continue
		}
		name, lit, ok := r.stringOf(entry.Key)
		if !ok {
			continue
		}
		m := Middleware{Name: name, Kind: kind, URI: r.uri, Range: r.literalRange(lit)}
		if kind == MiddlewareAlias {
			m.Class = r.class(entry.Value)
		}
		out = append(out, m)
	}
	return out
}

func (r authReader) resourceMap(array *ast.Array) []resourceAbility {
	out := make([]resourceAbility, 0, len(array.Items))
	for _, entry := range array.Items {
		if entry == nil {
			continue
		}
		ability, _, okKey := r.stringOf(entry.Key)
		method, _, okValue := r.stringOf(entry.Value)
		if okKey && okValue {
			out = append(out, resourceAbility{ability: ability, method: method})
		}
	}
	return out
}

func (r authReader) argString(args []*ast.Arg, i int) (string, *ast.Literal, bool) {
	if i >= len(args) || args[i].Name != nil {
		return "", nil, false
	}
	return r.stringOf(args[i].Value)
}

func (r authReader) argClass(args []*ast.Arg, i int) string {
	if i >= len(args) || args[i].Name != nil {
		return ""
	}
	return r.class(args[i].Value)
}

func (r authReader) stringOf(e ast.Expr) (string, *ast.Literal, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != token.ConstantString {
		return "", nil, false
	}
	value, ok := parser.StringLiteral(r.text(lit))
	return value, lit, ok && value != ""
}

func (r authReader) class(e ast.Expr) string {
	if value, _, ok := r.stringOf(e); ok && strings.Contains(value, `\`) {
		return strings.TrimPrefix(value, `\`)
	}
	fetch, ok := e.(*ast.ClassConstFetch)
	if !ok || !strings.EqualFold(r.text(fetch.Name), "class") {
		return ""
	}
	class, ok := fetch.Class.(*ast.ConstFetch)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(r.file.ResolveClassName(r.text(class)), `\`)
}

func (r authReader) text(n ast.Node) string {
	return n.Bounds().Text(r.src)
}

func (r authReader) literalRange(lit *ast.Literal) textpos.Range {
	startLine, startCol := r.lines.Position(lit.Start + 1)
	endLine, endCol := r.lines.Position(lit.End - 1)
	return textpos.Range{
		Start: textpos.Position{Line: startLine, Character: startCol},
		End:   textpos.Position{Line: endLine, Character: endCol},
	}
}

func (r authReader) methodArray(tree *ast.File, name string) *ast.Array {
	var found *ast.Array
	ast.Walk(tree, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		method, ok := n.(*ast.Method)
		if !ok {
			return true
		}
		if method.Name == nil || !strings.EqualFold(r.text(method.Name), name) || method.Body == nil {
			return false
		}
		ast.Walk(method.Body, func(n ast.Node) bool {
			if array, ok := n.(*ast.Array); ok && found == nil {
				found = array
			}
			return found == nil
		})
		return false
	})
	return found
}

func readFrameworkAuth(rootPath string, grammars phpparser.GrammarRange) frameworkAuth {
	src := filepath.Join(rootPath, "vendor", "laravel", "framework", "src", "Illuminate")
	var out frameworkAuth
	if r, tree, ok := readVendorFile(filepath.Join(src, "Foundation", "Configuration", "Middleware.php"), grammars); ok {
		if array := r.methodArray(tree, "defaultAliases"); array != nil {
			out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareAlias)...)
		}
		if array := r.methodArray(tree, "getMiddlewareGroups"); array != nil {
			out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareGroup)...)
		}
	}
	if r, tree, ok := readVendorFile(filepath.Join(src, "Auth", "Access", "Gate.php"), grammars); ok {
		if array := r.methodArray(tree, "resource"); array != nil {
			out.resource = r.resourceMap(array)
		}
	}
	return out
}

func readVendorFile(path string, grammars phpparser.GrammarRange) (authReader, *ast.File, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return authReader{}, nil, false
	}
	r, tree := newAuthReader(symbols.PathToURI(path), string(data), grammars)
	return r, tree, true
}

func mayRegisterNames(src string, version token.Version) bool {
	tokens := significantTokens(lexer.Lex(src, version))
	gateAliased := false
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i+1].Kind == token.As && isGateName(tokens[i], src) {
			gateAliased = true
		}
	}
	for i := 1; i+1 < len(tokens); i++ {
		next := tokens[i+1]
		if next.Kind != token.Identifier {
			continue
		}
		name := next.Text(src)
		switch tokens[i].Kind {
		case token.DoubleColon:
			if isOneOf(name, middlewareRegistrations) || isOneOf(name, gateRegistrations) && (gateAliased || isGateName(tokens[i-1], src)) {
				return true
			}
		case token.ObjectOperator, token.NullsafeObjectOperator:
			if isOneOf(name, instanceRegistrations) {
				return true
			}
		}
	}
	return false
}

func significantTokens(tokens []lexer.Token) []lexer.Token {
	out := tokens[:0]
	for _, tok := range tokens {
		if tok.Kind != token.Whitespace && tok.Kind != token.Comment && tok.Kind != token.DocComment {
			out = append(out, tok)
		}
	}
	return out
}

func isGateName(tok lexer.Token, src string) bool {
	switch tok.Kind {
	case token.Identifier, token.NameQualified, token.NameFullyQualified:
		return ast.LastSegment(tok.Text(src)) == "Gate"
	}
	return false
}
