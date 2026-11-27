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

const (
	translationFunction  = "t"
	translationMethod    = "trans"
	translatableMessage  = "TranslatableMessage"
	translationDomainArg = 2
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

func TranslationKeyArg(arg sourcectx.CallArg) (*TranslationContext, bool) {
	if arg.Index != 0 || arg.InArray || arg.Value != sourcectx.ArgString {
		return nil, false
	}
	switch {
	case arg.New:
		if sourcectx.LastSegment(arg.Class) != translatableMessage {
			return nil, false
		}
	case arg.Receiver != nil:
		if arg.Static || arg.Method != translationMethod {
			return nil, false
		}
	case sourcectx.LastSegment(arg.Function) != translationFunction:
		return nil, false
	}
	domain, _ := arg.StringArg(translationDomainArg)
	return &TranslationContext{Key: arg.Text, Partial: arg.Partial, Domain: domain, Quote: arg.Quote}, true
}
