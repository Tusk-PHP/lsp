package diagnostics

import (
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

// NativeRulesOptions carries the optional resolver dependencies used by rules
// that require cross-file context. Nil fields cause those rules to be omitted
// (matching what server.go and cmd/tusk-php already do) so callers that lack
// certain resolvers at call time still get a consistent, usable rule set.
type NativeRulesOptions struct {
	// TypeResolver resolves the type of a PHP expression at a given source line.
	// Required by UnknownMemberRule and RedundantNullsafeRule; omit both when nil.
	TypeResolver func(expr string, source string, line int, file *parser.FileNode) string

	// BuiltinUnavailableRule is the pre-built rule that reports usages of PHP
	// built-ins not available in the resolved PHP profile. Omitted when nil.
	BuiltinUnavailableRule *checks.BuiltinUnavailableRule

	// BuilderModelResolver and BuilderMemberChecker are required together for
	// InvalidBuilderArgRule. Omitted when either is nil.
	BuilderModelResolver checks.ModelResolverFunc
	BuilderMemberChecker checks.MemberChecker
}

// NativeRules returns the ordered list of native (non-external) static analysis
// rules for a given set of optional resolver dependencies. The list is the
// canonical one used by the LSP server's debugDocument command, the CLI
// introspect command, and — at the rule-set level — by the diagnostics provider.
//
// Rules are always returned in a deterministic order: parameterless rules first,
// followed by dependency-requiring rules (UnknownMember, BuiltinUnavailable,
// RedundantNullsafe, InvalidBuilderArg) only when the required deps are present.
//
// NOTE: the diagnostics provider (provider.go/Analyze+AnalyzeOnSave) runs rules
// individually rather than through this list so it can apply per-rule severity
// overrides and enable/disable flags. Callers that need per-rule config filtering
// should continue to use the provider directly; this constructor is for the
// "run all native rules, no per-rule config" path (CLI and debugDocument).
func NativeRules(opts NativeRulesOptions) []checks.Rule {
	rules := []checks.Rule{
		&checks.UnknownClassRule{},
		&checks.UnknownFunctionRule{},
		&checks.UnusedImportsRule{},
		&checks.UnusedPrivateRule{},
		&checks.UnreachableCodeRule{},
		&checks.RedundantUnionRule{},
		&checks.UndefinedVariableRule{},
		&checks.UnusedVariableRule{},
		&checks.ArgumentCountRule{},
	}
	// Rules that require optional deps follow in a stable order.
	if opts.TypeResolver != nil {
		rules = append(rules, &checks.UnknownMemberRule{TypeResolver: opts.TypeResolver})
	}
	if opts.BuiltinUnavailableRule != nil {
		rules = append(rules, opts.BuiltinUnavailableRule)
	}
	if opts.TypeResolver != nil {
		rules = append(rules, &checks.RedundantNullsafeRule{TypeResolver: opts.TypeResolver})
	}
	if opts.BuilderModelResolver != nil && opts.BuilderMemberChecker != nil {
		rules = append(rules, &checks.InvalidBuilderArgRule{
			ModelResolver: opts.BuilderModelResolver,
			Members:       opts.BuilderMemberChecker,
		})
	}
	return rules
}
