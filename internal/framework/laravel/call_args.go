package laravel

import (
	"slices"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

const viewHelper = "view"

var (
	routeNameFunctions       = []string{"route", "to_route"}
	routeNameMethods         = []string{"route", "routeIs"}
	translationFunctions     = []string{"__", "trans", "trans_choice"}
	translationFacadeMethods = []string{"get", "choice"}
)

const translationFacade = "Lang"

func IsRouteNameArg(arg ast.CallArg) bool {
	if arg.Index != 0 || arg.InArray {
		return false
	}
	return slices.Contains(routeNameFunctions, arg.Function) || slices.Contains(routeNameMethods, arg.Method)
}

func ViewNameArg(arg ast.CallArg) (*ViewContext, bool) {
	if arg.Value != ast.ArgString || arg.InArray {
		return nil, false
	}
	if arg.Function == viewHelper && arg.Index == 0 || arg.Method == viewHelper && arg.Index == 1 {
		return &ViewContext{Name: arg.Text, Partial: arg.Partial, Quote: arg.Quote}, true
	}
	return nil, false
}

func TranslationKeyArg(arg ast.CallArg) (*TranslationStringContext, bool) {
	if arg.Index != 0 || arg.InArray || arg.Value != ast.ArgString {
		return nil, false
	}
	if !slices.Contains(translationFunctions, arg.Function) &&
		!(arg.Static && ast.LastSegment(arg.Class) == translationFacade && slices.Contains(translationFacadeMethods, arg.Method)) {
		return nil, false
	}
	return &TranslationStringContext{Key: arg.Text, Partial: arg.Partial, Quote: arg.Quote}, true
}
