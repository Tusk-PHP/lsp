package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

func checkUnusedPrivateConstants(cls parser.ClassNode, fileTokens func() []parser.Token, lines, traits []string) []Finding {
	var private []parser.ConstantNode
	for _, c := range cls.Constants {
		if c.Visibility == "private" {
			private = append(private, c)
		}
	}
	if len(private) == 0 {
		return nil
	}
	used, dynamic := constantsReferenced(fileTokens(), declarationStartLine(cls), endLineOf(cls, lines))
	if dynamic {
		return nil
	}
	var findings []Finding
	for _, c := range private {
		if used[c.Name] || anySource(traits, c.Name, sourceUsesConstant) {
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

func constantsReferenced(tokens []parser.Token, startLine, endLine int) (used map[string]bool, dynamic bool) {
	used = map[string]bool{}
	for i, tok := range tokens {
		if tok.Line < startLine || tok.Line > endLine {
			continue
		}
		if tok.Kind == parser.TokenIdentifier && strings.EqualFold(tok.Value, "constant") {
			if next := nextSignificantTokenIdx(tokens, i+1); tokens[next].Kind == parser.TokenOpenParen {
				return nil, true
			}
		}
		if tok.Kind != parser.TokenDoubleColon {
			continue
		}
		next := tokens[nextSignificantTokenIdx(tokens, i+1)]
		switch next.Kind {
		case parser.TokenOpenBrace:
			return nil, true
		case parser.TokenIdentifier:
			used[next.Value] = true
		}
	}
	return used, false
}

func sourceUsesConstant(src, name string) bool {
	return containsWord(src, "::"+name)
}

func containsWord(src, word string) bool {
	for rest := src; ; {
		i := strings.Index(rest, word)
		if i < 0 {
			return false
		}
		after := i + len(word)
		if after >= len(rest) || !isIdentChar(rest[after]) {
			return true
		}
		rest = rest[after:]
	}
}
