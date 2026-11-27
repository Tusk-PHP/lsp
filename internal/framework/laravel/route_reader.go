package laravel

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

var supportedRouteMethods = map[string]bool{
	"get":      true,
	"post":     true,
	"put":      true,
	"patch":    true,
	"delete":   true,
	"options":  true,
	"match":    true,
	"any":      true,
	"view":     true,
	"livewire": true,
	"redirect": true,
}

type routeAttr uint8

const (
	routeAs routeAttr = 1 << iota
	routePrefix
	routeController
	routeNamespace
)

var registrarAttrs = map[string]routeAttr{
	"as":         routeAs,
	"name":       routeAs,
	"prefix":     routePrefix,
	"controller": routeController,
	"namespace":  routeNamespace,
}

type routeGroup struct {
	as         string
	prefix     string
	controller string
	namespace  string
	unknown    routeAttr
}

func (g *routeGroup) assign(attr routeAttr, value string, ok bool) {
	if ok {
		g.unknown &^= attr
	} else {
		g.unknown |= attr
		value = ""
	}
	switch attr {
	case routeAs:
		g.as = value
	case routePrefix:
		g.prefix = strings.Trim(value, "/")
	case routeController:
		g.controller = value
	case routeNamespace:
		g.namespace = value
	}
}

func (g routeGroup) nest(inner routeGroup) routeGroup {
	out := g
	out.as += inner.as
	out.prefix = joinRoutePath(g.prefix, inner.prefix)
	if inner.controller != "" {
		out.controller = inner.controller
		out.unknown &^= routeController
	}
	if inner.namespace != "" {
		out.namespace = nestNamespace(g.namespace, inner.namespace)
	}
	out.unknown |= inner.unknown
	return out
}

