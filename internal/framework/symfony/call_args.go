package symfony

import (
	"slices"
	"strings"

	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
)

var (
	routeNameCalls   = []string{"generateUrl", "redirectToRoute"}
	routeNameMethods = []string{"generate"}
)

func IsRouteNameArg(arg sourcectx.CallArg) bool {
	if arg.Index != 0 || arg.InArray {
		return false
	}
	return slices.Contains(routeNameCalls, arg.Function) || slices.Contains(routeNameCalls, arg.Method) ||
		!arg.Static && slices.Contains(routeNameMethods, arg.Method)
}

func IsCommandNameArg(arg sourcectx.CallArg) bool {
	if arg.Index != 0 || arg.InArray || arg.Static || !slices.Contains(commandNameMethods, arg.Method) {
		return false
	}
	switch arg.Value {
	case sourcectx.ArgEmpty:
		return true
	case sourcectx.ArgString:
		return !strings.ContainsAny(arg.Partial, `\$`)
	}
	return false
}
