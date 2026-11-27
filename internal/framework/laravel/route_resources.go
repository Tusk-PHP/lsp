package laravel

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

type resourceSpec struct {
	actions   []string
	only      []string
	singleton bool
}

var (
	resourceActions  = []string{"index", "create", "store", "show", "edit", "update", "destroy"}
	singletonActions = []string{"show", "edit", "update"}
)

var resourceRegistrations = map[string]resourceSpec{
	"resource":     {actions: resourceActions},
	"apiResource":  {actions: resourceActions, only: []string{"index", "show", "store", "update", "destroy"}},
	"singleton":    {actions: singletonActions, singleton: true},
	"apiSingleton": {actions: singletonActions, only: []string{"store", "show", "update", "destroy"}, singleton: true},
}

var resourceMaps = map[string]resourceSpec{
	"resources":     resourceRegistrations["resource"],
	"apiResources":  resourceRegistrations["apiResource"],
	"singletons":    resourceRegistrations["singleton"],
	"apiSingletons": resourceRegistrations["apiSingleton"],
}

type resourceName struct {
	value string
	span  ast.Span
}

type resourceOptions struct {
	only        []string
	onlySet     bool
	except      []string
	names       string
	actionNames map[string]resourceName
	as          string
	shallow     bool
	creatable   bool
	destroyable bool
	unknown     bool
}

func (o resourceOptions) actions(spec resourceSpec) []string {
	defaults := spec.actions
	if spec.singleton {
		switch {
		case o.creatable:
			defaults = append(slices.Clip(defaults), "create", "store", "destroy")
		case o.destroyable:
			defaults = append(slices.Clip(defaults), "destroy")
		}
	}
	only, restricted := spec.only, spec.only != nil
	if o.onlySet {
		only, restricted = o.only, true
	}
	actions := make([]string, 0, len(defaults))
	for _, action := range defaults {
		if restricted && !slices.Contains(only, action) || slices.Contains(o.except, action) {
			continue
		}
		actions = append(actions, action)
	}
	return actions
}

func (o resourceOptions) routeName(resource, action string, singleton bool) string {
	base := resource
	if o.shallow && !singleton && isResourceMemberAction(action) {
		base = base[strings.LastIndexByte(base, '.')+1:]
	}
	if o.names != "" {
		base = o.names
	}
	prefix := ""
	if o.as != "" {
		prefix = o.as + "."
	}
	return strings.Trim(prefix+base+"."+action, ".")
}

func isResourceMemberAction(action string) bool {
	switch action {
	case "show", "edit", "update", "destroy":
		return true
	}
	return false
}

func resourcePath(base, resource, action string, singleton bool) string {
	if strings.Contains(resource, ".") {
		return ""
	}
	uri := "/" + joinRoutePath(base, resource)
	switch {
	case action == "create":
		return uri + "/create"
	case action == "edit" && singleton:
		return uri + "/edit"
	case action == "index" || action == "store" || singleton:
		return uri
	}
	return ""
}

func (r *routeReader) resource(spec resourceSpec, c routeCall, mods []routeCall, g routeGroup) {
	resource, span, ok := r.str(c.arg(0))
	if !ok || resource == "" {
		return
	}
	var opts resourceOptions
	r.resourceOptionArray(&opts, c.arg(2))
	for _, m := range mods {
		if m.name == "name" {
			r.resourceActionName(&opts, m.arg(0), m.arg(1))
			continue
		}
		r.resourceOption(&opts, m.name, m.values())
	}
	r.expandResource(spec, resourceName{resource, span}, c.arg(1), opts, g)
}

func (r *routeReader) resourceMap(spec resourceSpec, c routeCall, g routeGroup) {
	resources, ok := c.arg(0).(*ast.Array)
	if !ok {
		return
	}
	var opts resourceOptions
	r.resourceOptionArray(&opts, c.arg(1))
	for _, item := range resources.Items {
		resource, span, ok := r.str(item.Key)
		if !ok || resource == "" {
			continue
		}
		r.expandResource(spec, resourceName{resource, span}, item.Value, opts, g)
	}
}

