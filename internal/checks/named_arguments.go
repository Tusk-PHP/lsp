package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type UnknownNamedArgumentRule struct {
	TypeResolver TypeResolver
	Shared       *Shared
}

func (r *UnknownNamedArgumentRule) Codes() []rule.Code {
	return []rule.Code{rule.UnknownNamedArgument}
}

func (r *UnknownNamedArgumentRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || index == nil {
		return nil
	}
	var findings []Finding
	syntax := r.Shared.tree(source, file)
	eachResolvedCall(syntax, file, source, index, r.TypeResolver, hasNamedArguments, func(call resolvedCall) {
		if !allCheckable(call.callees) {
			return
		}
		callName := syntax.text(call.name)
		for _, arg := range call.args.Args {
			if arg.Name == nil {
				continue
			}
			name := syntax.text(arg.Name)
			if anyHasParam(call.callees, name) {
				continue
			}
			findings = append(findings, syntax.at(arg.Name, Finding{
				Severity: rule.SeverityError,
				Code:     rule.UnknownNamedArgument,
				Message:  fmt.Sprintf("Unknown named argument '%s' to %s()", name, callName),
			}))
		}
	})
	return findings
}

func anyHasParam(callees []callee, name string) bool {
	for _, c := range callees {
		for _, p := range c.sym.Params {
			if strings.TrimPrefix(p.Name, "$") == name {
				return true
			}
		}
	}
	return false
}

func hasNamedArguments(args *ast.ArgList) bool {
	named := false
	for _, a := range args.Args {
		if a.Unpack {
			return false
		}
		named = named || a.Name != nil
	}
	return named && !args.RestPlaceholder
}
