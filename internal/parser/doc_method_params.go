package parser

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/types"
)

func parseDocMethodParams(list string) (params []DocMethodParam, description string, ok bool) {
	rest := strings.TrimSpace(list)
	for rest != "" {
		switch rest[0] {
		case ')':
			return params, strings.TrimSpace(rest[1:]), true
		case ',':
			rest = strings.TrimSpace(rest[1:])
			continue
		}
		if params == nil {
			params = make([]DocMethodParam, 0, strings.Count(rest, ",")+1)
		}
		var param DocMethodParam
		param, rest = parseDocMethodParam(rest)
		params = append(params, param)
	}
	return nil, "", false
}

func parseDocMethodParam(s string) (DocMethodParam, string) {
	var param DocMethodParam
	if s[0] != '$' && s[0] != '&' && !strings.HasPrefix(s, "...") {
		param.Type, s = types.ExtractParamListType(s)
	}
	if rest, ok := strings.CutPrefix(s, "&"); ok {
		param.IsReference = true
		s = strings.TrimSpace(rest)
	}
	if rest, ok := strings.CutPrefix(s, "..."); ok {
		param.IsVariadic = true
		s = strings.TrimSpace(rest)
	}
	if strings.HasPrefix(s, "$") {
		n := 1
		for n < len(s) && IsIdentByte(s[n]) {
			n++
		}
		param.Name, s = s[:n], s[n:]
	}
	code, rest := docParamCode(s)
	if value, ok := strings.CutPrefix(code, "="); ok {
		param.Default = strings.TrimSpace(value)
	}
	return param, rest
}

func docParamCode(s string) (code, rest string) {
	s = strings.TrimSpace(s)
	if s == "" || s[0] == ',' || s[0] == ')' {
		return "", s
	}
	tokens, _ := tokenizeFrom(s, false)
	depth := 0
	for _, tok := range tokens {
		switch tok.Kind {
		case TokenOpenParen, TokenOpenBracket, TokenOpenBrace:
			depth++
		case TokenCloseBracket, TokenCloseBrace:
			depth--
		case TokenCloseParen:
			if depth == 0 {
				return strings.TrimSpace(s[:tok.Offset]), s[tok.Offset:]
			}
			depth--
		case TokenComma:
			if depth == 0 {
				return strings.TrimSpace(s[:tok.Offset]), s[tok.Offset:]
			}
		}
	}
	return s, ""
}
