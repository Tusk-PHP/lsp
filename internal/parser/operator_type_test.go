package parser

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestOperatorTypesFollowPHP(t *testing.T) {
	operands := map[string]Scalars{
		"$i": ScalarInt, "$f": ScalarFloat, "$s": ScalarString, "$b": ScalarBool, "$n": ScalarNull,
		"$a": ScalarArray, "$num": ScalarInt | ScalarFloat, "$ni": ScalarInt | ScalarNull, "$o": 0,
	}
	for expr, want := range map[string]string{
		"$i + $i":           "int",
		"$i - $i":           "int",
		"$i * $i":           "int",
		"$i + $f":           "float",
		"$f * $i":           "float",
		"$i + $s":           "int|float",
		"$s - $s":           "int|float",
		"$s * $f":           "float",
		"$b + $n":           "int",
		"$ni + $i":          "int",
		"$num + $i":         "int|float",
		"$a + $a":           "array",
		"$a - $a":           "",
		"$a + $i":           "",
		"$o + $i":           "",
		"$i + $x":           "",
		"$i / $i":           "int|float",
		"$i / $f":           "float",
		"$s / $i":           "int|float",
		"$f % $s":           "int",
		"$a % $i":           "",
		"$i ** 2":           "int",
		"$i ** -1":          "float",
		"$i ** -0":          "int",
		"$i ** $i":          "int|float",
		"$i ** $b":          "int",
		"$f ** 2":           "float",
		"$s ** 2":           "int|float",
		"2 ** 62":           "int",
		"2 ** 63":           "float",
		"(-2) ** 63":        "int",
		"(-2) ** 64":        "float",
		"(-2) ** 3":         "int",
		"10 ** 0x12":        "int",
		"10 ** 0x13":        "float",
		"1 ** 1000":         "int",
		"$i << $i":          "int",
		"$s >> $f":          "int",
		"$i & $i":           "int",
		"$s & $s":           "string",
		"$s | $i":           "int",
		"$f ^ $f":           "int",
		"$a & $a":           "",
		"$o | $i":           "",
		"-$i":               "int",
		"-$f":               "float",
		"-$s":               "int|float",
		"+$b":               "int",
		"-$a":               "",
		"-$o":               "",
		"~$i":               "int",
		"~$f":               "int",
		"~$s":               "string",
		"~$b":               "",
		"!$o":               "bool",
		"$o && $x":          "bool",
		"$o || $x":          "bool",
		"$o and $x":         "bool",
		"$o or $x":          "bool",
		"$o xor $x":         "bool",
		"$o == $x":          "bool",
		"$o !== $x":         "bool",
		"$o < $x":           "bool",
		"$o >= $x":          "bool",
		"$o instanceof Foo": "bool",
		"$o <=> $x":         "int",
		"$o . $x":           "string",
		"$o ?? $x":          "",
		"$o":                "",
		"$i++":              "",
		"++$i":              "",
	} {
		src := "return " + expr + ";"
		e := phpparser.ParseCode(src, token.Latest).Stmts[0].(*ast.Return).Value
		var operand func(ast.Expr) Scalars
		operand = func(x ast.Expr) Scalars {
			if v, ok := x.(*ast.Variable); ok {
				return operands[v.Text(src)]
			}
			if s := OperatorType(x, src, operand); s != 0 {
				return s
			}
			return literalScalars(x, src)
		}
		if got := OperatorType(e, src, operand).String(); got != want {
			t.Errorf("OperatorType(%s) = %q, want %q", expr, got, want)
		}
	}
}

func TestLiteralOperatorsAreTyped(t *testing.T) {
	for expr, want := range map[string]string{
		"60 * 60":                   "int",
		"1 + 1.5":                   "float",
		"1 / 2":                     "int|float",
		"'a' . 'b'":                 "string",
		"1 < 2":                     "bool",
		"-(1 + 2)":                  "int",
		"PHP_INT_MAX + 1":           "",
		"[1] + [2]":                 "array",
		"2 ** -1":                   "float",
		"2 ** 99999999999999999999": "float",
		"2 ** 08":                   "int|float",
	} {
		src := "return " + expr + ";"
		e := phpparser.ParseCode(src, token.Latest).Stmts[0].(*ast.Return).Value
		if got := LiteralValueType(e, src); got != want {
			t.Errorf("LiteralValueType(%s) = %q, want %q", expr, got, want)
		}
	}
}

func TestScalarSets(t *testing.T) {
	for name, want := range map[string]string{
		"non-empty-string": "string",
		"true":             "bool",
		"null":             "null",
		"list":             "array",
		"numeric":          "int|float|string",
		"scalar":           "int|float|string|bool",
		"array-key":        "int|string",
		"object":           "",
	} {
		if got := ScalarOf(name).String(); got != want {
			t.Errorf("ScalarOf(%s) = %q, want %q", name, got, want)
		}
	}
	if got := (ScalarInt | ScalarString).Names(); len(got) != 2 || got[0] != "int" || got[1] != "string" {
		t.Errorf("Names = %q, want [int string]", got)
	}
}
