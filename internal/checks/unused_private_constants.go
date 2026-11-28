package checks

import (
	"fmt"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

func checkUnusedPrivateConstants(cls parser.ClassNode, uses *privateMemberUses, lines []string) []Finding {
	var findings []Finding
	for _, c := range cls.Constants {
		if c.Visibility != "private" || uses.usesConstant(c.Name) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: c.StartLine,
			StartCol:  c.StartCol,
			EndLine:   c.StartLine,
			EndCol:    len(lines[c.StartLine]),
			Severity:  rule.SeverityHint,
			Code:      rule.UnusedPrivateConstant,
			Message:   fmt.Sprintf("Private constant '%s::%s' is never used", cls.Name, c.Name),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

func declarationStartLine(cls parser.ClassNode) int {
	start := cls.StartLine
	for _, a := range cls.Attributes {
		start = min(start, a.Line)
	}
	return start
}
