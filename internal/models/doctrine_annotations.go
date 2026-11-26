package models

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

type annotationToken struct {
	punct byte
	text  string
}

func findDocAnnotation(doc string, match func(name string) bool) (parser.AttributeArgs, bool) {
	for i := 0; i < len(doc); i++ {
		if doc[i] != '@' || i > 0 && !isAnnotationSpace(doc[i-1]) {
			continue
		}
		end := i + 1
		for end < len(doc) && isAnnotationNameByte(doc[end], end == i+1) {
			end++
		}
		if end == i+1 {
			continue
		}
		wanted := match(doc[i+1 : end])
		i = end - 1
		if end == len(doc) || doc[end] != '(' {
			if wanted {
				return nil, true
			}
			continue
		}
		var args parser.AttributeArgs
		collect := &args
		if !wanted {
			collect = nil
		}
		next, ok := annotationArgs(doc, end+1, collect)
		if !ok {
			continue
		}
		if wanted {
			return args, true
		}
		i = next - 1
	}
	return nil, false
}

func annotationArgs(doc string, start int, args *parser.AttributeArgs) (int, bool) {
	var arg []annotationToken
	if args != nil {
		arg = make([]annotationToken, 0, 8)
	}
	add := func(tok annotationToken) {
		if args != nil {
			arg = append(arg, tok)
		}
	}
	depth := 0
	for i := start; i < len(doc); {
		c := doc[i]
		switch {
		case c == '*' && i+1 < len(doc) && doc[i+1] == '/':
			return 0, false
		case isAnnotationSpace(c):
			i++
		case c == '"':
			end, ok := annotationStringEnd(doc, i+1)
			if !ok {
				return 0, false
			}
			add(annotationToken{text: doc[i:end]})
			i = end
		case isAnnotationNameByte(c, false) || c == '-' || c == '.':
			end := annotationWordEnd(doc, i)
			add(annotationToken{text: doc[i:end]})
			i = end
		case (c == ')' || c == ',') && depth == 0:
			if args != nil {
				*args = appendAnnotationArg(*args, arg)
			}
			arg = arg[:0]
			i++
			if c == ')' {
				return i, true
			}
		case c == '(' || c == '{':
			depth++
			add(annotationToken{punct: c, text: doc[i : i+1]})
			i++
		case c == ')' || c == '}':
			depth--
			add(annotationToken{punct: c, text: doc[i : i+1]})
			i++
		case c == '=' || c == ':' || c == ',' || c == '@':
			add(annotationToken{punct: c, text: doc[i : i+1]})
			i++
		default:
			return 0, false
		}
	}
	return 0, false
}

func appendAnnotationArg(args parser.AttributeArgs, arg []annotationToken) parser.AttributeArgs {
	if len(arg) == 0 {
		return args
	}
	var named parser.AttributeArg
	if len(arg) > 2 && arg[0].punct == 0 && (arg[1].punct == '=' || arg[1].punct == ':') {
		named.Name = arg[0].text
		arg = arg[2:]
	}
	var value strings.Builder
	for _, tok := range arg {
		value.WriteString(tok.text)
	}
	named.Value = value.String()
	return append(args, named)
}

func annotationStringEnd(doc string, i int) (int, bool) {
	for i < len(doc) {
		if doc[i] != '"' {
			i++
			continue
		}
		if i+1 < len(doc) && doc[i+1] == '"' {
			i += 2
			continue
		}
		return i + 1, true
	}
	return 0, false
}

func annotationWordEnd(doc string, i int) int {
	for i < len(doc) {
		switch c := doc[i]; {
		case isAnnotationNameByte(c, false) || c == '-' || c == '.':
			i++
		case c == ':' && i+1 < len(doc) && doc[i+1] == ':':
			i += 2
		default:
			return i
		}
	}
	return i
}

func isAnnotationSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '*'
}

func isAnnotationNameByte(c byte, first bool) bool {
	switch {
	case c == '_' || c == '\\' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
		return true
	case '0' <= c && c <= '9':
		return !first
	}
	return c >= 0x80
}
