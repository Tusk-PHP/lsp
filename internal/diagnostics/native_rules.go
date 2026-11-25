package diagnostics

import (
	"github.com/Tusk-PHP/lsp/internal/checks"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

type NativeRulesOptions struct {
	TypeResolver           func(expr string, source string, line, col int, file *parser.FileNode) string
	BuiltinUnavailableRule *checks.BuiltinUnavailableRule
	BuilderModelResolver   checks.ModelResolverFunc
	BuilderMemberChecker   checks.MemberChecker
	BladePHP               bool
	BladeControlFlow       bool
	IncludedFile           bool
}

func EditRules(opts NativeRulesOptions, shared *checks.Shared) []checks.Rule {
	typeResolver := MemoizeTypeResolver(opts.TypeResolver)
	uses := &checks.MemberUseCache{Shared: shared}
	deprecatedFunctions := &checks.DeprecatedFunctionRule{Shared: shared}
	if opts.BuiltinUnavailableRule != nil {
		deprecatedFunctions.PHPVersion = opts.BuiltinUnavailableRule.PHPVersion
	}
	rules := []checks.Rule{
		&checks.SyntaxErrorRule{},
		&checks.AbstractInConcreteRule{},
		&checks.UnknownClassRule{Shared: shared},
		&checks.UnknownFunctionRule{Shared: shared},
	}
	if opts.TypeResolver != nil {
		rules = append(rules, &checks.UnknownMemberRule{TypeResolver: typeResolver, ConventionMethod: frameworklaravel.ConventionMethod, Shared: shared})
	}
	if opts.BuiltinUnavailableRule != nil {
		builtins := *opts.BuiltinUnavailableRule
		builtins.Shared = shared
		rules = append(rules, &builtins)
	}
	rules = append(rules,
		&checks.UnusedImportsRule{},
		&checks.UnusedPrivateRule{Shared: shared},
		&checks.UnreachableCodeRule{Shared: shared},
		&checks.RedundantUnionRule{},
		&checks.UndefinedVariableRule{IncludedFile: opts.IncludedFile, Shared: shared},
	)
	if !opts.BladePHP {
		rules = append(rules, &checks.UnusedVariableRule{Shared: shared})
	}
	if !opts.BladePHP || opts.BladeControlFlow {
		rules = append(rules, &checks.CompileErrorRule{Shared: shared, View: opts.BladePHP})
	}
	return append(rules,
		&checks.DeprecatedUsageRule{TypeResolver: typeResolver, Uses: uses},
		deprecatedFunctions,
		&checks.UnusedResultRule{TypeResolver: typeResolver, Uses: uses},
		&checks.InvalidOverrideRule{},
		&checks.ArgumentCountRule{Shared: shared, TypeResolver: typeResolver},
		&checks.UnknownNamedArgumentRule{Shared: shared, TypeResolver: typeResolver},
	)
}

func SaveRules(opts NativeRulesOptions) []checks.Rule {
	var rules []checks.Rule
	if opts.TypeResolver != nil {
		rules = append(rules, &checks.RedundantNullsafeRule{TypeResolver: opts.TypeResolver})
	}
	if opts.BuilderModelResolver != nil && opts.BuilderMemberChecker != nil {
		rules = append(rules, &checks.InvalidBuilderArgRule{ModelResolver: opts.BuilderModelResolver, Members: opts.BuilderMemberChecker, Shared: &checks.Shared{}})
	}
	return rules
}

func NativeRules(opts NativeRulesOptions) []checks.Rule {
	return append(EditRules(opts, &checks.Shared{}), SaveRules(opts)...)
}
