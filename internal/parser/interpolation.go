package parser

import "strings"

func Interpolates(lit string) bool {
	if rest, ok := strings.CutPrefix(lit, "<<<"); ok {
		return !strings.HasPrefix(strings.TrimLeft(rest, " \t"), "'")
	}
	return len(lit) >= 2 && lit[0] == '"'
}

func InterpolatedExpressions(lit string) []string {
	if !Interpolates(lit) {
		return nil
	}
	var exprs []string
	for i := 0; i+1 < len(lit); i++ {
		start := -1
		switch {
		case lit[i] == '\\':
			i++
			continue
		case lit[i] == '{' && lit[i+1] == '$':
			start = i + 1
		case lit[i] == '$' && lit[i+1] == '{':
			start = i + 2
		default:
			continue
		}
		end := closingBrace(lit, start)
		if end < 0 {
			return exprs
		}
		exprs = append(exprs, lit[start:end])
		i = end
	}
	return exprs
}

func closingBrace(s string, from int) int {
	depth := 0
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '\'', '"':
			i = quotedEnd(s, i)
			if i < 0 {
				return -1
			}
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func quotedEnd(s string, open int) int {
	for i := open + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case s[open]:
			return i
		}
	}
	return -1
}