func (r *routeReader) resourceOptionArray(o *resourceOptions, e ast.Expr) {
	if e == nil {
		return
	}
	arr, ok := e.(*ast.Array)
	if !ok {
		o.unknown = true
		return
	}
	for _, item := range arr.Items {
		key, _, ok := r.str(item.Key)
		if !ok {
			o.unknown = true
			continue
		}
		r.resourceOption(o, key, []ast.Expr{item.Value})
	}
}

func (r *routeReader) resourceOption(o *resourceOptions, key string, args []ast.Expr) {
	switch key {
	case "only":
		o.only, o.onlySet = r.resourceActionList(o, args), true
	case "except":
		o.except = r.resourceActionList(o, args)
	case "names":
		r.resourceNames(o, args)
	case "as":
		as, ok := r.singleString(args)
		o.as, o.unknown = as, o.unknown || !ok
	case "shallow":
		o.shallow = r.flag(args)
	case "creatable":
		o.creatable = r.flag(args)
	case "destroyable":
		o.destroyable = r.flag(args)
	}
}

func (r *routeReader) resourceActionList(o *resourceOptions, args []ast.Expr) []string {
	var actions []string
	for _, arg := range args {
		if arr, ok := arg.(*ast.Array); ok {
			for _, item := range arr.Items {
				actions = r.appendAction(o, actions, item.Value)
			}
			continue
		}
		actions = r.appendAction(o, actions, arg)
	}
	return actions
}

func (r *routeReader) appendAction(o *resourceOptions, actions []string, e ast.Expr) []string {
	action, _, ok := r.str(e)
	if !ok {
		o.unknown = true
		return actions
	}
	return append(actions, action)
}

func (r *routeReader) resourceNames(o *resourceOptions, args []ast.Expr) {
	if len(args) != 1 {
		o.unknown = true
		return
	}
	if names, ok := args[0].(*ast.Array); ok {
		for _, item := range names.Items {
			r.resourceActionName(o, item.Key, item.Value)
		}
		return
	}
	names, _, ok := r.str(args[0])
	o.names, o.unknown = names, o.unknown || !ok
}

func (r *routeReader) resourceActionName(o *resourceOptions, actionExpr, nameExpr ast.Expr) {
	action, _, ok := r.str(actionExpr)
	name, span, nameOK := r.str(nameExpr)
	if !ok || !nameOK {
		o.unknown = true
		return
	}
	if o.actionNames == nil {
		o.actionNames = make(map[string]resourceName)
	}
	o.actionNames[action] = resourceName{name, span}
}

func (r *routeReader) singleString(args []ast.Expr) (string, bool) {
	if len(args) != 1 {
		return "", false
	}
	value, _, ok := r.str(args[0])
	return value, ok
}

func (r *routeReader) flag(args []ast.Expr) bool {
	if len(args) == 0 {
		return true
	}
	c, ok := args[0].(*ast.ConstFetch)
	return !ok || c.Name == nil || !strings.EqualFold(r.text(c.Name), "false")
}

func (r *routeReader) expandResource(spec resourceSpec, resource resourceName, controller ast.Expr, opts resourceOptions, g routeGroup) {
	if opts.unknown || g.unknown&routeAs != 0 {
		return
	}
	uriPrefix, name := "", resource.value
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		uriPrefix, name = name[:i], name[i+1:]
	}
	fqn, hasController := r.controllerClass(controller, g.namespace)
	for _, action := range opts.actions(spec) {
		route := RouteName{
			Name:       g.as + opts.routeName(name, action, spec.singleton),
			URI:        r.uri,
			Range:      r.literalRange(resource.span),
			TargetKind: "route",
		}
		if own, ok := opts.actionNames[action]; ok {
			route.Name = g.as + own.value
			route.Range = r.literalRange(own.span)
		}
		if g.unknown&routePrefix == 0 {
			route.Path = resourcePath(joinRoutePath(g.prefix, uriPrefix), name, action, spec.singleton)
		}
		if hasController {
			route.TargetKind, route.Target = "controller", fqn+"@"+action
			route.HandlerFQN, route.HandlerMethod = fqn, action
		}
		r.routes = append(r.routes, route)
	}
}