func nestNamespace(outer, inner string) string {
	if absolute, ok := strings.CutPrefix(inner, `\`); ok || outer == "" {
		return absolute
	}
	return strings.TrimSuffix(outer, `\`) + `\` + inner
}

func joinRoutePath(prefix, path string) string {
	prefix, path = strings.Trim(prefix, "/"), strings.Trim(path, "/")
	if prefix == "" || path == "" {
		return prefix + path
	}
	return prefix + "/" + path
}

type routeCall struct {
	name string
	args []*ast.Arg
}

func (c routeCall) arg(i int) ast.Expr {
	if i >= len(c.args) || c.args[i].Name != nil || c.args[i].Unpack {
		return nil
	}
	return c.args[i].Value
}

func (c routeCall) values() []ast.Expr {
	values := make([]ast.Expr, 0, len(c.args))
	for i := range c.args {
		if v := c.arg(i); v != nil {
			values = append(values, v)
		}
	}
	return values
}

type routeReader struct {
	session   *routeSession
	file      *routeFile
	namespace string
	imports   map[string]string
}

func (r *routeReader) node(n ast.Node, g routeGroup) {
	switch n := n.(type) {
	case *ast.Namespace:
		r.namespace, r.imports = "", nil
		if n.Name != nil {
			r.namespace = r.text(n.Name)
		}
	case *ast.Use:
		r.use(n)
		return
	case *ast.Include:
		if r.include(n.Operand, g) {
			return
		}
	case *ast.MethodCall, *ast.StaticCall:
		if calls, ok := r.routeChain(n.(ast.Expr)); ok {
			r.chain(calls, g)
			return
		}
	}
	ast.Children(n, func(c ast.Node) { r.node(c, g) })
}

func (r *routeReader) use(u *ast.Use) {
	prefix := ""
	if u.Prefix != nil {
		prefix = r.text(u.Prefix) + `\`
	}
	for _, item := range u.Items {
		if item.Kind != token.Use || item.Name == nil {
			continue
		}
		full := strings.TrimPrefix(prefix+r.text(item.Name), `\`)
		alias := lastNameSegment(full)
		if item.Alias != nil {
			alias = r.text(item.Alias)
		}
		if r.imports == nil {
			r.imports = make(map[string]string)
		}
		r.imports[strings.ToLower(alias)] = full
	}
}

func (r *routeReader) routeChain(e ast.Expr) ([]routeCall, bool) {
	var calls []routeCall
	for {
		switch c := e.(type) {
		case *ast.MethodCall:
			calls = append(calls, r.call(c.Name, c.Args))
			e = c.Object
		case *ast.StaticCall:
			class, ok := c.Class.(*ast.ConstFetch)
			if !ok || class.Name == nil || class.Name.Kind == token.Static || lastNameSegment(r.text(class.Name)) != "Route" {
				return nil, false
			}
			calls = append(calls, r.call(c.Name, c.Args))
			slices.Reverse(calls)
			return calls, true
		default:
			return nil, false
		}
	}
}

func (r *routeReader) call(name ast.Expr, args *ast.ArgList) routeCall {
	var c routeCall
	if id, ok := name.(*ast.IdentifierExpr); ok {
		c.name = r.text(id)
	}
	if args != nil {
		c.args = args.Args
	}
	return c
}

func (r *routeReader) chain(calls []routeCall, outer routeGroup) {
	var own routeGroup
	for i, c := range calls {
		if spec, ok := resourceRegistrations[c.name]; ok {
			r.resource(spec, c, calls[i+1:], outer.nest(own))
			return
		}
		if spec, ok := resourceMaps[c.name]; ok {
			r.resourceMap(spec, c, outer.nest(own))
			return
		}
		if supportedRouteMethods[c.name] {
			r.route(c, calls[i+1:], outer.nest(own))
			return
		}
		if c.name == "group" {
			r.group(c, outer, own)
			return
		}
		if attr, ok := registrarAttrs[c.name]; ok {
			r.assign(&own, attr, c.arg(0), outer.nest(own).namespace)
		}
	}
}

func (r *routeReader) assign(g *routeGroup, attr routeAttr, e ast.Expr, namespace string) {
	if attr == routeController {
		class, ok := r.controllerClass(e, namespace)
		g.assign(attr, class, ok)
		return
	}
	value, _, ok := r.str(e)
	g.assign(attr, value, ok)
}

func (r *routeReader) group(c routeCall, outer, own routeGroup) {
	values := c.values()
	for _, v := range values {
		if attrs, ok := v.(*ast.Array); ok {
			r.groupAttributes(&own, attrs, outer.nest(own).namespace)
		}
	}
	inner := outer.nest(own)
	for _, v := range values {
		switch fn := v.(type) {
		case *ast.Literal, *ast.Binary, *ast.MagicConst, *ast.Call:
			r.include(fn, inner)
		case *ast.Closure:
			if fn.Body != nil {
				r.node(fn.Body, inner)
			}
		case *ast.ArrowFunction:
			if fn.Body != nil {
				r.node(fn.Body, inner)
			}
		}
	}
}

func (r *routeReader) groupAttributes(g *routeGroup, attrs *ast.Array, namespace string) {
	for _, item := range attrs.Items {
		key, _, ok := r.str(item.Key)
		if !ok || key == "name" {
			continue
		}
		if attr, ok := registrarAttrs[key]; ok {
			r.assign(g, attr, item.Value, namespace)
		}
	}
}

func (r *routeReader) route(verb routeCall, mods []routeCall, g routeGroup) {
	at := 0
	if verb.name == "match" {
		at = 1
	}
	route := RouteName{URI: r.file.uri}
	if path, _, ok := r.str(verb.arg(at)); ok && g.unknown&routePrefix == 0 {
		route.Path = "/" + joinRoutePath(g.prefix, path)
	}

	action := verb.arg(at + 1)
	name, named, known := g.as, false, g.unknown&routeAs == 0
	var nameSpan ast.Span
	addName := func(e ast.Expr) {
		named = true
		part, span, ok := r.str(e)
		if !ok {
			known = false
			return
		}
		name += part
		nameSpan = span
	}
	if arr, ok := action.(*ast.Array); ok {
		if as, ok := r.keyed(arr, "as"); ok {
			addName(as)
		}
		if uses, ok := r.keyed(arr, "uses"); ok {
			action = uses
		}
	}
	for _, m := range mods {
		if m.name == "name" {
			addName(m.arg(0))
		}
	}
	if !named || !known || name == "" {
		return
	}
	route.Name = name
	route.Range = r.literalRange(nameSpan)
	route.TargetKind, route.Target, route.HandlerFQN, route.HandlerMethod = r.target(verb.name, action, g)
	r.session.routes = append(r.session.routes, route)
}

func (r *routeReader) target(method string, action ast.Expr, g routeGroup) (kind, target, handlerFQN, handlerMethod string) {
	switch method {
	case "view", "livewire", "redirect":
		value, _, _ := r.str(action)
		return method, value, "", ""
	}
	if action == nil {
		return "route", "", "", ""
	}
	if className, methodName, ok := r.controllerPair(action); ok {
		return "controller", className + "@" + methodName, className, methodName
	}
	if value, _, ok := r.str(action); ok {
		if className, methodName, ok := strings.Cut(value, "@"); ok {
			className = r.classString(className, g.namespace)
			return "controller", className + "@" + methodName, className, methodName
		}
		if g.controller != "" {
			return "controller", g.controller + "@" + value, g.controller, value
		}
		return "string", value, "", ""
	}
	if className, ok := r.classConst(action); ok {
		return "controller", className + "@__invoke", className, "__invoke"
	}
	return "route", r.text(action), "", ""
}

func (r *routeReader) controllerPair(e ast.Expr) (className, methodName string, ok bool) {
	arr, ok := e.(*ast.Array)
	if !ok || len(arr.Items) != 2 || arr.Items[0].Key != nil || arr.Items[1].Key != nil {
		return "", "", false
	}
	className, ok = r.classConst(arr.Items[0].Value)
	if !ok {
		return "", "", false
	}
	methodName, _, ok = r.str(arr.Items[1].Value)
	if !ok || methodName == "" {
		return "", "", false
	}
	return className, methodName, true
}

func (r *routeReader) controllerClass(e ast.Expr, namespace string) (string, bool) {
	if className, ok := r.classConst(e); ok {
		return className, true
	}
	if value, _, ok := r.str(e); ok && value != "" {
		return r.classString(value, namespace), true
	}
	return "", false
}

func (r *routeReader) classConst(e ast.Expr) (string, bool) {
	fetch, ok := e.(*ast.ClassConstFetch)
	if !ok {
		return "", false
	}
	name, ok := fetch.Name.(*ast.IdentifierExpr)
	if !ok || !strings.EqualFold(r.text(name), "class") {
		return "", false
	}
	class, ok := fetch.Class.(*ast.ConstFetch)
	if !ok || class.Name == nil || class.Name.Kind == token.Static {
		return "", false
	}
	return r.className(r.text(class.Name)), true
}

func (r *routeReader) className(name string) string {
	if full, ok := strings.CutPrefix(name, `\`); ok {
		return full
	}
	first, rest, qualified := strings.Cut(name, `\`)
	if full, ok := r.imports[strings.ToLower(first)]; ok {
		if qualified {
			return full + `\` + rest
		}
		return full
	}
	if r.namespace != "" {
		return r.namespace + `\` + name
	}
	return name
}

func (r *routeReader) classString(name, namespace string) string {
	if full, ok := strings.CutPrefix(name, `\`); ok {
		return full
	}
	if namespace != "" {
		return nestNamespace(namespace, name)
	}
	if strings.Contains(name, `\`) {
		return name
	}
	return r.className(name)
}

func (r *routeReader) keyed(arr *ast.Array, key string) (ast.Expr, bool) {
	for _, item := range arr.Items {
		if k, _, ok := r.str(item.Key); ok && k == key {
			return item.Value, true
		}
	}
	return nil, false
}

func (r *routeReader) str(e ast.Expr) (string, ast.Span, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != token.ConstantString {
		return "", ast.Span{}, false
	}
	value, ok := lexer.Unquote(r.text(lit))
	return value, lit.Span, ok
}

func (r *routeReader) text(n ast.Node) string {
	return n.Bounds().Text(r.file.src)
}

func (r *routeReader) literalRange(s ast.Span) textpos.Range {
	startLine, startCol := r.file.lines.Position(s.Start + 1)
	endLine, endCol := r.file.lines.Position(s.End - 1)
	return textpos.Range{
		Start: textpos.Position{Line: startLine, Character: startCol},
		End:   textpos.Position{Line: endLine, Character: endCol},
	}
}

func lastNameSegment(name string) string {
	return name[strings.LastIndexByte(name, '\\')+1:]
}
