package checks

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

var reflectiveConstantReads = map[string]bool{
	"getconstants":            true,
	"getconstant":             true,
	"hasconstant":             true,
	"getreflectionconstants":  true,
	"getreflectionconstant":   true,
	"reflectionclassconstant": true,
}

var constantLookupFunctions = map[string]bool{
	"constant": true,
	"defined":  true,
}

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
	first, _ := slices.BinarySearchFunc(tokens, startLine, func(t parser.Token, line int) int { return cmp.Compare(t.Line, line) })
	last, _ := slices.BinarySearchFunc(tokens, endLine+1, func(t parser.Token, line int) int { return cmp.Compare(t.Line, line) })
	used = map[string]bool{}
	if collectConstantReads(tokens[first:], last-first, used) {
		return nil, true
	}
	return used, false
}

func collectConstantReads(tokens []parser.Token, end int, used map[string]bool) (dynamic bool) {
	for i, tok := range tokens[:end] {
		switch tok.Kind {
		case parser.TokenStringLiteral:
			for _, expr := range parser.InterpolatedExpressions(tok.Value) {
				exprTokens := parser.Tokenize("<?php " + expr)
				if collectConstantReads(exprTokens, len(exprTokens), used) {
					return true
				}
			}
		case parser.TokenIdentifier:
			name := strings.ToLower(tok.Value[strings.LastIndexByte(tok.Value, '\\')+1:])
			if reflectiveConstantReads[name] {
				return true
			}
			if !constantLookupFunctions[name] {
				continue
			}
			constant, lookup := constantLookupArgument(tokens, i+1)
			switch lookup {
			case lookupComputed:
				return true
			case lookupLiteral:
				if _, member, ok := strings.Cut(constant, "::"); ok {
					used[member] = true
				}
			}
		case parser.TokenDoubleColon:
			next := tokens[nextSignificantTokenIdx(tokens, i+1)]
			switch next.Kind {
			case parser.TokenOpenBrace:
				return true
			case parser.TokenIdentifier:
				used[next.Value] = true
			}
		}
	}
	return false
}

type constantLookup int

const (
	lookupNotACall constantLookup = iota
	lookupComputed
	lookupLiteral
)

func constantLookupArgument(tokens []parser.Token, from int) (string, constantLookup) {
	open := nextSignificantTokenIdx(tokens, from)
	if tokens[open].Kind != parser.TokenOpenParen {
		return "", lookupNotACall
	}
	arg := nextSignificantTokenIdx(tokens, open+1)
	if tokens[arg].Kind != parser.TokenStringLiteral {
		return "", lookupComputed
	}
	value, ok := plainStringLiteral(tokens[arg].Value)
	if !ok {
		return "", lookupComputed
	}
	after := nextSignificantTokenIdx(tokens, arg+1)
	if tokens[after].Kind != parser.TokenCloseParen && tokens[after].Kind != parser.TokenComma {
		return "", lookupComputed
	}
	return value, lookupLiteral
}

func plainStringLiteral(lit string) (string, bool) {
	if len(lit) < 2 {
		return "", false
	}
	quote := lit[0]
	if (quote != '\'' && quote != '"') || lit[len(lit)-1] != quote {
		return "", false
	}
	body := lit[1 : len(lit)-1]
	if strings.ContainsAny(body, "\\$") {
		return "", false
	}
	return body, true
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
