package laravel

import (
	"slices"

	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
)

const viewHelper = "view"

var (
	routeNameFunctions       = []string{"route", "to_route"}
	routeNameMethods         = []string{"route", "routeIs"}
	translationFunctions     = []string{"__", "trans", "trans_choice"}
	translationFacadeMethods = []string{"get", "choice"}
)

const translationFacade = "Lang"

func IsRouteNameArg(arg sourcectx.CallArg) bool {
	if arg.Index != 0 || arg.InArray {
		return false
	}
	return slices.Contains(routeNameFunctions, arg.Function) || slices.Contains(routeNameMethods, arg.Method)
}

func ViewNameArg(arg sourcectx.CallArg) (*ViewContext, bool) {
	if arg.Value != sourcectx.ArgString || arg.InArray {
		return nil, false
	}
	if arg.Function == viewHelper && arg.Index == 0 || arg.Method == viewHelper && arg.Index == 1 {
		return &ViewContext{Name: arg.Text, Partial: arg.Partial, Quote: arg.Quote}, true
	}
	return nil, false
}

func TranslationKeyArg(arg sourcectx.CallArg) (*TranslationStringContext, bool) {
	if arg.Index != 0 || arg.InArray || arg.Value != sourcectx.ArgString {
		return nil, false
	}
	if !slices.Contains(translationFunctions, arg.Function) &&
		!(arg.Static && sourcectx.LastSegment(arg.Class) == translationFacade && slices.Contains(translationFacadeMethods, arg.Method)) {
		return nil, false
	}
	return &TranslationStringContext{Key: arg.Text, Partial: arg.Partial, Quote: arg.Quote}, true
}
