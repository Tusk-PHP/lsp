package parser

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func LiteralExprType(e ast.Expr, src string) string {
	if a, ok := e.(*ast.Array); ok {
		return arrayLiteralType(a, src)
	}
	general, literal := scalarType(e, src)
	if literal != "" {
		return literal
	}
	return general
}

func LiteralValueType(e ast.Expr, src string) string {
	if _, ok := e.(*ast.Array); ok {
		return "array"
	}
	general, _ := scalarType(e, src)
	return general
}

func CastType(kind token.Kind) string {
	switch kind {
	case token.IntCast:
		return "int"
	case token.FloatCast:
		return "float"
	case token.StringCast:
		return "string"
	case token.BoolCast:
		return "bool"
	case token.ArrayCast:
		return "array"
	case token.ObjectCast:
		return "object"
	case token.UnsetCast:
		return "null"
	}
	return ""
}

func scalarType(e ast.Expr, src string) (general, literal string) {
	switch n := e.(type) {
	case *ast.Literal:
		text := n.Text(src)
		switch n.Kind {
		case token.ConstantString:
			return "string", text
		case token.IntegerLiteral:
			return "int", decimal(text)
		case token.FloatLiteral:
			return "float", decimal(text)
		}
	case *ast.Unary:
		lit, ok := n.Operand.(*ast.Literal)
		if !ok || n.Op != token.Minus && n.Op != token.Plus || lit.Kind != token.IntegerLiteral && lit.Kind != token.FloatLiteral {
			return "", ""
		}
		general, literal := scalarType(lit, src)
		if literal != "" && n.Op == token.Minus {
			literal = "-" + literal
		}
		return general, literal
	case *ast.InterpolatedString:
		return "string", ""
	case *ast.ConstFetch:
		switch lower := strings.ToLower(strings.TrimPrefix(n.Text(src), "\\")); lower {
		case "true", "false":
			return "bool", lower
		case "null":
			return "null", "null"
		}
	case *ast.Cast:
		return CastType(n.Kind), ""
	case *ast.ClassConstFetch:
		if _, named := n.Class.(*ast.ConstFetch); named && strings.EqualFold(n.Name.Bounds().Text(src), "class") {
			return "string", ""
		}
	}
	return "", ""
}

func decimal(text string) string {
	for i := 0; i < len(text); i++ {
		if c := text[i]; (c < '0' || c > '9') && c != '.' {
			return ""
		}
	}
	return text
}

func arrayLiteralType(n *ast.Array, src string) string {
	var fields []string
	for _, item := range n.Items {
		if item == nil || item.Key == nil {
			continue
		}
		fields = append(fields, arrayKey(item.Key, src)+": "+arrayValueType(item.Value, src))
	}
	if fields != nil {
		return "array{" + strings.Join(fields, ", ") + "}"
	}
	common := ""
	for i, item := range n.Items {
		if item == nil {
			continue
		}
		vt := "mixed"
		if !item.Unpack {
			vt = arrayValueType(item.Value, src)
		}
		if i > 0 && vt != common {
			return "array"
		}
		common = vt
	}
	if common == "" || common == "mixed" {
		return "array"
	}
	return "array<int, " + common + ">"
}

func arrayValueType(e ast.Expr, src string) string {
	if a, ok := e.(*ast.Array); ok {
		if t := arrayLiteralType(a, src); strings.HasPrefix(t, "array{") {
			return t
		}
		return "array"
	}
	if t := LiteralValueType(e, src); t != "" {
		return t
	}
	return "mixed"
}

func arrayKey(key ast.Expr, src string) string {
	text := key.Bounds().Text(src)
	if lit, ok := key.(*ast.Literal); ok && lit.Kind == token.ConstantString {
		return text[1 : len(text)-1]
	}
	return text
}
