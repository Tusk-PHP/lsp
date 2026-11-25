package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type UnknownNamedArgumentRule struct {
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string
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
	eachResolvedCall(file, source, index, r.Shared, r.TypeResolver, hasNamedArguments, func(call resolvedCall) {
		if !allCheckable(call.callees) {
			return
		}
		names, ok := namedArguments(call.tokens, call.open, call.close)
		if !ok {
			return
		}
		for _, name := range names {
			if anyHasParam(call.callees, name.Value) {
				continue
			}
			findings = append(findings, Finding{
				StartLine: name.Line, StartCol: name.Column,
				EndLine: name.Line, EndCol: name.Column + len(name.Value),
				Severity: rule.SeverityError,
				Code:     rule.UnknownNamedArgument,
				Message:  fmt.Sprintf("Unknown named argument '%s' to %s()", name.Value, call.name),
			})
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

func hasNamedArguments(tokens []parser.Token, open, close int) bool {
	names, ok := namedArguments(tokens, open, close)
	return ok && len(names) > 0
}

func namedArguments(tokens []parser.Token, open, close int) ([]parser.Token, bool) {
	var names []parser.Token
	depth := 0
	argStart := true
	for i := open + 1; i < close; i++ {
		tok := tokens[i]
		if tok.Kind.IsComment() {
			continue
		}
		switch tok.Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
		case parser.TokenComma:
			if depth == 0 {
				argStart = true
				continue
			}
		case parser.TokenUnknown:
			if depth == 0 && argStart && tok.Value == "." {
				return nil, false
			}
		case parser.TokenIdentifier:
			if depth == 0 && argStart {
				if next := parser.NextSignificant(tokens, i+1); next > 0 && next < close && tokens[next].Kind == parser.TokenColon {
					names = append(names, tok)
				}
			}
		}
		argStart = false
	}
	return names, true
}
