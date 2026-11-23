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
	if file == nil || index == nil || !strings.Contains(source, ":") {
		return nil
	}
	var findings []Finding
	eachResolvedCall(file, source, index, r.Shared, r.TypeResolver, func(call resolvedCall) {
		if !shouldCheckArgCount(call.sym, call.owner, index) {
			return
		}
		names, ok := namedArguments(call.tokens, call.open, call.close)
		if !ok {
			return
		}
		for _, name := range names {
			if hasParam(call.sym.Params, name.Value) {
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

func hasParam(params []symbols.ParamInfo, name string) bool {
	for _, p := range params {
		if strings.TrimPrefix(p.Name, "$") == name {
			return true
		}
	}
	return false
}

func namedArguments(tokens []parser.Token, open, close int) ([]parser.Token, bool) {
	var names []parser.Token
	depth := 0
	argStart := true
	for i := open + 1; i < close; i++ {
		tok := tokens[i]
		switch tok.Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			continue
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
				if next := nextSignificantTokenIdx(tokens, i+1); next > 0 && next < close && tokens[next].Kind == parser.TokenColon {
					names = append(names, tok)
				}
			}
		}
		argStart = false
	}
	return names, true
}
