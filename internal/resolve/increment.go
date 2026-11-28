package resolve

import (
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func incrementedType(op token.Kind, operand ResolvedType) ResolvedType {
	if operand.IsEmpty() || len(operand.Intersection) > 0 || operand.Conditional != nil {
		return ResolvedType{}
	}
	parts := operand.Parts()
	out := make([]ResolvedType, 0, len(parts)+1)
	if operand.Nullable {
		parts = append(parts[:len(parts):len(parts)], ResolvedType{FQN: "null"})
	}
	for _, part := range parts {
		rt := incrementedPart(op, part)
		if rt.IsEmpty() {
			return ResolvedType{}
		}
		out = append(out, rt)
	}
	if len(out) == 1 {
		return out[0]
	}
	return MakeUnion(out, false)
}

func incrementedPart(op token.Kind, part ResolvedType) ResolvedType {
	switch part.FQN {
	case "null":
		if op == token.Decrement {
			return part
		}
		return ResolvedType{FQN: "int"}
	case "numeric-string":
		return MakeUnion([]ResolvedType{{FQN: "int"}, {FQN: "float"}}, false)
	case "string":
		return incrementedString(op, part.Literal)
	}
	switch parser.ScalarOf(part.FQN) {
	case parser.ScalarInt:
		return ResolvedType{FQN: "int"}
	case parser.ScalarFloat:
		return ResolvedType{FQN: "float"}
	case parser.ScalarBool:
		return part
	}
	return ResolvedType{}
}

func incrementedString(op token.Kind, literal string) ResolvedType {
	value, ok := plainStringValue(literal)
	switch {
	case !ok:
		return ResolvedType{}
	case value == "":
		if op == token.Decrement {
			return ResolvedType{FQN: "int"}
		}
		return ResolvedType{FQN: "string"}
	case strings.TrimRight(value, phpWhitespace) != value:
		return ResolvedType{}
	}
	number := strings.TrimLeft(value, phpWhitespace)
	if !isNumericString(number) {
		return ResolvedType{FQN: "string"}
	}
	if _, err := strconv.ParseInt(number, 10, 64); err == nil {
		return ResolvedType{FQN: "int"}
	}
	if strings.ContainsAny(number, ".eE") {
		return ResolvedType{FQN: "float"}
	}
	return ResolvedType{}
}

const phpWhitespace = " \t\n\r\v\f"

func plainStringValue(literal string) (string, bool) {
	if len(literal) < 2 {
		return "", false
	}
	quote, inner := literal[0], literal[1:len(literal)-1]
	switch {
	case quote == '\'' && !strings.ContainsRune(inner, '\\'):
		return inner, true
	case quote == '"' && !strings.ContainsAny(inner, "\\$"):
		return inner, true
	}
	return "", false
}

func isNumericString(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	intDigits := digitsFrom(s, i)
	i += intDigits
	fracDigits := 0
	if i < len(s) && s[i] == '.' {
		i++
		fracDigits = digitsFrom(s, i)
		i += fracDigits
	}
	if intDigits == 0 && fracDigits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		expDigits := digitsFrom(s, j)
		if expDigits == 0 {
			return false
		}
		i = j + expDigits
	}
	return i == len(s)
}

func digitsFrom(s string, i int) int {
	n := 0
	for i+n < len(s) && s[i+n] >= '0' && s[i+n] <= '9' {
		n++
	}
	return n
}
