package laravel

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// AuthNameKind says which names a string argument takes.
type AuthNameKind uint8

const (
	AbilityName AuthNameKind = iota + 1
	MiddlewareName
)

// AuthName is a Gate ability or middleware name under the cursor. Name is
// the whole name the string holds and Partial what is typed of it before the
// cursor; in `'can:update,post'` the ability is `update`. Model is the
// argument an ability is checked against, when the call passes one.
type AuthName struct {
	Kind    AuthNameKind
	Name    string
	Partial string
	Quote   string
	Model   ast.Expr
}

type calleeShape uint8

const (
	onInstance calleeShape = iota
	onStatic
	onNew
)

type authCallee struct {
	class   string
	shape   calleeShape
	methods []string
	arg     int
	kind    AuthNameKind
}

const (
	gateContract          = `Illuminate\Contracts\Auth\Access\Gate`
	routeFacade           = `Illuminate\Support\Facades\Route`
	authorizesRequests    = `Illuminate\Foundation\Auth\Access\AuthorizesRequests`
	canMiddleware         = "can"
	middlewareParamsStart = ":"
	middlewareParamsSep   = ","
)

var (
	gateCheckMethods  = []string{"allows", "denies", "check", "any", "authorize", "inspect", "has", "raw"}
	routeMiddleware   = []string{"middleware", "withoutMiddleware"}
	routeAbility      = []string{"can"}
	routeAttributeOns = []string{`Illuminate\Routing\Route`, `Illuminate\Routing\RouteRegistrar`}
)

var authCallees = func() []authCallee {
	callees := []authCallee{
		{class: gateContract, methods: gateCheckMethods, kind: AbilityName},
		{class: `Illuminate\Auth\Access\Gate`, methods: []string{"none"}, kind: AbilityName},
		{class: gateFacade, shape: onStatic, methods: append(slices.Clone(gateCheckMethods), "none"), kind: AbilityName},
		{class: `Illuminate\Contracts\Auth\Access\Authorizable`, methods: []string{"can"}, kind: AbilityName},
		{class: `Illuminate\Foundation\Auth\Access\Authorizable`, methods: []string{"can", "canAny", "cant", "cannot"}, kind: AbilityName},
		{class: authorizesRequests, methods: []string{"authorize"}, kind: AbilityName},
		{class: authorizesRequests, methods: []string{"authorizeForUser"}, arg: 1, kind: AbilityName},
		{class: routeFacade, shape: onStatic, methods: routeAbility, kind: AbilityName},
		{class: routeFacade, shape: onStatic, methods: routeMiddleware, kind: MiddlewareName},
		{class: `Illuminate\Routing\Controller`, methods: []string{"middleware"}, kind: MiddlewareName},
		{class: `Illuminate\Routing\Controllers\Middleware`, shape: onNew, kind: MiddlewareName},
	}
	for _, class := range routeAttributeOns {
		callees = append(callees,
			authCallee{class: class, methods: routeAbility, kind: AbilityName},
			authCallee{class: class, methods: routeMiddleware, kind: MiddlewareName})
	}
	return callees
}()

// AuthNameArg reports whether arg is a Gate ability or middleware name: an
// argument of a Gate check, of can() on a user or a route, of a controller's
// authorize(), or of middleware() and withoutMiddleware() on a route or a
// controller. The receiver is typed with r only when the method's name is one
// of those.
func AuthNameArg(r *resolve.Resolver, index *symbols.Index, file *parser.FileNode, arg ast.CallArg) (AuthName, bool) {
	if arg.Value == ast.ArgName {
		return AuthName{}, false
	}
	var receivers []string
	typed := false
	for _, callee := range authCallees {
		if !callee.matches(arg) {
			continue
		}
		if !typed {
			receivers, typed = calleeClasses(r, file, arg), true
		}
		if slices.ContainsFunc(receivers, func(class string) bool { return index.IsSubtypeOf(class, callee.class) }) {
			return authName(callee.kind, arg)
		}
	}
	return AuthName{}, false
}

func (c authCallee) matches(arg ast.CallArg) bool {
	if arg.Index != c.arg {
		return false
	}
	switch c.shape {
	case onNew:
		return arg.New
	case onStatic:
		if !arg.Static {
			return false
		}
	default:
		if arg.Static || arg.New || arg.Receiver == nil {
			return false
		}
	}
	return slices.ContainsFunc(c.methods, func(m string) bool { return strings.EqualFold(m, arg.Method) })
}

func calleeClasses(r *resolve.Resolver, file *parser.FileNode, arg ast.CallArg) []string {
	if arg.Static || arg.New {
		if arg.Class == "" {
			return nil
		}
		return []string{r.ResolveClassName(arg.Class, file)}
	}
	t := r.NodeType(file, arg.Receiver)
	if len(t.Union) == 0 {
		return []string{t.FQN}
	}
	classes := make([]string, 0, len(t.Union))
	for _, part := range t.Union {
		classes = append(classes, part.FQN)
	}
	return classes
}

func authName(kind AuthNameKind, arg ast.CallArg) (AuthName, bool) {
	name := AuthName{Kind: kind, Name: arg.Text, Partial: arg.Partial, Quote: arg.Quote}
	if kind == AbilityName {
		name.Model = arg.Arg(arg.Index + 1)
		return name, true
	}
	alias, params, hasParams := strings.Cut(arg.Text, middlewareParamsStart)
	if !hasParams || len(arg.Partial) <= len(alias) {
		name.Name = alias
		return name, true
	}
	typed := arg.Partial[len(alias)+len(middlewareParamsStart):]
	if alias != canMiddleware || strings.Contains(typed, middlewareParamsSep) {
		return AuthName{}, false
	}
	ability, _, _ := strings.Cut(params, middlewareParamsSep)
	return AuthName{Kind: AbilityName, Name: ability, Partial: typed, Quote: arg.Quote}, true
}

// AbilityOf returns the definition of the ability name holds, preferring the
// policy of the model it is checked against.
func (a *Authorization) AbilityOf(r *resolve.Resolver, file *parser.FileNode, name AuthName) (Ability, bool) {
	return a.AbilityFor(name.Name, modelClass(r, file, name.Model))
}

func modelClass(r *resolve.Resolver, file *parser.FileNode, e ast.Expr) string {
	if e == nil || ast.IsMissing(e) {
		return ""
	}
	if fetch, ok := e.(*ast.ClassConstFetch); ok {
		class, isName := fetch.Class.(*ast.ConstFetch)
		if !isName || !strings.EqualFold(fetch.Name.Bounds().Text(file.Source), "class") {
			return ""
		}
		return r.ResolveClassName(class.Bounds().Text(file.Source), file)
	}
	return resolve.PlainTypeName(r.NodeType(file, e))
}
