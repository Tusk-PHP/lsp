package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer"
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
	abilities     []Ability
	resources     []resourceRegistration
	policies      []policyRegistration
	middleware    []Middleware
	open          authOpen
	kernelAliases bool
}

func (f authFile) empty() bool {
	return len(f.abilities)+len(f.resources)+len(f.policies)+len(f.middleware) == 0 && f.open == (authOpen{}) && !f.kernelAliases
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
	gateHooks               = []string{"before", "after", "guessPolicyNamesUsing"}
	gateRegistrations       = []string{"define", "resource", "policy", "before", "after", "guessPolicyNamesUsing"}
	middlewareRegistrations = []string{"aliasMiddleware", "middlewareGroup", "pushMiddlewareToGroup", "prependMiddlewareToGroup"}
	instanceRegistrations   = append([]string{"define", "withMiddleware", "guessPolicyNamesUsing"}, middlewareRegistrations...)
)

func isOneOf(name string, names []string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) })
}

func readAuthFile(uri, src, rel string, grammars phpparser.GrammarRange, projectNamespaces []string) authFile {
	r, tree := newAuthReader(uri, src, grammars)
	provider := isProvider(rel)
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
				if !r.instanceDefine(n.Args, &out) && provider {
					out.open.abilities = true
				}
			case strings.EqualFold(name, "guessPolicyNamesUsing"), provider && isOneOf(name, gateRegistrations):
				out.open.abilities = true
			default:
				r.routerCall(name, n.Args, &out)
			}
		case *ast.PropertyItem:
			r.property(n, &out)
		}
		return true
	})
	if (rel == configApp || rel == bootstrapProviders) && r.registersForeignProviders(tree, rel == configApp, projectNamespaces) {
		out.open = authOpen{abilities: true, middleware: true}
	}
	return out
}

