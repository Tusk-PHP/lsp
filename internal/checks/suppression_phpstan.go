package checks

import "github.com/Tusk-PHP/lsp/internal/rule"

var phpstanIdentifierCodes = map[string]rule.Code{
	"class.notFound":                      rule.UnknownClass,
	"function.notFound":                   rule.UnknownFunction,
	"method.notFound":                     rule.UnknownMember,
	"staticMethod.notFound":               rule.UnknownMember,
	"property.notFound":                   rule.UnknownMember,
	"staticProperty.notFound":             rule.UnknownMember,
	"classConstant.notFound":              rule.UnknownMember,
	"variable.undefined":                  rule.UndefinedVariable,
	"variable.unused":                     rule.UnusedVariable,
	"arguments.count":                     rule.ArgumentCountMismatch,
	"deadCode.unreachable":                rule.UnreachableCode,
	"method.unused":                       rule.UnusedPrivateMethod,
	"property.unused":                     rule.UnusedPrivateProperty,
	"property.onlyWritten":                rule.UnusedPrivateProperty,
	"nullsafe.neverNull":                  rule.RedundantNullsafe,
	"method.override":                     rule.InvalidOverride,
	"property.override":                   rule.InvalidOverride,
	"classConstant.override":              rule.InvalidOverride,
	"function.deprecated":                 rule.Deprecated,
	"method.deprecated":                   rule.Deprecated,
	"staticMethod.deprecated":             rule.Deprecated,
	"staticProperty.deprecated":           rule.Deprecated,
	"classConstant.deprecated":            rule.Deprecated,
	"constant.deprecated":                 rule.Deprecated,
	"method.deprecatedClass":              rule.Deprecated,
	"staticMethod.deprecatedClass":        rule.Deprecated,
	"staticProperty.deprecatedClass":      rule.Deprecated,
	"classConstant.deprecatedClass":       rule.Deprecated,
	"new.deprecatedClass":                 rule.Deprecated,
	"instanceof.deprecatedClass":          rule.Deprecated,
	"catch.deprecatedClass":               rule.Deprecated,
	"argument.unknown":                    rule.UnknownNamedArgument,
	"class.extendsDeprecatedClass":        rule.Deprecated,
	"class.implementsDeprecatedInterface": rule.Deprecated,
}

func phpstanTargets(identifiers []string) []rule.Target {
	targets := make([]rule.Target, 0, 2*len(identifiers))
	for _, id := range identifiers {
		targets = append(targets, rule.ExternalTarget(rule.SourcePHPStan, id))
		if native, ok := phpstanIdentifierCodes[id]; ok {
			targets = append(targets, rule.NativeTarget(native))
		}
	}
	return targets
}
