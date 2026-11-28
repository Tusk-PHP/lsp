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

const (
	translationFunction  = "t"
	translationMethod    = "trans"
	translatableMessage  = "TranslatableMessage"
	translationDomainArg = 2
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

func TranslationKeyArg(arg ast.CallArg) (*TranslationContext, bool) {
	if arg.Index != 0 || arg.InArray || arg.Value != ast.ArgString {
		return nil, false
	}
	switch {
	case arg.New:
		if ast.LastSegment(arg.Class) != translatableMessage {
			return nil, false
		}
	case arg.Receiver != nil:
		if arg.Static || arg.Method != translationMethod {
			return nil, false
		}
	case ast.LastSegment(arg.Function) != translationFunction:
		return nil, false
	}
	domain, _ := arg.StringArg(translationDomainArg)
	return &TranslationContext{Key: arg.Text, Partial: arg.Partial, Domain: domain, Quote: arg.Quote}, true
}
