package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type SyntaxErrorRule struct{}

func (r *SyntaxErrorRule) Codes() []rule.Code { return []rule.Code{rule.SyntaxError} }

func (r *SyntaxErrorRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil || len(file.Errors) == 0 {
		return nil
	}
	lines := strings.Split(source, "\n")
	findings := make([]Finding, 0, len(file.Errors))
	for _, parseErr := range file.Errors {
		line := clamp(parseErr.Line, 0, max(len(lines)-1, 0))
		lineText := ""
		if line < len(lines) {
			lineText = lines[line]
		}
		lineLen := len(lineText)
		startCol := clamp(parseErr.Column, 0, lineLen)
		endCol := min(startCol+parserErrorWidth(parseErr.Message), lineLen)
		if endCol <= startCol {
			switch {
			case lineLen == 0, startCol < lineLen:
				endCol = startCol + 1
			default:
				startCol = max(lineLen-1, 0)
				endCol = lineLen
			}
		}
		findings = append(findings, Finding{
			StartLine: line, StartCol: startCol, EndLine: line, EndCol: endCol,
			Severity: rule.SeverityError,
			Code:     rule.SyntaxError,
			Message:  parseErr.Message,
		})
	}
	return findings
}

func parserErrorWidth(message string) int {
	switch {
	case strings.Contains(message, "doc comment"):
		return 3
	case strings.Contains(message, "comment"):
		return 2
	default:
		return 1
	}
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

type AbstractInConcreteRule struct{}

func (r *AbstractInConcreteRule) Codes() []rule.Code { return []rule.Code{rule.AbstractInConcrete} }

func (r *AbstractInConcreteRule) Check(file *parser.FileNode, _ string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding
	for _, cls := range file.Classes {
		if cls.IsAbstract {
			continue
		}
		for _, m := range cls.Methods {
			if m.IsAbstract {
				findings = append(findings, Finding{
					StartLine: m.StartLine, EndLine: m.StartLine,
					Severity: rule.SeverityError,
					Code:     rule.AbstractInConcrete,
					Message:  fmt.Sprintf("Class '%s' contains abstract method '%s' but is not declared abstract", cls.Name, m.Name),
				})
			}
		}
	}
	return findings
}
