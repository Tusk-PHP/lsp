package parser

import "strings"

var doubleQuotedEscapes = map[byte]byte{'\\': '\\', '"': '"', '$': '$', 'n': '\n', 't': '\t', 'r': '\r', 'v': '\v', 'f': '\f', 'e': 0x1b}

func ConstantString(tok Token) (string, bool) {
	v := tok.Value
	if tok.Kind != TokenStringLiteral || len(v) < 2 || (v[0] != '\'' && v[0] != '"') || v[len(v)-1] != v[0] {
		return "", false
	}
	body := v[1 : len(v)-1]
	if v[0] == '\'' {
		return singleQuotedValue(body), true
	}
	var out strings.Builder
	out.Grow(len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '$' {
			return "", false
		}
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		if i+1 == len(body) {
			out.WriteByte(c)
			continue
		}
		decoded, ok := doubleQuotedEscapes[body[i+1]]
		if !ok {
			return "", false
		}
		out.WriteByte(decoded)
		i++
	}
	return out.String(), true
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
