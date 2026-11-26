package laravel

import (
	"slices"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

const (
	viewHelper   = "view"
	configHelper = "config"
)

var (
	routeNameFunctions       = []string{"route", "to_route"}
	routeNameMethods         = []string{"route", "routeIs"}
	translationFunctions     = []string{"__", "trans", "trans_choice"}
	translationFacadeMethods = []string{"get", "choice"}
)

const translationFacade = "Lang"

type ConfigCall struct {
	Repository bool
	Key        string
}

func ParseConfigCall(src string, e ast.Expr) (ConfigCall, bool) {
	call, ok := e.(*ast.Call)
	if !ok || ast.FunctionName(src, call) != configHelper {
		return ConfigCall{}, false
	}
	if len(call.Args.Args) == 0 {
		return ConfigCall{Repository: true}, true
	}
	key, _ := ast.StringValue(src, call.Args.Args[0].Value)
	return ConfigCall{Key: key}, true
}

func IsConfigKeyArg(arg ast.CallArg) bool {
	return arg.Function == configHelper && arg.Index == 0 && !arg.InArray
}

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
