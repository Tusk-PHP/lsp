package parser

import "strings"

var doubleQuotedEscapes = map[byte]byte{'\\': '\\', '"': '"', '$': '$', 'n': '\n', 't': '\t', 'r': '\r', 'v': '\v', 'f': '\f', 'e': 0x1b}

func ConstantString(tok Token) (string, bool) {
	if tok.Kind != TokenStringLiteral {
		return "", false
	}
	return Unquote(tok.Value)
}

func Unquote(lit string) (string, bool) {
	if len(lit) < 2 || (lit[0] != '\'' && lit[0] != '"') || lit[len(lit)-1] != lit[0] {
		return "", false
	}
	body := lit[1 : len(lit)-1]
	if lit[0] == '\'' {
		return singleQuotedValue(body), true
	}
	var out strings.Builder
	out.Grow(len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '$' {
			return "", false
		}
		if c != '\\' || i+1 == len(body) {
			out.WriteByte(c)
			continue
		}
		next := body[i+1]
		if decoded, ok := doubleQuotedEscapes[next]; ok {
			out.WriteByte(decoded)
			i++
			continue
		}
		if isNumericEscape(next) {
			return "", false
		}
		out.WriteByte(c)
	}
	return out.String(), true
}

func isNumericEscape(c byte) bool {
	return c == 'x' || c == 'u' || '0' <= c && c <= '7'
}

func singleQuotedValue(body string) string {
	if !strings.Contains(body, `\`) {
		return body
	}
	var out strings.Builder
	out.Grow(len(body))
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) && (body[i+1] == '\\' || body[i+1] == '\'') {
			i++
		}
		out.WriteByte(body[i])
	}
	return out.String()
}
