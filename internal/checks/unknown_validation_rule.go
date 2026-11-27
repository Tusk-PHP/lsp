package checks

import (
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// UnknownValidationRuleRule reports a name in a Laravel validation rule
// string that is neither a rule of the installed framework nor one the
// app's providers register, such as 'requierd|email'. It says nothing
// unless every rule the app can use is known.
type UnknownValidationRuleRule struct {
	Rules        *frameworklaravel.ValidationRules
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string
}

func (r *UnknownValidationRuleRule) Codes() []rule.Code {
	return []rule.Code{rule.UnknownValidationRule}
}

func (r *UnknownValidationRuleRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if r.Rules == nil || r.TypeResolver == nil || index == nil || file == nil || !r.Rules.Complete() {
		return nil
	}
	lines := blade.NewLines(source)
	syn := frameworklaravel.ValidationSyntax{
		ClassName: func(name string) string {
			return resolve.ImportedName(name, file.Namespace, file.Uses, "")
		},
		ExprClass: func(start, end uint32) string {
			line, col := lines.Position(int(end))
			owners := resolvedClassLikes(r.TypeResolver(source[start:end], source, line, col, file), file, index)
			if len(owners) != 1 {
				return ""
			}
			return owners[0]
		},
		IsSubtype: index.IsSubtypeOf,
	}
	var findings []Finding
	frameworklaravel.ValidationRuleStrings(source, phpparser.Parse(source, token.Latest), syn, func(rs frameworklaravel.RuleString) {
		for _, seg := range rs.Segments(source) {
			name := seg.Name(source)
			if rs.Piped && isRegexRule(name) {
				return
			}
			if !plainRuleName(name) {
				continue
			}
			if _, ok := r.Rules.Lookup(name); ok {
				continue
			}
			startLine, startCol := lines.Position(int(seg.Start))
			endLine, endCol := lines.Position(int(seg.NameEnd))
			findings = append(findings, Finding{
				StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol,
				Severity: rule.SeverityWarning,
				Code:     rule.UnknownValidationRule,
				Message:  "Unknown validation rule `" + name + "`",
			})
		}
	})
	return findings
}

func isRegexRule(name string) bool {
	switch frameworklaravel.ValidationRuleKey(name) {
	case "regex", "notregex":
		return true
	}
	return false
}

func plainRuleName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