func (r authReader) registersForeignProviders(tree *ast.File, onlyProvidersKey bool, projectNamespaces []string) bool {
	foreign := false
	visit := func(root ast.Node) {
		ast.Walk(root, func(n ast.Node) bool {
			e, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			class := r.class(e)
			if class == "" {
				return true
			}
			if !strings.HasPrefix(class, `Illuminate\`) && !slices.ContainsFunc(projectNamespaces, func(ns string) bool { return strings.HasPrefix(class, ns) }) {
				foreign = true
			}
			return false
		})
	}
	if !onlyProvidersKey {
		visit(tree)
		return foreign
	}
	ast.Walk(tree, func(n ast.Node) bool {
		item, ok := n.(*ast.ArrayItem)
		if !ok {
			return true
		}
		if key, _, isString := r.stringOf(item.Key); isString && key == "providers" && item.Value != nil {
			visit(item.Value)
			return false
		}
		return true
	})
	return foreign
}

func (r authReader) isGate(call *ast.StaticCall) bool {
	class, ok := call.Class.(*ast.ConstFetch)
	if !ok {
		return false
	}
	resolved := r.file.ResolveClassName(r.text(class))
	return strings.EqualFold(resolved, gateFacade) || strings.EqualFold(resolved, gateAlias)
}

func (r authReader) instanceDefine(args *ast.ArgList, out *authFile) bool {
	name, lit, ok := r.argString(args.Args, 0)
	if ok {
		out.abilities = append(out.abilities, Ability{Name: name, URI: r.uri, Range: r.literalRange(lit)})
	}
	return ok
}

func (r authReader) routerCall(method string, args *ast.ArgList, out *authFile) {
	if !isOneOf(method, middlewareRegistrations) {
		return
	}
	name, lit, ok := r.argString(args.Args, 0)
	if !ok {
		out.open.middleware = true
		return
	}
	m := Middleware{Name: name, Kind: MiddlewareGroup, URI: r.uri, Range: r.literalRange(lit)}
	if strings.EqualFold(method, "aliasMiddleware") {
		m.Kind, m.Class = MiddlewareAlias, r.argClass(args.Args, 1)
	}
	out.middleware = append(out.middleware, m)
}

func (r authReader) gateCall(call *ast.StaticCall, out *authFile) {
	method := ast.MethodName(r.src, call)
	if isOneOf(method, gateHooks) || method == "" {
		out.open.abilities = true
		return
	}
	args := call.Args.Args
	switch strings.ToLower(method) {
	case "define":
		name, lit, ok := r.argString(args, 0)
		if !ok {
			out.open.abilities = true
			return
		}
		out.abilities = append(out.abilities, Ability{Name: name, URI: r.uri, Range: r.literalRange(lit)})
	case "resource":
		name, lit, ok := r.argString(args, 0)
		policy := r.argClass(args, 1)
		if !ok || policy == "" {
			out.open.abilities = true
			return
		}
		res := resourceRegistration{name: name, policy: policy, uri: r.uri, rng: r.literalRange(lit)}
		if len(args) > 2 {
			array, isArray := args[2].Value.(*ast.Array)
			if !isArray {
				out.open.abilities = true
				return
			}
			res.abilities = r.resourceMap(array, &out.open)
		}
		out.resources = append(out.resources, res)
	case "policy":
		model, policy := r.argClass(args, 0), r.argClass(args, 1)
		if model == "" || policy == "" {
			out.open.abilities = true
			return
		}
		out.policies = append(out.policies, policyRegistration{model: model, policy: policy})
	}
}

func (r authReader) middlewareConfiguration(args *ast.ArgList, out *authFile) {
	ast.Walk(args, func(n ast.Node) bool {
		call, ok := n.(*ast.MethodCall)
		if !ok {
			return true
		}
		switch strings.ToLower(ast.MethodName(r.src, call)) {
		case "alias":
			array, ok := r.argArray(call.Args.Args, 0)
			if !ok {
				out.open.middleware = true
				return true
			}
			out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareAlias, &out.open)...)
		case "group", "appendtogroup", "prependtogroup":
			name, lit, ok := r.argString(call.Args.Args, 0)
			if !ok {
				out.open.middleware = true
				return true
			}
			out.middleware = append(out.middleware, Middleware{Name: name, Kind: MiddlewareGroup, URI: r.uri, Range: r.literalRange(lit)})
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
		out.kernelAliases = true
		out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareAlias, &out.open)...)
	case "$middlewareGroups":
		out.middleware = append(out.middleware, r.middlewareArray(array, MiddlewareGroup, &out.open)...)
	case "$policies":
		for _, entry := range array.Items {
			if entry == nil {
				continue
			}
			model, policy := r.class(entry.Key), r.class(entry.Value)
			if model == "" || policy == "" {
				out.open.abilities = true
				continue
			}
			out.policies = append(out.policies, policyRegistration{model: model, policy: policy})
		}
	}
}

func (r authReader) middlewareArray(array *ast.Array, kind MiddlewareKind, open *authOpen) []Middleware {
	out := make([]Middleware, 0, len(array.Items))
	for _, entry := range array.Items {
		if entry == nil {
			continue
		}
		name, lit, ok := r.stringOf(entry.Key)
		if !ok {
			open.middleware = true
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

func (r authReader) resourceMap(array *ast.Array, open *authOpen) []resourceAbility {
	out := make([]resourceAbility, 0, len(array.Items))
	for _, entry := range array.Items {
		if entry == nil {
			continue
		}
		ability, _, okKey := r.stringOf(entry.Key)
		method, _, okValue := r.stringOf(entry.Value)
		if !okKey || !okValue {
			open.abilities = true
			continue
		}
		out = append(out, resourceAbility{ability: ability, method: method})
	}
	return out
}

func (r authReader) argArray(args []*ast.Arg, i int) (*ast.Array, bool) {
	if i >= len(args) || args[i].Name != nil {
		return nil, false
	}
	array, ok := args[i].Value.(*ast.Array)
	return array, ok
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
		var open authOpen
		aliases := r.methodArray(tree, "defaultAliases")
		groups := r.methodArray(tree, "getMiddlewareGroups")
		if aliases != nil && groups != nil {
			out.middleware = append(r.middlewareArray(aliases, MiddlewareAlias, &open), r.middlewareArray(groups, MiddlewareGroup, &open)...)
			out.defaults = !open.middleware
		}
	}
	if r, tree, ok := readVendorFile(filepath.Join(src, "Auth", "Access", "Gate.php"), grammars); ok {
		if array := r.methodArray(tree, "resource"); array != nil {
			var open authOpen
			if resource := r.resourceMap(array, &open); !open.abilities {
				out.resource = resource
			}
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

func readPackageProvider(src string, grammars phpparser.GrammarRange) authOpen {
	r, tree := newAuthReader("", src, grammars)
	var open authOpen
	ast.Walk(tree, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.StaticCall:
			method := ast.MethodName(r.src, n)
			if r.isGate(n) {
				open.abilities = true
			}
			if isOneOf(method, middlewareRegistrations) {
				open.middleware = true
			}
		case *ast.MethodCall:
			method := ast.MethodName(r.src, n)
			if isOneOf(method, gateRegistrations) {
				open.abilities = true
			}
			if isOneOf(method, middlewareRegistrations) {
				open.middleware = true
			}
		}
		return true
	})
	return open
}

func readPackageProviders(rootPath string, autoload []composer.AutoloadEntry, grammars phpparser.GrammarRange) authOpen {
	if _, err := os.Stat(filepath.Join(rootPath, "vendor", "composer", "installed.json")); err != nil {
		return authOpen{abilities: true, middleware: true}
	}
	var open authOpen
	for _, provider := range composer.LaravelPackageProviders(rootPath) {
		src, ok := readClassFile(provider, autoload)
		if !ok {
			return authOpen{abilities: true, middleware: true}
		}
		open.add(readPackageProvider(src, grammars))
	}
	return open
}

func readClassFile(fqn string, autoload []composer.AutoloadEntry) (string, bool) {
	for _, file := range composer.ClassFiles(fqn, autoload) {
		if data, err := os.ReadFile(file.Path()); err == nil {
			return string(data), true
		}
	}
	return "", false
}
