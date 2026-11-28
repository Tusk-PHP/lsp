package symfony

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

var (
	routeNameCalls   = []string{"generateUrl", "redirectToRoute"}
	routeNameMethods = []string{"generate"}
)

func IsRouteNameArg(arg ast.CallArg) bool {
	if arg.Index != 0 || arg.InArray {
		return false
	}
	return slices.Contains(routeNameCalls, arg.Function) || slices.Contains(routeNameCalls, arg.Method) ||
		!arg.Static && slices.Contains(routeNameMethods, arg.Method)
}

func IsCommandNameArg(arg ast.CallArg) bool {
	if arg.Index != 0 || arg.InArray || arg.Static || !slices.Contains(commandNameMethods, arg.Method) {
		return false
	}
	switch arg.Value {
	case ast.ArgEmpty:
		return true
	case ast.ArgString:
		return !strings.ContainsAny(arg.Partial, `\$`)
	}
	return false
}
